package httpx

import "net/http"

// ContentSecurityPolicy 是整站的 CSP。写死成一条常量：策略与产物同源代码，不靠运行时拼装，
// 免得两处不一致时浏览器静默挡掉资源。
//
// 页面里不许有内联脚本（防闪的主题脚本是 /theme.js），所以 script-src 能收成 'self'；
// 不必再维护 hash。样式放开内联，因为 Vue 的 :style 绑定落在 style 属性上。
// 图片额外允许 https：站点图标是被监控站点自己的 URL，由服务端发现后发给前端。
const ContentSecurityPolicy = "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; " +
	"img-src 'self' https:; connect-src 'self'; base-uri 'none'; form-action 'self'; " +
	"frame-ancestors 'none'; object-src 'none'; upgrade-insecure-requests"

// Security 给所有响应补上默认安全头。
func Security(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		header := w.Header()
		header.Set("X-Content-Type-Options", "nosniff")
		header.Set("Referrer-Policy", "no-referrer")
		header.Set("X-Frame-Options", "DENY")
		header.Set("Content-Security-Policy", ContentSecurityPolicy)
		next.ServeHTTP(w, r)
	})
}
