package core

import (
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/hm-service/scope-status/i18n"
)

// Text 是一段还没渲染的文案：要么直接给的原文，要么是文案键 + 参数。
type Text struct {
	Key  i18n.Key
	Args []any
	Raw  string
}

func NewText(key i18n.Key, args ...any) *Text { return &Text{Key: key, Args: args} }

func (t *Text) Render(lang i18n.Lang) string {
	if t == nil {
		return ""
	}
	if t.Key != "" {
		return i18n.T(lang, t.Key, t.Args...)
	}
	return t.Raw
}

type ProbeResult struct {
	Address   string
	Status    string
	LatencyMs *float64
	Detail    *Text
	Method    string
}

func (r ProbeResult) Online() bool { return r.Status == StatusUp }

func latency(started int64, now int64) *float64 {
	value := float64(now-started) / 1e6
	return &value
}

// ProbeAddress 按 probe 配置探测一个内网地址。
func ProbeAddress(address string, config ProbeConfig) (ProbeResult, error) {
	switch config.Method {
	case MethodICMP:
		return probeICMP(address, config)
	case MethodTCP:
		return probeTCP(address, config)
	case MethodHTTP:
		return probeHTTP(address, config)
	default:
		return ProbeResult{}, BadRequest(i18n.KeyProbeMethod, config.Method)
	}
}

var rttRe = regexp.MustCompile(`(?i)time[=<]([\d.]+)\s*ms`)

func probeICMP(address string, config ProbeConfig) (ProbeResult, error) {
	pingPath, err := lookPath("ping")
	if err != nil {
		return ProbeResult{}, ServerError(i18n.KeyProbeNoPing)
	}
	wait := max(1, int(config.Timeout.Seconds()))
	timeout := config.Timeout + 3*time.Second // ping 自己会超时，这里再放宽 3 秒兜底
	started := nowNanos()
	stdout, stderr, code, err := runCommand(pingPath,
		[]string{"-n", "-c", strconv.Itoa(config.Count), "-W", strconv.Itoa(wait), address}, timeout)
	if err != nil {
		return ProbeResult{}, ServerError(i18n.KeyProbeMethod, err.Error())
	}
	elapsed := latency(started, nowNanos())
	if code == 0 {
		if match := rttRe.FindStringSubmatch(stdout); match != nil {
			if parsed, err := strconv.ParseFloat(match[1], 64); err == nil {
				elapsed = &parsed
			}
		}
		return ProbeResult{Address: address, Status: StatusUp, LatencyMs: elapsed, Method: MethodICMP}, nil
	}
	return ProbeResult{Address: address, Status: StatusDown, Detail: pingFailureDetail(stdout, stderr, code), Method: MethodICMP}, nil
}

func pingFailureDetail(stdout, stderr string, code int) *Text {
	text := strings.ToLower(stdout + "\n" + stderr)
	switch {
	case strings.Contains(text, "100% packet loss"), strings.Contains(text, "100% loss"):
		return NewText(i18n.KeyDetailICMPNoReply)
	case strings.Contains(text, "unknown host"), strings.Contains(text, "name or service not known"):
		return NewText(i18n.KeyDetailDNSFailed)
	case strings.Contains(text, "network is unreachable"):
		return NewText(i18n.KeyDetailUnreachable)
	default:
		return NewText(i18n.KeyDetailPingExit, code)
	}
}

func probeTCP(address string, config ProbeConfig) (ProbeResult, error) {
	host, port := splitHostPort(address, config.Port)
	started := nowNanos()
	conn, err := net.DialTimeout(ProtocolTCP, net.JoinHostPort(host, strconv.Itoa(port)), config.Timeout)
	if err == nil {
		conn.Close()
		return ProbeResult{Address: address, Status: StatusUp, LatencyMs: latency(started, nowNanos()), Method: MethodTCP}, nil
	}
	if isDNSError(err) {
		return ProbeResult{}, ServerError(i18n.KeyProbeDNS, address)
	}
	if isTimeout(err) {
		return ProbeResult{Address: address, Status: StatusDown, Detail: NewText(i18n.KeyDetailTCPTimeout, port), Method: MethodTCP}, nil
	}
	return ProbeResult{Address: address, Status: StatusDown, Detail: NewText(i18n.KeyDetailTCPFailed, err.Error()), Method: MethodTCP}, nil
}

