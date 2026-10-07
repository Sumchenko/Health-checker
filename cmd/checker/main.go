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

	sched := scheduler.New(store, worker.NewWorker(cfg.CheckTimeout), scheduler.Config{
		DefaultInterval: cfg.CheckInterval,
		ReloadInterval:  cfg.ReloadInterval,
		MaxConcurrent:   cfg.Workers,
	})
	proc := processor.NewProcessor(store)
	results := make(chan models.Result, cfg.Workers)

	go func() {
		<-ctx.Done()
		// Возвращаем стандартную обработку сигналов: повторный Ctrl+C завершит процесс сразу.
		cancel()
	}()

	slog.Info("сервис запущен",
		"default_interval", cfg.CheckInterval,
		"default_timeout", cfg.CheckTimeout,
		"reload_interval", cfg.ReloadInterval,
		"max_concurrent", cfg.Workers,
	)

	// Порядок остановки: ctx отменён → планировщик дожидается всех проверок и закрывает results →
	// процессор дописывает оставшиеся результаты и возвращается → закрываем БД.
	go sched.Run(ctx, results)
	proc.Run(results)

	slog.Info("сервис остановлен")
	return nil
}
