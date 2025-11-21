package netx

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/time/rate"
)

// MockBucket to track calls
type mockBucket struct {
	takeFunc func(tokens int64) error
}

func (m *mockBucket) Take(ctx context.Context, tokens int64) error {
	if m.takeFunc != nil {
		return m.takeFunc(tokens)
	}
	return nil
}

func TestWithShaper(t *testing.T) {
	readCalled := false
	writeCalled := false

	factory := func(c net.Conn) (Bucket, Bucket) {
		rB := &mockBucket{
			takeFunc: func(tokens int64) error {
				readCalled = true
				if tokens != 5 {
					t.Errorf("Expected to take 5 read tokens, got %d", tokens)
				}
				return nil
			},
		}
		wB := &mockBucket{
			takeFunc: func(tokens int64) error {
				writeCalled = true
				if tokens != 5 {
					t.Errorf("Expected to take 5 write tokens, got %d", tokens)
				}
				return nil
			},
		}
		return rB, wB
	}

	// Setup listener and connection
	mockC := &mockConn{
		readFunc: func(b []byte) (int, error) {
			// Simulate reading 5 bytes
			return 5, nil
		},
		writeFunc: func(b []byte) (int, error) {
			return len(b), nil
		},
	}
	l := WithShaper(factory)(&mockListener{
		acceptFunc: func() (net.Conn, error) {
			return mockC, nil
		},
	})

	conn, _ := l.Accept()

	// Test Read
	buf := make([]byte, 10)
	conn.Read(buf)
	if !readCalled {
		t.Error("Read bucket was not called")
	}

	// Test Write
	data := []byte("12345")
	conn.Write(data)
	if !writeCalled {
		t.Error("Write bucket was not called")
	}
}

func TestWithShaper_NilBuckets(t *testing.T) {
	// Ensure no panic or overhead if factory returns nil
	factory := func(c net.Conn) (Bucket, Bucket) {
		return nil, nil
	}
	l := WithShaper(factory)(&mockListener{})
	conn, _ := l.Accept()

	// Should return the raw connection (mockConn) not wrapped
	if _, ok := conn.(*throttledConn); ok {
		t.Error("Expected raw connection when buckets are nil")
	}
}

// TestWithPPSLimit_UDP_WriteDrop 验证 UDP 发送限流（模拟丢包）
// 策略：设置发送限流为 0 (禁止发送)。调用 WriteTo 应该返回成功，但接收端应超时收不到数据。
func TestWithPPSLimit_UDP_WriteDrop(t *testing.T) {
	// 1. 启动接收端 (Server)
	serverConn, err := net.ListenPacket("udp", "127.0.0.1:0")
	require.NoError(t, err)
	defer serverConn.Close()

	serverAddr := serverConn.LocalAddr()

	// 2. 启动发送端 (Client)，配置极端限流：Burst=0, Limit=0 (不允许任何包通过)
	clientConnRaw, err := net.ListenPacket("udp", "127.0.0.1:0")
	require.NoError(t, err)
	defer clientConnRaw.Close()

	// 应用限流中间件
	limiter := rate.NewLimiter(0, 0) // 永远不允许
	clientConn := WithPPSLimit(LimitConfig{
		Write: limiter,
	})(clientConnRaw)

	// 3. 尝试发送数据
	msg := []byte("hello world")
	n, err := clientConn.WriteTo(msg, serverAddr)

	// 验证：WriteTo 应该 "欺骗" 调用者发送成功 (为了不让上层逻辑报错)
	assert.NoError(t, err)
	assert.Equal(t, len(msg), n, "WriteTo should return full length even if dropped")

	// 4. 验证接收端：应该收不到任何数据
	buf := make([]byte, 1024)
	// 设置很短的超时
	serverConn.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
	_, _, err = serverConn.ReadFrom(buf)

	// 期望超时错误 (因为包被丢弃了)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "i/o timeout", "Server should not receive dropped packet")
}

