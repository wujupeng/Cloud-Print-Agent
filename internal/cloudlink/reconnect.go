package cloudlink

import (
	"context"
	"math"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/cloud-print/agent/internal/domain"
	"github.com/cloud-print/agent/internal/errs"
	"github.com/cloud-print/agent/internal/netprobe"
)

const (
	reconnectInitDelay    = 5 * time.Second
	reconnectMaxDelay     = 300 * time.Second
	reconnectLocalWait    = 10 * time.Second
)

type Reconnect struct {
	prober    netprobe.Prober
	logger    *zap.Logger
	reconnect func(ctx context.Context) error

	mu          sync.Mutex
	attempt     int
	cancel      context.CancelFunc
	done        chan struct{}
	running     bool
}

func NewReconnect(prober netprobe.Prober, logger *zap.Logger, reconnectFn func(ctx context.Context) error) *Reconnect {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Reconnect{
		prober:    prober,
		logger:    logger,
		reconnect: reconnectFn,
	}
}

func (r *Reconnect) HandleDisconnect(ctx context.Context, err error) {
	class := classifyDisconnect(ctx, r.prober, err)
	r.logger.Warn("cloud disconnected, classify",
		zap.String("net_class", string(class)),
		zap.Error(err),
	)

	switch class {
	case domain.NetClassLocalNetFail:
		r.waitForLocalRecovery(ctx, class)
	case domain.NetClassDNSResolveFail, domain.NetClassCloudGatewayUnreachable:
		r.backoffReconnect(ctx, class)
	default:
		r.backoffReconnect(ctx, class)
	}
}

func classifyDisconnect(ctx context.Context, prober netprobe.Prober, err error) domain.NetClass {
	if ae := errs.NetClassFromErr(err); ae != domain.NetClassOK {
		return ae
	}
	if prober != nil {
		if class, _, perr := prober.Probe(ctx); perr == nil {
			return class
		}
	}
	return domain.NetClassCloudGatewayUnreachable
}

func (r *Reconnect) waitForLocalRecovery(ctx context.Context, class domain.NetClass) {
	r.mu.Lock()
	if r.running {
		r.mu.Unlock()
		return
	}
	subCtx, cancel := context.WithCancel(ctx)
	r.cancel = cancel
	r.done = make(chan struct{})
	r.running = true
	r.mu.Unlock()

	go func() {
		defer close(r.done)
		ticker := time.NewTicker(reconnectLocalWait)
		defer ticker.Stop()
		for {
			select {
			case <-subCtx.Done():
				return
			case <-ticker.C:
				if r.prober == nil {
					r.tryReconnect(subCtx)
					return
				}
				c, _, perr := r.prober.Probe(subCtx)
				if perr != nil {
					continue
				}
				if c == domain.NetClassLocalNetFail {
					r.logger.Debug("local net still down, keep waiting")
					continue
				}
				r.tryReconnect(subCtx)
				return
			}
		}
	}()
}

func (r *Reconnect) backoffReconnect(ctx context.Context, class domain.NetClass) {
	r.mu.Lock()
	if r.running {
		r.mu.Unlock()
		return
	}
	subCtx, cancel := context.WithCancel(ctx)
	r.cancel = cancel
	r.done = make(chan struct{})
	r.running = true
	r.mu.Unlock()

	go func() {
		defer close(r.done)
		for {
			if subCtx.Err() != nil {
				return
			}
			delay := r.nextDelay()
			r.logger.Info("reconnect backoff",
				zap.Int("attempt", r.attempt),
				zap.Duration("delay", delay),
				zap.String("net_class", string(class)),
			)
			select {
			case <-subCtx.Done():
				return
			case <-time.After(delay):
			}
			if r.tryReconnect(subCtx) {
				return
			}
		}
	}()
}

func (r *Reconnect) nextDelay() time.Duration {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.attempt++
	delay := reconnectInitDelay * time.Duration(int64(math.Pow(2, float64(r.attempt-1))))
	if delay > reconnectMaxDelay {
		delay = reconnectMaxDelay
	}
	return delay
}

func (r *Reconnect) tryReconnect(ctx context.Context) bool {
	if r.reconnect == nil {
		return true
	}
	if err := r.reconnect(ctx); err != nil {
		r.logger.Warn("reconnect attempt failed", zap.Error(err))
		return false
	}
	r.mu.Lock()
	r.attempt = 0
	r.running = false
	r.mu.Unlock()
	r.logger.Info("reconnect succeeded")
	return true
}

func (r *Reconnect) Stop() {
	r.mu.Lock()
	if !r.running {
		r.mu.Unlock()
		return
	}
	r.running = false
	cancel := r.cancel
	done := r.done
	r.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if done != nil {
		<-done
	}
}