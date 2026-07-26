package cgi

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
)

const (
	maxHeaderSize    = 64 * 1024 // 64KB max for all headers combined
	maxHeaderBytes   = 8192      // 8KB per single header line
	maxHeaderCount   = 100
	lineEnding       = "\r\n"
	headerTerminator = "\r\n\r\n"
)

var (
	ErrHeaderTooLarge  = errors.New("CGI headers exceed size limit")
	ErrTooManyHeaders  = errors.New("too many CGI headers")
	ErrInvalidHeader   = errors.New("invalid CGI header format")
	ErrMissingStatus   = errors.New("missing Status header")
	ErrHeaderInjection = errors.New("control character in CGI header")
)

// Response holds the parsed PHP response.
type Response struct {
	StatusCode int
	Headers    http.Header
	Body       io.Reader
	Stderr     []byte
}

// ParseResponse reads CGI-style headers from stdout and pairs them with the body.
// stderr is the PHP stderr output (not part of headers).
func ParseResponse(stdout, stderr []byte) (*Response, error) {
	if len(stdout) == 0 {
		return &Response{
			StatusCode: 200,
			Headers:    http.Header{},
			Body:       bytes.NewReader(nil),
			Stderr:     stderr,
		}, nil
	}

	// Find header terminator.
	termIdx := bytes.Index(stdout, []byte(headerTerminator))
	if termIdx < 0 {
		// No header terminator found — body-only mode.
		return &Response{
			StatusCode: 200,
			Headers:    http.Header{},
			Body:       bytes.NewReader(stdout),
			Stderr:     stderr,
		}, nil
	}

	headerBytes := stdout[:termIdx]
	bodyBytes := stdout[termIdx+len(headerTerminator):]

	headers, err := parseHeaders(headerBytes)
	if err != nil {
		return nil, err
	}

	statusCode := 200
	if statusLine := headers.Get("Status"); statusLine != "" {
		statusCode, err = parseStatusLine(statusLine)
		if err != nil {
			return nil, err
		}
		headers.Del("Status")
	}

	return &Response{
		StatusCode: statusCode,
		Headers:    headers,
		Body:       bytes.NewReader(bodyBytes),
		Stderr:     stderr,
	}, nil
}

// ParseResponseStream reads CGI headers from a streaming reader.
func ParseResponseStream(stdout io.Reader, stderr []byte) (*Response, error) {
	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 0, maxHeaderBytes), maxHeaderBytes)
	scanner.Split(scanCRLFHeaders)

	headers := http.Header{}
	totalBytes := 0
	lineCount := 0
	foundTerminator := false

	for scanner.Scan() {
		line := scanner.Text()

		// Empty line signals end of headers.
		if line == "" {
			foundTerminator = true
			break
		}

		totalBytes += len(line) + 2 // +2 for CRLF
		if totalBytes > maxHeaderSize {
			return nil, ErrHeaderTooLarge
		}
		lineCount++
		if lineCount > maxHeaderCount {
			return nil, ErrTooManyHeaders
		}

		// Parse header line.
		name, value, err := parseHeaderLine(line)
		if err != nil {
			return nil, err
		}
		headers.Add(name, value)
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	statusCode := 200
	if statusLine := headers.Get("Status"); statusLine != "" {
		var err error
		statusCode, err = parseStatusLine(statusLine)
		if err != nil {
			return nil, err
		}
		headers.Del("Status")
	}

	// Read remaining body.
	var body io.Reader
	if foundTerminator {
		body = stdout
	} else {
		// No terminator found — body-only.
		body = bytes.NewReader(nil)
	}

	return &Response{
		StatusCode: statusCode,
		Headers:    headers,
		Body:       body,
		Stderr:     stderr,
	}, nil
}

// parseHeaders parses a block of CGI header lines.
func parseHeaders(data []byte) (http.Header, error) {
	headers := http.Header{}
	totalBytes := 0
	lineCount := 0

	lines := bytes.Split(data, []byte(lineEnding))
	for _, line := range lines {
		if len(line) == 0 {
			continue
		}

		// Per-line size check.
		if len(line) > maxHeaderBytes {
			return nil, ErrHeaderTooLarge
		}

		totalBytes += len(line) + 2
		if totalBytes > maxHeaderSize {
			return nil, ErrHeaderTooLarge
		}
		lineCount++
		if lineCount > maxHeaderCount {
			return nil, ErrTooManyHeaders
		}

		name, value, err := parseHeaderLine(string(line))
		if err != nil {
			return nil, err
		}
		headers.Add(name, value)
	}

	return headers, nil
}

// parseHeaderLine parses a single "Name: Value" header line.
func parseHeaderLine(line string) (name, value string, err error) {
	colonIdx := strings.IndexByte(line, ':')
	if colonIdx < 0 {
		return "", "", ErrInvalidHeader
	}

	name = strings.TrimSpace(line[:colonIdx])
	value = strings.TrimSpace(line[colonIdx+1:])

	if name == "" {
		return "", "", ErrInvalidHeader
	}

	// Reject control characters in header names and values (except CR/LF already stripped).
	for _, r := range name {
		if r < 32 || r == 127 {
			return "", "", ErrHeaderInjection
		}
	}
	for _, r := range value {
		if (r < 32 && r != '\t') || r == 127 {
			return "", "", ErrHeaderInjection
		}
	}

	return name, value, nil
}

// parseStatusLine parses "200 OK" or "200" into a status code.
func parseStatusLine(s string) (int, error) {
	s = strings.TrimSpace(s)
	parts := strings.SplitN(s, " ", 2)
	code, err := strconv.Atoi(parts[0])
	if err != nil || code < 100 || code > 599 {
		return 0, ErrInvalidHeader
	}
	return code, nil
}

// scanCRLFHeaders is a bufio.Scanner split function for CRLF-terminated header lines.
func scanCRLFHeaders(data []byte, atEOF bool) (advance int, token []byte, err error) {
	if atEOF && len(data) == 0 {
		return 0, nil, nil
	}

	// Look for CRLF.
	for i := 0; i < len(data)-1; i++ {
		if data[i] == '\r' && data[i+1] == '\n' {
			line := data[:i]
			return i + 2, line, nil
		}
	}

	// If we have "\r\n\r\n" ending, we've hit the terminator.
	if len(data) >= 2 && data[len(data)-2] == '\r' && data[len(data)-1] == '\n' {
		return len(data), data[:len(data)-2], nil
	}

	// Request more data.
	if !atEOF {
		return 0, nil, nil
	}

	// At EOF, return what we have.
	return len(data), data, nil
}
