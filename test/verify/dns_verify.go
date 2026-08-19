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
	verifyEndpoint  = "print.oascii.com"
	expectedGateway = "210.22.123.254"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	start := time.Now()
	ip, latencyMs, err := netprobe.ProbeDNS(ctx, verifyEndpoint)
	elapsed := time.Since(start)

	if err != nil {
		fmt.Fprintf(os.Stderr, "[FAIL] dns resolve %s: %v\n", verifyEndpoint, err)
		os.Exit(1)
	}

	fmt.Printf("[OK] dns resolve %s -> %s (probe=%dms, total=%v)\n",
		verifyEndpoint, ip, latencyMs, elapsed)

	if ip != expectedGateway {
		fmt.Fprintf(os.Stderr, "[WARN] resolved IP %s != expected gateway %s\n", ip, expectedGateway)
	} else {
		fmt.Printf("[OK] resolved IP matches expected gateway %s\n", expectedGateway)
	}

	if latencyMs > 2000 {
		fmt.Fprintf(os.Stderr, "[WARN] dns latency %dms exceeds 2000ms threshold\n", latencyMs)
	} else {
		fmt.Printf("[OK] dns latency %dms within threshold\n", latencyMs)
	}

	fmt.Println("[DONE] dns_verify passed")
}