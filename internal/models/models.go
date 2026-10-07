package models

import (
	"errors"
	"fmt"
	"maps"
	"net/url"
	"strings"
	"time"
)

type Target struct {
	ID   int
	Name string
	URL  string
	// ExpectedStatus — ожидаемый HTTP-код. 0 — подходит любой 2xx.
	ExpectedStatus int
	// Keyword — строка, которая должна присутствовать в теле ответа. Пусто — не проверяется.
	Keyword string
	// Headers — дополнительные заголовки запроса (например, apikey для Supabase).
	Headers map[string]string
	// Interval — как часто проверять цель. 0 — использовать значение по умолчанию.
	Interval time.Duration
	// Timeout — таймаут одной проверки. 0 — использовать значение по умолчанию.
	Timeout time.Duration
	Enabled bool
}

// Validate проверяет, что настройки цели корректны и по ней можно выполнять проверки.
func (t Target) Validate() error {
	var errs []error

	if strings.TrimSpace(t.Name) == "" {
		errs = append(errs, errors.New("не задано название"))
	}
	if u, err := url.Parse(t.URL); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		errs = append(errs, fmt.Errorf("URL %q должен быть вида http(s)://host/...", t.URL))
	}
	if t.ExpectedStatus != 0 && (t.ExpectedStatus < 100 || t.ExpectedStatus > 599) {
		errs = append(errs, fmt.Errorf("ожидаемый HTTP-код %d вне диапазона 100–599", t.ExpectedStatus))
	}
	if t.Interval < 0 || t.Timeout < 0 {
		errs = append(errs, errors.New("интервал и таймаут не могут быть отрицательными"))
	}
	if t.Interval > 0 && t.Timeout > 0 && t.Timeout >= t.Interval {
		errs = append(errs, fmt.Errorf("таймаут (%s) должен быть меньше интервала (%s)", t.Timeout, t.Interval))
	}
	for name := range t.Headers {
		if strings.TrimSpace(name) == "" {
			errs = append(errs, errors.New("пустое имя заголовка"))
			break
		}
	}

	return errors.Join(errs...)
}

// Equal сообщает, совпадают ли все настройки целей.
func (t Target) Equal(o Target) bool {
	return t.ID == o.ID &&
		t.Name == o.Name &&
		t.URL == o.URL &&
		t.ExpectedStatus == o.ExpectedStatus &&
		t.Keyword == o.Keyword &&
		maps.Equal(t.Headers, o.Headers) &&
		t.Interval == o.Interval &&
		t.Timeout == o.Timeout &&
		t.Enabled == o.Enabled
}

type Result struct {
	TargetID int
	URL      string
	// IsUp — итоговый вердикт проверки: цель доступна и ответила ожидаемым образом.
	IsUp bool
	// StatusCode — HTTP-код ответа, 0 если ответа не было.
	StatusCode   int
	ResponseTime time.Duration
	CheckedAt    time.Time
	// Error — причина неудачи (сетевая ошибка, неожиданный код, нет ключевого слова), пусто при успехе.
	Error string
}
