package storage

import (
	"database/sql"
	"fmt"
	"health-checker/internal/models"
	"log"
	"time"
)

type Storage struct {
	db *sql.DB
}

func NewStorage(host, port, user, password, dbname string) (*Storage, error) {
	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		host, port, user, password, dbname)

	var db *sql.DB
	var err error

	for i := 0; i < 5; i++ {
		db, err := sql.Open("postgres", dsn)
		if err != nil {
			err = db.Ping()
			if err == nil {
				break
			}
		}
		log.Printf("База данных еще не готова, ждем... (попытка %d/5)", i+1)
		time.Sleep(2 * time.Second)
	}

	if err != nil {
		return nil, fmt.Errorf("не удалось подключиться к БД: %w", err)
	}

	query := `
	CREATE TABLE IF NOT EXISTS health_checks (
		id SERIAL PRIMARY KEY,
		target_id INT,
		url TEXT,
		status_code INT,
		response_time_ms INT,
		error_msg TEXT,
		created_at TIMESTAMP
	);`

	if _, err := db.Exec(query); err != nil {
		return nil, fmt.Errorf("ошибка создания таблицы: %w", err)
	}

	return &Storage{db: db}, nil
}

func (s *Storage) SaveResult(res models.Result) error {
	errMsg := ""
	if res.Err != nil {
		errMsg = res.Err.Error()
	}

	query := `INSERT INTO health_checks (target_id, url, status_code, response_time_ms, error_msg, created_at)
			  VALUES ($1, $2, $3, $4, $5, $6)`

	_, err := s.db.Exec(query,
		res.TargetID,
		res.URL,
		res.StatusCode,
		res.ResponseTime.Milliseconds(),
		errMsg,
		res.Timestamp,
	)
	return err
}

func (s *Storage) Close() {
	s.db.Close()
}
