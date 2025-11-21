package netx

import (
	"net"
	"testing"
	"time"
)

func TestWithLimit(t *testing.T) {
	maxConns := 2

	// Create a listener wrapper
	l := WithLimit(maxConns)(&mockListener{
		acceptFunc: func() (net.Conn, error) {
			return &mockConn{}, nil
		},
	})

	// 1. Take up all slots
	conns := make([]net.Conn, maxConns)
	for i := 0; i < maxConns; i++ {
		c, err := l.Accept()
		if err != nil {
			t.Fatalf("Failed to accept valid connection %d: %v", i, err)
		}
		conns[i] = c
	}

	// 2. Try to accept one more in a goroutine (should block)
	done := make(chan struct{})
	go func() {
		c, _ := l.Accept()
		c.Close()
		close(done)
	}()

	// Ensure it blocks
	select {
	case <-done:
		t.Fatal("Accept should have blocked")
	case <-time.After(50 * time.Millisecond):
		// Pass: it blocked
	}

	// 3. Release a slot by closing a connection
	conns[0].Close()

	// 4. Verify the blocked Accept proceeds
	select {
	case <-done:
		// Pass
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Accept should have proceeded after slot release")
	}
}

// TestLimitConn_CloseIdempotent 验证 Close 的幂等性，确保不会多次归还信号量
func TestLimitConn_CloseIdempotent(t *testing.T) {
	// 限制只能有 1 个连接
	l := WithLimit(1)(&mockListener{
		acceptFunc: func() (net.Conn, error) {
			return &mockConn{}, nil
		},
	})
	// 确保测试结束时关闭 Listener，这会释放阻塞在 Accept 中的 Goroutine
	defer l.Close()

	// 1. 消耗掉这唯一的名额
	c1, err := l.Accept()
	if err != nil {
		t.Fatal(err)
	}

	// 2. 第一次关闭，应该释放名额
	c1.Close()

	// 3. 第二次关闭，不应 panic，也不应错误地释放名额（导致信号量计数错误）
	c1.Close()

	// 4. 验证名额是否可用
	// 重新获取
	c2, err := l.Accept()
	if err != nil {
		t.Fatal("Should be able to accept again")
	}
	defer c2.Close()

	// 尝试获取第二个（应该阻塞/失败，这里用非阻塞 select 验证）
	done := make(chan struct{})
	go func() {
		l.Accept() // 如果 l.Close() 被调用，这里会返回 error，不再阻塞
		close(done)
	}()

	select {
	case <-done:
		t.Fatal("Should restrict to 1 connection, idempotency check failed")
	case <-time.After(50 * time.Millisecond):
		// Pass: blocked as expected
	}
}

// TestLimitListener_AcceptError 验证底层 Accept 失败时，预扣的信号量被归还
func TestLimitListener_AcceptError(t *testing.T) {
	mockErr := net.ErrClosed
	l := WithLimit(1)(&mockListener{
		acceptFunc: func() (net.Conn, error) {
			// 模拟底层 Accept 失败
			return nil, mockErr
		},
	})

	// 1. 调用 Accept，内部会先获取信号量，然后调用底层 Accept 失败
	_, err := l.Accept()
	if err != mockErr {
		t.Fatalf("Expected mock error, got %v", err)
	}

	// 2. 验证信号量是否已归还
	// 如果未归还，下一次成功的 Accept 将会阻塞（因为 limit=1）
	// 我们替换底层的 acceptFunc 让它成功
	l.(*limitListener).Listener = &mockListener{
		acceptFunc: func() (net.Conn, error) {
			return &mockConn{}, nil
		},
	}

	successConn, err := l.Accept()
	if err != nil {
		t.Fatalf("Semaphore should be released on Accept error: %v", err)
	}
	successConn.Close()
}
