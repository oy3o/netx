package netx

import (
	"context"
	"crypto/tls"
	"net"
	"testing"
)

type simpleWrapper struct {
	net.Conn
}

func (s *simpleWrapper) Unwrap() net.Conn { return s.Conn }

// --- Tests for netx.go ---

func TestChain(t *testing.T) {
	l := &mockListener{}

	// Middleware that adds a tag
	tagMiddleware := func(tag string) Middleware {
		return func(next net.Listener) net.Listener {
			// In a real case this would wrap, here we just return next to verify chain call order works types
			return next
		}
	}

	// Apply chain
	wrapped := Chain(l, tagMiddleware("A"), tagMiddleware("B"))

	if wrapped != l {
		// Since our mock middleware returns 'next', it should effectively be 'l'
		// In a real wrapping scenario, 'wrapped' would be the outermost wrapper.
		// This test mainly ensures compile-time types and runtime execution don't panic.
	}
}

func TestGetContext(t *testing.T) {
	baseConn := &mockConn{}

	// Case 1: No Context
	ctx := GetContext(baseConn)
	if ctx == nil {
		t.Fatal("Expected background context, got nil")
	}

	// Case 2: With Context (using our own WithContext middleware logic manually)
	factory := func(c net.Conn) context.Context {
		return context.WithValue(context.Background(), "key", "value")
	}

	// Create a listener to generate a ctxConn
	l := WithContext(factory)(&mockListener{
		acceptFunc: func() (net.Conn, error) {
			return baseConn, nil
		},
	})

	ctxConn, _ := l.Accept()

	// Wrap it multiple times to test recursion
	wrappedConn := &simpleWrapper{Conn: &simpleWrapper{Conn: ctxConn}}

	// Test retrieval
	foundCtx := GetContext(wrappedConn)
	if foundCtx.Value("key") != "value" {
		t.Errorf("Failed to retrieve context through wrappers")
	}
}

// TestAsTCPConn_TLS 验证能够穿透 TLS 连接获取底层的 TCP 连接
func TestAsTCPConn_TLS(t *testing.T) {
	// 1. 创建真实的 TCP 监听和连接，因为 AsTCPConn 需要 *net.TCPConn 类型断言
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Setup listener failed: %v", err)
	}
	defer l.Close()

	go func() {
		c, err := l.Accept()
		if err == nil {
			c.Close()
		}
	}()

	rawConn, err := net.Dial("tcp", l.Addr().String())
	if err != nil {
		t.Fatalf("Dial failed: %v", err)
	}
	defer rawConn.Close()

	// 2. 模拟 TLS 包装
	// 我们不需要进行握手，只需要包装结构体
	tlsConn := tls.Client(rawConn, &tls.Config{InsecureSkipVerify: true})

	// 3. 模拟中间件 Wrapper 包装 (如流量整形、Context等)
	var wrappedConn net.Conn = &simpleWrapper{Conn: tlsConn}

	// 4. 测试解包
	tc := AsTCPConn(wrappedConn)
	if tc == nil {
		t.Fatal("Expected *net.TCPConn, got nil")
	}

	// 确保获取到的是真的 TCP 连接 (可以设置 KeepAlive)
	// 注意：在某些平台上，对已关闭的连接设置 KeepAlive 可能会报错，但这证明了我们拿到了对象
	_ = tc.SetKeepAlive(true)
}

// TestAsTCPConn_DeepNested 验证复杂嵌套下的解包能力：Wrapper -> TLS -> Wrapper -> TCPConn
func TestAsTCPConn_DeepNested(t *testing.T) {
	// 1. 建立真实 TCP 连接
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()

	go func() {
		// 忽略 Accept 错误，如果测试结束导致 l 被关闭，c 可能为 nil
		c, err := l.Accept()
		if err == nil && c != nil {
			c.Close()
		}
	}()

	tcpConn, err := net.Dial("tcp", l.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer tcpConn.Close()

	// 2. 构建洋葱模型：Layer3(TLS(Layer2(TCP)))
	layer1 := &simpleWrapper{Conn: tcpConn}
	// 模拟 TLS (使用 Client 包装，不需要握手即可获得结构体)
	tlsConn := tls.Client(layer1, &tls.Config{InsecureSkipVerify: true})
	layer2 := &simpleWrapper{Conn: tlsConn}

	// 3. 尝试解包
	unwrapped := AsTCPConn(layer2)
	if unwrapped == nil {
		t.Fatal("Failed to unwrap deep nested connection")
	}

	// 验证是否真的是底层的 TCP 连接
	// 如果是，我们应该能设置 TCP 特有属性
	if err := unwrapped.SetNoDelay(true); err != nil {
		t.Errorf("Unwrapped connection is not functional: %v", err)
	}
}
