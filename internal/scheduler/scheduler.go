package scheduler

import (
	"context"
	"health-checker/internal/models"
	"log/slog"
	"sync"
	"time"
)

const loadTimeout = 10 * time.Second

type TargetSource interface {
	ListEnabledTargets(ctx context.Context) ([]models.Target, error)
}

type Checker interface {
	Check(ctx context.Context, target models.Target) models.Result
}

type Config struct {
	// DefaultInterval — интервал для целей без собственного интервала.
	DefaultInterval time.Duration
	// ReloadInterval — как часто перечитывать список целей из источника.
	ReloadInterval time.Duration
	// MaxConcurrent — максимум одновременно выполняющихся проверок.
	MaxConcurrent int
}

// Scheduler держит по горутине на каждую активную цель. Внутри горутины проверки идут
// последовательно, поэтому одна цель никогда не проверяется параллельно сама с собой.
type Scheduler struct {
	source  TargetSource
	checker Checker
	cfg     Config
	sem     chan struct{}
}

type running struct {
	target models.Target
	cancel context.CancelFunc
}

func New(source TargetSource, checker Checker, cfg Config) *Scheduler {
	return &Scheduler{
		source:  source,
		checker: checker,
		cfg:     cfg,
		sem:     make(chan struct{}, cfg.MaxConcurrent),
	}
}

// Run выполняет проверки до отмены ctx. Перед возвратом дожидается всех проверок
// и закрывает results, чтобы обработчик результатов мог завершиться.
func (s *Scheduler) Run(ctx context.Context, results chan<- models.Result) {
	var wg sync.WaitGroup
	defer close(results)
	defer wg.Wait()

	active := make(map[int]*running)
	s.reconcile(ctx, active, &wg, results)
	if len(active) == 0 {
		slog.Warn("нет активных целей: добавьте проекты в таблицу targets")
	}

	ticker := time.NewTicker(s.cfg.ReloadInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			slog.Info("остановка: дожидаемся завершения текущих проверок...")
			return
		case <-ticker.C:
			s.reconcile(ctx, active, &wg, results)
		}
	}
}

// reconcile приводит набор запущенных горутин в соответствие с актуальным списком целей:
// новые запускает, удалённые и отключённые останавливает, изменённые перезапускает.
func (s *Scheduler) reconcile(ctx context.Context, active map[int]*running, wg *sync.WaitGroup, results chan<- models.Result) {
	loadCtx, cancel := context.WithTimeout(ctx, loadTimeout)
	targets, err := s.source.ListEnabledTargets(loadCtx)
	cancel()
	if err != nil {
		// Список не обновился — продолжаем проверять прежний набор целей.
		if ctx.Err() == nil {
			slog.Error("не удалось загрузить список целей", "err", err)
		}
		return
	}

	seen := make(map[int]bool, len(targets))
	for _, t := range targets {
		if err := t.Validate(); err != nil {
			slog.Warn("цель пропущена: некорректные настройки", "target_id", t.ID, "name", t.Name, "err", err)
			continue
		}
		seen[t.ID] = true

		if r, ok := active[t.ID]; ok {
			if r.target.Equal(t) {
				continue
			}
			r.cancel()
			slog.Info("настройки цели изменены, перезапуск", "target_id", t.ID, "name", t.Name)
		} else {
			slog.Info("цель добавлена в мониторинг", "target_id", t.ID, "name", t.Name, "url", t.URL)
		}

		targetCtx, cancel := context.WithCancel(ctx)
		active[t.ID] = &running{target: t, cancel: cancel}
		wg.Go(func() { s.runTarget(targetCtx, t, results) })
	}

	for id, r := range active {
		if !seen[id] {
			r.cancel()
			delete(active, id)
			slog.Info("цель снята с мониторинга", "target_id", id, "name", r.target.Name)
		}
	}
}

func (s *Scheduler) runTarget(ctx context.Context, t models.Target, results chan<- models.Result) {
	interval := t.Interval
	if interval <= 0 {
		interval = s.cfg.DefaultInterval
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		if !s.check(ctx, t, results) {
			return
		}

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// check выполняет одну проверку. Возвращает false, если горутина цели должна завершиться.
func (s *Scheduler) check(ctx context.Context, t models.Target, results chan<- models.Result) bool {
	select {
	case s.sem <- struct{}{}:
	case <-ctx.Done():
		return false
	}
	res := s.checker.Check(ctx, t)
	<-s.sem

	// Проверка, прерванная остановкой сервиса или изменением цели, ничего не говорит о цели — не сохраняем её.
	if !res.IsUp && ctx.Err() != nil {
		return false
	}
	// Отправка не блокируется навсегда: results читают, пока Run его не закроет.
	results <- res
	return true
}
