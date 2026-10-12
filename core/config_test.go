package core

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/hm-service/scope-status/i18n"
)

func errorKey(t *testing.T, err error) i18n.Key {
	t.Helper()
	var business *Error
	if !errors.As(err, &business) {
		t.Fatalf("想要带文案键的业务错误，拿到 %v", err)
	}
	return business.Key
}

func TestParseSourceFull(t *testing.T) {
	source, err := ParseSource("test.yaml", []byte(`
title: 领域状态
probe:
  method: tcp
  timeout: 1.5
  port: 1234
  concurrency: 2
  count: 3
categories:
  - id: 朋友
    label: 朋友们
    order: 2
friends:
  - name: 小明
    note: 备注
    category: 朋友
    addresses:
      - 10.0.0.1
      - 10.0.0.1
      - 10.0.0.2:51820
    properties:
      密码: secret
      端口: 51820
services:
  - name: MC
    host: mc.example.com
    port: 25565
    protocol: a2s
    connect_cmd: mc mc.example.com
    properties:
      版本: 1.20
links:
  - name: 面板
    url: https://panel.example.com/
    probe_url: https://panel.example.com/healthz
    expect_status: 204
`))
	if err != nil {
		t.Fatalf("解析失败：%v", err)
	}
	if source.Title != "领域状态" {
		t.Fatalf("title = %q", source.Title)
	}
	if source.Probe.Method != MethodTCP || source.Probe.Timeout != 1500*time.Millisecond ||
		source.Probe.Port != 1234 || source.Probe.Concurrency != 2 || source.Probe.Count != 3 {
		t.Fatalf("probe 解析不对：%+v", source.Probe)
	}
	if len(source.Friends) != 1 {
		t.Fatalf("friends = %d", len(source.Friends))
	}
	friend := source.Friends[0]
	if len(friend.Addresses) != 2 || friend.Addresses[0] != "10.0.0.1" || friend.Addresses[1] != "10.0.0.2:51820" {
		t.Fatalf("地址没去重或顺序不对：%v", friend.Addresses)
	}
	if friend.Note == nil || *friend.Note != "备注" {
		t.Fatalf("note = %v", friend.Note)
	}
	if len(source.Services) != 1 || source.Services[0].Protocol != ProtocolA2S || source.Services[0].ConnectCmd == nil {
		t.Fatalf("services 解析不对：%+v", source.Services)
	}
	if source.Services[0].Category != ServiceCategory {
		t.Fatalf("服务默认分类 = %q", source.Services[0].Category)
	}
	if len(source.Links) != 1 || source.Links[0].Target() != "https://panel.example.com/healthz" ||
		source.Links[0].ExpectStatus != 204 {
		t.Fatalf("links 解析不对：%+v", source.Links)
	}
	if source.Category("朋友").Render(i18n.Zh) != "朋友们" {
		t.Fatalf("分类组名 = %q", source.Category("朋友").Render(i18n.Zh))
	}
}

// properties 是有序的：写在前面的先显示。
func TestPropertiesKeepOrder(t *testing.T) {
	source, err := ParseSource("test.yaml", []byte(`
friends:
  - name: 小明
    addresses: ["10.0.0.1"]
    properties:
      乙: 2
      甲: 1
`))
	if err != nil {
		t.Fatalf("解析失败：%v", err)
	}
	properties := source.Friends[0].Properties
	if len(properties) != 2 || properties[0].Name != "乙" || properties[1].Name != "甲" {
		t.Fatalf("顺序被改了：%+v", properties)
	}
	body, err := json.Marshal(properties)
	if err != nil {
		t.Fatalf("序列化失败：%v", err)
	}
	if string(body) != `{"乙":"2","甲":"1"}` {
		t.Fatalf("序列化结果 = %s", body)
	}
}

