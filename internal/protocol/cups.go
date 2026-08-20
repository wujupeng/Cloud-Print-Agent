package protocol

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

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

	printFile := tmpPath

	if converted, err := convertOfficeToPDF(ctx, tmpPath); err == nil && converted != "" {
		printFile = converted
		defer os.Remove(converted)
	}

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
	args = append(args, printFile)

	cmd := exec.CommandContext(ctx, "lp", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return errs.Wrap(errs.ErrProtocolSendFail, fmt.Sprintf("lp failed: %s", string(output)), err)
	}
	return nil
}

func convertOfficeToPDF(ctx context.Context, filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	buf := make([]byte, 8)
	n, _ := file.Read(buf)
	file.Close()
	if n < 4 {
		return "", nil
	}

	isOffice := false
	ext := ""
	if bytes.HasPrefix(buf, []byte{0x50, 0x4B, 0x03, 0x04}) {
		isOffice = true
		ext = ".docx"
	}
	if bytes.HasPrefix(buf, []byte{0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0xE1}) {
		isOffice = true
		ext = ".doc"
	}

	if !isOffice {
		return "", nil
	}

	extPath := filePath + ext
	if err := os.Rename(filePath, extPath); err != nil {
		return "", err
	}
	defer func() {
		if _, err := os.Stat(extPath); err == nil {
			os.Rename(extPath, filePath)
		}
	}()

	dir := filepath.Dir(extPath)
	loCmd := exec.CommandContext(ctx, "libreoffice",
		"--headless",
		"-env:UserInstallation=file:///var/lib/cloud-print-agent/lo_profile",
		"--convert-to", "pdf",
		"--outdir", dir, extPath)
	loEnv := os.Environ()
	for i, e := range loEnv {
		if strings.HasPrefix(e, "HOME=") {
			loEnv[i] = "HOME=/var/lib/cloud-print-agent"
			break
		}
	}
	loCmd.Env = loEnv
	if err := loCmd.Run(); err != nil {
		return "", err
	}

	base := strings.TrimSuffix(filepath.Base(extPath), filepath.Ext(extPath))
	pdfPath := filepath.Join(dir, base+".pdf")

	if _, err := os.Stat(pdfPath); err != nil {
		return "", err
	}

	return pdfPath, nil
}
