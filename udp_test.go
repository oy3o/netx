package netx

import (
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChainUDP(t *testing.T) {
	// 1. 创建真实的 UDP 连接
	pc, err := net.ListenPacket("udp", "127.0.0.1:0")
	require.NoError(t, err)
	defer pc.Close()

	// 2. 定义一个简单的中间件来验证链条执行
	called := false
	dummyMw := func(next net.PacketConn) net.PacketConn {
		called = true
		return next
	}

	// 3. 应用链
	// 同时测试 WithUDPBuffer (即使在非 UDPConn 上也不应 panic，虽然这里是 UDPConn)
	wrapped := ChainUDP(pc,
		WithUDPBuffer(1024, 1024),
		dummyMw,
	)

	assert.True(t, called, "Middleware in chain should be executed")
	assert.NotNil(t, wrapped)
}

// TestWithUDPBuffer_Safety 验证在非 *net.UDPConn 上调用不会 panic
func TestWithUDPBuffer_Safety(t *testing.T) {
	// mock 一个 PacketConn，它不是 *net.UDPConn
	mockPC := &mockPacketConn{
		readFromFunc: func(p []byte) (int, net.Addr, error) { return 0, nil, nil },
	}

	// 应用中间件
	// 代码中包含类型断言 c.(*net.UDPConn)，如果断言失败应直接返回原连接
	wrapped := WithUDPBuffer(1024, 1024)(mockPC)

	// 验证返回的是原对象，且没有 panic
	assert.Equal(t, mockPC, wrapped)
}
