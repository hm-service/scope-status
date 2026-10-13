// Package httpx 是 HTTP 小工具：JSON / 文本响应、客户端 IP、访问日志。
package httpx

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
)

func JSON(w http.ResponseWriter, status int, payload any) {
	body, err := json.Marshal(payload)
	if err != nil {
		Fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	w.Write(body)
}

// Fail 是 /api/* 统一的错误体：{"detail": "…"}，前端只读 detail。
func Fail(w http.ResponseWriter, status int, detail string) {
	JSON(w, status, map[string]string{"detail": detail})
}

func Text(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	io.WriteString(w, body)
}

// ContentDisposition 回原始文件名：ASCII 兜底 + RFC 5987 的 UTF-8 形式。
func ContentDisposition(name string) string {
	fallback := strings.Map(func(r rune) rune {
		switch {
		case r < ' ' || r > '~':
			return '?'
		case r == '"':
			return '\''
		default:
			return r
		}
	}, name)
	return `attachment; filename="` + fallback + `"; filename*=UTF-8''` + url.PathEscape(name)
}
