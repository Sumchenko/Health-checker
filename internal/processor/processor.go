package processor

import (
	"context"
	"fmt"
	"health-checker/internal/models"
)

type Processor struct {
}

func NewProcessor() *Processor {
	return &Processor{}
}

func (p *Processor) Run(ctx context.Context, result <-chan models.Result) {
	for {
		select {
		case <-ctx.Done():
			fmt.Println("Processor: остановка обработки...")
			return

		case res, ok := <-result:
			if !ok {
				return
			}

			p.process(res)
		}
	}
}

func (p *Processor) process(res models.Result) {
	if res.Err != nil {
		fmt.Printf("[ERROR] %s (ID:%d) | Err: %v\n", res.URL, res.TargetID, res.Err)
		return
	}

	fmt.Printf("[%d] %s (ID:%d) | Time: %dms\n", res.StatusCode, res.URL, res.TargetID, res.ResponseTime.Milliseconds())
}
