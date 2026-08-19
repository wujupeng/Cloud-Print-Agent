//go:build ignore

package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"sync/atomic"
	"time"

	"github.com/cloud-print/agent/internal/domain"
	"github.com/cloud-print/agent/internal/protocol"
)

const rawProbePort = 9100

func main() {
	ln, err := net.Listen("tcp", "127.0.0.1:9100")
	if err != nil {
		fmt.Fprintf(os.Stderr, "[FAIL] listen on 9100: %v\n", err)
		os.Exit(1)
	}
	defer ln.Close()

	received := &bytes.Buffer{}
	var connCount atomic.Int32
	serverDone := make(chan struct{})

	go func() {
		defer close(serverDone)
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

	time.Sleep(100 * time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	adapter := protocol.NewRawAdapter()

	fmt.Println("[STEP 1] probe 127.0.0.1:9100")
	ok, err := adapter.Probe(ctx, "127.0.0.1", rawProbePort)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[FAIL] probe error: %v\n", err)
		os.Exit(1)
	}
	if !ok {
		fmt.Fprintln(os.Stderr, "[FAIL] probe returned false, expected port 9100 reachable")
		os.Exit(1)
	}
	fmt.Println("[OK] probe 9100 reachable")

	fmt.Println("[STEP 2] probe protocol order (9100 -> 515 -> 631)")
	proto, status, err := protocol.ProbeProtocol(ctx, "127.0.0.1")
	if err != nil {
		fmt.Fprintf(os.Stderr, "[FAIL] probe protocol error: %v\n", err)
		os.Exit(1)
	}
	if proto != domain.ProtocolRAW {
		fmt.Fprintf(os.Stderr, "[FAIL] expected RAW, got %s\n", proto)
		os.Exit(1)
	}
	if status != domain.DeviceStatusOnline {
		fmt.Fprintf(os.Stderr, "[FAIL] expected ONLINE, got %s\n", status)
		os.Exit(1)
	}
	fmt.Printf("[OK] probe protocol detected %s / %s (short-circuit at 9100)\n", proto, status)

	fmt.Println("[STEP 3] send print data via RAW 9100")
	payload := []byte("VERIFY-RAW-9100-PRINT-JOB\nTestPage\n\x0c")
	err = adapter.Send(ctx, "127.0.0.1", rawProbePort, bytes.NewReader(payload), domain.PrintParams{Copies: 1})
	if err != nil {
		fmt.Fprintf(os.Stderr, "[FAIL] send error: %v\n", err)
		os.Exit(1)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && received.Len() < len(payload) {
		time.Sleep(10 * time.Millisecond)
	}

	if !bytes.Equal(payload, received.Bytes()) {
		fmt.Fprintf(os.Stderr, "[FAIL] device received %d bytes, expected %d bytes\n", received.Len(), len(payload))
		os.Exit(1)
	}
	fmt.Printf("[OK] device received %d bytes matching payload\n", received.Len())

	if connCount.Load() < 1 {
		fmt.Fprintln(os.Stderr, "[FAIL] no connections observed by mock device")
		os.Exit(1)
	}
	fmt.Printf("[OK] mock device observed %d connections\n", connCount.Load())

	fmt.Println("[DONE] raw_probe_verify passed")
}