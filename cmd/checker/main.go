package main

import (
	"context"
	"health-checker/internal/config"
	"health-checker/internal/models"
	"health-checker/internal/processor"
	"health-checker/internal/scheduler"
	"health-checker/internal/storage"
	"health-checker/internal/worker"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"
)

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, nil)))

	if err := run(); err != nil {
		slog.Error("сервис остановлен с ошибкой", "err", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	store, err := storage.New(ctx, cfg.DB.DSN())
	if err != nil {
		return err
	}
	defer store.Close()

	// Временно захардкожены — на этапе 2 цели переедут в БД.
	targets := []models.Target{
		{ID: 1, Name: "Google", URL: "https://google.com"},
		{ID: 2, Name: "GitHub", URL: "https://github.com"},
		{ID: 3, Name: "Несуществующий сайт", URL: "https://non-existent-site-123.com"},
	}

	taskChan := make(chan models.Target, len(targets))
	resultChan := make(chan models.Result, cfg.Workers)

	sched := scheduler.NewScheduler(targets, cfg.CheckInterval)
	wrk := worker.NewWorker(cfg.CheckTimeout)
	proc := processor.NewProcessor(store)

	// Порядок остановки: ctx отменён → планировщик закрывает taskChan → воркеры завершаются →
	// закрываем resultChan → процессор дописывает оставшиеся результаты → закрываем БД.
	go sched.Run(ctx, taskChan)

	var workers sync.WaitGroup
	for range cfg.Workers {
		workers.Go(func() { wrk.Start(ctx, taskChan, resultChan) })
	}

	procDone := make(chan struct{})
	go func() {
		proc.Run(resultChan)
		close(procDone)
	}()

	slog.Info("сервис запущен",
		"targets", len(targets),
		"workers", cfg.Workers,
		"interval", cfg.CheckInterval,
		"timeout", cfg.CheckTimeout,
	)

	<-ctx.Done()
	// Возвращаем стандартную обработку сигналов: повторный Ctrl+C завершит процесс сразу.
	cancel()
	slog.Info("получен сигнал остановки, завершаем работу...")

	workers.Wait()
	close(resultChan)
	<-procDone

	slog.Info("сервис остановлен")
	return nil
}
