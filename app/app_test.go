package app

import (
	"os"
	"path/filepath"
	"testing"
)

// 数据目录由部署方交进来：不存在、不是目录都当场失败，存在就收紧到 0700 并确认可写。
func TestPrepareDataDir(t *testing.T) {
	parent := t.TempDir()

	if err := prepareDataDir(filepath.Join(parent, "missing")); err == nil {
		t.Fatal("不存在的 DATA_DIR 应该拒绝启动")
	}

	file := filepath.Join(parent, "file")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatalf("造文件失败：%v", err)
	}
	if err := prepareDataDir(file); err == nil {
		t.Fatal("DATA_DIR 是文件应该拒绝启动")
	}

	dir := filepath.Join(parent, "data")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatalf("建目录失败：%v", err)
	}
	if err := prepareDataDir(dir); err != nil {
		t.Fatalf("已存在的目录不该报错：%v", err)
	}
	info, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat 失败：%v", err)
	}
	if info.Mode().Perm() != 0o700 {
		t.Fatalf("权限 = %v，该收紧到 0700", info.Mode().Perm())
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("读目录失败：%v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("探针文件没清掉：%v", entries)
	}
}
