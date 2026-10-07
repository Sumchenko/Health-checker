-- Цели мониторинга. NULL в interval_seconds / timeout_seconds / expected_status —
-- использовать значения по умолчанию (CHECK_INTERVAL, CHECK_TIMEOUT, любой 2xx).
CREATE TABLE targets (
    id SERIAL PRIMARY KEY,
    name TEXT NOT NULL CHECK (name <> ''),
    url TEXT NOT NULL CHECK (url ~ '^https?://'),
    expected_status INT CHECK (expected_status BETWEEN 100 AND 599),
    keyword TEXT,
    headers JSONB NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(headers) = 'object'),
    interval_seconds INT CHECK (interval_seconds > 0),
    timeout_seconds INT CHECK (timeout_seconds > 0),
    enabled BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Цели, которые раньше были захардкожены в коде, переносим в таблицу, чтобы сохранить историю проверок.
INSERT INTO targets (id, name, url)
SELECT DISTINCT ON (target_id) target_id, url, url
FROM health_checks
WHERE target_id > 0 AND url ~ '^https?://'
ORDER BY target_id, checked_at DESC;

SELECT setval(pg_get_serial_sequence('targets', 'id'), COALESCE((SELECT max(id) FROM targets), 0) + 1, false);

-- История удаляется вместе с целью.
DELETE FROM health_checks WHERE target_id NOT IN (SELECT id FROM targets);
ALTER TABLE health_checks
    ADD CONSTRAINT health_checks_target_id_fkey
    FOREIGN KEY (target_id) REFERENCES targets (id) ON DELETE CASCADE;
