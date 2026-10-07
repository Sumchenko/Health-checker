package storage

import (
	"context"
	"health-checker/internal/config"
	"health-checker/internal/models"
	"os"
	"testing"
	"time"
)

// Интеграционный тест: нужен запущенный Postgres и переменные DB_*.
func newTestStorage(t *testing.T) *Storage {
	t.Helper()
	if os.Getenv("DB_HOST") == "" {
		t.Skip("Пропуск интеграционного теста: DB_HOST не задан")
	}

	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	store, err := New(ctx, cfg.DB.DSN())
	if err != nil {
		t.Fatalf("Не удалось подключиться к тестовой БД: %v", err)
	}
	t.Cleanup(store.Close)
	return store
}

func TestMigrations_Idempotent(t *testing.T) {
	store := newTestStorage(t)

	// Повторный запуск не должен ничего применять и падать.
	if err := migrate(context.Background(), store.pool); err != nil {
		t.Fatalf("повторное применение миграций: %v", err)
	}

	migrations, err := loadMigrations()
	if err != nil {
		t.Fatal(err)
	}
	var applied int
	err = store.pool.QueryRow(context.Background(), "SELECT count(*) FROM schema_migrations").Scan(&applied)
	if err != nil {
		t.Fatal(err)
	}
	if applied != len(migrations) {
		t.Errorf("применено %d миграций, want %d", applied, len(migrations))
	}
}

func TestSaveResult(t *testing.T) {
	store := newTestStorage(t)
	ctx := context.Background()
	const targetID = -1 // отрицательный ID, чтобы не пересекаться с реальными целями

	t.Cleanup(func() {
		store.pool.Exec(context.Background(), "DELETE FROM health_checks WHERE target_id = $1", targetID)
	})

	checkedAt := time.Now().Truncate(time.Microsecond)
	results := []models.Result{
		{TargetID: targetID, URL: "https://up.test", IsUp: true, StatusCode: 200, ResponseTime: 120 * time.Millisecond, CheckedAt: checkedAt},
		{TargetID: targetID, URL: "https://down.test", Error: "таймаут", ResponseTime: time.Second, CheckedAt: checkedAt},
	}
	for _, r := range results {
		if err := store.SaveResult(ctx, r); err != nil {
			t.Fatalf("SaveResult: %v", err)
		}
	}

	rows, err := store.pool.Query(ctx, `
		SELECT url, is_up, status_code, response_time_ms, error, checked_at
		FROM health_checks WHERE target_id = $1 ORDER BY url DESC`, targetID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

	type row struct {
		url        string
		isUp       bool
		statusCode *int
		timeMs     int
		errMsg     *string
		checkedAt  time.Time
	}
	var got []row
	for rows.Next() {
		var r row
		if err := rows.Scan(&r.url, &r.isUp, &r.statusCode, &r.timeMs, &r.errMsg, &r.checkedAt); err != nil {
			t.Fatal(err)
		}
		got = append(got, r)
	}
	if len(got) != 2 {
		t.Fatalf("сохранено %d строк, want 2", len(got))
	}

	up, down := got[0], got[1]
	if !up.isUp || up.statusCode == nil || *up.statusCode != 200 || up.timeMs != 120 || up.errMsg != nil {
		t.Errorf("неверная строка успешной проверки: %+v", up)
	}
	if down.isUp || down.statusCode != nil || down.errMsg == nil || *down.errMsg != "таймаут" {
		t.Errorf("неверная строка неудачной проверки: %+v", down)
	}
	if !up.checkedAt.Equal(checkedAt) {
		t.Errorf("checked_at = %v, want %v (сдвиг часового пояса?)", up.checkedAt, checkedAt)
	}
}
