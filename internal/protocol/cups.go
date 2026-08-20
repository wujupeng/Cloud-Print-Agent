package protocol

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"

	"github.com/cloud-print/agent/internal/domain"
	"github.com/cloud-print/agent/internal/errs"
)

type CupsAdapter struct{}

func NewCupsAdapter() *CupsAdapter {
	return &CupsAdapter{}
}

func (a *CupsAdapter) Probe(ctx context.Context, printerName string, port int) (bool, error) {
	cmd := exec.CommandContext(ctx, "lpstat", "-p", printerName)
	if err := cmd.Run(); err != nil {
		return false, nil
	}
	return true, nil
}

func (a *CupsAdapter) Send(ctx context.Context, printerName string, port int, data io.Reader, params domain.PrintParams) error {
	tmpFile, err := os.CreateTemp("", "cups-print-*")
	if err != nil {
		return errs.Wrap(errs.ErrProtocolSendFail, "create temp file", err)
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	if _, err := io.Copy(tmpFile, data); err != nil {
		tmpFile.Close()
		return errs.Wrap(errs.ErrProtocolSendFail, "write temp file", err)
	}
	tmpFile.Close()

	args := []string{"-d", printerName}
	if params.Copies > 1 {
		args = append(args, "-n", fmt.Sprintf("%d", params.Copies))
	}
	if size, ok := params.Extra["paper_size"]; ok && size != "" {
		args = append(args, "-o", "media="+size)
	}
	if params.Orientation == "landscape" {
		args = append(args, "-o", "landscape")
	}
	args = append(args, tmpPath)

	cmd := exec.CommandContext(ctx, "lp", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return errs.Wrap(errs.ErrProtocolSendFail, fmt.Sprintf("lp failed: %s", string(output)), err)
	}
	return nil
}