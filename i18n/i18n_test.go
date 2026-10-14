package i18n

import "testing"

func TestNegotiate(t *testing.T) {
	cases := []struct {
		name   string
		header string
		want   Lang
	}{
		{"空头用默认语言", "", Default},
		{"中文", "zh-CN,zh;q=0.9", Zh},
		{"英文", "en-US,en;q=0.9", En},
		{"按 q 值挑最高的", "en;q=0.3,zh;q=0.8", Zh},
		{"不认识的语言退回默认", "fr-FR,de;q=0.8", Default},
		{"通配符", "*", Default},
		{"英文在中文前面但 q 相同", "en,zh", En},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := Negotiate(testCase.header); got != testCase.want {
				t.Fatalf("Negotiate(%q) = %q，想要 %q", testCase.header, got, testCase.want)
			}
		})
	}
}

func TestUnknownKeyReturnsKey(t *testing.T) {
	const missing Key = "nope.nothing-here"
	for _, lang := range []Lang{Zh, En} {
		if got := T(lang, missing, 1); got != string(missing) {
			t.Fatalf("T(%q, %q) = %q，想要键名本身", lang, missing, got)
		}
	}
}

// 两张表必须一一对应：漏一条英文就能在界面上看见，但这里先挡住。
func TestTablesMatch(t *testing.T) {
	for key := range zh {
		if _, ok := en[key]; !ok {
			t.Errorf("英文表缺 %q", key)
		}
	}
	for key := range en {
		if _, ok := zh[key]; !ok {
			t.Errorf("中文表缺 %q", key)
		}
	}
}

func TestPlaceholdersRender(t *testing.T) {
	if got := T(Zh, KeyConfigPeerName, 3); got != "第 3 项缺少非空字符串字段 name" {
		t.Fatalf("中文占位符没渲染：%q", got)
	}
	if got := T(En, KeyConfigPeerName, 3); got != "entry #3 has no non-empty string field name" {
		t.Fatalf("英文占位符没渲染：%q", got)
	}
}
