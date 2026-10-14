package main

import "testing"

// AUTH_TOKEN 是唯一的门禁：没设就不启动，设了才继续。
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
	if cfg.AuthToken != "s3cret-token" {
		t.Fatalf("AuthToken = %q，两头空白该去掉", cfg.AuthToken)
	}
}
