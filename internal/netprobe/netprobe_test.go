package netprobe_test

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloud-print/agent/internal/domain"
	"github.com/cloud-print/agent/internal/netprobe"
)

func TestClassify(t *testing.T) {
	cases := []struct {
		name      string
		local     bool
		dns       bool
		gateway   bool
		want      domain.NetClass
	}{
		{"all ok", true, true, true, domain.NetClassOK},
		{"local fail", false, true, true, domain.NetClassLocalNetFail},
		{"local fail ignores dns", false, false, true, domain.NetClassLocalNetFail},
		{"local fail ignores all", false, false, false, domain.NetClassLocalNetFail},
		{"dns fail", true, false, true, domain.NetClassDNSResolveFail},
		{"dns fail ignores gateway", true, false, false, domain.NetClassDNSResolveFail},
		{"gateway fail", true, true, false, domain.NetClassCloudGatewayUnreachable},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := netprobe.Classify(c.local, c.dns, c.gateway)
			assert.Equal(t, c.want, got)
		})
	}
}

func TestClassify_OKHelpers(t *testing.T) {
	ok := netprobe.Classify(true, true, true)
	assert.True(t, ok.IsOK())
	assert.False(t, ok.IsLocalFail())

	lf := netprobe.Classify(false, true, true)
	assert.False(t, lf.IsOK())
	assert.True(t, lf.IsLocalFail())
}

func TestProbeGateway_Reachable(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer ln.Close()

	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			c.Close()
		}
	}()

	addr := ln.Addr().(*net.TCPAddr)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	ok, err := netprobe.ProbeGateway(ctx, "127.0.0.1", addr.Port)
	require.NoError(t, err)
	assert.True(t, ok)
}

func TestProbeGateway_Unreachable(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := ln.Addr().(*net.TCPAddr)
	ln.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ok, err := netprobe.ProbeGateway(ctx, "127.0.0.1", addr.Port)
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestProbeGateway_Cancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	ok, err := netprobe.ProbeGateway(ctx, "192.0.2.1", 443)
	assert.Error(t, err)
	assert.False(t, ok)
}

func TestProbeDNS_Cancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, err := netprobe.ProbeDNS(ctx, "nonexistent.invalid")
	require.Error(t, err)
}