package scheduler

import (
	"context"
	"health-checker/internal/models"
	"sync"
	"time"
)

type Scheduler struct {
	targets         []models.Target
	defaultInterval time.Duration
}

func NewScheduler(targets []models.Target, defaultInterval time.Duration) *Scheduler {
	return &Scheduler{
		targets:         targets,
		defaultInterval: defaultInterval,
	}
}

// Run ставит цели в очередь каждую со своим интервалом. Возвращается после отмены ctx
// и закрывает канал tasks, чтобы воркеры могли завершиться.
func (s *Scheduler) Run(ctx context.Context, tasks chan<- models.Target) {
	defer close(tasks)

	var wg sync.WaitGroup
	for _, t := range s.targets {
		wg.Go(func() { s.runTarget(ctx, t, tasks) })
	}
	wg.Wait()
}

func (s *Scheduler) runTarget(ctx context.Context, t models.Target, tasks chan<- models.Target) {
	interval := t.Interval
	if interval <= 0 {
		interval = s.defaultInterval
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case tasks <- t:
		case <-ctx.Done():
			return
		}

		select {
		case <-ticker.C:
		case <-ctx.Done():
			return
		}
	}
}
