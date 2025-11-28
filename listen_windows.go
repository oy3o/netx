//go:build windows

package netx

import (
	"syscall"

	"github.com/rs/zerolog/log"
)

func controlReusePort(network, address string, c syscall.RawConn) error {
	// Windows 不支持标准的 SO_REUSEPORT 负载均衡机制。
	// 在开发环境下直接忽略此选项，并打印日志告知用户。
	log.Warn().Msgf("SO_REUSEPORT is not supported on Windows, ignoring for %s/%s", network, address)
	return nil
}
