-- Время с часовым поясом. Старые записи считаем записанными в UTC (так работал контейнер).
ALTER TABLE health_checks RENAME COLUMN created_at TO checked_at;
ALTER TABLE health_checks ALTER COLUMN checked_at TYPE TIMESTAMPTZ USING checked_at AT TIME ZONE 'UTC';
UPDATE health_checks SET checked_at = now() WHERE checked_at IS NULL;
ALTER TABLE health_checks ALTER COLUMN checked_at SET NOT NULL;
ALTER TABLE health_checks ALTER COLUMN checked_at SET DEFAULT now();

-- Отсутствие значения — NULL, а не пустая строка / 0.
ALTER TABLE health_checks RENAME COLUMN error_msg TO error;
UPDATE health_checks SET error = NULL WHERE error = '';
UPDATE health_checks SET status_code = NULL WHERE status_code = 0;

-- Явный вердикт проверки. Для старых записей: успех = нет ошибки и код 2xx.
ALTER TABLE health_checks ADD COLUMN is_up BOOLEAN;
UPDATE health_checks SET is_up = (error IS NULL AND status_code BETWEEN 200 AND 299);
UPDATE health_checks SET error = 'неожиданный HTTP-код ' || status_code
    WHERE NOT is_up AND error IS NULL AND status_code IS NOT NULL;
ALTER TABLE health_checks ALTER COLUMN is_up SET NOT NULL;

DELETE FROM health_checks WHERE target_id IS NULL OR url IS NULL;
ALTER TABLE health_checks ALTER COLUMN target_id SET NOT NULL;
ALTER TABLE health_checks ALTER COLUMN url SET NOT NULL;
UPDATE health_checks SET response_time_ms = 0 WHERE response_time_ms IS NULL;
ALTER TABLE health_checks ALTER COLUMN response_time_ms SET NOT NULL;

-- История одной цели и выборки по диапазону времени для Grafana / очистки старых данных.
CREATE INDEX health_checks_target_checked_idx ON health_checks (target_id, checked_at DESC);
CREATE INDEX health_checks_checked_at_idx ON health_checks (checked_at);
