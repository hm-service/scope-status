package core

import (
	"net"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hm-service/scope-status/i18n"
)

func TestSplitHostPort(t *testing.T) {
	cases := []struct {
		address string
		want    int
	}{
		{"10.0.0.1:51820", 51820},
		{"10.0.0.1", 80},
		{"10.0.0.1:abc", 80},
		{"::1", 80},
	}
	for _, testCase := range cases {
		_, port := splitHostPort(testCase.address, 80)
		if port != testCase.want {
			t.Errorf("splitHostPort(%q) 的端口 = %d，想要 %d", testCase.address, port, testCase.want)
		}
	}
}

func TestPingFailureDetail(t *testing.T) {
	cases := []struct {
		name   string
		stdout string
		stderr string
		code   int
		key    i18n.Key
	}{
		{"全丢包", "3 packets transmitted, 3 received, 100% packet loss, time 0ms", "", 1, i18n.KeyDetailICMPNoReply},
		{"解析失败", "", "ping: unknown host nope.invalid", 2, i18n.KeyDetailDNSFailed},
		{"网络不可达", "connect: Network is unreachable", "", 2, i18n.KeyDetailUnreachable},
		{"其它退出码", "something else", "", 3, i18n.KeyDetailPingExit},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			detail := pingFailureDetail(testCase.stdout, testCase.stderr, testCase.code)
			if detail.Key != testCase.key {
				t.Fatalf("错误键 = %q，想要 %q", detail.Key, testCase.key)
			}
		})
	}
}

func TestProbeAddressTCP(t *testing.T) {
	listener := listenTCP(t)
	address := listener.Addr().String()
	result, err := ProbeAddress(address, ProbeConfig{Method: MethodTCP, Timeout: time.Second})
	if err != nil {
		t.Fatalf("探测失败：%v", err)
	}
	if !result.Online() || result.LatencyMs == nil || result.Method != MethodTCP {
		t.Fatalf("结果不对：%+v", result)
	}

	closed := listenTCP(t)
	deadAddress := closed.Addr().String()
	closed.Close()
	result, err = ProbeAddress(deadAddress, ProbeConfig{Method: MethodTCP, Timeout: time.Second})
	if err != nil {
		t.Fatalf("探测失败：%v", err)
	}
	if result.Online() || result.Detail == nil || result.Detail.Key != i18n.KeyDetailTCPFailed {
		t.Fatalf("关掉的端口应该算不通：%+v", result)
	}
}

func TestProbeAddressHTTP(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()
	result, err := ProbeAddress(strings.TrimPrefix(server.URL, "http://"), ProbeConfig{Method: MethodHTTP, Timeout: 2 * time.Second})
	if err != nil {
		t.Fatalf("探测失败：%v", err)
	}
	if !result.Online() {
		t.Fatalf("回了 503 说明端口是通的：%+v", result)
	}
	if result.Detail == nil || result.Detail.Key != i18n.KeyDetailHTTPStatus {
		t.Fatalf("详情不对：%+v", result.Detail)
	}
}

func TestProbeLink(t *testing.T) {
	cases := []struct {
		name         string
		status       int
		expectStatus int
		online       bool
		key          i18n.Key
	}{
		{"期望 200 拿到 200", 200, 200, true, i18n.KeyDetailLinkStatus},
		{"期望 204 拿到 200", 200, 204, false, i18n.KeyDetailLinkMismatch},
		{"期望 200 拿到 500", 500, 200, false, i18n.KeyDetailLinkStatus},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(testCase.status)
			}))
			defer server.Close()
			link := Link{Name: "站点", URL: server.URL, ExpectStatus: testCase.expectStatus}
			result, err := ProbeLink(link, 2*time.Second)
			if err != nil {
				t.Fatalf("探测失败：%v", err)
			}
			if result.Online() != testCase.online {
				t.Fatalf("online = %v，想要 %v（%+v）", result.Online(), testCase.online, result)
			}
			if result.Detail == nil || result.Detail.Key != testCase.key {
				t.Fatalf("详情键 = %+v，想要 %q", result.Detail, testCase.key)
			}
		})
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	target := server.URL
	server.Close()
	result, err := ProbeLink(Link{Name: "站点", URL: target, ExpectStatus: 200}, time.Second)
	if err != nil {
		t.Fatalf("连不上不该算错误：%v", err)
	}
	if result.Online() || result.Detail == nil || result.Detail.Key != i18n.KeyDetailLinkFailed {
		t.Fatalf("关掉的站点应该算不通：%+v", result)
	}
}

func TestProbeServiceTCP(t *testing.T) {
	listener := listenTCP(t)
	host, port := splitAddress(t, listener.Addr().String())
	result, err := ProbeService(Service{Name: "tcp", Host: host, Port: port, Protocol: ProtocolTCP}, time.Second)
	if err != nil {
		t.Fatalf("探测失败：%v", err)
	}
	if !result.Online() || result.Detail.Key != i18n.KeyDetailTCPOK {
		t.Fatalf("结果不对：%+v", result)
	}
}

