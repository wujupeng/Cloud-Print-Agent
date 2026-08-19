package cloudlink

import (
	"context"
	"fmt"
	"net"
	"time"

	"go.uber.org/zap"

	"github.com/cloud-print/agent/internal/domain"
	"github.com/cloud-print/agent/internal/errs"
)

type Resolver struct {
	logger *zap.Logger
}

func NewResolver(logger *zap.Logger) *Resolver {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Resolver{logger: logger}
}

func (r *Resolver) Resolve(ctx context.Context, endpoint string) (ip string, latencyMs int, err error) {
	start := time.Now()

	type result struct {
		ips []net.IP
		err error
	}
	ch := make(chan result, 1)
	go func() {
		ips, err := net.DefaultResolver.LookupIP(ctx, "ip4", endpoint)
		ch <- result{ips, err}
	}()

	select {
	case <-ctx.Done():
		latencyMs = int(time.Since(start).Milliseconds())
		r.logger.Warn("dns resolve cancelled",
			zap.String("endpoint", endpoint),
			zap.Int("latency_ms", latencyMs),
		)
		return "", latencyMs, errs.Wrap(errs.ErrDNSResolveFail, "dns resolve cancelled", ctx.Err())
	case res := <-ch:
		latencyMs = int(time.Since(start).Milliseconds())
		if res.err != nil {
			r.logger.Warn("dns resolve failed",
				zap.String("endpoint", endpoint),
				zap.Int("latency_ms", latencyMs),
				zap.Error(res.err),
			)
			return "", latencyMs, errs.Wrap(errs.ErrDNSResolveFail, "resolve "+endpoint, res.err)
		}
		if len(res.ips) == 0 {
			r.logger.Warn("dns resolve returned no ip",
				zap.String("endpoint", endpoint),
			)
			return "", latencyMs, errs.Newf(errs.ErrDNSResolveFail, "no IP for %s", endpoint)
		}
		ip = res.ips[0].String()
		r.logger.Debug("dns resolve ok",
			zap.String("endpoint", endpoint),
			zap.String("ip", ip),
			zap.Int("latency_ms", latencyMs),
		)
		return ip, latencyMs, nil
	}
}

func DeriveURL(protocol domain.CloudProtocol, endpoint string) string {
	return fmt.Sprintf("%s://%s", protocol, endpoint)
}