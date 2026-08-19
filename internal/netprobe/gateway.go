package netprobe

import (
	"context"
	"fmt"
	"net"
	"time"
)

const gatewayProbeTimeout = 3 * time.Second

func ProbeGateway(ctx context.Context, ip string, port int) (bool, error) {
	if port <= 0 {
		port = 443
	}
	addr := fmt.Sprintf("%s:%d", ip, port)

	type result struct {
		ok  bool
		err error
	}
	ch := make(chan result, 1)
	go func() {
		conn, err := net.DialTimeout("tcp", addr, gatewayProbeTimeout)
		if err != nil {
			ch <- result{false, err}
			return
		}
		_ = conn.Close()
		ch <- result{true, nil}
	}()

	select {
	case <-ctx.Done():
		return false, ctx.Err()
	case r := <-ch:
		return r.ok, r.err
	}
}