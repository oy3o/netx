package netx

import (
	"io"
	"net"
	"time"
)

// --- Mocks ---
type mockListener struct {
	acceptFunc func() (net.Conn, error)
	closeFunc  func() error
}

func (m *mockListener) Accept() (net.Conn, error) {
	if m.acceptFunc != nil {
		return m.acceptFunc()
	}
	return &mockConn{}, nil
}

func (m *mockListener) Close() error {
	if m.closeFunc != nil {
		return m.closeFunc()
	}
	return nil
}

func (m *mockListener) Addr() net.Addr {
	return &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 8080}
}

// mockConn 用于模拟 net.Conn 接口
type mockConn struct {
	readFunc  func(b []byte) (n int, err error)
	writeFunc func(b []byte) (n int, err error)
	closeFunc func() error
}

func (c *mockConn) Read(b []byte) (n int, err error) {
	if c.readFunc != nil {
		return c.readFunc(b)
	}
	return 0, io.EOF
}

func (c *mockConn) Write(b []byte) (n int, err error) {
	if c.writeFunc != nil {
		return c.writeFunc(b)
	}
	return len(b), nil
}

func (c *mockConn) Close() error {
	if c.closeFunc != nil {
		return c.closeFunc()
	}
	return nil
}

// 辅助接口实现
func (c *mockConn) LocalAddr() net.Addr {
	return &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 12345}
}

func (c *mockConn) RemoteAddr() net.Addr {
	return &net.TCPAddr{IP: net.ParseIP("10.0.0.1"), Port: 54321}
}

func (c *mockConn) SetDeadline(t time.Time) error      { return nil }
func (c *mockConn) SetReadDeadline(t time.Time) error  { return nil }
func (c *mockConn) SetWriteDeadline(t time.Time) error { return nil }

// --- mockPacketConn definition ---

type mockPacketConn struct {
	readFromFunc func(p []byte) (n int, addr net.Addr, err error)
	writeToFunc  func(p []byte, addr net.Addr) (n int, err error)
	closeFunc    func() error
}

func (m *mockPacketConn) ReadFrom(p []byte) (n int, addr net.Addr, err error) {
	if m.readFromFunc != nil {
		return m.readFromFunc(p)
	}
	return 0, nil, io.EOF
}

func (m *mockPacketConn) WriteTo(p []byte, addr net.Addr) (n int, err error) {
	if m.writeToFunc != nil {
		return m.writeToFunc(p, addr)
	}
	return len(p), nil
}

func (m *mockPacketConn) Close() error {
	if m.closeFunc != nil {
		return m.closeFunc()
	}
	return nil
}

// 辅助方法，满足接口但测试中通常不重要
func (m *mockPacketConn) LocalAddr() net.Addr {
	return &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0}
}
func (m *mockPacketConn) SetDeadline(t time.Time) error      { return nil }
func (m *mockPacketConn) SetReadDeadline(t time.Time) error  { return nil }
func (m *mockPacketConn) SetWriteDeadline(t time.Time) error { return nil }
