package fixtures

import (
	"context"
	"fmt"
)

type Service interface {
	Run(ctx context.Context) error
}

type Worker struct{}

func (w *Worker) Run(ctx context.Context) error {
	logStart()
	return helper(ctx)
}

func helper(ctx context.Context) error {
	fmt.Println("running")
	return nil
}

func logStart() {
	fmt.Println("start")
}
