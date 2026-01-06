package main

import (
	"context"
	"health-checker/internal/models"
	"health-checker/internal/processor"
	"health-checker/internal/scheduler"
	"health-checker/internal/worker"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	targets := []models.Target{
		{ID: 1, URL: "https://google.com"},
		{ID: 2, URL: "https://github.com"},
		{ID: 3, URL: "https://non-existent-site-123.com"},
	}

	taskChan := make(chan models.Target, 10)
	resultChan := make(chan models.Result, 10)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	sched := scheduler.NewScheduler(targets)
	wrk := worker.NewWorker()
	proc := processor.NewProcessor()

	go sched.Run(ctx, taskChan)

	for i := 0; i < 3; i++ {
		go wrk.Start(ctx, taskChan, resultChan)
	}

	go proc.Run(ctx, resultChan)
	<-ctx.Done()
	time.Sleep(time.Second)
}
