package main

import (
	"bytes"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"testing"

	"github.com/hm-service/scope-status/core"
)

const (
	testAuthToken   = "s3cret_token_0123456789"
	testSessionKey  = "test_session_signing_key_0123456789"
	testReadyzToken = "Readyz_Token_0123456789"
)

// AUTH_TOKEN 是登录口令：没设就不启动；长度或字符集不合规范同样不启动。
func TestLoadEnvRequiresAuthToken(t *testing.T) {
	t.Setenv("SESSION_SIGNING_KEY", testSessionKey)

	t.Setenv("AUTH_TOKEN", "")
	if _, err := loadEnv(); err == nil {
		t.Fatal("AUTH_TOKEN 为空应该拒绝启动")
	} else if err.Error() != "AUTH_TOKEN must be set" {
		t.Fatalf("报错文案 = %q", err)
	}

	t.Setenv("AUTH_TOKEN", "   ")
	if _, err := loadEnv(); err == nil {
		t.Fatal("只有空白的 AUTH_TOKEN 也该拒绝")
	}

	// 太短、太长、带 [A-Za-z0-9_] 以外的字符都不启动
	for _, bad := range []string{"short", strings.Repeat("a", 15), strings.Repeat("a", 129), "s3cret-token-0123456789"} {
		t.Setenv("AUTH_TOKEN", bad)
		if _, err := loadEnv(); err == nil {
			t.Fatalf("AUTH_TOKEN %q（%d 字符）应该拒绝启动", bad, len(bad))
		} else if !strings.Contains(err.Error(), "16-128 chars of [A-Za-z0-9_]") {
			t.Fatalf("报错文案 = %q", err)
		}
	}

	t.Setenv("AUTH_TOKEN", "  "+testAuthToken+"  ")
	cfg, err := loadEnv()
	if err != nil {
		t.Fatalf("设了 AUTH_TOKEN 不该报错：%v", err)
	}
	if !core.ValidTokenHash(cfg.AuthTokenHash) {
		t.Fatalf("该拿到 Argon2id 摘要，拿到 %q", cfg.AuthTokenHash)
	}
	if strings.Contains(cfg.AuthTokenHash, testAuthToken) {
		t.Fatal("摘要里不该出现明文 token")
	}
	if matched, err := core.VerifyToken(testAuthToken, cfg.AuthTokenHash); err != nil || !matched {
		t.Fatalf("去掉两头空白的 token 应该能验过：%v / %v", matched, err)
	}
	if matched, err := core.VerifyToken("  "+testAuthToken+"  ", cfg.AuthTokenHash); err != nil || matched {
		t.Fatalf("带空白的原始值不该验过：%v / %v", matched, err)
	}
}

// READYZ_TOKEN 为空是合法的（那就不要口令）；非空就得是 16-128 位大小写字母、数字或下划线。
func TestLoadEnvReadyzTokenShape(t *testing.T) {
	put := func(token string) error {
		t.Setenv("AUTH_TOKEN", testAuthToken)
		t.Setenv("SESSION_SIGNING_KEY", testSessionKey)
		t.Setenv("READYZ_TOKEN", token)
		_, err := loadEnv()
		return err
	}

	for _, good := range []string{"", testReadyzToken, strings.Repeat("A", 128)} {
		if err := put(good); err != nil {
			t.Fatalf("READYZ_TOKEN %q（%d 字符）不该报错：%v", good, len(good), err)
		}
	}
	for _, bad := range []string{"short", strings.Repeat("a", 15), strings.Repeat("a", 129), "readyz-token-0123456789"} {
		if err := put(bad); err == nil {
			t.Fatalf("READYZ_TOKEN %q（%d 字符）应该拒绝启动", bad, len(bad))
		} else if !strings.Contains(err.Error(), "READYZ_TOKEN must be 16-128 chars of [A-Za-z0-9_]") {
			t.Fatalf("报错文案 = %q", err)
		}
	}
}

