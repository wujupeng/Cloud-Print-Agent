package protocol

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"time"

	"github.com/cloud-print/agent/internal/domain"
	"github.com/cloud-print/agent/internal/errs"
)

const (
	lprProbeTimeout = 3 * time.Second
	lprDialTimeout  = 3 * time.Second
	lprDefaultPort  = 515
	lprDefaultQueue = "lp"
	lprLocalHost    = "localhost"
)

type LprAdapter struct {
	queue string
}

func NewLprAdapter() *LprAdapter {
	return &LprAdapter{queue: lprDefaultQueue}
}

func (a *LprAdapter) WithQueue(q string) *LprAdapter {
	if q != "" {
		a.queue = q
	}
	return a
}

func (a *LprAdapter) Probe(ctx context.Context, ip string, port int) (bool, error) {
	if port <= 0 {
		port = lprDefaultPort
	}
	addr := fmt.Sprintf("%s:%d", ip, port)

	type result struct {
		ok  bool
		err error
	}
	ch := make(chan result, 1)
	go func() {
		conn, err := net.DialTimeout("tcp", addr, lprProbeTimeout)
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

func (a *LprAdapter) Send(ctx context.Context, ip string, port int, data io.Reader, _ domain.PrintParams) error {
	if port <= 0 {
		port = lprDefaultPort
	}
	addr := fmt.Sprintf("%s:%d", ip, port)

	d := net.Dialer{Timeout: lprDialTimeout}
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return errs.Wrap(errs.ErrProtocolSendFail, "lpr dial", err)
	}
	defer conn.Close()

	payload, err := io.ReadAll(data)
	if err != nil {
		return errs.Wrap(errs.ErrProtocolSendFail, "lpr read data", err)
	}

	bw := bufio.NewWriter(conn)
	br := bufio.NewReader(conn)

	if err := lprSendDataFile(bw, br, payload); err != nil {
		return err
	}
	if err := lprSendControlFile(bw, br, a.queue, len(payload)); err != nil {
		return err
	}
	return nil
}

func lprSendDataFile(bw *bufio.Writer, br *bufio.Reader, payload []byte) error {
	dfName := "dfA001" + lprLocalHost

	if _, err := fmt.Fprintf(bw, "\x02%d %s\x00", len(payload), dfName); err != nil {
		return errs.Wrap(errs.ErrProtocolSendFail, "lpr data cmd", err)
	}
	if err := bw.Flush(); err != nil {
		return errs.Wrap(errs.ErrProtocolSendFail, "lpr data cmd flush", err)
	}
	if err := lprReadAck(br); err != nil {
		return err
	}

	if _, err := bw.Write(payload); err != nil {
		return errs.Wrap(errs.ErrProtocolSendFail, "lpr data write", err)
	}
	if err := bw.WriteByte(0); err != nil {
		return errs.Wrap(errs.ErrProtocolSendFail, "lpr data end", err)
	}
	if err := bw.Flush(); err != nil {
		return errs.Wrap(errs.ErrProtocolSendFail, "lpr data flush", err)
	}
	return lprReadAck(br)
}

func lprSendControlFile(bw *bufio.Writer, br *bufio.Reader, queue string, dataLen int) error {
	cfName := "cfA001" + lprLocalHost
	ctrl := fmt.Sprintf("H%s\nPagent\nfdfA001\nNdoc\nUdfA001\nL%s\n",
		lprLocalHost, queue)
	_ = dataLen

	if _, err := fmt.Fprintf(bw, "\x03%d %s\x00", len(ctrl), cfName); err != nil {
		return errs.Wrap(errs.ErrProtocolSendFail, "lpr ctrl cmd", err)
	}
	if err := bw.Flush(); err != nil {
		return errs.Wrap(errs.ErrProtocolSendFail, "lpr ctrl cmd flush", err)
	}
	if err := lprReadAck(br); err != nil {
		return err
	}

	if _, err := bw.WriteString(ctrl); err != nil {
		return errs.Wrap(errs.ErrProtocolSendFail, "lpr ctrl write", err)
	}
	if err := bw.WriteByte(0); err != nil {
		return errs.Wrap(errs.ErrProtocolSendFail, "lpr ctrl end", err)
	}
	if err := bw.Flush(); err != nil {
		return errs.Wrap(errs.ErrProtocolSendFail, "lpr ctrl flush", err)
	}
	return lprReadAck(br)
}

func lprReadAck(br *bufio.Reader) error {
	b, err := br.ReadByte()
	if err != nil {
		return errs.Wrap(errs.ErrProtocolSendFail, "lpr read ack", err)
	}
	if b != 0 {
		return errs.Newf(errs.ErrProtocolSendFail, "lpr rejected: code 0x%02x", b)
	}
	return nil
}