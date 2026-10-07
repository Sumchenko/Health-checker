package models

import "time"

type Target struct {
	ID   int
	Name string
	URL  string
	// Interval — как часто проверять цель. 0 — использовать значение по умолчанию.
	Interval time.Duration
	// Timeout — таймаут одной проверки. 0 — использовать значение по умолчанию.
	Timeout time.Duration
}

type Result struct {
	TargetID int
	URL      string
	// IsUp — итоговый вердикт проверки: цель доступна и ответила ожидаемым кодом.
	IsUp bool
	// StatusCode — HTTP-код ответа, 0 если ответа не было.
	StatusCode   int
	ResponseTime time.Duration
	CheckedAt    time.Time
	// Error — причина неудачи (сетевая ошибка или неожиданный код), пусто при успехе.
	Error string
}
