package main

import (
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/hm-service/scope-status/core"
)

const testSessionKey = "session-signing-key-0123456789"

// AUTH_TOKEN 是登录口令：没设就不启动；设了也只在内存里留摘要，不留明文。
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

	t.Setenv("AUTH_TOKEN", "  s3cret-token  ")
	cfg, err := loadEnv()
	if err != nil {
		t.Fatalf("设了 AUTH_TOKEN 不该报错：%v", err)
	}
	if !core.ValidTokenHash(cfg.AuthTokenHash) {
		t.Fatalf("该拿到 Argon2id 摘要，拿到 %q", cfg.AuthTokenHash)
	}
	if strings.Contains(cfg.AuthTokenHash, "s3cret-token") {
		t.Fatal("摘要里不该出现明文 token")
	}
	if matched, err := core.VerifyToken("s3cret-token", cfg.AuthTokenHash); err != nil || !matched {
		t.Fatalf("去掉两头空白的 token 应该能验过：%v / %v", matched, err)
	}
	if matched, err := core.VerifyToken("  s3cret-token  ", cfg.AuthTokenHash); err != nil || matched {
		t.Fatalf("带空白的原始值不该验过：%v / %v", matched, err)
	}
}

// SESSION_SIGNING_KEY 是会话的签名密钥：没设、太短都不启动。
func TestLoadEnvRequiresSessionSigningKey(t *testing.T) {
	// loadEnv 每次都会把两个变量清掉，所以每次调用前都重新放一份
	put := func(key string) {
		t.Setenv("AUTH_TOKEN", "s3cret-token")
		t.Setenv("SESSION_SIGNING_KEY", key)
	}

	put("")
	if _, err := loadEnv(); err == nil {
		t.Fatal("SESSION_SIGNING_KEY 为空应该拒绝启动")
	} else if err.Error() != "SESSION_SIGNING_KEY must be set" {
		t.Fatalf("报错文案 = %q", err)
	}

	put("short")
	if _, err := loadEnv(); err == nil {
		t.Fatal("太短的签名密钥应该拒绝启动")
	} else if !strings.Contains(err.Error(), "at least 16") {
		t.Fatalf("报错文案 = %q", err)
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

// 读完就把环境变量清掉：ping 之类的子进程不该继承到口令与签名密钥。
func TestSecretsNotInheritedByChildren(t *testing.T) {
	t.Setenv("AUTH_TOKEN", "s3cret-token")
	t.Setenv("SESSION_SIGNING_KEY", testSessionKey)
	if _, err := loadEnv(); err != nil {
		t.Fatalf("不该报错：%v", err)
	}
	for _, name := range []string{"AUTH_TOKEN", "SESSION_SIGNING_KEY"} {
		if value := os.Getenv(name); value != "" {
			t.Fatalf("%s 还在自己的环境里：%q", name, value)
		}
	}
	out, err := exec.Command("sh", "-c", `printf '%s,%s' "$AUTH_TOKEN" "$SESSION_SIGNING_KEY"`).Output()
	if err != nil {
		t.Fatalf("起子进程失败：%v", err)
	}
	if len(out) != 1 || out[0] != ',' {
		t.Fatalf("子进程继承了密钥：%q", out)
	}
}
