package netx

import (
	"net"
	"testing"
	"time"
)

func TestWithKeepAlive_Integration(t *testing.T) {
	// Listen on a random port
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen failed: %v", err)
	}
	defer ln.Close()

	// Wrap with KeepAlive
	wrappedLn := WithKeepAlive(time.Minute)(ln)

	// Accept in background
	errChan := make(chan error, 1)
	go func() {
		conn, err := wrappedLn.Accept()
		if err != nil {
			errChan <- err
			return
		}
		// Ensure it's usable
		conn.Close()
		errChan <- nil
	}()

	// Dial
	cli, err := net.Dial("tcp", ln.Addr().String())
	if err != nil {
		t.Fatalf("Dial failed: %v", err)
	}
	cli.Close()

	// Check result
	if err := <-errChan; err != nil {
		t.Errorf("Accept failed with KeepAlive wrapper: %v", err)
	}
}

func TestWithKeepAlive_NonTCP(t *testing.T) {
	// Ensure it doesn't panic for non-TCP connections (e.g. Unix socket or Mock)
	l := WithKeepAlive(0)(&mockListener{})
	conn, err := l.Accept()
	if err != nil {
		t.Fatalf("Accept failed: %v", err)
	}
	conn.Close()
}
