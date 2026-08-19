package protocol_test

import (
	"bytes"
	"context"
	"io"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloud-print/agent/internal/domain"
	"github.com/cloud-print/agent/internal/protocol"
)

func startTCPListener(t *testing.T) (string, int, *bytes.Buffer, *atomic.Int32, func()) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	received := &bytes.Buffer{}
	var connCount atomic.Int32

	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			connCount.Add(1)
			go func(conn net.Conn) {
				defer conn.Close()
				_, _ = io.Copy(received, conn)
			}(c)
		}
	}()

	addr := ln.Addr().(*net.TCPAddr)
	cleanup := func() {
		_ = ln.Close()
		<-done
	}
	return addr.IP.String(), addr.Port, received, &connCount, cleanup
}

func TestRawAdapter_ProbeOK(t *testing.T) {
	ip, port, _, _, cleanup := startTCPListener(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	a := protocol.NewRawAdapter()
	ok, err := a.Probe(ctx, ip, port)
	require.NoError(t, err)
	assert.True(t, ok)
}

func TestRawAdapter_ProbeFail(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := ln.Addr().(*net.TCPAddr)
	ln.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	a := protocol.NewRawAdapter()
	ok, err := a.Probe(ctx, addr.IP.String(), addr.Port)
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestRawAdapter_Send(t *testing.T) {
	ip, port, received, connCount, cleanup := startTCPListener(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	payload := []byte("RAW-PRINT-JOB-DATA-1234567890")
	a := protocol.NewRawAdapter()
	err := a.Send(ctx, ip, port, bytes.NewReader(payload), domain.PrintParams{Copies: 1})
	require.NoError(t, err)

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && received.Len() < len(payload) {
		time.Sleep(10 * time.Millisecond)
	}
	assert.Equal(t, payload, received.Bytes())
	assert.GreaterOrEqual(t, connCount.Load(), int32(1))
}

func TestRawAdapter_SendDefaultPort(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:9100")
	if err != nil {
		t.Skip("port 9100 unavailable:", err)
	}
	defer ln.Close()

	received := &bytes.Buffer{}
	done := make(chan struct{})
	go func() {
		defer close(done)
		c, err := ln.Accept()
		if err != nil {
			return
		}
		defer c.Close()
		_, _ = io.Copy(received, c)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	payload := []byte("default-port-test")
	a := protocol.NewRawAdapter()
	err = a.Send(ctx, "127.0.0.1", 0, bytes.NewReader(payload), domain.PrintParams{})
	require.NoError(t, err)
	<-done
	assert.Equal(t, payload, received.Bytes())
}

func TestRawAdapter_SendCancelled(t *testing.T) {
	ip, port, _, _, cleanup := startTCPListener(t)
	defer cleanup()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	a := protocol.NewRawAdapter()
	err := a.Send(ctx, ip, port, bytes.NewReader([]byte("x")), domain.PrintParams{})
	require.Error(t, err)
}

func TestProbeProtocol_RAWFirst(t *testing.T) {
	ln9100, err := net.Listen("tcp", "127.0.0.1:9100")
	if err != nil {
		t.Skip("port 9100 unavailable:", err)
	}
	defer ln9100.Close()
	go func() {
		for {
			c, err := ln9100.Accept()
			if err != nil {
				return
			}
			c.Close()
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	proto, status, err := protocol.ProbeProtocol(ctx, "127.0.0.1")
	require.NoError(t, err)
	assert.Equal(t, domain.ProtocolRAW, proto)
	assert.Equal(t, domain.DeviceStatusOnline, status)
}

func TestProbeProtocol_FallsToLPR(t *testing.T) {
	ln515, err := net.Listen("tcp", "127.0.0.1:515")
	if err != nil {
		t.Skip("port 515 unavailable:", err)
	}
	defer ln515.Close()
	go func() {
		for {
			c, err := ln515.Accept()
			if err != nil {
				return
			}
			c.Close()
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	proto, status, err := protocol.ProbeProtocol(ctx, "127.0.0.1")
	require.NoError(t, err)
	assert.Equal(t, domain.ProtocolLPR, proto)
	assert.Equal(t, domain.DeviceStatusOnline, status)
}

func TestProbeProtocol_FallsToIPP(t *testing.T) {
	ln631, err := net.Listen("tcp", "127.0.0.1:631")
	if err != nil {
		t.Skip("port 631 unavailable:", err)
	}
	defer ln631.Close()
	go func() {
		for {
			c, err := ln631.Accept()
			if err != nil {
				return
			}
			c.Close()
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	proto, status, err := protocol.ProbeProtocol(ctx, "127.0.0.1")
	require.NoError(t, err)
	assert.Equal(t, domain.ProtocolIPP, proto)
	assert.Equal(t, domain.DeviceStatusOnline, status)
}

func TestProbeProtocol_AllClosed(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	proto, status, err := protocol.ProbeProtocol(ctx, "192.0.2.1")
	require.NoError(t, err)
	assert.Equal(t, domain.ProtocolUnknown, proto)
	assert.Equal(t, domain.DeviceStatusProbeFailed, status)
}

func TestAdapterFor(t *testing.T) {
	assert.NotNil(t, protocol.AdapterFor(domain.ProtocolRAW))
	assert.NotNil(t, protocol.AdapterFor(domain.ProtocolLPR))
	assert.NotNil(t, protocol.AdapterFor(domain.ProtocolIPP))
	assert.Nil(t, protocol.AdapterFor(domain.ProtocolUnknown))
	assert.Nil(t, protocol.AdapterFor(domain.Protocol("XYZ")))
}

func TestProbeProtocol_Cancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, err := protocol.ProbeProtocol(ctx, "127.0.0.1")
	require.Error(t, err)
}