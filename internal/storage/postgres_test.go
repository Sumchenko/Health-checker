package storage

import (
	"context"
	"health-checker/internal/config"
	"health-checker/internal/models"
	"maps"
	"os"
	"testing"
	"time"
)

// Интеграционные тесты: нужен запущенный Postgres и переменные DB_*.
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

// insertTarget создаёт цель напрямую в БД и удаляет её (вместе с историей) после теста.
func insertTarget(t *testing.T, store *Storage, sql string, args ...any) int {
	t.Helper()
	var id int
	if err := store.pool.QueryRow(context.Background(), sql+" RETURNING id", args...).Scan(&id); err != nil {
		t.Fatalf("создание цели: %v", err)
	}
	t.Cleanup(func() {
		store.pool.Exec(context.Background(), "DELETE FROM targets WHERE id = $1", id)
	})
	return id
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

func TestListEnabledTargets(t *testing.T) {
	store := newTestStorage(t)

	fullID := insertTarget(t, store, `
		INSERT INTO targets (name, url, expected_status, keyword, headers, interval_seconds, timeout_seconds)
		VALUES ('Supabase', 'https://abc.supabase.co/rest/v1/', 200, 'ok', '{"apikey": "anon"}', 300, 20)`)
	defaultsID := insertTarget(t, store, `INSERT INTO targets (name, url) VALUES ('Портфолио', 'https://example.com')`)
	disabledID := insertTarget(t, store, `INSERT INTO targets (name, url, enabled) VALUES ('Выключен', 'https://off.test', false)`)

	targets, err := store.ListEnabledTargets(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	byID := make(map[int]models.Target)
	for _, tg := range targets {
		byID[tg.ID] = tg
	}

	if _, ok := byID[disabledID]; ok {
		t.Error("выключенная цель не должна возвращаться")
	}

	full := byID[fullID]
	if full.Name != "Supabase" || full.ExpectedStatus != 200 || full.Keyword != "ok" ||
		full.Interval != 5*time.Minute || full.Timeout != 20*time.Second || !full.Enabled ||
		!maps.Equal(full.Headers, map[string]string{"apikey": "anon"}) {
		t.Errorf("неверно прочитана цель со всеми настройками: %+v", full)
	}

	def := byID[defaultsID]
	if def.URL != "https://example.com" || def.ExpectedStatus != 0 || def.Keyword != "" ||
		def.Interval != 0 || def.Timeout != 0 || len(def.Headers) != 0 {
		t.Errorf("незаданные настройки должны читаться нулями: %+v", def)
	}
}

func TestSaveResult(t *testing.T) {
	store := newTestStorage(t)
	ctx := context.Background()
	targetID := insertTarget(t, store, `INSERT INTO targets (name, url) VALUES ('Тест', 'https://up.test')`)

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

func TestSaveResult_UnknownTarget(t *testing.T) {
	store := newTestStorage(t)

	err := store.SaveResult(context.Background(), models.Result{TargetID: -1, URL: "https://x.test", CheckedAt: time.Now()})
	if err == nil {
		t.Error("результат для несуществующей цели должен отклоняться внешним ключом")
	}
}
