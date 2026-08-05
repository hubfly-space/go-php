// Package fastcgi implements a FastCGI client for communicating with PHP-FPM.
//
// This package implements the Responder role only. It handles FCGI_BEGIN_REQUEST,
// FCGI_PARAMS, FCGI_STDIN, FCGI_STDOUT, FCGI_STDERR, and FCGI_END_REQUEST records.
package fastcgi

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"time"
)

// FastCGI protocol constants.
const (
	protoVersion   uint8 = 1
	typeBeginReq   uint8 = 1
	typeAbortReq   uint8 = 2
	typeEndRequest uint8 = 3
	typeParams     uint8 = 4
	typeStdin      uint8 = 5
	typeStdout     uint8 = 6
	typeStderr     uint8 = 7

	roleResponder uint16 = 1

	flagKeepConn uint8 = 1

	statusRequestComplete int32 = 0
	statusCantMultiplex   int32 = 1
	statusOverloaded      int32 = 2
	statusUnknownRole     int32 = 3

	maxRecordContent = 65535
	paddingSize      = 8

	// FCGI header size.
	headerSize = 8
)

// ProtocolStatus represents the FastCGI protocol status from END_REQUEST.
type ProtocolStatus int32

const (
	ProtocolRequestComplete ProtocolStatus = ProtocolStatus(statusRequestComplete)
	ProtocolCantMultiplex   ProtocolStatus = ProtocolStatus(statusCantMultiplex)
	ProtocolOverloaded      ProtocolStatus = ProtocolStatus(statusOverloaded)
	ProtocolUnknownRole     ProtocolStatus = ProtocolStatus(statusUnknownRole)
)

// DefaultMaxResponseSize is the maximum size (64MB) for stdout/stderr response buffers.
const DefaultMaxResponseSize = 64 * 1024 * 1024

// ErrResponseTooLarge indicates the backend response exceeded the maximum size.
var ErrResponseTooLarge = errors.New("fastcgi: response size exceeded limit")

// Record is a parsed FastCGI record.
type Record struct {
	Version       uint8
	Type          uint8
	RequestID     uint16
	ContentLength uint16
	PaddingLength uint8
	Content       []byte
}

// EndRequestData holds the fields from an FCGI_END_REQUEST record.
type EndRequestData struct {
	AppStatus      int32
	ProtocolStatus ProtocolStatus
}

// Client is a FastCGI client that sends one request per connection.
type Client struct {
	conn   net.Conn
	reader *bufio.Reader
	nextID uint16
	mu     sync.Mutex
}

// NewClient creates a FastCGI client connected to addr.
func NewClient(addr string, timeout time.Duration) (*Client, error) {
	conn, err := net.DialTimeout("unix", addr, timeout)
	if err != nil {
		return nil, fmt.Errorf("fastcgi: dial: %w", err)
	}
	return &Client{
		conn:   conn,
		reader: bufio.NewReader(conn),
		nextID: 1,
	}, nil
}

// Close closes the connection.
func (c *Client) Close() error {
	return c.conn.Close()
}

// Execute sends a FastCGI request and reads the response.
// params is the CGI environment map. stdin is the request body.
// It returns stdout, stderr, and the end-request status.
func (c *Client) Execute(ctx context.Context, params map[string]string, stdin io.Reader) (stdout, stderr []byte, endReq *EndRequestData, err error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, nil, nil, err
	}

	c.mu.Lock()
	reqID := c.nextID
	c.nextID++
	c.mu.Unlock()

	// Set write/read deadline based on context or default.
	if dl, ok := ctx.Deadline(); ok {
		_ = c.conn.SetDeadline(dl)
	} else {
		_ = c.conn.SetDeadline(time.Now().Add(60 * time.Second))
	}

	// Send BEGIN_REQUEST.
	if err := c.sendBeginRequest(reqID); err != nil {
		return nil, nil, nil, err
	}

	// Send PARAMS.
	if err := c.sendParams(reqID, params); err != nil {
		return nil, nil, nil, err
	}

	// Send STDIN.
	if err := c.sendStdin(reqID, stdin); err != nil {
		return nil, nil, nil, err
	}

	// Read response.
	return c.readResponse(ctx, reqID)
}

func (c *Client) sendBeginRequest(reqID uint16) error {
	content := make([]byte, 8)
	binary.BigEndian.PutUint16(content[0:2], roleResponder)
	content[4] = flagKeepConn // keep connection open for response

	return c.writeRecord(Record{
		Version:       protoVersion,
		Type:          typeBeginReq,
		RequestID:     reqID,
		ContentLength: 8,
		Content:       content,
	})
}