// SESSION_SIGNING_KEY 是会话的签名密钥：没设、太短、字符集不对都不启动。
func TestLoadEnvRequiresSessionSigningKey(t *testing.T) {
	// loadEnv 每次都会把密钥变量清掉，所以每次调用前都重新放一份
	put := func(key string) {
		t.Setenv("AUTH_TOKEN", testAuthToken)
		t.Setenv("SESSION_SIGNING_KEY", key)
	}

	put("")
	if _, err := loadEnv(); err == nil {
		t.Fatal("SESSION_SIGNING_KEY 为空应该拒绝启动")
	} else if err.Error() != "SESSION_SIGNING_KEY must be set" {
		t.Fatalf("报错文案 = %q", err)
	}

	for _, bad := range []string{"short", strings.Repeat("a", 31), strings.Repeat("a", 129), "session-signing-key-0123456789ab"} {
		put(bad)
		if _, err := loadEnv(); err == nil {
			t.Fatalf("SESSION_SIGNING_KEY %q（%d 字符）应该拒绝启动", bad, len(bad))
		} else if !strings.Contains(err.Error(), "32-128 chars of [A-Za-z0-9_]") {
			t.Fatalf("报错文案 = %q", err)
		}
	}

	put("  " + testSessionKey + "  ")
	cfg, err := loadEnv()
	if err != nil {
		t.Fatalf("设了签名密钥不该报错：%v", err)
	}
	if cfg.SessionSigningKey != testSessionKey {
		t.Fatalf("签名密钥 = %q，两头空白该去掉", cfg.SessionSigningKey)
	}
}

// 读完就把环境变量清掉：ping 之类的子进程不该继承到任何一个密钥。
func TestSecretsNotInheritedByChildren(t *testing.T) {
	secrets := map[string]string{
		"AUTH_TOKEN":          testAuthToken,
		"SESSION_SIGNING_KEY": testSessionKey,
		"READYZ_TOKEN":        testReadyzToken,
	}
	for name, value := range secrets {
		t.Setenv(name, value)
	}
	if _, err := loadEnv(); err != nil {
		t.Fatalf("不该报错：%v", err)
	}
	for name := range secrets {
		if value := os.Getenv(name); value != "" {
			t.Fatalf("%s 还在自己的环境里：%q", name, value)
		}
	}
	out, err := exec.Command("sh", "-c", `printf '%s,%s,%s' "$AUTH_TOKEN" "$SESSION_SIGNING_KEY" "$READYZ_TOKEN"`).Output()
	if err != nil {
		t.Fatalf("起子进程失败：%v", err)
	}
	if len(out) != 2 || out[0] != ',' || out[1] != ',' {
		t.Fatalf("子进程继承了密钥：%q", out)
	}
}

// MAX_AUTH_CONCURRENCY 是 /api/auth 那道闸门：不设走默认 32，写错了拒绝启动。
func TestLoadEnvMaxAuthConcurrency(t *testing.T) {
	load := func(value string) (envConfig, error) {
		t.Setenv("AUTH_TOKEN", testAuthToken)
		t.Setenv("SESSION_SIGNING_KEY", testSessionKey)
		t.Setenv("MAX_AUTH_CONCURRENCY", value)
		return loadEnv()
	}

	cfg, err := load("")
	if err != nil {
		t.Fatalf("不设 MAX_AUTH_CONCURRENCY 该走默认值：%v", err)
	}
	if cfg.MaxAuthConcurrency != 32 {
		t.Fatalf("默认值 = %d，想要 32", cfg.MaxAuthConcurrency)
	}

	cfg, err = load("7")
	if err != nil {
		t.Fatalf("显式值不该报错：%v", err)
	}
	if cfg.MaxAuthConcurrency != 7 {
		t.Fatalf("显式值 = %d，想要 7", cfg.MaxAuthConcurrency)
	}

	for _, bad := range []string{"0", "-1", "abc"} {
		if _, err := load(bad); err == nil {
			t.Fatalf("MAX_AUTH_CONCURRENCY=%q 应该拒绝启动", bad)
		}
	}
}

// CSP 的 script-src 停在 'self'，所以入口页面里既不能有内联脚本，也不能引外部脚本：
// 违反的话浏览器会静默挡掉，挡掉的正是防闪的主题脚本。
func TestEntryPagesOnlyLoadSameOriginScripts(t *testing.T) {
	opening := regexp.MustCompile(`(?is)<script([^>]*)>`)
	source := regexp.MustCompile(`(?is)\bsrc\s*=\s*"([^"]*)"`)

	for _, path := range []string{"frontend/index.html", "frontend/auth.html"} {
		page, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("读不下 %s：%v", path, err)
		}
		for _, match := range opening.FindAllStringSubmatch(string(page), -1) {
			src := source.FindStringSubmatch(match[1])
			if src == nil {
				t.Errorf("%s 里有内联脚本，CSP 会把它挡掉", path)
				continue
			}
			if !strings.HasPrefix(src[1], "/") {
				t.Errorf("%s 引了非同源脚本 %q，CSP 会把它挡掉", path, src[1])
			}
		}
	}

	theme, err := os.ReadFile("frontend/public/theme.js")
	if err != nil {
		t.Fatalf("读不下主题脚本：%v", err)
	}
	if !bytes.Contains(theme, []byte("scope-status-theme")) {
		t.Error("主题脚本没读主题键，首屏防闪就没了")
	}
}
