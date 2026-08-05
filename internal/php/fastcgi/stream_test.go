package fastcgi

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"net"
	"testing"
)

func TestExecuteStream(t *testing.T) {
	// Create a mock FastCGI server listener
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}
	defer l.Close()

	go func() {
		conn, err := l.Accept()
		if err != nil {
			return
		}
		defer conn.Close()

		// Read BEGIN_REQUEST, PARAMS, STDIN
		buf := make([]byte, 1024)
		_, _ = conn.Read(buf)

		// Send STDOUT record
		content := []byte("Status: 200 OK\r\nContent-Type: text/plain\r\n\r\nHello Streaming World!")
		stdoutRec := Record{
			Version:       protoVersion,
			Type:          typeStdout,
			RequestID:     1,
			ContentLength: uint16(len(content)),
			Content:       content,
		}
		_ = writeRecordTo(conn, stdoutRec)

		// Send END_REQUEST record
		endContent := make([]byte, 8)
		endRec := Record{
			Version:       protoVersion,
			Type:          typeEndRequest,
			RequestID:     1,
			ContentLength: 8,
			Content:       endContent,
		}
		_ = writeRecordTo(conn, endRec)
	}()

	conn, err := net.Dial("tcp", l.Addr().String())
	if err != nil {
		t.Fatalf("failed to dial mock: %v", err)
	}
	client := &Client{
		conn:   conn,
		reader: bufio.NewReader(conn),
		nextID: 1,
	}
	defer client.Close()

	stream, err := client.ExecuteStream(context.Background(), map[string]string{"SCRIPT_FILENAME": "/app.php"}, nil)
	if err != nil {
		t.Fatalf("ExecuteStream failed: %v", err)
	}
	defer stream.Close()

	data, err := io.ReadAll(stream)
	if err != nil {
		t.Fatalf("ReadAll failed: %v", err)
	}
	if !bytes.Contains(data, []byte("Hello Streaming World!")) {
		t.Errorf("got %q, want stream containing Hello Streaming World!", string(data))
	}
}

func writeRecordTo(conn net.Conn, rec Record) error {
	var buf bytes.Buffer
	buf.WriteByte(rec.Version)
	buf.WriteByte(rec.Type)
	buf.WriteByte(byte(rec.RequestID >> 8))
	buf.WriteByte(byte(rec.RequestID))
	buf.WriteByte(byte(rec.ContentLength >> 8))
	buf.WriteByte(byte(rec.ContentLength))
	buf.WriteByte(0) // padding
	buf.WriteByte(0) // reserved
	buf.Write(rec.Content)
	_, err := conn.Write(buf.Bytes())
	return err
}


