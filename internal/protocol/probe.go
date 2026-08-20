package protocol

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/cloud-print/agent/internal/domain"
)

const probeTimeout = 3 * time.Second

type protoPort struct {
	proto domain.Protocol
	port  int
}

var probeOrder = []protoPort{
	{domain.ProtocolRAW, 9100},
	{domain.ProtocolLPR, 515},
	{domain.ProtocolIPP, 631},
}

func ProbeProtocol(ctx context.Context, ip string) (domain.Protocol, domain.DeviceStatus, error) {
	for _, pp := range probeOrder {
		ok, err := probePort(ctx, ip, pp.port)
		if err != nil {
			return domain.ProtocolUnknown, domain.DeviceStatusProbeFailed, err
		}
		if ok {
			return pp.proto, domain.DeviceStatusOnline, nil
		}
	}
	return domain.ProtocolUnknown, domain.DeviceStatusProbeFailed, nil
}

func probePort(ctx context.Context, ip string, port int) (bool, error) {
	addr := fmt.Sprintf("%s:%d", ip, port)

	type result struct {
		ok  bool
		err error
	}
	ch := make(chan result, 1)
	go func() {
		conn, err := net.DialTimeout("tcp", addr, probeTimeout)
		if err != nil {
			ch <- result{false, nil}
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

func AdapterFor(protocol domain.Protocol) PrintAdapter {
	switch protocol {
	case domain.ProtocolRAW:
		return NewRawAdapter()
	case domain.ProtocolLPR:
		return NewLprAdapter()
	case domain.ProtocolIPP:
		return NewIppAdapter()
	case domain.ProtocolCUPS:
		return NewCupsAdapter()
	default:
		return nil
	}
}