package scheduler

import (
	"context"
	"errors"
	"health-checker/internal/models"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type fakeSource struct {
	mu      sync.Mutex
	targets []models.Target
	err     error
}

func (f *fakeSource) set(err error, targets ...models.Target) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.targets, f.err = targets, err
}

func (f *fakeSource) ListEnabledTargets(context.Context) ([]models.Target, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.targets, f.err
}

// fakeChecker «проверяет» цель за delay и следит за количеством одновременных проверок.
type fakeChecker struct {
	delay time.Duration
	// blockUntilCancel — проверка висит до отмены контекста и завершается неудачей.
	blockUntilCancel bool

	mu            sync.Mutex
	inFlight      map[int]int
	overlap       bool
	total         atomic.Int32
	maxConcurrent atomic.Int32
}

func (f *fakeChecker) Check(ctx context.Context, t models.Target) models.Result {
	f.mu.Lock()
	if f.inFlight == nil {
		f.inFlight = map[int]int{}
	}
	f.inFlight[t.ID]++
	if f.inFlight[t.ID] > 1 {
		f.overlap = true
	}
	f.mu.Unlock()

	cur := f.total.Add(1)
	for {
		prev := f.maxConcurrent.Load()
		if cur <= prev || f.maxConcurrent.CompareAndSwap(prev, cur) {
			break
		}
	}

	defer func() {
		f.total.Add(-1)
		f.mu.Lock()
		f.inFlight[t.ID]--
		f.mu.Unlock()
	}()

	if f.blockUntilCancel {
		<-ctx.Done()
		return models.Result{TargetID: t.ID, URL: t.URL, Error: "прервано"}
	}
	select {
	case <-time.After(f.delay):
	case <-ctx.Done():
		return models.Result{TargetID: t.ID, URL: t.URL, Error: "прервано"}
	}
	return models.Result{TargetID: t.ID, URL: t.URL, IsUp: true}
}

type collector struct {
	mu   sync.Mutex
	got  []models.Result
	done chan struct{}
}

func (c *collector) count(pred func(models.Result) bool) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	n := 0
	for _, r := range c.got {
		if pred(r) {
			n++
		}
	}
	return n
}

