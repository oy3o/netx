package netx

import (
	"net"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestListenTCP_ReusePort 验证 TCP 端口复用
func TestListenTCP_ReusePort(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("SO_REUSEPORT is not supported on Windows")
	}

	// 1. 获取一个随机的空闲端口
	// 我们先绑定再释放，拿到一个确定的地址
	l0, err := net.Listen("tcp", "localhost:0")
	require.NoError(t, err)
	addr := l0.Addr().String()
	l0.Close()

	cfg := ListenConfig{EnableReusePort: true}

	// 2. 启动第一个监听器
	l1, err := ListenTCP("tcp", addr, cfg)
	require.NoError(t, err, "First listener should succeed")
	defer l1.Close()

	// 3. 启动第二个监听器 (绑定同一地址)
	// 如果 SO_REUSEPORT 生效，这里应该成功；否则会报 "address already in use"
	l2, err := ListenTCP("tcp", addr, cfg)
	require.NoError(t, err, "Second listener should succeed with ReusePort enabled")
	defer l2.Close()

	assert.Equal(t, l1.Addr().String(), l2.Addr().String())
}

// TestListenUDP_ReusePort 验证 UDP 端口复用 (用于 QUIC)
func TestListenUDP_ReusePort(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("SO_REUSEPORT is not supported on Windows")
	}

	// 1. 获取随机端口
	c0, err := net.ListenPacket("udp", "localhost:0")
	require.NoError(t, err)
	addr := c0.LocalAddr().String()
	c0.Close()

	cfg := ListenConfig{EnableReusePort: true}

	// 2. 启动第一个 UDP 连接
	c1, err := ListenUDP("udp", addr, cfg)
	require.NoError(t, err)
	defer c1.Close()

	// 3. 启动第二个 UDP 连接
	c2, err := ListenUDP("udp", addr, cfg)
	require.NoError(t, err)
	defer c2.Close()
}

// TestListen_Default 验证默认行为 (关闭 ReusePort)
func TestListen_Default(t *testing.T) {
	l0, err := net.Listen("tcp", "localhost:0")
	require.NoError(t, err)
	addr := l0.Addr().String()
	l0.Close()

	cfg := ListenConfig{EnableReusePort: false}

	// 1. Start L1
	l1, err := ListenTCP("tcp", addr, cfg)
	require.NoError(t, err)
	defer l1.Close()

	// 2. Start L2 (Should fail)
	_, err = ListenTCP("tcp", addr, cfg)
	assert.Error(t, err, "Should fail to bind same port without ReusePort")
}
