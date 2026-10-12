package httpx

import (
	"net"
	"net/http"
	"net/netip"
	"strings"
)

// ClientIP 取真实客户端 IP：来源不在可信反代里就用连接地址，
// 在的话从 X-Forwarded-For 右边往回找第一个不可信地址。
func ClientIP(r *http.Request, trusted []netip.Prefix) string {
	remote := remoteHost(r.RemoteAddr)
	remoteAddr, err := netip.ParseAddr(remote)
	if err != nil || !trustedContains(trusted, remoteAddr) {
		return remote
	}
	chain := make([]string, 0, 4)
	for _, part := range strings.Split(r.Header.Get("X-Forwarded-For"), ",") {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			chain = append(chain, trimmed)
		}
	}
	chain = append(chain, remote)
	for index := len(chain) - 1; index >= 0; index-- {
		addr, err := netip.ParseAddr(chain[index])
		if err != nil {
			return chain[index]
		}
		if !trustedContains(trusted, addr) {
			return chain[index]
		}
	}
	return remote
}

func remoteHost(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		return remoteAddr
	}
	return host
}

func trustedContains(trusted []netip.Prefix, addr netip.Addr) bool {
	for _, prefix := range trusted {
		if prefix.Contains(addr.Unmap()) {
			return true
		}
	}
	return false
}