func (c *collector) waitFor(t *testing.T, n int, pred func(models.Result) bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for c.count(pred) < n {
		if time.Now().After(deadline) {
			t.Fatalf("не дождались %d подходящих результатов, получено %d", n, c.count(pred))
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func byTarget(id int) func(models.Result) bool {
	return func(r models.Result) bool { return r.TargetID == id }
}

func start(t *testing.T, src TargetSource, chk Checker, cfg Config) (*collector, context.CancelFunc) {
	t.Helper()
	if cfg.DefaultInterval == 0 {
		cfg.DefaultInterval = time.Hour
	}
	if cfg.ReloadInterval == 0 {
		cfg.ReloadInterval = 10 * time.Millisecond
	}
	if cfg.MaxConcurrent == 0 {
		cfg.MaxConcurrent = 10
	}

	ctx, cancel := context.WithCancel(context.Background())
	results := make(chan models.Result)
	c := &collector{done: make(chan struct{})}
	go func() {
		for r := range results {
			c.mu.Lock()
			c.got = append(c.got, r)
			c.mu.Unlock()
		}
		close(c.done)
	}()
	go New(src, chk, cfg).Run(ctx, results)

	t.Cleanup(func() {
		cancel()
		select {
		case <-c.done:
		case <-time.After(2 * time.Second):
			t.Error("Run не завершился и не закрыл канал результатов после отмены контекста")
		}
	})
	return c, cancel
}

func target(id int, interval time.Duration) models.Target {
	return models.Target{ID: id, Name: "t", URL: "https://example.com", Interval: interval, Enabled: true}
}

func TestRun_ChecksByInterval(t *testing.T) {
	src := &fakeSource{}
	src.set(nil, target(1, 20*time.Millisecond), target(2, 0)) // у цели 2 интервал по умолчанию — час

	c, _ := start(t, src, &fakeChecker{}, Config{})
	c.waitFor(t, 3, byTarget(1))

	if n := c.count(byTarget(2)); n != 1 {
		t.Errorf("цель 2 проверена %d раз, want 1", n)
	}
}

func TestRun_NoOverlappingChecksOfSameTarget(t *testing.T) {
	src := &fakeSource{}
	src.set(nil, target(1, 5*time.Millisecond))
	chk := &fakeChecker{delay: 30 * time.Millisecond} // проверка дольше интервала

	c, _ := start(t, src, chk, Config{})
	c.waitFor(t, 4, byTarget(1))

	chk.mu.Lock()
	defer chk.mu.Unlock()
	if chk.overlap {
		t.Error("цель проверялась параллельно сама с собой")
	}
}

func TestRun_LimitsConcurrency(t *testing.T) {
	src := &fakeSource{}
	src.set(nil, target(1, 0), target(2, 0), target(3, 0), target(4, 0))
	chk := &fakeChecker{delay: 20 * time.Millisecond}

	c, _ := start(t, src, chk, Config{MaxConcurrent: 2})
	c.waitFor(t, 4, func(models.Result) bool { return true })

	if m := chk.maxConcurrent.Load(); m > 2 {
		t.Errorf("одновременно выполнялось %d проверок, лимит 2", m)
	}
}

func TestRun_ReconcilesTargets(t *testing.T) {
	src := &fakeSource{}
	src.set(nil, target(1, 10*time.Millisecond))

	c, _ := start(t, src, &fakeChecker{}, Config{})
	c.waitFor(t, 1, byTarget(1))

	// Цель 1 удалена, цель 2 добавлена.
	src.set(nil, target(2, 0))
	c.waitFor(t, 1, byTarget(2))

	time.Sleep(30 * time.Millisecond) // возможная проверка, начатая до удаления
	before := c.count(byTarget(1))
	time.Sleep(60 * time.Millisecond)
	if after := c.count(byTarget(1)); after != before {
		t.Errorf("удалённая цель продолжает проверяться: %d → %d", before, after)
	}

	// Изменение настроек перезапускает цель: сразу идёт проверка с новым URL.
	changed := target(2, 0)
	changed.URL = "https://changed.example.com"
	src.set(nil, changed)
	c.waitFor(t, 1, func(r models.Result) bool { return r.URL == changed.URL })
}

func TestRun_SkipsInvalidTargets(t *testing.T) {
	invalid := target(1, 0)
	invalid.URL = "ftp://example.com"
	src := &fakeSource{}
	src.set(nil, invalid, target(2, 0))

	c, _ := start(t, src, &fakeChecker{}, Config{})
	c.waitFor(t, 1, byTarget(2))

	if n := c.count(byTarget(1)); n != 0 {
		t.Errorf("некорректная цель проверена %d раз", n)
	}
}

func TestRun_KeepsTargetsWhenSourceFails(t *testing.T) {
	src := &fakeSource{}
	src.set(errors.New("база недоступна"))

	c, _ := start(t, src, &fakeChecker{}, Config{})
	time.Sleep(30 * time.Millisecond)

	src.set(nil, target(1, 20*time.Millisecond))
	c.waitFor(t, 1, byTarget(1))

	// Источник снова падает — уже запущенная цель продолжает проверяться.
	src.set(errors.New("база недоступна"))
	before := c.count(byTarget(1))
	c.waitFor(t, before+2, byTarget(1))
}

func TestRun_DropsChecksInterruptedByShutdown(t *testing.T) {
	src := &fakeSource{}
	src.set(nil, target(1, 0))
	chk := &fakeChecker{blockUntilCancel: true}

	c, cancel := start(t, src, chk, Config{})
	for chk.total.Load() == 0 {
		time.Sleep(time.Millisecond)
	}
	cancel()

	select {
	case <-c.done:
	case <-time.After(2 * time.Second):
		t.Fatal("канал результатов не закрыт после отмены")
	}
	if n := c.count(byTarget(1)); n != 0 {
		t.Errorf("прерванная проверка сохранена (%d результатов)", n)
	}
}
