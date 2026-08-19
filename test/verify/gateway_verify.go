//go:build ignore

package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/cloud-print/agent/internal/netprobe"
)

const (
	gatewayIP   = "210.22.123.254"
	gatewayPort = 443
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	start := time.Now()
	ok, err := netprobe.ProbeGateway(ctx, gatewayIP, gatewayPort)
	elapsed := time.Since(start)

	if err != nil {
		fmt.Fprintf(os.Stderr, "[FAIL] gateway probe %s:%d: %v\n", gatewayIP, gatewayPort, err)
		os.Exit(1)
	}

	if !ok {
		fmt.Fprintf(os.Stderr, "[FAIL] gateway %s:%d unreachable (elapsed=%v)\n", gatewayIP, gatewayPort, elapsed)
		os.Exit(1)
	}

	fmt.Printf("[OK] gateway %s:%d reachable (elapsed=%v)\n", gatewayIP, gatewayPort, elapsed)
	fmt.Println("[DONE] gateway_verify passed")
}