// TestWithPPSLimit_GenericWrapper 验证非 *net.UDPConn 的 PacketConn 包装逻辑
// coverage: 覆盖 packetShaperConn 结构体
func TestWithPPSLimit_GenericWrapper(t *testing.T) {
	// 使用 mock 来模拟一个非 *net.UDPConn 的 PacketConn
	mockPC := &mockPacketConn{
		readFromFunc: func(p []byte) (n int, addr net.Addr, err error) {
			return 0, nil, nil
		},
		writeToFunc: func(p []byte, addr net.Addr) (n int, err error) {
			return len(p), nil
		},
	}

	// 包装
	// 允许通过
	limiter := rate.NewLimiter(rate.Inf, 1)
	wrapped := WithPPSLimit(LimitConfig{Write: limiter})(mockPC)

	// 验证类型：应该是 packetShaperConn 而不是 udpShaperConn
	_, isUDP := wrapped.(*udpShaperConn)
	assert.False(t, isUDP, "Should use generic wrapper for non-UDPConn")

	// 执行 Write
	n, err := wrapped.WriteTo([]byte("test"), nil)
	assert.NoError(t, err)
	assert.Equal(t, 4, n)
}

// blockingBucket 用于测试 Context 取消
type blockingBucket struct{}

func (b *blockingBucket) Take(ctx context.Context, tokens int64) error {
	// 模拟一直等待直到 Context 取消
	<-ctx.Done()
	return ctx.Err()
}

// TestWithShaper_ContextCancel 验证当等待令牌时 Context 被取消，Read/Write 能够中断
func TestWithShaper_ContextCancel(t *testing.T) {
	// 创建一个带 Cancel Context 的连接
	ctx, cancel := context.WithCancel(context.Background())
	baseConn := &mockConn{
		readFunc: func(b []byte) (int, error) { return 10, nil },
	}

	// 手动组装 throttledConn，测试其中断逻辑
	conn := &throttledConn{
		Conn: baseConn,
		ctx:  ctx,
		rB:   &blockingBucket{},
		wB:   &blockingBucket{},
	}

	// 1. 测试 Write 阻塞被取消
	errCh := make(chan error)
	go func() {
		_, err := conn.Write([]byte("test"))
		errCh <- err
	}()

	time.Sleep(10 * time.Millisecond)
	cancel() // 取消 Context

	select {
	case err := <-errCh:
		assert.ErrorIs(t, err, context.Canceled)
	case <-time.After(100 * time.Millisecond):
		t.Fatal("Write did not unblock on context cancel")
	}
}

// TestWithPPSLimit_UDP_MsgMethods 覆盖 WriteMsgUDP 和 ReadMsgUDP
func TestWithPPSLimit_UDP_MsgMethods(t *testing.T) {
	// 1. 启动真实的 UDP Server
	addr, err := net.ResolveUDPAddr("udp", "127.0.0.1:0")
	require.NoError(t, err)

	serverConn, err := net.ListenUDP("udp", addr)
	require.NoError(t, err)
	defer serverConn.Close()

	// 应用限流中间件 (Limit 设为 Inf 确保不丢包)
	limiter := rate.NewLimiter(rate.Inf, 100)
	wrappedServer := WithPPSLimit(LimitConfig{
		Read:  limiter,
		Write: limiter,
	})(serverConn).(*udpShaperConn)

	// 2. Client 发送数据
	clientConn, err := net.DialUDP("udp", nil, serverConn.LocalAddr().(*net.UDPAddr))
	require.NoError(t, err)
	defer clientConn.Close()

	msg := []byte("OOB TEST")
	_, err = clientConn.Write(msg)
	require.NoError(t, err)

	// 3. Server 使用 ReadMsgUDP 读取
	buf := make([]byte, 1024)
	oob := make([]byte, 1024)
	n, oobn, _, _, err := wrappedServer.ReadMsgUDP(buf, oob)
	assert.NoError(t, err)
	assert.Equal(t, len(msg), n)
	assert.Equal(t, 0, oobn)

	// 4. Server 使用 WriteMsgUDP 回复
	n, oobn, err = wrappedServer.WriteMsgUDP(buf[:n], nil, clientConn.LocalAddr().(*net.UDPAddr))
	assert.NoError(t, err)
	assert.Equal(t, len(msg), n)
}
