package storage

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"log/slog"
	"sort"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// migrationLockID — ключ advisory lock, чтобы два экземпляра не применяли миграции одновременно.
const migrationLockID = 7_412_093

type migration struct {
	version int
	name    string
	sql     string
}

// migrate применяет ещё не применённые миграции из migrations/ по возрастанию номера.
// Каждая миграция выполняется в своей транзакции вместе с записью в schema_migrations.
func migrate(ctx context.Context, pool *pgxpool.Pool) error {
	migrations, err := loadMigrations()
	if err != nil {
		return err
	}

	conn, err := pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()

	if _, err := conn.Exec(ctx, "SELECT pg_advisory_lock($1)", migrationLockID); err != nil {
		return fmt.Errorf("блокировка миграций: %w", err)
	}
	defer conn.Exec(context.Background(), "SELECT pg_advisory_unlock($1)", migrationLockID)

	_, err = conn.Exec(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version INT PRIMARY KEY,
			name TEXT NOT NULL,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)`)
	if err != nil {
		return fmt.Errorf("создание schema_migrations: %w", err)
	}

	rows, err := conn.Query(ctx, "SELECT version FROM schema_migrations")
	if err != nil {
		return err
	}
	applied, err := pgx.CollectRows(rows, pgx.RowTo[int])
	if err != nil {
		return err
	}
	isApplied := make(map[int]bool, len(applied))
	for _, v := range applied {
		isApplied[v] = true
	}

	for _, m := range migrations {
		if isApplied[m.version] {
			continue
		}
		err := pgx.BeginFunc(ctx, conn, func(tx pgx.Tx) error {
			if _, err := tx.Exec(ctx, m.sql); err != nil {
				return err
			}
			_, err := tx.Exec(ctx, "INSERT INTO schema_migrations (version, name) VALUES ($1, $2)", m.version, m.name)
			return err
		})
		if err != nil {
			return fmt.Errorf("миграция %s: %w", m.name, err)
		}
		slog.Info("применена миграция", "name", m.name)
	}

	return nil
}

func loadMigrations() ([]migration, error) {
	entries, err := fs.ReadDir(migrationsFS, "migrations")
	if err != nil {
		return nil, err
	}

	migrations := make([]migration, 0, len(entries))
	seen := make(map[int]string)
	for _, e := range entries {
		name := e.Name()
		prefix, _, ok := strings.Cut(name, "_")
		version, err := strconv.Atoi(prefix)
		if !ok || err != nil {
			return nil, fmt.Errorf("имя миграции %q должно начинаться с номера: 0001_name.sql", name)
		}
		if prev, dup := seen[version]; dup {
			return nil, fmt.Errorf("миграции %q и %q имеют одинаковый номер", prev, name)
		}
		seen[version] = name

		body, err := migrationsFS.ReadFile("migrations/" + name)
		if err != nil {
			return nil, err
		}
		migrations = append(migrations, migration{version: version, name: name, sql: string(body)})
	}

	sort.Slice(migrations, func(i, j int) bool { return migrations[i].version < migrations[j].version })
	return migrations, nil
}
