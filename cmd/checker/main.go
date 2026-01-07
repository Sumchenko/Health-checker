package main

import (
	"context"
	"health-checker/internal/models"
	"health-checker/internal/processor"
	"health-checker/internal/scheduler"
	"health-checker/internal/storage"
	"health-checker/internal/worker"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	dbHost := getEnv("DB_HOST", "localhost")
	dbPort := getEnv("DB_PORT", "5432")
	dbUser := getEnv("DB_USER", "postgres")
	dbPass := getEnv("DB_PASSWORD", "password")
	dbName := getEnv("DB_NAME", "health_db")

	store, err := storage.NewStorage(dbHost, dbPort, dbUser, dbPass, dbName)
	if err != nil {
		log.Fatalf("Ошибка инициализации хранилища: %v", err)
	}
	defer store.Close()

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
	proc := processor.NewProcessor(store)

	go sched.Run(ctx, taskChan)

	for i := 0; i < 3; i++ {
		go wrk.Start(ctx, taskChan, resultChan)
	}

	go proc.Run(ctx, resultChan)
	log.Println("Сервис запущен. Нажми Ctrl+C для остановки...")
	<-ctx.Done()
	log.Println("Завершение работы...")
	time.Sleep(time.Second)
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}