func probeHTTP(address string, config ProbeConfig) (ProbeResult, error) {
	host, port := splitHostPort(address, config.Port)
	scheme := "http"
	if port == 443 {
		scheme = "https"
	}
	started := nowNanos()
	client := &http.Client{Timeout: config.Timeout}
	response, err := client.Do(newProbeRequest(scheme+"://"+net.JoinHostPort(host, strconv.Itoa(port))+"/", "text/html"))
	elapsed := latency(started, nowNanos())
	if err != nil {
		if isDNSError(err) {
			return ProbeResult{}, ServerError(i18n.KeyProbeDNS, address)
		}
		if isTimeout(err) {
			return ProbeResult{Address: address, Status: StatusDown, Detail: NewText(i18n.KeyDetailHTTPTimeout), Method: MethodHTTP}, nil
		}
		return ProbeResult{Address: address, Status: StatusDown, Detail: NewText(i18n.KeyDetailHTTPFailed, err.Error()), Method: MethodHTTP}, nil
	}
	defer drain(response)
	result := ProbeResult{Address: address, Status: StatusUp, LatencyMs: elapsed, Method: MethodHTTP}
	if response.StatusCode >= 400 {
		result.Detail = NewText(i18n.KeyDetailHTTPStatus, response.StatusCode)
	}
	return result, nil
}

// ProbeService 按协议探测一个服务的活性：tcp 能连上算通；udp / a2s / q3 都是「发一问、
// 收到符合该协议的回包」才算通——UDP 没有握手，空包发过去得不到任何结论。
func ProbeService(service Service, timeout time.Duration) (ProbeResult, error) {
	switch service.Protocol {
	case ProtocolTCP:
		return probeServiceTCP(service, timeout)
	case ProtocolUDP:
		return probeServiceUDP(service, timeout, service.Payload, acceptAnyReply)
	case ProtocolA2S:
		return probeServiceA2S(service, timeout)
	case ProtocolQ3:
		return probeServiceQ3(service, timeout)
	default:
		return ProbeResult{}, BadRequest(i18n.KeyProbeProtocol, service.Protocol)
	}
}

func probeServiceTCP(service Service, timeout time.Duration) (ProbeResult, error) {
	started := nowNanos()
	conn, err := net.DialTimeout(ProtocolTCP, service.Endpoint(), timeout)
	if err == nil {
		conn.Close()
		return ProbeResult{Address: service.Endpoint(), Status: StatusUp, LatencyMs: latency(started, nowNanos()),
			Detail: NewText(i18n.KeyDetailTCPOK), Method: MethodTCP}, nil
	}
	if isDNSError(err) {
		return ProbeResult{}, ServerError(i18n.KeyProbeDNS, service.Host)
	}
	if isTimeout(err) {
		return ProbeResult{Address: service.Endpoint(), Status: StatusDown,
			Detail: NewText(i18n.KeyDetailTCPTimeout, service.Port), Method: MethodTCP}, nil
	}
	return ProbeResult{Address: service.Endpoint(), Status: StatusDown,
		Detail: NewText(i18n.KeyDetailTCPRefused, err.Error()), Method: MethodTCP}, nil
}

// acceptReply 判断回包是不是「这个协议的应答」：目标端口上蹲着的可能是别的服务，
// 收到什么就一律算在线等于给自己发假消息。
type acceptReply func(reply []byte) bool

func acceptAnyReply([]byte) bool { return true }

func probeServiceUDP(service Service, timeout time.Duration, query []byte, accept acceptReply) (ProbeResult, error) {
	started := nowNanos()
	conn, err := net.DialTimeout(ProtocolUDP, service.Endpoint(), timeout)
	if err != nil {
		if isDNSError(err) {
			return ProbeResult{}, ServerError(i18n.KeyProbeDNS, service.Host)
		}
		return ProbeResult{}, ServerError(i18n.KeyProbeMethod, err.Error())
	}
	defer conn.Close()
	if err := conn.SetDeadline(now().Add(timeout)); err != nil {
		return ProbeResult{}, ServerError(i18n.KeyProbeMethod, err.Error())
	}
	if _, err := conn.Write(query); err != nil {
		if isConnectionRefused(err) {
			return ProbeResult{Address: service.Endpoint(), Status: StatusDown,
				Detail: NewText(i18n.KeyDetailUDPRefused), Method: service.Protocol}, nil
		}
		return ProbeResult{}, ServerError(i18n.KeyProbeMethod, err.Error())
	}
	buf := make([]byte, 4096)
	read, err := conn.Read(buf)
	if err != nil {
		if isConnectionRefused(err) {
			return ProbeResult{Address: service.Endpoint(), Status: StatusDown,
				Detail: NewText(i18n.KeyDetailUDPRefused), Method: service.Protocol}, nil
		}
		if isTimeout(err) {
			return ProbeResult{Address: service.Endpoint(), Status: StatusDown,
				Detail: NewText(i18n.KeyDetailUDPNoReply), Method: service.Protocol}, nil
		}
		return ProbeResult{}, ServerError(i18n.KeyProbeMethod, err.Error())
	}
	if !accept(buf[:read]) {
		return ProbeResult{Address: service.Endpoint(), Status: StatusDown,
			Detail: NewText(i18n.KeyDetailUDPBadReply), Method: service.Protocol}, nil
	}
	return ProbeResult{Address: service.Endpoint(), Status: StatusUp, LatencyMs: latency(started, nowNanos()),
		Detail: NewText(i18n.KeyDetailUDPBytes, read), Method: service.Protocol}, nil
}

