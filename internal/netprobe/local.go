package netprobe

import (
	"context"
	"net"
	"time"

	"github.com/cloud-print/agent/internal/errs"
)

const (
	localProbeTimeout   = 3 * time.Second
	localProbeTarget    = "8.8.8.8:80"
)

func ProbeLocalNet(ctx context.Context) (bool, error) {
	type result struct {
		ok  bool
		err error
	}
	ch := make(chan result, 1)
	go func() {
		ok, err := probeLocalNet()
		ch <- result{ok, err}
	}()

	select {
	case <-ctx.Done():
		return false, ctx.Err()
	case r := <-ch:
		return r.ok, r.err
	}
}

func probeLocalNet() (bool, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return false, errs.Wrap(errs.ErrLocalNetFail, "list interfaces", err)
	}

	if !hasUpIPv4Interface(ifaces) {
		return false, errs.New(errs.ErrLocalNetFail, "no up non-loopback IPv4 interface")
	}

	conn, err := net.DialTimeout("udp", localProbeTarget, localProbeTimeout)
	if err != nil {
		return false, errs.Wrap(errs.ErrLocalNetFail, "local route probe", err)
	}
	_ = conn.Close()
	return true, nil
}

func hasUpIPv4Interface(ifaces []net.Interface) bool {
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip == nil || ip.IsLoopback() {
				continue
			}
			if ip.To4() != nil {
				return true
			}
		}
	}
	return false
}