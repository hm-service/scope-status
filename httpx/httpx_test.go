package httpx

import (
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"testing"
)

func TestContentDisposition(t *testing.T) {
	cases := []struct {
		name     string
		expected string
	}{
		{"report.txt", `attachment; filename="report.txt"; filename*=UTF-8''report.txt`},
		{"测试.txt", `attachment; filename="??.txt"; filename*=UTF-8''%E6%B5%8B%E8%AF%95.txt`},
		{`we"ird.txt`, `attachment; filename="we'ird.txt"; filename*=UTF-8''we%22ird.txt`},
	}
	for _, testCase := range cases {
		if got := ContentDisposition(testCase.name); got != testCase.expected {
			t.Errorf("ContentDisposition(%q) = %q，想要 %q", testCase.name, got, testCase.expected)
		}
	}
}

func TestClientIP(t *testing.T) {
	trusted := []netip.Prefix{netip.MustParsePrefix("127.0.0.1/32"), netip.MustParsePrefix("10.0.0.0/8")}
	cases := []struct {
		name   string
		remote string
		header string
		want   string
	}{
		{"直连不看转发头", "192.168.1.5:1234", "1.2.3.4", "192.168.1.5"},
		{"可信反代取最后一个不可信地址", "127.0.0.1:1234", "1.2.3.4", "1.2.3.4"},
		{"多级反代一路跳过可信段", "127.0.0.1:1234", "1.2.3.4, 10.0.0.1", "1.2.3.4"},
		{"整条链都可信就回连接地址", "127.0.0.1:1234", "10.0.0.1", "127.0.0.1"},
		{"空转发头", "127.0.0.1:1234", "", "127.0.0.1"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/", nil)
			request.RemoteAddr = testCase.remote
			if testCase.header != "" {
				request.Header.Set("X-Forwarded-For", testCase.header)
			}
			if got := ClientIP(request, trusted); got != testCase.want {
				t.Fatalf("ClientIP = %q，想要 %q", got, testCase.want)
			}
		})
	}
}

func TestRecorderKeepsStatusAndFlush(t *testing.T) {
	inner := httptest.NewRecorder()
	recorder := &recorder{ResponseWriter: inner, status: http.StatusOK}
	recorder.WriteHeader(http.StatusTeapot)
	recorder.Write([]byte("hello"))
	recorder.Flush()
	if recorder.status != http.StatusTeapot || recorder.bytes != 5 {
		t.Fatalf("记录不对：%d / %d", recorder.status, recorder.bytes)
	}
	if !inner.Flushed {
		t.Fatal("Flush 没有透传下去，SSE 会被缓冲")
	}
	if strings.Count(inner.Body.String(), "hello") != 1 {
		t.Fatalf("body = %q", inner.Body.String())
	}
}
