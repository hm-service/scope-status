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
peers:
  - name: 小明
    kind: friend
    note: 备注
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
	if len(source.Peers) != 1 {
		t.Fatalf("peers = %d", len(source.Peers))
	}
	peer := source.Peers[0]
	if len(peer.Addresses) != 2 || peer.Addresses[0] != "10.0.0.1" || peer.Addresses[1] != "10.0.0.2:51820" {
		t.Fatalf("地址没去重或顺序不对：%v", peer.Addresses)
	}
	if peer.Note == nil || *peer.Note != "备注" {
		t.Fatalf("note = %v", peer.Note)
	}
	if peer.Kind != KindFriend || peer.Category != KindFriend {
		t.Fatalf("kind/category = %q/%q", peer.Kind, peer.Category)
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
	if source.Category(KindFriend).Render(i18n.Zh) != "朋友" {
		t.Fatalf("分类组名 = %q", source.Category(KindFriend).Render(i18n.Zh))
	}
}

// properties 是有序的：写在前面的先显示。
func TestPropertiesKeepOrder(t *testing.T) {
	source, err := ParseSource("test.yaml", []byte(`
peers:
  - name: 小明
    kind: friend
    addresses: ["10.0.0.1"]
    properties:
      乙: 2
      甲: 1
`))
	if err != nil {
		t.Fatalf("解析失败：%v", err)
	}
	properties := source.Peers[0].Properties
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
		{"顶层不是 mapping", "- 1\n- 2\n- 3\n", i18n.KeyConfigPeerNotObject},
		{"peers 不是数组", "peers: 1\n", i18n.KeyConfigPeersNotList},
		{"缺 name", "peers:\n  - kind: friend\n    addresses: [\"10.0.0.1\"]\n", i18n.KeyConfigPeerName},
		{"name 类型不对", "peers:\n  - name: 5\n    kind: friend\n    addresses: [\"10.0.0.1\"]\n", i18n.KeyConfigPeerName},
		{"缺 kind", "peers:\n  - name: 小明\n    addresses: [\"10.0.0.1\"]\n", i18n.KeyConfigPeerKind},
		{"kind 不认识", "peers:\n  - name: 小明\n    kind: phone\n    addresses: [\"10.0.0.1\"]\n", i18n.KeyConfigPeerKind},
		{"kind 不是字符串", "peers:\n  - name: 小明\n    kind: 5\n    addresses: [\"10.0.0.1\"]\n", i18n.KeyConfigPeerKind},
		{"kind 写成 cloud", "peers:\n  - name: 小明\n    kind: cloud\n    addresses: [\"10.0.0.1\"]\n", i18n.KeyConfigPeerKind},
		{"缺 addresses", "peers:\n  - name: 小明\n    kind: friend\n", i18n.KeyConfigAddressesMissing},
		{"addresses 写成字符串", "peers:\n  - name: 小明\n    kind: friend\n    addresses: 10.0.0.1\n", i18n.KeyConfigAddressesNotList},
		{"空 addresses", "peers:\n  - name: 小明\n    kind: friend\n    addresses: []\n", i18n.KeyConfigAddressesNotList},
		{"地址非法", "peers:\n  - name: 小明\n    kind: friend\n    addresses: [\"10.0.0.1/\"]\n", i18n.KeyConfigAddressInvalid},
		{"note 类型不对", "peers:\n  - name: 小明\n    kind: friend\n    note: 5\n    addresses: [\"10.0.0.1\"]\n", i18n.KeyConfigPeerNote},
		{"properties 不是映射", "peers:\n  - name: 小明\n    kind: friend\n    addresses: [\"10.0.0.1\"]\n    properties: [1]\n", i18n.KeyConfigPropsNotMapping},
		{"properties 值是对象", "peers:\n  - name: 小明\n    kind: friend\n    addresses: [\"10.0.0.1\"]\n    properties:\n      甲:\n        乙: 1\n", i18n.KeyConfigPropNotScalar},
		{"重复字段", "peers:\n  - name: 小明\n    kind: friend\n    name: 小红\n    addresses: [\"10.0.0.1\"]\n", i18n.KeyConfigDuplicateKey},
		{"重复属性名", "peers:\n  - name: 小明\n    kind: friend\n    addresses: [\"10.0.0.1\"]\n    properties:\n      甲: 1\n      甲: 2\n", i18n.KeyConfigDuplicateKey},

		{"cloud_servers 不是数组", "cloud_servers: 1\n", i18n.KeyConfigCloudServersNotList},
		{"cloud_servers 项不是对象", "cloud_servers:\n  - 1\n", i18n.KeyConfigCloudServerNotObject},
		{"云服务器缺 name", "cloud_servers:\n  - private: [\"10.0.0.1\"]\n", i18n.KeyConfigCloudServerName},
		{"云服务器没有地址", "cloud_servers:\n  - name: 云\n", i18n.KeyConfigCloudServerAddress},
		{"云服务器地址是字符串", "cloud_servers:\n  - name: 云\n    private: 10.0.0.1\n", i18n.KeyConfigAddressesNotList},
		{"云服务器地址非法", "cloud_servers:\n  - name: 云\n    private: [\"10.0.0.1/\"]\n", i18n.KeyConfigAddressInvalid},
		{"云服务器 core 不是布尔", "cloud_servers:\n  - name: 云\n    private: [\"10.0.0.1\"]\n    core: 1\n", i18n.KeyConfigCloudServerCore},
		{"两个 core", "cloud_servers:\n  - name: 甲\n    private: [\"10.0.0.1\"]\n    core: true\n  - name: 乙\n    private: [\"10.0.0.2\"]\n    core: true\n", i18n.KeyConfigCloudServerCoreMultiple},

		{"probe 不是对象", "probe: 5\n", i18n.KeyConfigProbeNotMapping},
		{"probe.method 不认识", "probe:\n  method: arp\n", i18n.KeyConfigProbeMethod},
		{"probe.timeout 不是数字", "probe:\n  timeout: soon\n", i18n.KeyConfigProbeTimeout},
		{"services 不是数组", "services: 1\n", i18n.KeyConfigServicesNotList},
		{"服务缺 host", "services:\n  - name: MC\n    port: 25565\n", i18n.KeyConfigServiceHost},
		{"服务 port 越界", "services:\n  - name: MC\n    host: 10.0.0.9\n    port: 70000\n", i18n.KeyConfigServicePortRange},
		{"服务 protocol 不认识", "services:\n  - name: MC\n    host: 10.0.0.9\n    port: 25565\n    protocol: quic\n", i18n.KeyConfigServiceProtocol},
		{"服务 category 类型不对", "services:\n  - name: MC\n    host: 10.0.0.9\n    port: 25565\n    category: [1]\n", i18n.KeyConfigCategoryType},
		{"links 不是数组", "links: 1\n", i18n.KeyConfigLinksNotList},
		{"link 缺 url", "links:\n  - name: 面板\n", i18n.KeyConfigLinkURL},
		{"link url 不对", "links:\n  - name: 面板\n    url: ftp://x/\n", i18n.KeyConfigLinkURLInvalid},
		{"link expect_status 类型不对", "links:\n  - name: 面板\n    url: https://x/\n    expect_status: x\n", i18n.KeyConfigLinkExpectStatus},
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

func TestTopLevelListIsPeers(t *testing.T) {
	source, err := ParseSource("test.yaml", []byte("- name: 小明\n  kind: friend\n  addresses: [\"10.0.0.1\"]\n"))
	if err != nil {
		t.Fatalf("解析失败：%v", err)
	}
	if len(source.Peers) != 1 || source.Peers[0].Name != "小明" {
		t.Fatalf("顶层数组没当成 peers：%+v", source.Peers)
	}
}

// 分组：peers 按 kind 走内置名字与顺序，服务按 category，没声明的也补内置名字。
func TestUsedCategories(t *testing.T) {
	source, err := ParseSource("test.yaml", []byte(`
peers:
  - name: 小明
    kind: server
    addresses: ["10.0.0.1"]
  - name: 小红
    kind: friend
    addresses: ["10.0.0.2"]
services:
  - name: MC
    host: 10.0.0.9
    port: 25565
    category: nas
`))
	if err != nil {
		t.Fatalf("解析失败：%v", err)
	}
	if len(source.Categories) != 3 {
		t.Fatalf("分类数 = %d：%+v", len(source.Categories), source.Categories)
	}
	// 顺序：friend(10) → server(20) → nas(50)
	if source.Categories[0].ID != "friend" || source.Categories[1].ID != "server" || source.Categories[2].ID != "nas" {
		t.Fatalf("分类顺序不对：%+v", source.Categories)
	}
	if source.Category("nas").Render(i18n.Zh) != "NAS / 存储" || source.Category("nas").Order != 50 {
		t.Fatalf("内置分类没补上：%+v", source.Category("nas"))
	}
	if source.Category(ServiceCategory).Render(i18n.En) != "Services" {
		t.Fatalf("服务默认分类的英文名 = %q", source.Category(ServiceCategory).Render(i18n.En))
	}
}

// kind 决定分组：friend → 「朋友」，server → 「服务器」，大小写不敏感。
func TestPeerKindDrivesCategory(t *testing.T) {
	source, err := ParseSource("test.yaml", []byte(`
peers:
  - name: 小明
    kind: Friend
    addresses: ["10.0.0.1"]
  - name: 游戏服务器
    kind: server
    addresses: ["10.0.0.2"]
`))
	if err != nil {
		t.Fatalf("解析失败：%v", err)
	}
	if source.Peers[0].Kind != KindFriend || source.Peers[0].Category != KindFriend {
		t.Fatalf("第一台 kind/category = %q/%q", source.Peers[0].Kind, source.Peers[0].Category)
	}
	if source.Peers[1].Kind != KindServer || source.Peers[1].Category != KindServer {
		t.Fatalf("第二台 kind/category = %q/%q", source.Peers[1].Kind, source.Peers[1].Category)
	}
	friends := source.Category(KindFriend)
	if friends.Render(i18n.Zh) != "朋友" || friends.Render(i18n.En) != "Friends" || friends.Order != 10 {
		t.Fatalf("朋友分组 = %+v", friends)
	}
	servers := source.Category(KindServer)
	if servers.Render(i18n.Zh) != "服务器" || servers.Order != 20 {
		t.Fatalf("服务器分组 = %+v", servers)
	}
}

// cloud_servers：中继服务器，private / public 分开写，core 最多一个；解析完并进 peers 列表。
func TestCloudServers(t *testing.T) {
	source, err := ParseSource("test.yaml", []byte(`
peers:
  - name: 小明
    kind: friend
    addresses: ["10.8.0.101"]
cloud_servers:
  - name: 云服务器-0
    core: true
    private: [10.8.0.1]
    public: [47.108.249.34, "47.108.249.34"]
    note: 主力
    properties:
      机房: 成都
  - name: 云服务器-1
    private: [10.8.1.1]
    public: [139.155.158.68]
`))
	if err != nil {
		t.Fatalf("解析失败：%v", err)
	}
	if len(source.Peers) != 3 {
		t.Fatalf("peers = %d（云服务器应该并进来）：%+v", len(source.Peers), source.Peers)
	}
	cloud0 := source.Peers[1]
	if cloud0.Name != "云服务器-0" || cloud0.Kind != KindCloud || cloud0.Category != KindCloud {
		t.Fatalf("第一朵云 = %+v", cloud0)
	}
	if !cloud0.Core {
		t.Fatalf("core 没解析出来：%+v", cloud0)
	}
	if len(cloud0.Private) != 1 || cloud0.Private[0] != "10.8.0.1" {
		t.Fatalf("private = %v", cloud0.Private)
	}
	// 去重、保持书写顺序
	if len(cloud0.Public) != 1 || cloud0.Public[0] != "47.108.249.34" {
		t.Fatalf("public = %v", cloud0.Public)
	}
	// 探测用的地址 = private + public
	if len(cloud0.Addresses) != 2 || cloud0.Addresses[0] != "10.8.0.1" || cloud0.Addresses[1] != "47.108.249.34" {
		t.Fatalf("addresses = %v", cloud0.Addresses)
	}
	if cloud0.Note == nil || *cloud0.Note != "主力" || len(cloud0.Properties) != 1 {
		t.Fatalf("note/properties = %v/%v", cloud0.Note, cloud0.Properties)
	}
	if cloud0.Index != 1 {
		t.Fatalf("index 应该接着 peers 排：%d", cloud0.Index)
	}

	cloud1 := source.Peers[2]
	if cloud1.Core || len(cloud1.Private) != 1 || len(cloud1.Public) != 1 {
		t.Fatalf("第二朵云 = %+v", cloud1)
	}
	if len(cloud1.Addresses) != 2 {
		t.Fatalf("第二朵云的 addresses = %v", cloud1.Addresses)
	}
	group := source.Category(KindCloud)
	if group.Render(i18n.Zh) != "云服务器" || group.Render(i18n.En) != "Cloud servers" || group.Order != 30 {
		t.Fatalf("云服务器分组 = %+v", group)
	}
}

// 只写 private 或只写 public 都行。
func TestCloudServerSingleSideAddress(t *testing.T) {
	source, err := ParseSource("test.yaml", []byte("cloud_servers:\n  - name: 云\n    public: [203.0.113.9]\n"))
	if err != nil {
		t.Fatalf("解析失败：%v", err)
	}
	if len(source.Peers) != 1 || len(source.Peers[0].Public) != 1 || source.Peers[0].Private != nil {
		t.Fatalf("解析结果 = %+v", source.Peers[0])
	}
}

// 旧版（Python 原型）认过的别名一律不再认，别为它们留兼容分支。
func TestLegacyAliasesAreNotAccepted(t *testing.T) {
	source, err := ParseSource("test.yaml", []byte(`
Peers:
  - name: 小明
    kind: friend
    addresses: ["10.0.0.1"]
links:
  - name: 面板
    url: https://panel.example.com/
    probeUrl: https://panel.example.com/healthz
    expectStatus: 204
services:
  - name: MC
    host: 10.0.0.9
    port: 25565
    connectCmd: mc 10.0.0.9
`))
	if err != nil {
		t.Fatalf("解析失败：%v", err)
	}
	if len(source.Peers) != 0 {
		t.Fatalf("Peers 这个别名不该被认：%+v", source.Peers)
	}
	if len(source.Links) != 1 || source.Links[0].ProbeURL != nil || source.Links[0].ExpectStatus != 200 {
		t.Fatalf("probeUrl / expectStatus 这两个别名不该被认：%+v", source.Links)
	}
	if len(source.Services) != 1 || source.Services[0].ConnectCmd != nil {
		t.Fatalf("connectCmd 这个别名不该被认：%+v", source.Services)
	}

	// categories 块也取消了：写了也不再认（用 kind 分组）
	withCategories, err := ParseSource("test.yaml", []byte(`
categories:
  - id: 朋友
    label: 自定义组名
    order: 1
peers:
  - name: 小明
    kind: friend
    addresses: ["10.0.0.1"]
`))
	if err != nil {
		t.Fatalf("解析失败：%v", err)
	}
	if withCategories.Category("friend").Render(i18n.Zh) != "朋友" {
		t.Fatalf("categories 块不该再生效：%q", withCategories.Category("friend").Render(i18n.Zh))
	}
}

func TestProbeDefaultsAndClamp(t *testing.T) {
	source, err := ParseSource("test.yaml", []byte("peers: []\nprobe:\n  concurrency: 0\n  count: -3\n"))
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
