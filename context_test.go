package netx

import (
	"context"
	"net"
	"testing"
	"time"
)

func TestWithContext_Lifecycle(t *testing.T) {
	// Mock the underlying connection
	mc := &mockConn{}

	// Create the listener
	l := WithContext(func(c net.Conn) context.Context {
		return context.WithValue(context.Background(), "id", 123)
	})(&mockListener{
		acceptFunc: func() (net.Conn, error) {
			return mc, nil
		},
	})

	// Accept connection
	conn, err := l.Accept()
	if err != nil {
		t.Fatalf("Accept failed: %v", err)
	}

	// Verify Context values
	ctx := GetContext(conn)
	if val, ok := ctx.Value("id").(int); !ok || val != 123 {
		t.Errorf("Context value missing or incorrect")
	}

	// Verify Context is NOT done yet
	select {
	case <-ctx.Done():
		t.Fatal("Context should not be done yet")
	default:
	}

	// Close the connection
	if err := conn.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// Verify Context IS done now (wait a tiny bit for async propagation if any, usually sync)
	select {
	case <-ctx.Done():
		// Success
		if ctx.Err() != context.Canceled {
			t.Errorf("Expected context.Canceled, got %v", ctx.Err())
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Context was not canceled after connection close")
	}
}