func TestProbeServiceUDP(t *testing.T) {
	host, port := echoUDP(t)
	result, err := ProbeService(Service{Name: "udp", Host: host, Port: port, Protocol: ProtocolUDP}, time.Second)
	if err != nil {
		t.Fatalf("探测失败：%v", err)
	}
	if !result.Online() || result.Detail.Key != i18n.KeyDetailUDPBytes {
		t.Fatalf("结果不对：%+v", result)
	}

	a2s, err := ProbeService(Service{Name: "a2s", Host: host, Port: port, Protocol: ProtocolA2S}, time.Second)
	if err != nil {
		t.Fatalf("探测失败：%v", err)
	}
	if !a2s.Online() || a2s.Detail.Key != i18n.KeyDetailA2SOK || a2s.Method != ProtocolA2S {
		t.Fatalf("A2S 结果不对：%+v", a2s)
	}
}

func TestProbeServiceUDPTimeout(t *testing.T) {
	packet, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("起不了 UDP：%v", err)
	}
	defer packet.Close()
	host, port := splitAddress(t, packet.LocalAddr().String())
	result, err := ProbeService(Service{Name: "udp", Host: host, Port: port, Protocol: ProtocolUDP}, 100*time.Millisecond)
	if err != nil {
		t.Fatalf("探测失败：%v", err)
	}
	if result.Online() || result.Detail.Key != i18n.KeyDetailUDPNoReply {
		t.Fatalf("没人回话应该算不通：%+v", result)
	}
}

func TestProbeAddressICMP(t *testing.T) {
	if _, err := exec.LookPath("ping"); err != nil {
		t.Skip("系统里没有 ping")
	}
	result, err := ProbeAddress("127.0.0.1", ProbeConfig{Method: MethodICMP, Timeout: time.Second, Count: 1})
	if err != nil {
		t.Fatalf("探测失败：%v", err)
	}
	if !result.Online() || result.LatencyMs == nil {
		t.Fatalf("回环地址应该通：%+v", result)
	}
}

func TestProbeAddressUnknownMethod(t *testing.T) {
	_, err := ProbeAddress("127.0.0.1", ProbeConfig{Method: "arp"})
	if err == nil {
		t.Fatal("不认识的探测方式应该报错")
	}
	if key := errorKey(t, err); key != i18n.KeyProbeMethod {
		t.Fatalf("错误键 = %q", key)
	}
}

func TestFaviconFetchCachesNegativeResult(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer server.Close()
	cache := NewFaviconCache()
	if found := cache.fetch(server.URL); found != nil {
		t.Fatalf("没有图标时应该返回 nil，拿到 %q", *found)
	}
	// 失败结果也要进缓存，不然每轮探测都去打人家站点
	cache.entries[server.URL] = nil
	if found := cache.For(server.URL); found != nil {
		t.Fatalf("缓存了 nil 之后也该返回 nil，拿到 %q", *found)
	}
	if inflight := len(cache.inflight); inflight != 0 {
		t.Fatalf("命中缓存不该再发起抓取，in-flight = %d", inflight)
	}
}

// For 不能阻塞：后台抓完之前先给 nil，抓完自然进缓存。
func TestFaviconForIsAsync(t *testing.T) {
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			<-release
			w.Write([]byte(`<link rel="icon" href="/icon.svg">`))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()
	var releaseOnce sync.Once
	releaseNow := func() { releaseOnce.Do(func() { close(release) }) }
	defer releaseNow()

	cache := NewFaviconCache()
	started := time.Now()
	if found := cache.For(server.URL); found != nil {
		t.Fatalf("第一次调用应该先给 nil，拿到 %q", *found)
	}
	if elapsed := time.Since(started); elapsed > 200*time.Millisecond {
		t.Fatalf("For 不该等抓取完成，花了 %v", elapsed)
	}
	releaseNow()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if found := cache.For(server.URL); found != nil {
			if *found != server.URL+"/icon.svg" {
				t.Fatalf("图标地址 = %q", *found)
			}
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("抓完的图标没有进缓存")
}

func TestFaviconFetchReadsLinkTag(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			w.Write([]byte(`<html><head><link rel="icon" href="/static/icon.svg"></head></html>`))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()
	found := NewFaviconCache().fetch(server.URL)
	if found == nil || *found != server.URL+"/static/icon.svg" {
		t.Fatalf("图标地址 = %v", found)
	}
}

func listenTCP(t *testing.T) net.Listener {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("起不了监听：%v", err)
	}
	t.Cleanup(func() { listener.Close() })
	return listener
}

func echoUDP(t *testing.T) (string, int) {
	t.Helper()
	packet, err := net.ListenPacket("udp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("起不了 UDP：%v", err)
	}
	t.Cleanup(func() { packet.Close() })
	go func() {
		buf := make([]byte, 2048)
		for {
			read, from, err := packet.ReadFrom(buf)
			if err != nil {
				return
			}
			packet.WriteTo(buf[:read], from)
		}
	}()
	return splitAddress(t, packet.LocalAddr().String())
}

func splitAddress(t *testing.T, address string) (string, int) {
	t.Helper()
	host, rawPort, err := net.SplitHostPort(address)
	if err != nil {
		t.Fatalf("地址不对：%v", err)
	}
	port, err := strconv.Atoi(rawPort)
	if err != nil {
		t.Fatalf("端口不对：%v", err)
	}
	return host, port
}
