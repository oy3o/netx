package netx

import (
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// 扩展 mockConn 以记录 Deadline 调用
type mockDeadlineConn struct {
	mockConn
	readDeadline  time.Time
	writeDeadline time.Time
}

func (m *mockDeadlineConn) SetReadDeadline(t time.Time) error {
	m.readDeadline = t
	return nil
}

func (m *mockDeadlineConn) SetWriteDeadline(t time.Time) error {
	m.writeDeadline = t
	return nil
}

func TestWithIOTimeout(t *testing.T) {
	readTimeout := 500 * time.Millisecond
	writeTimeout := 1000 * time.Millisecond

	// 模拟底层连接
	baseConn := &mockDeadlineConn{}

	// 创建带超时的 Listener
	l := WithIOTimeout(readTimeout, writeTimeout)(&mockListener{
		acceptFunc: func() (net.Conn, error) {
			return baseConn, nil
		},
	})

	// 接受连接
	conn, err := l.Accept()
	assert.NoError(t, err)
	assert.NotNil(t, conn)

	// 验证是否设置了 Deadline
	// 注意：由于执行时间差异，Deadline 应该是 time.Now() + timeout，这里检查是否非零且大致在范围内
	assert.False(t, baseConn.readDeadline.IsZero(), "ReadDeadline should be set")
	assert.False(t, baseConn.writeDeadline.IsZero(), "WriteDeadline should be set")

	assert.WithinDuration(t, time.Now().Add(readTimeout), baseConn.readDeadline, 100*time.Millisecond)
	assert.WithinDuration(t, time.Now().Add(writeTimeout), baseConn.writeDeadline, 100*time.Millisecond)
}
