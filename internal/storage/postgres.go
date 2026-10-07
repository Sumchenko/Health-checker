package storage

import (
	"context"
	"fmt"
	"health-checker/internal/models"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	connectAttempts = 10
	connectDelay    = 2 * time.Second
)

type Storage struct {
	pool *pgxpool.Pool
}

// New подключается к БД (с повторными попытками, пока база поднимается) и применяет миграции.
func New(ctx context.Context, dsn string) (*Storage, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("некорректная строка подключения: %w", err)
	}

	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}

	if err := ping(ctx, pool); err != nil {
		pool.Close()
		return nil, err
	}
	slog.Info("подключение к базе данных установлено")

	if err := migrate(ctx, pool); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ошибка миграций: %w", err)
	}

	return &Storage{pool: pool}, nil
}

func ping(ctx context.Context, pool *pgxpool.Pool) error {
	var err error
	for attempt := 1; attempt <= connectAttempts; attempt++ {
		if err = pool.Ping(ctx); err == nil {
			return nil
		}
		slog.Warn("база не отвечает", "attempt", attempt, "err", err)

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(connectDelay):
		}
	}
	return fmt.Errorf("не удалось подключиться к БД после %d попыток: %w", connectAttempts, err)
}

func (s *Storage) SaveResult(ctx context.Context, res models.Result) error {
	var statusCode *int
	if res.StatusCode != 0 {
		statusCode = &res.StatusCode
	}
	var errMsg *string
	if res.Error != "" {
		errMsg = &res.Error
	}

	_, err := s.pool.Exec(ctx, `
		INSERT INTO health_checks (target_id, url, is_up, status_code, response_time_ms, error, checked_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		res.TargetID,
		res.URL,
		res.IsUp,
		statusCode,
		res.ResponseTime.Milliseconds(),
		errMsg,
		res.CheckedAt,
	)
	return err
}

func (s *Storage) Close() {
	s.pool.Close()
}
