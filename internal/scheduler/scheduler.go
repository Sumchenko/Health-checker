package scheduler

import (
	"context"
	"health-checker/internal/models"
	"time"
)

type Scheduler struct {
	targets []models.Target
}

func NewScheduler(targets []models.Target) *Scheduler {
	return &Scheduler{
		targets: targets,
	}
}

func (s *Scheduler) Run(ctx context.Context, tasks chan<- models.Target) {
	s.sendTasks(tasks)

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.sendTasks(tasks)
		}
	}
}

func (s *Scheduler) sendTasks(tasks chan<- models.Target) {
	for _, t := range s.targets {
		tasks <- t
	}
}
