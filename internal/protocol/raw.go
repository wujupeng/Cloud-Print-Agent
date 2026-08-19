package protocol

import (
	"context"
	"fmt"
	"io"
	"net"
	"time"

	"github.com/cloud-print/agent/internal/domain"
	"github.com/cloud-print/agent/internal/errs"
)

const (
	rawProbeTimeout  = 3 * time.Second
	rawDialTimeout   = 3 * time.Second
	rawDefaultPort   = 9100
)

type RawAdapter struct{}

func NewRawAdapter() *RawAdapter {
	return &RawAdapter{}
}

func (a *RawAdapter) Probe(ctx context.Context, ip string, port int) (bool, error) {
	if port <= 0 {
		port = rawDefaultPort
	}
	addr := fmt.Sprintf("%s:%d", ip, port)

	type result struct {
		ok  bool
		err error
	}
	ch := make(chan result, 1)
	go func() {
		conn, err := net.DialTimeout("tcp", addr, rawProbeTimeout)
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

func (a *RawAdapter) Send(ctx context.Context, ip string, port int, data io.Reader, _ domain.PrintParams) error {
	if port <= 0 {
		port = rawDefaultPort
	}
	addr := fmt.Sprintf("%s:%d", ip, port)

	d := net.Dialer{Timeout: rawDialTimeout}
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return errs.Wrap(errs.ErrProtocolSendFail, "raw dial", err)
	}
	defer conn.Close()

	done := make(chan error, 1)
	go func() {
		_, err := io.Copy(conn, data)
		done <- err
	}()

	select {
	case <-ctx.Done():
		return errs.Wrap(errs.ErrTaskTimeout, "raw send timeout", ctx.Err())
	case err := <-done:
		if err != nil {
			return errs.Wrap(errs.ErrProtocolSendFail, "raw copy", err)
		}
		return nil
	}
}