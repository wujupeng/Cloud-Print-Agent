package opsapi

import (
	"net"
	"net/http"
	"strings"

	"go.uber.org/zap"

	"github.com/cloud-print/agent/internal/observability"
)

func isLocalAddr(remoteAddr string) bool {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	host = strings.TrimSpace(host)
	switch host {
	case "127.0.0.1", "::1", "localhost", "":
		return true
	default:
		ip := net.ParseIP(host)
		if ip != nil && ip.IsLoopback() {
			return true
		}
	}
	return false
}

func LocalOnly(audit *observability.AuditLogger, logger *zap.Logger) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !isLocalAddr(r.RemoteAddr) {
				if audit != nil {
					audit.LogOpsAccess(r.URL.Path, r.Method, r.RemoteAddr)
				}
				if logger != nil {
					logger.Warn("ops api access denied",
						zap.String("remote_addr", r.RemoteAddr),
						zap.String("method", r.Method),
						zap.String("path", r.URL.Path),
					)
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte(`{"error":"access denied","code":"OPS_ACCESS_DENIED"}`))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}