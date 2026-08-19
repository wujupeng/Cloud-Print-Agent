package protocol

import (
	"context"
	"io"

	"github.com/cloud-print/agent/internal/domain"
)

type PrintAdapter interface {
	Probe(ctx context.Context, ip string, port int) (bool, error)
	Send(ctx context.Context, ip string, port int, data io.Reader, params domain.PrintParams) error
}