func (c *Client) sendParams(reqID uint16, params map[string]string) error {
	// Sort params for deterministic encoding? No, FastCGI doesn't require order.
	// But we encode length-prefixed name/value pairs.
	var buf bytes.Buffer
	for k, v := range params {
		if err := encodeNameValuePair(&buf, k, v); err != nil {
			return err
		}
	}

	// Send the params content.
	if err := c.writeRecord(Record{
		Version:       protoVersion,
		Type:          typeParams,
		RequestID:     reqID,
		ContentLength: uint16(buf.Len()),
		Content:       buf.Bytes(),
	}); err != nil {
		return err
	}

	// Send empty PARAMS terminator.
	return c.writeRecord(Record{
		Version:       protoVersion,
		Type:          typeParams,
		RequestID:     reqID,
		ContentLength: 0,
	})
}

func (c *Client) sendStdin(reqID uint16, stdin io.Reader) error {
	if stdin == nil {
		// Send empty STDIN terminator.
		return c.writeRecord(Record{
			Version:       protoVersion,
			Type:          typeStdin,
			RequestID:     reqID,
			ContentLength: 0,
		})
	}

	buf := make([]byte, maxRecordContent)
	for {
		n, readErr := stdin.Read(buf)
		if n > 0 {
			if err := c.writeRecord(Record{
				Version:       protoVersion,
				Type:          typeStdin,
				RequestID:     reqID,
				ContentLength: uint16(n),
				Content:       append([]byte(nil), buf[:n]...),
			}); err != nil {
				return err
			}
		}
		if readErr != nil {
			if errors.Is(readErr, io.EOF) {
				break
			}
			return fmt.Errorf("fastcgi: stdin read: %w", readErr)
		}
	}

	// Send empty STDIN terminator.
	return c.writeRecord(Record{
		Version:       protoVersion,
		Type:          typeStdin,
		RequestID:     reqID,
		ContentLength: 0,
	})
}

func (c *Client) readResponse(ctx context.Context, reqID uint16) (stdout, stderr []byte, endReq *EndRequestData, err error) {
	var stdoutBuf, stderrBuf bytes.Buffer

	for {
		select {
		case <-ctx.Done():
			return nil, nil, nil, ctx.Err()
		default:
		}

		rec, err := c.readRecord()
		if err != nil {
			if ctxErr := ctx.Err(); ctxErr != nil {
				return nil, nil, nil, ctxErr
			}
			return nil, nil, nil, fmt.Errorf("fastcgi: read record: %w", err)
		}

		if rec.RequestID != reqID {
			continue // ignore records for other requests
		}

		switch rec.Type {
		case typeStdout:
			if len(rec.Content) == 0 {
				// Empty STDOUT means end of stdout stream.
				continue
			}
			if stdoutBuf.Len()+len(rec.Content) > DefaultMaxResponseSize {
				return nil, nil, nil, ErrResponseTooLarge
			}
			stdoutBuf.Write(rec.Content)
		case typeStderr:
			if len(rec.Content) == 0 {
				continue
			}
			if stderrBuf.Len()+len(rec.Content) > DefaultMaxResponseSize {
				return nil, nil, nil, ErrResponseTooLarge
			}
			stderrBuf.Write(rec.Content)
		case typeEndRequest:
			if len(rec.Content) < 8 {
				return nil, nil, nil, errors.New("fastcgi: short END_REQUEST")
			}
			endReq = &EndRequestData{
				AppStatus:      int32(binary.BigEndian.Uint32(rec.Content[0:4])),
				ProtocolStatus: ProtocolStatus(binary.BigEndian.Uint32(rec.Content[4:8])),
			}
			return stdoutBuf.Bytes(), stderrBuf.Bytes(), endReq, nil
		default:
			// Ignore unknown record types.
		}
	}
}

func (c *Client) writeRecord(rec Record) error {
	var buf bytes.Buffer
	buf.Grow(headerSize + int(rec.ContentLength) + paddingSize)

	// Header.
	buf.WriteByte(rec.Version)
	buf.WriteByte(rec.Type)
	binary.Write(&buf, binary.BigEndian, rec.RequestID)
	binary.Write(&buf, binary.BigEndian, rec.ContentLength)
	// Padding length (align to 8 bytes).
	pad := (paddingSize - (int(rec.ContentLength) % paddingSize)) % paddingSize
	buf.WriteByte(byte(pad))
	buf.WriteByte(0) // reserved

	// Content.
	buf.Write(rec.Content)

	// Padding.
	for i := 0; i < pad; i++ {
		buf.WriteByte(0)
	}

	_, err := c.conn.Write(buf.Bytes())
	return err
}

