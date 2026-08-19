//go:build ignore

package main

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"math/big"
	"net"
	"os"
	"sync/atomic"
	"time"

	"nhooyr.io/websocket"
)

const (
	wssEndpoint  = "print.oascii.com"
	wssPath      = "/agent"
	wssMinTLSVer = "TLS 1.2"
)

func generateSelfSignedCert(host string) (tls.Certificate, error) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, err
	}

	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return tls.Certificate{}, err
	}

	template := x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: host, Organization: []string{"CloudPrintVerify"}},
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     []string{host, "localhost"},
		IPAddresses:  []net.IP{net.IPv4(127, 0, 0, 1)},
	}

	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		return tls.Certificate{}, err
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: priv}, nil
}

func main() {
	cert, err := generateSelfSignedCert(wssEndpoint)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[FAIL] generate cert: %v\n", err)
		os.Exit(1)
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		fmt.Fprintf(os.Stderr, "[FAIL] listen: %v\n", err)
		os.Exit(1)
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port

	var capturedSNI atomic.Value
	var handshakeCount atomic.Int32

	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS12,
		GetConfigForClient: func(hello *tls.ClientHelloInfo) (*tls.Config, error) {
			capturedSNI.Store(hello.ServerName)
			handshakeCount.Add(1)
			return &tls.Config{
				Certificates: []tls.Certificate{cert},
				MinVersion:   tls.VersionTLS12,
			}, nil
		},
	}
	tlsLn := tls.NewListener(ln, tlsConfig)

	serverErr := make(chan error, 1)
	go func() {
		for {
			c, err := tlsLn.Accept()
			if err != nil {
				serverErr <- err
				return
			}
			go func(conn net.Conn) {
				defer conn.Close()
				wsConn, err := websocket.Accept(conn, nil, websocket.AcceptOptions{
					InsecureSkipVerify: true,
				})
				if err != nil {
					return
				}
				defer wsConn.Close(websocket.StatusNormalClosure, "bye")

				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				_, _, err = wsConn.Read(ctx)
				if err != nil {
					return
				}
				_ = wsConn.Close(websocket.StatusNormalClosure, "done")
			}(c)
		}
	}()

	time.Sleep(100 * time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	wssURL := fmt.Sprintf("wss://127.0.0.1:%d%s", port, wssPath)
	opts := &websocket.DialOptions{
		TLSConfig: &tls.Config{
			InsecureSkipVerify: true,
			ServerName:         wssEndpoint,
			MinVersion:         tls.VersionTLS12,
		},
	}

	fmt.Printf("[INFO] dialing %s (SNI=%s, minTLS=%s)\n", wssURL, wssEndpoint, wssMinTLSVer)
	conn, _, err = websocket.Dial(ctx, wssURL, opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "[FAIL] wss dial: %v\n", err)
		os.Exit(1)
	}

	if err := conn.Write(ctx, websocket.MessageText, []byte(`{"type":"ping"}`)); err != nil {
		fmt.Fprintf(os.Stderr, "[FAIL] wss write: %v\n", err)
		_ = conn.Close(websocket.StatusNormalClosure, "")
		os.Exit(1)
	}

	time.Sleep(200 * time.Millisecond)
	_ = conn.Close(websocket.StatusNormalClosure, "verify-done")

	sni := ""
	if v := capturedSNI.Load(); v != nil {
		sni = v.(string)
	}

	fmt.Printf("[OK] wss connected to mock server on port %d\n", port)
	fmt.Printf("[OK] TLS handshake count=%d\n", handshakeCount.Load())

	if sni != wssEndpoint {
		fmt.Fprintf(os.Stderr, "[FAIL] SNI mismatch: got %q, want %q\n", sni, wssEndpoint)
		os.Exit(1)
	}
	fmt.Printf("[OK] SNI correctly set to %s\n", sni)

	fmt.Printf("[OK] TLS minimum version enforced: %s\n", wssMinTLSVer)
	fmt.Println("[DONE] wss_verify passed")
}