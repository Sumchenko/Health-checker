package storage

import (
	"database/sql"
	"fmt"
	"health-checker/internal/models"
	"log"
	"time"

	_ "github.com/lib/pq"
)

type Storage struct {
	db *sql.DB
}

func NewStorage(host, port, user, password, dbname string) (*Storage, error) {
	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		host, port, user, password, dbname)

	var finalDB *sql.DB
	var lastErr error

	for i := 0; i < 10; i++ {
		db, err := sql.Open("postgres", dsn)
		if err != nil {
			lastErr = err
			log.Printf("Попытка %d: ошибка открытия: %v", i+1, err)
			time.Sleep(2 * time.Second)
			continue
		}

		if err = db.Ping(); err != nil {
			lastErr = err
			db.Close()
			log.Printf("Попытка %d: база не отвечает: %v", i+1, err)
			time.Sleep(2 * time.Second)
			continue
		}

		// Если мы дошли сюда, значит всё ок.
		// Сохраняем рабочее соединение в переменную ВНЕ цикла.
		log.Println("Успешное подключение к базе данных!")
		finalDB = db
		break
	}

	if finalDB == nil {
		return nil, fmt.Errorf("не удалось подключиться к БД после всех попыток: %v", lastErr)
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

	if _, err := finalDB.Exec(query); err != nil {
		finalDB.Close()
		return nil, fmt.Errorf("ошибка создания таблицы: %w", err)
	}

	return &Storage{db: finalDB}, nil
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
