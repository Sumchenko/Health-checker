package worker

import (
	"context"
	"health-checker/internal/models"
	"net/http"
	"time"
)

type Worker struct {
	client *http.Client
}

func NewWorker() *Worker {
	return &Worker{
		client: &http.Client{
			Timeout: 5 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:        100,
				IdleConnTimeout:     90 * time.Second,
				TLSHandshakeTimeout: 10 * time.Second,
			},
		},
	}
}

func (w *Worker) check(ctx context.Context, target models.Target) models.Result {
	start := time.Now()

	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target.URL, nil)
	if err != nil {
		return models.Result{
			TargetID:  target.ID,
			Err:       err,
			Timestamp: time.Now()}
	}

	resp, err := w.client.Do(req)
	if err != nil {
		return models.Result{
			TargetID:     target.ID,
			Err:          err,
			ResponseTime: time.Since(start),
			Timestamp:    time.Now()}
	}
	defer resp.Body.Close()

	return models.Result{
		TargetID:     target.ID,
		StasusCode:   resp.StatusCode,
		ResponseTime: time.Since(start),
		Timestamp:    time.Now()}
}

func (w *Worker) start(ctx context.Context, tasks <-chan models.Target, results chan<- models.Result) {
	for {
		select {
		case <-ctx.Done():
			return
		case task, ok := <-tasks:
			if !ok {
				return
			}

			results <- w.check(ctx, task)
		}

	}
}
