package processor

import (
	"context"
	"health-checker/internal/models"
	"log/slog"
	"time"
)

const saveTimeout = 5 * time.Second

type ResultSaver interface {
	SaveResult(ctx context.Context, res models.Result) error
}

type Processor struct {
	store ResultSaver
}

func NewProcessor(store ResultSaver) *Processor {
	return &Processor{
		store: store,
	}
}

// Run сохраняет результаты, пока канал не закрыт. Контекст не принимает намеренно:
// при остановке сервиса нужно дописать всё, что воркеры успели отправить.
func (p *Processor) Run(results <-chan models.Result) {
	for res := range results {
		p.process(res)
	}
}

func (p *Processor) process(res models.Result) {
	ctx, cancel := context.WithTimeout(context.Background(), saveTimeout)
	defer cancel()

	if err := p.store.SaveResult(ctx, res); err != nil {
		slog.Error("не удалось сохранить результат", "target_id", res.TargetID, "url", res.URL, "err", err)
	}

	attrs := []any{
		"target_id", res.TargetID,
		"url", res.URL,
		"status", res.StatusCode,
		"time_ms", res.ResponseTime.Milliseconds(),
	}
	if !res.IsUp {
		slog.Warn("цель недоступна", append(attrs, "err", res.Error)...)
		return
	}
	slog.Info("цель доступна", attrs...)
}
