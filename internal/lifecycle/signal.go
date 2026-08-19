package lifecycle

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"go.uber.org/zap"
)

func WaitForSignal(ctx context.Context) os.Signal {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)

	select {
	case sig := <-sigCh:
		return sig
	case <-ctx.Done():
		return syscall.SIGTERM
	}
}

func SetupGracefulShutdown(ctx context.Context, app *App) context.Context {
	ctx, cancel := context.WithCancel(ctx)

	go func() {
		sig := WaitForSignal(ctx)
		if app != nil && app.logger != nil {
			app.logger.Info("received signal, initiating graceful shutdown",
				zap.String("signal", sig.String()),
			)
		}
		cancel()
	}()

	return ctx
}