package protocol

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/cloud-print/agent/internal/domain"
	"github.com/cloud-print/agent/internal/errs"
)

const (
	ippProbeTimeout = 3 * time.Second
	ippDefaultPort  = 631
	ippDefaultURI   = "/ipp"
	ippHTTPTimeout  = 60 * time.Second
)

const (
	ippVersion11Major = 0x01
	ippVersion11Minor = 0x01
	ippOpCreateJob    = 0x0002
	ippOpSendDocument = 0x0006
)

const (
	ippTagOperationAttrs    = 0x01
	ippTagCharset           = 0x47
	ippTagNaturalLanguage   = 0x48
	ippTagMimeMediaType     = 0x49
	ippTagEndOfAttributes   = 0x03
	ippTagDocumentData      = 0x04
)

type IppAdapter struct {
	client *http.Client
	uri    string
}

func NewIppAdapter() *IppAdapter {
	return &IppAdapter{
		client: &http.Client{Timeout: ippHTTPTimeout},
		uri:    ippDefaultURI,
	}
}

func (a *IppAdapter) WithURI(uri string) *IppAdapter {
	if uri != "" {
		a.uri = uri
	}
	return a
}

func (a *IppAdapter) Probe(ctx context.Context, ip string, port int) (bool, error) {
	if port <= 0 {
		port = ippDefaultPort
	}
	addr := fmt.Sprintf("%s:%d", ip, port)

	type result struct {
		ok  bool
		err error
	}
	ch := make(chan result, 1)
	go func() {
		conn, err := net.DialTimeout("tcp", addr, ippProbeTimeout)
		if err != nil {
			ch <- result{false, err}
			return
		}
		_ = conn.Close()
		ch <- result{true, nil}
	}()

	select {
	case <-ctx.Done():
		return false, ctx.Err()
	case r := <-ch:
		return r.ok, r.err
	}
}

func (a *IppAdapter) Send(ctx context.Context, ip string, port int, data io.Reader, params domain.PrintParams) error {
	if port <= 0 {
		port = ippDefaultPort
	}

	payload, err := io.ReadAll(data)
	if err != nil {
		return errs.Wrap(errs.ErrProtocolSendFail, "ipp read data", err)
	}

	body := buildIPPRequest(payload, params)
	url := fmt.Sprintf("http://%s:%d%s", ip, port, a.uri)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return errs.Wrap(errs.ErrProtocolSendFail, "ipp build request", err)
	}
	req.Header.Set("Content-Type", "application/ipp")

	resp, err := a.client.Do(req)
	if err != nil {
		return errs.Wrap(errs.ErrProtocolSendFail, "ipp send", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return errs.Newf(errs.ErrProtocolSendFail, "ipp http status %d", resp.StatusCode)
	}
	return nil
}

func buildIPPRequest(data []byte, params domain.PrintParams) []byte {
	var buf bytes.Buffer

	buf.WriteByte(ippVersion11Major)
	buf.WriteByte(ippVersion11Minor)

	binary.Write(&buf, binary.BigEndian, uint16(ippOpSendDocument))
	binary.Write(&buf, binary.BigEndian, uint32(1))

	buf.WriteByte(ippTagOperationAttrs)

	writeIPPStringAttr(&buf, ippTagCharset, "attributes-charset", "utf-8")
	writeIPPStringAttr(&buf, ippTagNaturalLanguage, "attributes-natural-language", "en")

	copies := params.Copies
	if copies <= 0 {
		copies = 1
	}
	writeIPPIntegerAttr(&buf, 0x02, "copies", int32(copies))

	lastDoc := 1
	writeIPPBooleanAttr(&buf, 0x22, "last-document", lastDoc == 1)

	buf.WriteByte(ippTagDocumentData)
	buf.Write(data)

	buf.WriteByte(ippTagEndOfAttributes)
	return buf.Bytes()
}

func writeIPPStringAttr(buf *bytes.Buffer, tag byte, name, value string) {
	buf.WriteByte(tag)
	binary.Write(buf, binary.BigEndian, uint16(len(name)))
	buf.WriteString(name)
	binary.Write(buf, binary.BigEndian, uint16(len(value)))
	buf.WriteString(value)
}

func writeIPPIntegerAttr(buf *bytes.Buffer, tag byte, name string, value int32) {
	buf.WriteByte(tag)
	binary.Write(buf, binary.BigEndian, uint16(len(name)))
	buf.WriteString(name)
	binary.Write(buf, binary.BigEndian, uint16(4))
	binary.Write(buf, binary.BigEndian, value)
}

func writeIPPBooleanAttr(buf *bytes.Buffer, tag byte, name string, value bool) {
	buf.WriteByte(tag)
	binary.Write(buf, binary.BigEndian, uint16(len(name)))
	buf.WriteString(name)
	binary.Write(buf, binary.BigEndian, uint16(1))
	if value {
		buf.WriteByte(0x01)
	} else {
		buf.WriteByte(0x00)
	}
}