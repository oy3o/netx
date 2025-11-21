package netx

import (
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestWithProxyProtocol(t *testing.T) {
	// 1. 创建监听器
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	// 应用 Proxy Protocol 中间件
	// 允许 localhost 发送 proxy header
	wrappedL := WithProxyProtocol([]string{"127.0.0.1"})(l)
	defer wrappedL.Close()

	errCh := make(chan error, 1)

	// 2. Server 端接受连接并验证 RemoteAddr
	go func() {
		conn, err := wrappedL.Accept()
		if err != nil {
			errCh <- err
			return
		}
		defer conn.Close()

		// 读取一点数据确保握手完成
		buf := make([]byte, 5)
		_, err = conn.Read(buf)
		if err != nil {
			errCh <- err
			return
		}

		// 验证：RemoteAddr 应该是 Proxy Header 里指定的 IP，而不是回环地址
		host, _, _ := net.SplitHostPort(conn.RemoteAddr().String())
		if host != "10.0.0.1" {
			errCh <- fmt.Errorf("expected remote IP 10.0.0.1, got %s", host)
			return
		}
		errCh <- nil
	}()

	// 3. Client 端模拟 LB 发送 PROXY Header
	conn, err := net.Dial("tcp", l.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	// 发送 PROXY v1 header
	// PROXY TCP4 SRC_IP DST_IP SRC_PORT DST_PORT\r\n
	header := "PROXY TCP4 10.0.0.1 20.0.0.1 12345 80\r\n"
	_, err = conn.Write([]byte(header))
	if err != nil {
		t.Fatal(err)
	}

	// 发送真实数据
	_, err = conn.Write([]byte("hello"))
	if err != nil {
		t.Fatal(err)
	}

	// 4. 等待结果
	select {
	case err := <-errCh:
		assert.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("Timeout waiting for proxy connection")
	}
}
