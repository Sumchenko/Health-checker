package scheduler

import (
	"context"
	"health-checker/internal/models"
	"testing"
	"time"
)

func TestRun_SchedulesByIntervalAndClosesOnCancel(t *testing.T) {
	targets := []models.Target{
		{ID: 1, Interval: 20 * time.Millisecond},
		{ID: 2}, // интервал по умолчанию — час, за время теста только первая проверка
	}
	ctx, cancel := context.WithCancel(context.Background())
	tasks := make(chan models.Target)

	go NewScheduler(targets, time.Hour).Run(ctx, tasks)

	counts := map[int]int{}
	timeout := time.After(time.Second)
	for counts[1] < 3 || counts[2] < 1 {
		select {
		case task := <-tasks:
			counts[task.ID]++
		case <-timeout:
			t.Fatalf("не дождались проверок, получено %v", counts)
		}
	}
	cancel()

	// После отмены канал должен закрыться (оставшиеся отправки прерываются).
	for {
		select {
		case task, ok := <-tasks:
			if !ok {
				if counts[2] != 1 {
					t.Errorf("цель 2 проверена %d раз, want 1", counts[2])
				}
				return
			}
			counts[task.ID]++
		case <-time.After(time.Second):
			t.Fatal("канал задач не закрыт после отмены контекста")
		}
	}
}