var (
	a2sQuery = append([]byte{0xff, 0xff, 0xff, 0xff}, []byte("TSource Engine Query\x00")...)
	q3Query  = append([]byte{0xff, 0xff, 0xff, 0xff}, []byte("getinfo xxx\n")...)
)

// queryHeader 是全包 / 分片的分界：0xFFFFFFFF 是整包，0xFFFFFFFE 是分片的第一片
// （满员的服务器回 getstatus / A2S_INFO 会超过一个 MTU）。不是这两种前缀就不是本协议的应答。
func queryHeader(reply []byte) byte {
	if len(reply) < 4 || reply[0] != 0xff || reply[1] != 0xff || reply[2] != 0xff {
		return 0
	}
	if reply[3] != 0xff && reply[3] != 0xfe {
		return 0
	}
	return reply[3]
}

// A2S_INFO 的回包是 0x49（信息）；0x41 是挑战码、0x45 是 GoldSrc 的老格式，都说明服务器活着。
// 分片回包的第一片看不出类型，只要前缀对就算应答。
func isA2SReply(reply []byte) bool {
	switch queryHeader(reply) {
	case 0xff:
		if len(reply) < 5 {
			return false
		}
		switch reply[4] {
		case 'I', 'A', 'E':
			return true
		}
		return false
	case 0xfe:
		return true
	default:
		return false
	}
}

// Q3 协议家族（Quake3 / Warfork 等）回 0xFFFFFFFF + "\ninfoResponse" 之类，认前缀就够。
func isQ3Reply(reply []byte) bool { return queryHeader(reply) != 0 }

func probeServiceA2S(service Service, timeout time.Duration) (ProbeResult, error) {
	result, err := probeServiceUDP(service, timeout, a2sQuery, isA2SReply)
	if err != nil {
		return result, err
	}
	result.Method = ProtocolA2S
	if result.Online() {
		result.Detail = NewText(i18n.KeyDetailA2SOK)
	}
	return result, nil
}

func probeServiceQ3(service Service, timeout time.Duration) (ProbeResult, error) {
	result, err := probeServiceUDP(service, timeout, q3Query, isQ3Reply)
	if err != nil {
		return result, err
	}
	result.Method = ProtocolQ3
	if result.Online() {
		result.Detail = NewText(i18n.KeyDetailQ3OK)
	}
	return result, nil
}

// ProbeLink 探测一个网页链接：状态码等于 expect_status 才算通。
func ProbeLink(link Link, timeout time.Duration) (ProbeResult, error) {
	target := link.Target()
	started := nowNanos()
	client := &http.Client{Timeout: timeout}
	response, err := client.Do(newProbeRequest(target, "*/*"))
	elapsed := latency(started, nowNanos())
	if err != nil {
		if isDNSError(err) {
			return ProbeResult{}, ServerError(i18n.KeyProbeDNS, target)
		}
		if isTimeout(err) {
			return ProbeResult{Address: target, Status: StatusDown, Detail: NewText(i18n.KeyDetailLinkTimeout), Method: MethodHTTP}, nil
		}
		return ProbeResult{Address: target, Status: StatusDown, Detail: NewText(i18n.KeyDetailLinkFailed, err.Error()), Method: MethodHTTP}, nil
	}
	defer drain(response)
	result := ProbeResult{Address: target, LatencyMs: elapsed, Method: MethodHTTP}
	switch {
	case response.StatusCode == link.ExpectStatus:
		result.Status = StatusUp
		result.Detail = NewText(i18n.KeyDetailLinkStatus, response.StatusCode)
	case response.StatusCode >= 400:
		result.Status = StatusDown
		result.Detail = NewText(i18n.KeyDetailLinkStatus, response.StatusCode)
	default:
		result.Status = StatusDown
		result.Detail = NewText(i18n.KeyDetailLinkMismatch, response.StatusCode, link.ExpectStatus)
	}
	return result, nil
}

