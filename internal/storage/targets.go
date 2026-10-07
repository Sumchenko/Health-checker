package storage

import (
	"context"
	"health-checker/internal/models"
	"time"

	"github.com/jackc/pgx/v5"
)

// ListEnabledTargets возвращает включённые цели. Незаданные (NULL) настройки возвращаются нулями,
// что для models.Target означает «использовать значение по умолчанию».
func (s *Storage) ListEnabledTargets(ctx context.Context) ([]models.Target, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, name, url,
		       COALESCE(expected_status, 0),
		       COALESCE(keyword, ''),
		       headers,
		       COALESCE(interval_seconds, 0),
		       COALESCE(timeout_seconds, 0),
		       enabled
		FROM targets
		WHERE enabled
		ORDER BY id`)
	if err != nil {
		return nil, err
	}

	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (models.Target, error) {
		var t models.Target
		var intervalSec, timeoutSec int
		err := row.Scan(&t.ID, &t.Name, &t.URL, &t.ExpectedStatus, &t.Keyword, &t.Headers,
			&intervalSec, &timeoutSec, &t.Enabled)
		t.Interval = time.Duration(intervalSec) * time.Second
		t.Timeout = time.Duration(timeoutSec) * time.Second
		return t, err
	})
}
