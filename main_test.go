package main

import (
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/hm-service/scope-status/core"
)

// AUTH_TOKEN 是唯一的门禁：没设就不启动；设了也只在内存里留摘要，不留明文。
func TestLoadEnvRequiresAuthToken(t *testing.T) {
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

// 读完就把环境变量清掉：ping 之类的子进程不该继承到 token。
func TestAuthTokenNotInheritedByChildren(t *testing.T) {
	t.Setenv("AUTH_TOKEN", "s3cret-token")
	if _, err := loadEnv(); err != nil {
		t.Fatalf("不该报错：%v", err)
	}
	if value := os.Getenv("AUTH_TOKEN"); value != "" {
		t.Fatalf("AUTH_TOKEN 还在自己的环境里：%q", value)
	}
	out, err := exec.Command("sh", "-c", `printf %s "$AUTH_TOKEN"`).Output()
	if err != nil {
		t.Fatalf("起子进程失败：%v", err)
	}
	if len(out) != 0 {
		t.Fatalf("子进程继承了 token：%q", out)
	}
}
