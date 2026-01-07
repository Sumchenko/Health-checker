package storage

import (
	"os"
	"testing"
)

func TestNewStorage_Integration(t *testing.T) {
	host := os.Getenv("DB_HOST")
	if host == "" {
		t.Skip("Пропуск интеграционного теста: DB_HOST не задан")
	}

	port := os.Getenv("DB_PORT")
	user := os.Getenv("DB_USER")
	pass := os.Getenv("DB_PASSWORD")
	dbname := os.Getenv("DB_NAME")

	store, err := NewStorage(host, port, user, pass, dbname)
	if err != nil {
		t.Fatalf("Не удалось подключиться к тестовой БД: %v", err)
	}
	defer store.Close()

	if store.db == nil {
		t.Fatal("Объект БД равен nil после инициализации")
	}
}