func (c *Client) readRecord() (*Record, error) {
	header := make([]byte, headerSize)
	if _, err := io.ReadFull(c.reader, header); err != nil {
		return nil, err
	}

	contentLen := binary.BigEndian.Uint16(header[4:6])
	padLen := header[6]

	rec := &Record{
		Version:       header[0],
		Type:          header[1],
		RequestID:     binary.BigEndian.Uint16(header[2:4]),
		ContentLength: contentLen,
		PaddingLength: padLen,
	}

	if contentLen > 0 {
		rec.Content = make([]byte, contentLen)
		if _, err := io.ReadFull(c.reader, rec.Content); err != nil {
			return nil, err
		}
	}

	// Skip padding.
	if padLen > 0 {
		pad := make([]byte, padLen)
		if _, err := io.ReadFull(c.reader, pad); err != nil {
			return nil, err
		}
	}

	return rec, nil
}

// encodeNameValuePair encodes a name/value pair into the FastCGI PARAMS format.
func encodeNameValuePair(w io.Writer, name, value string) error {
	nLen := len(name)
	vLen := len(value)

	// Encode name length.
	if err := encodeLength(w, nLen); err != nil {
		return err
	}
	// Encode value length.
	if err := encodeLength(w, vLen); err != nil {
		return err
	}

	// Write name and value.
	if _, err := io.WriteString(w, name); err != nil {
		return err
	}
	_, err := io.WriteString(w, value)
	return err
}

// encodeLength writes a FastCGI length value (high bit = 4-byte encoding).
func encodeLength(w io.Writer, length int) error {
	if length < 128 {
		_, err := w.Write([]byte{byte(length)})
		return err
	}
	b := make([]byte, 4)
	b[0] = byte(length>>24) | 0x80
	b[1] = byte(length >> 16)
	b[2] = byte(length >> 8)
	b[3] = byte(length)
	_, err := w.Write(b)
	return err
}

// DecodeLength reads a FastCGI length value from a byte slice.
// Returns the decoded length and number of bytes consumed.
func DecodeLength(data []byte) (int, int, error) {
	if len(data) == 0 {
		return 0, 0, errors.New("fastcgi: empty length data")
	}
	if data[0]&0x80 != 0 {
		// 4-byte encoding.
		if len(data) < 4 {
			return 0, 0, errors.New("fastcgi: truncated 4-byte length")
		}
		length := int(data[0]&0x7f)<<24 |
			int(data[1])<<16 |
			int(data[2])<<8 |
			int(data[3])
		return length, 4, nil
	}
	return int(data[0]), 1, nil
}

// EncodeRecord is exported for testing. Writes a raw FastCGI record.
func EncodeRecord(rec Record) []byte {
	var buf bytes.Buffer
	pad := (paddingSize - (int(rec.ContentLength) % paddingSize)) % paddingSize

	buf.WriteByte(rec.Version)
	buf.WriteByte(rec.Type)
	b := make([]byte, 2)
	binary.BigEndian.PutUint16(b, rec.RequestID)
	buf.Write(b)
	binary.BigEndian.PutUint16(b, rec.ContentLength)
	buf.Write(b)
	buf.WriteByte(byte(pad))
	buf.WriteByte(0)
	buf.Write(rec.Content)
	for i := 0; i < pad; i++ {
		buf.WriteByte(0)
	}
	return buf.Bytes()
}

// DecodeRecord is exported for testing. Decodes a raw FastCGI record.
func DecodeRecord(r io.Reader) (*Record, error) {
	header := make([]byte, headerSize)
	if _, err := io.ReadFull(r, header); err != nil {
		return nil, err
	}
	contentLen := binary.BigEndian.Uint16(header[4:6])
	padLen := header[6]
	rec := &Record{
		Version:       header[0],
		Type:          header[1],
		RequestID:     binary.BigEndian.Uint16(header[2:4]),
		ContentLength: contentLen,
		PaddingLength: padLen,
	}
	if contentLen > 0 {
		rec.Content = make([]byte, contentLen)
		if _, err := io.ReadFull(r, rec.Content); err != nil {
			return nil, err
		}
	}
	if padLen > 0 {
		pad := make([]byte, padLen)
		if _, err := io.ReadFull(r, pad); err != nil {
			return nil, err
		}
	}
	return rec, nil
}
