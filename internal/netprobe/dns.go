package netprobe

import (
	"context"
	"net"
	"time"

	"github.com/cloud-print/agent/internal/errs"
)

func ProbeDNS(ctx context.Context, endpoint string) (resolvedIP string, latencyMs int, err error) {
	resolver := net.DefaultResolver
	start := time.Now()

	type result struct {
		ips []net.IP
		err error
	}
	ch := make(chan result, 1)
	go func() {
		ips, err := resolver.LookupIP(ctx, "ip4", endpoint)
		ch <- result{ips, err}
	}()

	select {
	case <-ctx.Done():
		return "", int(time.Since(start).Milliseconds()), errs.Wrap(errs.ErrDNSResolveFail, "dns resolve cancelled", ctx.Err())
	case r := <-ch:
		latencyMs = int(time.Since(start).Milliseconds())
		if r.err != nil {
			return "", latencyMs, errs.Wrap(errs.ErrDNSResolveFail, "resolve "+endpoint, r.err)
		}
		if len(r.ips) == 0 {
			return "", latencyMs, errs.Newf(errs.ErrDNSResolveFail, "no IP for %s", endpoint)
		}
		return r.ips[0].String(), latencyMs, nil
	}
}