func newProbeRequest(target, accept string) *http.Request {
	request, _ := http.NewRequest(http.MethodGet, target, nil)
	request.Header.Set("User-Agent", "github.com/hm-service/scope-status/1.0")
	request.Header.Set("Accept", accept)
	request.Header.Set("Cache-Control", "no-cache")
	return request
}

func drain(response *http.Response) {
	io.Copy(io.Discard, io.LimitReader(response.Body, 64*1024))
	response.Body.Close()
}

func splitHostPort(address string, defaultPort int) (string, int) {
	if strings.Count(address, ":") == 1 && !strings.HasPrefix(address, "[") {
		host, rawPort, _ := strings.Cut(address, ":")
		if port, err := strconv.Atoi(rawPort); err == nil {
			return host, port
		}
	}
	return address, defaultPort
}

func isDNSError(err error) bool {
	var dnsErr *net.DNSError
	return errors.As(err, &dnsErr)
}

func isTimeout(err error) bool {
	var netErr net.Error
	if errors.As(err, &netErr) {
		return netErr.Timeout()
	}
	return isDeadline(err)
}

// 站点图标抓起来是慢活（不可达的站点每个要 4s×3），不能挂在探测的关键路径上：
// For 只读缓存，未命中就起一个后台抓取（同 origin 只抓一次），这次先给 nil。
type FaviconCache struct {
	mu       sync.Mutex
	entries  map[string]*string
	inflight map[string]bool
	slots    chan struct{}
}

func NewFaviconCache() *FaviconCache {
	return &FaviconCache{
		entries:  make(map[string]*string),
		inflight: make(map[string]bool),
		slots:    make(chan struct{}, 4),
	}
}

func (c *FaviconCache) For(rawURL string) *string {
	origin, ok := originOf(rawURL)
	if !ok {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if cached, ok := c.entries[origin]; ok {
		return cached
	}
	if !c.inflight[origin] {
		c.inflight[origin] = true
		go func() {
			found := c.fetch(origin)
			c.mu.Lock()
			c.entries[origin] = found
			delete(c.inflight, origin)
			c.mu.Unlock()
		}()
	}
	return nil
}

// fetch 同步抓一次；测试直接调它，免得跟后台 goroutine 抢时序。
func (c *FaviconCache) fetch(origin string) *string {
	c.slots <- struct{}{}
	defer func() { <-c.slots }()
	return discoverFavicon(origin)
}

func originOf(rawURL string) (string, bool) {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", false
	}
	return parsed.Scheme + "://" + parsed.Host, true
}

var (
	iconLinkRe = regexp.MustCompile(`(?is)<link\b[^>]*?\brel\s*=\s*["']?[^"'>]*icon[^"'>]*["']?[^>]*>`)
	hrefRe     = regexp.MustCompile(`(?is)\bhref\s*=\s*["']([^"']+)["']`)
)

func discoverFavicon(origin string) *string {
	if found := faviconFromHTML(origin); found != nil {
		return found
	}
	for _, path := range []string{"/favicon.ico", "/favicon.png"} {
		candidate := origin + path
		client := &http.Client{Timeout: 4e9}
		response, err := client.Do(newProbeRequest(candidate, "image/*"))
		if err != nil {
			return nil
		}
		drain(response)
		if response.StatusCode >= 200 && response.StatusCode < 300 {
			return &candidate
		}
	}
	return nil
}

func faviconFromHTML(origin string) *string {
	client := &http.Client{Timeout: 4e9}
	response, err := client.Do(newProbeRequest(origin+"/", "text/html"))
	if err != nil {
		return nil
	}
	defer response.Body.Close()
	head, _ := io.ReadAll(io.LimitReader(response.Body, 64*1024))
	base := origin + "/"
	if response.Request != nil && response.Request.URL != nil {
		base = response.Request.URL.String()
	}
	for _, tag := range iconLinkRe.FindAllString(string(head), -1) {
		match := hrefRe.FindStringSubmatch(tag)
		if match == nil {
			continue
		}
		href := strings.TrimSpace(htmlUnescape(match[1]))
		if href == "" || strings.HasPrefix(href, "data:") {
			continue
		}
		resolved, err := url.Parse(href)
		if err != nil {
			continue
		}
		baseURL, err := url.Parse(base)
		if err != nil {
			continue
		}
		found := baseURL.ResolveReference(resolved).String()
		return &found
	}
	return nil
}

func htmlUnescape(value string) string {
	replacer := strings.NewReplacer("&amp;", "&", "&lt;", "<", "&gt;", ">", "&quot;", `"`, "&#39;", "'", "&apos;", "'")
	return replacer.Replace(value)
}
