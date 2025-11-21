package netx

import (
	"net"
	"sync/atomic"
	"testing"
)

func TestWithObserve(t *testing.T) {
	var openCount, closeCount int32

	observer := func(c net.Conn, state ConnState) {
		if state == StateOpen {
			atomic.AddInt32(&openCount, 1)
		} else {
			atomic.AddInt32(&closeCount, 1)
		}
	}

	l := WithObserve(observer)(&mockListener{})

	// 1. Accept connection
	conn, err := l.Accept()
	if err != nil {
		t.Fatalf("Accept failed: %v", err)
	}

	if atomic.LoadInt32(&openCount) != 1 {
		t.Errorf("Expected openCount 1, got %d", openCount)
	}

	// 2. Close connection
	conn.Close()

	if atomic.LoadInt32(&closeCount) != 1 {
		t.Errorf("Expected closeCount 1, got %d", closeCount)
	}

	// 3. Ensure idempotent Close doesn't trigger callback twice
	conn.Close()
	if atomic.LoadInt32(&closeCount) != 1 {
		t.Errorf("Callback should only trigger once on Close")
	}
}
