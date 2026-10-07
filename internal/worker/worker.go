package worker

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"health-checker/internal/models"
	"io"
	"net/http"
	"time"
)

// maxBodySize ограничивает объём тела ответа, который вычитывается при проверке.
const maxBodySize = 1 << 20

const userAgent = "health-checker/1.0"

// Worker выполняет HTTP-проверки целей. Безопасен для одновременного использования.
type Worker struct {
	client         *http.Client
	defaultTimeout time.Duration
}

func NewWorker(defaultTimeout time.Duration) *Worker {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxIdleConnsPerHost = 2
	transport.IdleConnTimeout = 90 * time.Second

	return &Worker{
		// Таймаут задаётся контекстом каждого запроса, чтобы у целей мог быть свой.
		client:         &http.Client{Transport: transport},
		defaultTimeout: defaultTimeout,
	}
}

func (w *Worker) Check(ctx context.Context, target models.Target) models.Result {
	timeout := target.Timeout
	if timeout <= 0 {
		timeout = w.defaultTimeout
	}

	res := models.Result{
		TargetID:  target.ID,
		URL:       target.URL,
		CheckedAt: time.Now(),
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target.URL, nil)
	if err != nil {
		res.Error = fmt.Sprintf("некорректный запрос: %v", err)
		return res
	}
	req.Header.Set("User-Agent", userAgent)
	for name, value := range target.Headers {
		req.Header.Set(name, value)
	}

	start := time.Now()
	resp, err := w.client.Do(req)
	if err != nil {
		res.ResponseTime = time.Since(start)
		res.Error = describeError(err, timeout)
		return res
	}
	defer resp.Body.Close()

	res.StatusCode = resp.StatusCode

	// Вычитываем тело: время ответа включает загрузку страницы, а соединение возвращается в пул.
	// Целиком в память читаем, только если нужно искать ключевое слово.
	var body []byte
	limited := io.LimitReader(resp.Body, maxBodySize)
	if target.Keyword != "" {
		body, err = io.ReadAll(limited)
	} else {
		_, err = io.Copy(io.Discard, limited)
	}
	res.ResponseTime = time.Since(start)
	if err != nil {
		res.Error = "ошибка чтения ответа: " + describeError(err, timeout)
		return res
	}

	if !statusOK(resp.StatusCode, target.ExpectedStatus) {
		res.Error = fmt.Sprintf("неожиданный HTTP-код %d", resp.StatusCode)
		if target.ExpectedStatus != 0 {
			res.Error += fmt.Sprintf(" (ожидался %d)", target.ExpectedStatus)
		}
		return res
	}

	if target.Keyword != "" && !bytes.Contains(body, []byte(target.Keyword)) {
		res.Error = fmt.Sprintf("в ответе нет ключевого слова %q", target.Keyword)
		return res
	}

	res.IsUp = true
	return res
}

func statusOK(code, expected int) bool {
	if expected != 0 {
		return code == expected
	}
	return code >= 200 && code <= 299
}

func describeError(err error, timeout time.Duration) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return fmt.Sprintf("таймаут: нет ответа за %s", timeout)
	}
	return err.Error()
}
