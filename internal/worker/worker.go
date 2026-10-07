package worker

import (
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

// Start обрабатывает задачи, пока канал tasks не закрыт или не отменён ctx.
func (w *Worker) Start(ctx context.Context, tasks <-chan models.Target, results chan<- models.Result) {
	for {
		select {
		case <-ctx.Done():
			return
		case task, ok := <-tasks:
			if !ok {
				return
			}

			res := w.Check(ctx, task)
			// Проверка, прерванная остановкой сервиса, ничего не говорит о цели — не сохраняем её.
			if !res.IsUp && ctx.Err() != nil {
				return
			}
			results <- res
		}
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
	_, err = io.Copy(io.Discard, io.LimitReader(resp.Body, maxBodySize))
	res.ResponseTime = time.Since(start)
	if err != nil {
		res.Error = "ошибка чтения ответа: " + describeError(err, timeout)
		return res
	}

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		res.Error = fmt.Sprintf("неожиданный HTTP-код %d", resp.StatusCode)
		return res
	}

	res.IsUp = true
	return res
}

func describeError(err error, timeout time.Duration) string {
	if errors.Is(err, context.DeadlineExceeded) {
		return fmt.Sprintf("таймаут: нет ответа за %s", timeout)
	}
	return err.Error()
}
