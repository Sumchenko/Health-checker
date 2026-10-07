-- Исходная схема (как её создавала первая версия сервиса). IF NOT EXISTS — чтобы
-- миграции корректно применялись и к новой базе, и к базе, созданной старой версией.
CREATE TABLE IF NOT EXISTS health_checks (
    id SERIAL PRIMARY KEY,
    target_id INT,
    url TEXT,
    status_code INT,
    response_time_ms INT,
    error_msg TEXT,
    created_at TIMESTAMP
);
