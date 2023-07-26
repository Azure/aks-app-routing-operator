package clients

import (
	"context"
	"time"

	"github.com/Azure/aks-app-routing-operator/testing/e2e2/logger"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/runtime"
)

type errorCh[T any] struct {
	err  error
	data T
}

func pollWithLog[T any](ctx context.Context, p *runtime.Poller[T], msg string) (T, error) {
	lgr := logger.FromContext(ctx)

	resCh := make(chan errorCh[T], 1)
	go func() {
		result, err := p.PollUntilDone(nil, nil)
		resCh <- errorCh[T]{
			err:  err,
			data: result,
		}
	}()

	for {
		select {
		case res := <-resCh:
			return res.data, res.err
		case <-time.After(15 * time.Second):
			lgr.Info(msg)
		}
	}
}