func TestParseSourceErrors(t *testing.T) {
	cases := []struct {
		name string
		yaml string
		key  i18n.Key
	}{
		{"空文件", "", i18n.KeyConfigEmpty},
		{"顶层不是 mapping", "- 1\n- 2\n- 3\n", i18n.KeyConfigFriendNotObject},
		{"friends 不是数组", "friends: 1\n", i18n.KeyConfigFriendsNotList},
		{"缺 name", "friends:\n  - addresses: [\"10.0.0.1\"]\n", i18n.KeyConfigFriendName},
		{"name 类型不对", "friends:\n  - name: 5\n    addresses: [\"10.0.0.1\"]\n", i18n.KeyConfigFriendName},
		{"缺 addresses", "friends:\n  - name: 小明\n", i18n.KeyConfigAddressesMissing},
		{"addresses 写成字符串", "friends:\n  - name: 小明\n    addresses: 10.0.0.1\n", i18n.KeyConfigAddressesNotList},
		{"空 addresses", "friends:\n  - name: 小明\n    addresses: []\n", i18n.KeyConfigAddressesNotList},
		{"地址非法", "friends:\n  - name: 小明\n    addresses: [\"10.0.0.1/\"]\n", i18n.KeyConfigAddressInvalid},
		{"note 类型不对", "friends:\n  - name: 小明\n    note: 5\n    addresses: [\"10.0.0.1\"]\n", i18n.KeyConfigFriendNote},
		{"category 类型不对", "friends:\n  - name: 小明\n    category: [1]\n    addresses: [\"10.0.0.1\"]\n", i18n.KeyConfigCategoryType},
		{"properties 不是映射", "friends:\n  - name: 小明\n    addresses: [\"10.0.0.1\"]\n    properties: [1]\n", i18n.KeyConfigPropsNotMapping},
		{"properties 值是对象", "friends:\n  - name: 小明\n    addresses: [\"10.0.0.1\"]\n    properties:\n      甲:\n        乙: 1\n", i18n.KeyConfigPropNotScalar},
		{"重复字段", "friends:\n  - name: 小明\n    name: 小红\n    addresses: [\"10.0.0.1\"]\n", i18n.KeyConfigDuplicateKey},
		{"重复属性名", "friends:\n  - name: 小明\n    addresses: [\"10.0.0.1\"]\n    properties:\n      甲: 1\n      甲: 2\n", i18n.KeyConfigDuplicateKey},
		{"probe 不是对象", "probe: 5\n", i18n.KeyConfigProbeNotMapping},
		{"probe.method 不认识", "probe:\n  method: arp\n", i18n.KeyConfigProbeMethod},
		{"probe.timeout 不是数字", "probe:\n  timeout: soon\n", i18n.KeyConfigProbeTimeout},
		{"categories 不是数组", "categories: 1\n", i18n.KeyConfigCategoriesNotList},
		{"category 缺 id", "categories:\n  - label: 朋友\n", i18n.KeyConfigCategoryID},
		{"category label 类型不对", "categories:\n  - id: friend\n    label: 5\n", i18n.KeyConfigCategoryLabel},
		{"services 不是数组", "services: 1\n", i18n.KeyConfigServicesNotList},
		{"服务缺 host", "services:\n  - name: MC\n    port: 25565\n", i18n.KeyConfigServiceHost},
		{"服务 port 越界", "services:\n  - name: MC\n    host: 10.0.0.9\n    port: 70000\n", i18n.KeyConfigServicePortRange},
		{"服务 protocol 不认识", "services:\n  - name: MC\n    host: 10.0.0.9\n    port: 25565\n    protocol: quic\n", i18n.KeyConfigServiceProtocol},
		{"服务 connect_cmd 类型不对", "services:\n  - name: MC\n    host: 10.0.0.9\n    port: 25565\n    connect_cmd: 5\n", i18n.KeyConfigServiceConnectCmd},
		{"links 不是数组", "links: 1\n", i18n.KeyConfigLinksNotList},
		{"链接缺 url", "links:\n  - name: 面板\n", i18n.KeyConfigLinkURL},
		{"链接 url 不是 http", "links:\n  - name: 面板\n    url: ftp://example.com/\n", i18n.KeyConfigLinkURLInvalid},
		{"probe_url 非法", "links:\n  - name: 面板\n    url: https://example.com/\n    probe_url: /healthz\n", i18n.KeyConfigLinkURLInvalid},
		{"expect_status 不是整数", "links:\n  - name: 面板\n    url: https://example.com/\n    expect_status: ok\n", i18n.KeyConfigLinkExpectStatus},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := ParseSource("test.yaml", []byte(testCase.yaml))
			if err == nil {
				t.Fatalf("想要报错，结果通过了")
			}
			if key := errorKey(t, err); key != testCase.key {
				t.Fatalf("错误键 = %q，想要 %q（%v）", key, testCase.key, err)
			}
		})
	}
}

func TestTopLevelListIsFriends(t *testing.T) {
	source, err := ParseSource("test.yaml", []byte("- name: 小明\n  addresses: [\"10.0.0.1\"]\n"))
	if err != nil {
		t.Fatalf("解析失败：%v", err)
	}
	if len(source.Friends) != 1 || source.Friends[0].Name != "小明" {
		t.Fatalf("顶层数组没当成 friends：%+v", source.Friends)
	}
}

// 配置里没声明、但被用到的分类要补上内置名字，否则前端会丢组。
func TestCategoriesAreCompleted(t *testing.T) {
	source, err := ParseSource("test.yaml", []byte(`
friends:
  - name: 小明
    addresses: ["10.0.0.1"]
    category: nas
  - name: 小红
    addresses: ["10.0.0.2"]
services:
  - name: MC
    host: 10.0.0.9
    port: 25565
categories:
  - id: 朋友
    order: 1
`))
	if err != nil {
		t.Fatalf("解析失败：%v", err)
	}
	nas := source.Category("nas")
	if nas.Render(i18n.Zh) != "NAS / 存储" || nas.Order != 100 {
		t.Fatalf("内置分类没补上：%+v", nas)
	}
	service := source.Category(ServiceCategory)
	if service.Render(i18n.En) != "Services" {
		t.Fatalf("服务默认分类的英文名 = %q", service.Render(i18n.En))
	}
	if len(source.Categories) != 4 {
		t.Fatalf("分类数 = %d：%+v", len(source.Categories), source.Categories)
	}
	if source.Categories[0].ID != "朋友" {
		t.Fatalf("分类顺序不对：%+v", source.Categories)
	}
}

func TestProbeDefaultsAndClamp(t *testing.T) {
	source, err := ParseSource("test.yaml", []byte("friends: []\nprobe:\n  concurrency: 0\n  count: -3\n"))
	if err != nil {
		t.Fatalf("解析失败：%v", err)
	}
	if source.Probe.Method != MethodICMP || source.Probe.Timeout != 2*time.Second || source.Probe.Port != 80 {
		t.Fatalf("probe 默认值不对：%+v", source.Probe)
	}
	if source.Probe.Concurrency != 1 || source.Probe.Count != 1 {
		t.Fatalf("probe 没被夹到 1：%+v", source.Probe)
	}
}

func TestHostOf(t *testing.T) {
	cases := map[string]string{
		"10.0.0.1":        "10.0.0.1",
		"10.0.0.1:51820":  "10.0.0.1",
		"::1":             "::1",
		"example.com":     "example.com",
		"example.com:443": "example.com",
	}
	for input, want := range cases {
		if got := hostOf(input); got != want {
			t.Errorf("hostOf(%q) = %q，想要 %q", input, got, want)
		}
	}
}
