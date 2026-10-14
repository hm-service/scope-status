package core

import (
	"testing"

	"github.com/hm-service/scope-status/i18n"
)

// 回归：空文件要报「空文件」，而不是拿「顶层必须是 mapping」糊过去。
func TestRegressEmptyConfig(t *testing.T) {
	for _, body := range []string{"", "\n\n", "# 只有注释\n", "---\n", "null\n"} {
		_, err := ParseSource("test.yaml", []byte(body))
		if err == nil {
			t.Fatalf("%q 应该报错", body)
		}
		if key := errorKey(t, err); key != i18n.KeyConfigEmpty {
			t.Fatalf("%q 的错误键 = %q，想要 %q", body, key, i18n.KeyConfigEmpty)
		}
	}
}

// 回归：`peers:` 后面什么都没写是空列表，不是类型错误。
func TestRegressEmptySections(t *testing.T) {
	source, err := ParseSource("test.yaml", []byte("peers:\nservices:\nlinks:\n"))
	if err != nil {
		t.Fatalf("空字段不该报错：%v", err)
	}
	if len(source.Peers) != 0 || len(source.Services) != 0 || len(source.Links) != 0 {
		t.Fatalf("空字段应该解析成空列表：%+v", source)
	}
}

// 回归：`/` 这种只有路径分隔符的名字不能当成文件名放过去。
func TestRegressRootPathAsFileName(t *testing.T) {
	for _, input := range []string{"/", "//", "a/", "a//b/"} {
		got, err := CleanFileName(input)
		if err != nil {
			continue
		}
		if got == "" || got == "/" || got == "a/" {
			t.Fatalf("CleanFileName(%q) = %q，不该留下路径残留", input, got)
		}
	}
	if _, err := CleanFileName("/"); err == nil {
		t.Fatal("CleanFileName(\"/\") 应该报错")
	}
}

// 回归：带端口的地址只校验主机部分，端口写错字交给探测层兜（与配置文件的历史行为一致）。
func TestRegressAddressWithNonNumericPort(t *testing.T) {
	source, err := ParseSource("test.yaml", []byte("peers:\n  - name: 小明\n    kind: friend\n    addresses: [\"10.0.0.1:abc\"]\n"))
	if err != nil {
		t.Fatalf("主机合法就该过：%v", err)
	}
	if source.Peers[0].Addresses[0] != "10.0.0.1:abc" {
		t.Fatalf("地址被改写了：%v", source.Peers[0].Addresses)
	}
}
