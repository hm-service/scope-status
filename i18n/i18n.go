// Package i18n 是后端文案表：业务层只给「键 + 参数」，渲染成哪种语言由 HTTP 层决定。
//
// 未知键原样返回键名，表里缺一条就漏一条，不做回退。
package i18n

import (
	"fmt"
	"strconv"
	"strings"
)

type Lang string

const (
	Zh Lang = "zh"
	En Lang = "en"
)

// Default 是请求没带 Accept-Language 时的语言。
const Default = Zh

type Key string

func (k Key) String() string { return string(k) }

// 配置解析。
const (
	KeyConfigMissing           Key = "config.error.missing"
	KeyConfigUnreadable        Key = "config.error.unreadable"
	KeyConfigEmpty             Key = "config.error.empty"
	KeyConfigNotMapping        Key = "config.error.not-mapping"
	KeyConfigSyntax            Key = "config.error.syntax"
	KeyConfigDuplicateKey      Key = "config.error.duplicate-key"
	KeyConfigTitle             Key = "config.error.title"
	KeyConfigFriendsNotList    Key = "config.error.friends-not-list"
	KeyConfigFriendNotObject   Key = "config.error.friend-not-object"
	KeyConfigFriendName        Key = "config.error.friend-name"
	KeyConfigFriendNote        Key = "config.error.friend-note"
	KeyConfigAddressesMissing  Key = "config.error.addresses-missing"
	KeyConfigAddressesNotList  Key = "config.error.addresses-not-list"
	KeyConfigAddressesEmpty    Key = "config.error.addresses-empty"
	KeyConfigAddressInvalid    Key = "config.error.address-invalid"
	KeyConfigHostInvalid       Key = "config.error.host-invalid"
	KeyConfigCategoryType      Key = "config.error.category-type"
	KeyConfigPropsNotMapping   Key = "config.error.properties-not-mapping"
	KeyConfigPropEmptyName     Key = "config.error.property-empty-name"
	KeyConfigPropNotScalar     Key = "config.error.property-not-scalar"
	KeyConfigCategoriesNotList Key = "config.error.categories-not-list"
	KeyConfigCategoryNotObject Key = "config.error.category-not-object"
	KeyConfigCategoryID        Key = "config.error.category-id"
	KeyConfigCategoryLabel     Key = "config.error.category-label"
	KeyConfigCategoryOrder     Key = "config.error.category-order"
	KeyConfigServicesNotList   Key = "config.error.services-not-list"
	KeyConfigServiceNotObject  Key = "config.error.service-not-object"
	KeyConfigServiceName       Key = "config.error.service-name"
	KeyConfigServiceHost       Key = "config.error.service-host"
	KeyConfigServicePort       Key = "config.error.service-port"
	KeyConfigServicePortRange  Key = "config.error.service-port-range"
	KeyConfigServiceProtocol   Key = "config.error.service-protocol"
	KeyConfigServiceConnectCmd Key = "config.error.service-connect-cmd"
	KeyConfigLinksNotList      Key = "config.error.links-not-list"
	KeyConfigLinkNotObject     Key = "config.error.link-not-object"
	KeyConfigLinkName          Key = "config.error.link-name"
	KeyConfigLinkURL           Key = "config.error.link-url"
	KeyConfigLinkURLInvalid    Key = "config.error.link-url-invalid"
	KeyConfigLinkProbeURL      Key = "config.error.link-probe-url"
	KeyConfigLinkExpectStatus  Key = "config.error.link-expect-status"
	KeyConfigProbeNotMapping   Key = "config.error.probe-not-mapping"
	KeyConfigProbeMethod       Key = "config.error.probe-method"
	KeyConfigProbeTimeout      Key = "config.error.probe-timeout"
	KeyConfigProbePort         Key = "config.error.probe-port"
	KeyConfigProbeConcurrency  Key = "config.error.probe-concurrency"
	KeyConfigProbeCount        Key = "config.error.probe-count"
)

// 探测。
const (
	KeyProbeNoPing         Key = "probe.error.no-ping"
	KeyProbeDNS            Key = "probe.error.dns"
	KeyProbeMethod         Key = "probe.error.method"
	KeyProbeProtocol       Key = "probe.error.protocol"
	KeyDetailICMPNoReply   Key = "probe.detail.icmp-no-reply"
	KeyDetailDNSFailed     Key = "probe.detail.dns-failed"
	KeyDetailUnreachable   Key = "probe.detail.network-unreachable"
	KeyDetailPingExit      Key = "probe.detail.ping-exit"
	KeyDetailTCPTimeout    Key = "probe.detail.tcp-timeout"
	KeyDetailTCPFailed     Key = "probe.detail.tcp-failed"
	KeyDetailTCPRefused    Key = "probe.detail.tcp-refused"
	KeyDetailTCPOK         Key = "probe.detail.tcp-ok"
	KeyDetailHTTPStatus    Key = "probe.detail.http-status"
	KeyDetailHTTPTimeout   Key = "probe.detail.http-timeout"
	KeyDetailHTTPFailed    Key = "probe.detail.http-failed"
	KeyDetailUDPNoReply    Key = "probe.detail.udp-no-reply"
	KeyDetailUDPRefused    Key = "probe.detail.udp-refused"
	KeyDetailUDPBytes      Key = "probe.detail.udp-bytes"
	KeyDetailA2SOK         Key = "probe.detail.a2s-ok"
	KeyDetailLinkStatus    Key = "probe.detail.link-status"
	KeyDetailLinkMismatch  Key = "probe.detail.link-mismatch"
	KeyDetailLinkFailed    Key = "probe.detail.link-failed"
	KeyDetailLinkTimeout   Key = "probe.detail.link-timeout"
	KeyDetailWaitFirstScan Key = "probe.detail.wait-first-scan"
)

// 消息。
const (
	KeyMessageBadID          Key = "messages.error.id"
	KeyMessageNotFound       Key = "messages.error.not-found"
	KeyMessageDeleted        Key = "messages.error.deleted"
	KeyMessageKeyRequired    Key = "messages.error.key-required"
	KeyMessageKeyWrong       Key = "messages.error.key-wrong"
	KeyMessageNameMissing    Key = "messages.error.name-missing"
	KeyMessageNameInvalid    Key = "messages.error.name-invalid"
	KeyMessageTextNotString  Key = "messages.error.text-not-string"
	KeyMessageKindInvalid    Key = "messages.error.kind"
	KeyMessageEmpty          Key = "messages.error.empty"
	KeyMessageAttsNotArray   Key = "messages.error.attachments-not-array"
	KeyMessageAttNotObject   Key = "messages.error.attachment-not-object"
	KeyMessageSHABad         Key = "messages.error.sha-invalid"
	KeyMessageSHAFormat      Key = "messages.error.sha-format"
	KeyMessageFileNotInPool  Key = "messages.error.file-not-in-pool"
	KeyMessageFileTooBig     Key = "messages.error.file-too-big"
	KeyMessageDeclaredTooBig Key = "messages.error.declared-too-big"
	KeyMessageFileGone       Key = "messages.error.file-gone"
	KeyMessageNotReferenced  Key = "messages.error.not-referenced"
	KeyMessageIDRequired     Key = "messages.error.id-required"
	KeyMessageBodyNotJSON    Key = "messages.error.body-not-json"
	KeyMessageBodyNotObject  Key = "messages.error.body-not-object"
	KeyMessageBrokenFile     Key = "messages.error.broken-file"
	KeyMessageFileNotObject  Key = "messages.error.file-not-object"
	KeyMessageFileKind       Key = "messages.error.file-kind"
	KeyMessageFileFields     Key = "messages.error.file-fields"
	KeyMessageFileKey        Key = "messages.error.file-key"
	KeyMessageFileAtts       Key = "messages.error.file-attachments"
	KeyMessageFileAttEntry   Key = "messages.error.file-attachment-entry"
	KeyMessageFileUpdatedAt  Key = "messages.error.file-updated-at"
	KeyMessageBodyTooBig     Key = "messages.error.body-too-big"
	KeyMessageBodyType       Key = "messages.error.body-type"
)

// HTTP 层。
const (
	KeyAppTitle             Key = "app.title"
	KeyHTTPNotFound         Key = "http.error.not-found"
	KeyHTTPNotReady         Key = "http.error.not-ready"
	KeyHTTPNotReadyCause    Key = "http.error.not-ready-cause"
	KeyHTTPTokenBad         Key = "http.error.token-bad"
	KeyHTTPInternal         Key = "http.error.internal"
	KeyHTTPNoFlush          Key = "http.error.no-flush"
	KeyHTTPBusy             Key = "http.error.busy"
	KeyHTTPMethodNotAllowed Key = "http.error.method-not-allowed"
	KeyReloadOK             Key = "reload.message"
	KeyCategoryOther        Key = "category.other"
)

// 内置分类名。用户在 source.yaml 里写了 label 就用用户的，没写才落到这里。
const (
	KeyCategoryFriend  Key = "category.friend"
	KeyCategoryService Key = "category.service"
	KeyCategoryServer  Key = "category.server"
	KeyCategoryNAS     Key = "category.nas"
	KeyCategoryRouter  Key = "category.router"
	KeyCategoryIoT     Key = "category.iot"
	KeyCategoryWork    Key = "category.work"
)

var zh = map[Key]string{
	KeyConfigMissing:           "配置文件不存在：%s",
	KeyConfigUnreadable:        "读不了配置文件 %s：%s",
	KeyConfigEmpty:             "%s 是空文件",
	KeyConfigNotMapping:        "%s 顶层必须是 mapping 或数组",
	KeyConfigSyntax:            "%s 不是合法 YAML：%s",
	KeyConfigDuplicateKey:      "%s 里 %s 出现了两次",
	KeyConfigTitle:             "title 必须是字符串",
	KeyConfigFriendsNotList:    "friends 必须是数组",
	KeyConfigFriendNotObject:   "friends[%d] 必须是对象",
	KeyConfigFriendName:        "第 %d 项缺少非空字符串字段 name",
	KeyConfigFriendNote:        "条目 %s 的 note 必须是字符串",
	KeyConfigAddressesMissing:  "条目 %s 缺少 addresses",
	KeyConfigAddressesNotList:  "条目 %s 的 addresses 必须是非空数组",
	KeyConfigAddressesEmpty:    "条目 %s 的 addresses 里没有有效地址",
	KeyConfigAddressInvalid:    "%s 的%s %s 不是合法 IP 或域名",
	KeyConfigHostInvalid:       "服务 %s 的 host %s 不是合法 IP 或域名",
	KeyConfigCategoryType:      "%s 的 category 必须是字符串",
	KeyConfigPropsNotMapping:   "%s 的 properties 必须是「名称: 值」的映射",
	KeyConfigPropEmptyName:     "%s 的 properties 里有空名称",
	KeyConfigPropNotScalar:     "%s 的 properties 里 %s 的值不是标量",
	KeyConfigCategoriesNotList: "categories 必须是数组",
	KeyConfigCategoryNotObject: "categories[%d] 必须是对象",
	KeyConfigCategoryID:        "categories[%d] 缺少非空字符串字段 id",
	KeyConfigCategoryLabel:     "categories[%d] 的 label 必须是字符串",
	KeyConfigCategoryOrder:     "categories[%d] 的 order 必须是整数",
	KeyConfigServicesNotList:   "services 必须是数组",
	KeyConfigServiceNotObject:  "services[%d] 必须是对象",
	KeyConfigServiceName:       "services[%d] 缺少非空字符串字段 name",
	KeyConfigServiceHost:       "服务 %s 缺少非空字符串字段 host",
	KeyConfigServicePort:       "服务 %s 的 port 必须是整数",
	KeyConfigServicePortRange:  "服务 %s 的 port 超出 1-65535",
	KeyConfigServiceProtocol:   "服务 %s 的 protocol 只支持 udp / tcp / a2s，收到 %s",
	KeyConfigServiceConnectCmd: "服务 %s 的 connect_cmd 必须是字符串",
	KeyConfigLinksNotList:      "links 必须是数组",
	KeyConfigLinkNotObject:     "links[%d] 必须是对象",
	KeyConfigLinkName:          "links[%d] 缺少非空字符串字段 name",
	KeyConfigLinkURL:           "链接 %s 缺少非空字符串字段 url",
	KeyConfigLinkURLInvalid:    "链接 %s 的%s %s 必须是 http(s):// 开头的完整地址",
	KeyConfigLinkProbeURL:      "链接 %s 的 probe_url 必须是字符串",
	KeyConfigLinkExpectStatus:  "链接 %s 的 expect_status 必须是整数",
	KeyConfigProbeNotMapping:   "probe 必须是对象",
	KeyConfigProbeMethod:       "probe.method 只支持 icmp / tcp / http，收到 %s",
	KeyConfigProbeTimeout:      "probe.timeout 必须是数字（秒）",
	KeyConfigProbePort:         "probe.port 必须是整数",
	KeyConfigProbeConcurrency:  "probe.concurrency 必须是整数",
	KeyConfigProbeCount:        "probe.count 必须是整数",

	KeyProbeNoPing:         "系统里没有 ping 命令，把 probe.method 改成 tcp 吧",
	KeyProbeDNS:            "%s 解析失败",
	KeyProbeMethod:         "不支持的探测方式：%s",
	KeyProbeProtocol:       "不支持的协议：%s",
	KeyDetailICMPNoReply:   "无 ICMP 应答",
	KeyDetailDNSFailed:     "域名解析失败",
	KeyDetailUnreachable:   "网络不可达（WireGuard 隧道未建立？）",
	KeyDetailPingExit:      "ping 退出码 %d",
	KeyDetailTCPTimeout:    "TCP %d 超时",
	KeyDetailTCPFailed:     "TCP 连不上：%s",
	KeyDetailTCPRefused:    "端口拒绝连接（%s）",
	KeyDetailTCPOK:         "TCP 连接成功",
	KeyDetailHTTPStatus:    "HTTP %d",
	KeyDetailHTTPTimeout:   "HTTP 超时",
	KeyDetailHTTPFailed:    "HTTP 失败：%s",
	KeyDetailUDPNoReply:    "UDP 无回应",
	KeyDetailUDPRefused:    "端口拒绝（ICMP unreachable）",
	KeyDetailUDPBytes:      "收到 %d 字节回应",
	KeyDetailA2SOK:         "A2S_INFO 有回应",
	KeyDetailLinkStatus:    "HTTP %d",
	KeyDetailLinkMismatch:  "HTTP %d（期望 %d）",
	KeyDetailLinkFailed:    "连不上：%s",
	KeyDetailLinkTimeout:   "请求超时",
	KeyDetailWaitFirstScan: "等待第一轮探测",

	KeyMessageBadID:          "消息 id 不合法",
	KeyMessageNotFound:       "找不到消息 %s",
	KeyMessageDeleted:        "消息 %s 已删除",
	KeyMessageKeyRequired:    "密钥消息必须带 key",
	KeyMessageKeyWrong:       "密钥不对",
	KeyMessageNameMissing:    "缺少附件名（上传时带 ?name=）",
	KeyMessageNameInvalid:    "附件名不合法",
	KeyMessageTextNotString:  "正文必须是字符串",
	KeyMessageKindInvalid:    "kind 只能是 public / key",
	KeyMessageEmpty:          "正文和附件至少要有一个",
	KeyMessageAttsNotArray:   "attachments 必须是数组",
	KeyMessageAttNotObject:   "附件条目必须是对象",
	KeyMessageSHABad:         "附件 sha256 不合法：%s",
	KeyMessageSHAFormat:      "sha256 不合法",
	KeyMessageFileNotInPool:  "附件 %s 不在池子里，先上传",
	KeyMessageFileTooBig:     "文件超过上限 %d 字节",
	KeyMessageDeclaredTooBig: "文件 %d 字节，超过上限 %d 字节",
	KeyMessageFileGone:       "%s 在附件池里已经不存在了",
	KeyMessageNotReferenced:  "%s 没有引用附件 %s",
	KeyMessageIDRequired:     "下载附件要带 ?id=<消息 id>",
	KeyMessageBodyNotJSON:    "请求体不是 JSON：%s",
	KeyMessageBodyNotObject:  "请求体顶层必须是 JSON 对象",
	KeyMessageBrokenFile:     "%s 读不出来：%s",
	KeyMessageFileNotObject:  "%s 顶层不是对象",
	KeyMessageFileKind:       "%s 的 kind=%s 不认识",
	KeyMessageFileFields:     "%s 缺少 text / isDeleted",
	KeyMessageFileKey:        "%s 是密钥消息但没有 key",
	KeyMessageFileAtts:       "%s 的 attachments 不是数组",
	KeyMessageFileAttEntry:   "%s 里有不合法的附件条目：%s",
	KeyMessageFileUpdatedAt:  "%s 的 updatedAt 不是整数",
	KeyMessageBodyTooBig:     "请求体超过 %d 字节",
	KeyMessageBodyType:       "请求体字段 %s 类型不对",

	KeyAppTitle:             "Scope Status",
	KeyHTTPNotFound:         "接口不存在：%s",
	KeyHTTPNotReady:         "服务还没就绪",
	KeyHTTPNotReadyCause:    "没有就绪：%s",
	KeyHTTPTokenBad:         "token 不对",
	KeyHTTPInternal:         "服务端错误：%s",
	KeyHTTPNoFlush:          "这个连接不支持流式推送",
	KeyHTTPBusy:             "服务繁忙，稍后再试",
	KeyHTTPMethodNotAllowed: "不支持的方法：%s",
	KeyReloadOK:             "已重新加载 %d 个条目 / %d 个服务 / %d 个网站",

	KeyCategoryFriend:  "朋友",
	KeyCategoryService: "服务",
	KeyCategoryServer:  "服务器",
	KeyCategoryNAS:     "NAS / 存储",
	KeyCategoryRouter:  "路由器",
	KeyCategoryIoT:     "IoT / 智能家居",
	KeyCategoryWork:    "工作",
	KeyCategoryOther:   "其他",
}

var en = map[Key]string{
	KeyConfigMissing:           "config file not found: %s",
	KeyConfigUnreadable:        "cannot read config file %s: %s",
	KeyConfigEmpty:             "%s is empty",
	KeyConfigNotMapping:        "top level of %s must be a mapping or a list",
	KeyConfigSyntax:            "%s is not valid YAML: %s",
	KeyConfigDuplicateKey:      "%s defines %s twice",
	KeyConfigTitle:             "title must be a string",
	KeyConfigFriendsNotList:    "friends must be a list",
	KeyConfigFriendNotObject:   "friends[%d] must be a mapping",
	KeyConfigFriendName:        "entry #%d has no non-empty string field name",
	KeyConfigFriendNote:        "note of %s must be a string",
	KeyConfigAddressesMissing:  "%s has no addresses",
	KeyConfigAddressesNotList:  "addresses of %s must be a non-empty list",
	KeyConfigAddressesEmpty:    "addresses of %s has no usable address",
	KeyConfigAddressInvalid:    "%s of %s is neither an IP nor a domain: %s",
	KeyConfigHostInvalid:       "host of service %s is neither an IP nor a domain: %s",
	KeyConfigCategoryType:      "category of %s must be a string",
	KeyConfigPropsNotMapping:   "properties of %s must be a name: value mapping",
	KeyConfigPropEmptyName:     "properties of %s has an empty name",
	KeyConfigPropNotScalar:     "value of property %s of %s is not a scalar",
	KeyConfigCategoriesNotList: "categories must be a list",
	KeyConfigCategoryNotObject: "categories[%d] must be a mapping",
	KeyConfigCategoryID:        "categories[%d] has no non-empty string field id",
	KeyConfigCategoryLabel:     "label of categories[%d] must be a string",
	KeyConfigCategoryOrder:     "order of categories[%d] must be an integer",
	KeyConfigServicesNotList:   "services must be a list",
	KeyConfigServiceNotObject:  "services[%d] must be a mapping",
	KeyConfigServiceName:       "services[%d] has no non-empty string field name",
	KeyConfigServiceHost:       "service %s has no non-empty string field host",
	KeyConfigServicePort:       "port of service %s must be an integer",
	KeyConfigServicePortRange:  "port of service %s is out of 1-65535",
	KeyConfigServiceProtocol:   "protocol of service %s must be udp / tcp / a2s, got %s",
	KeyConfigServiceConnectCmd: "connect_cmd of service %s must be a string",
	KeyConfigLinksNotList:      "links must be a list",
	KeyConfigLinkNotObject:     "links[%d] must be a mapping",
	KeyConfigLinkName:          "links[%d] has no non-empty string field name",
	KeyConfigLinkURL:           "link %s has no non-empty string field url",
	KeyConfigLinkURLInvalid:    "%s of link %s must be an absolute http(s):// URL: %s",
	KeyConfigLinkProbeURL:      "probe_url of link %s must be a string",
	KeyConfigLinkExpectStatus:  "expect_status of link %s must be an integer",
	KeyConfigProbeNotMapping:   "probe must be a mapping",
	KeyConfigProbeMethod:       "probe.method must be icmp / tcp / http, got %s",
	KeyConfigProbeTimeout:      "probe.timeout must be a number of seconds",
	KeyConfigProbePort:         "probe.port must be an integer",
	KeyConfigProbeConcurrency:  "probe.concurrency must be an integer",
	KeyConfigProbeCount:        "probe.count must be an integer",

	KeyProbeNoPing:         "no ping command on this system, switch probe.method to tcp",
	KeyProbeDNS:            "cannot resolve %s",
	KeyProbeMethod:         "unsupported probe method: %s",
	KeyProbeProtocol:       "unsupported protocol: %s",
	KeyDetailICMPNoReply:   "no ICMP reply",
	KeyDetailDNSFailed:     "DNS lookup failed",
	KeyDetailUnreachable:   "network unreachable (is the WireGuard tunnel up?)",
	KeyDetailPingExit:      "ping exited with %d",
	KeyDetailTCPTimeout:    "TCP %d timed out",
	KeyDetailTCPFailed:     "cannot connect over TCP: %s",
	KeyDetailTCPRefused:    "connection refused (%s)",
	KeyDetailTCPOK:         "TCP connected",
	KeyDetailHTTPStatus:    "HTTP %d",
	KeyDetailHTTPTimeout:   "HTTP timed out",
	KeyDetailHTTPFailed:    "HTTP failed: %s",
	KeyDetailUDPNoReply:    "no UDP reply",
	KeyDetailUDPRefused:    "port refused (ICMP unreachable)",
	KeyDetailUDPBytes:      "got %d bytes back",
	KeyDetailA2SOK:         "A2S_INFO answered",
	KeyDetailLinkStatus:    "HTTP %d",
	KeyDetailLinkMismatch:  "HTTP %d (want %d)",
	KeyDetailLinkFailed:    "unreachable: %s",
	KeyDetailLinkTimeout:   "request timed out",
	KeyDetailWaitFirstScan: "waiting for the first scan",

	KeyMessageBadID:          "bad message id",
	KeyMessageNotFound:       "no such message: %s",
	KeyMessageDeleted:        "message %s is deleted",
	KeyMessageKeyRequired:    "a key message needs a key",
	KeyMessageKeyWrong:       "wrong key",
	KeyMessageNameMissing:    "missing attachment name (upload with ?name=)",
	KeyMessageNameInvalid:    "bad attachment name",
	KeyMessageTextNotString:  "text must be a string",
	KeyMessageKindInvalid:    "kind must be public / key",
	KeyMessageEmpty:          "a message needs text or an attachment",
	KeyMessageAttsNotArray:   "attachments must be a list",
	KeyMessageAttNotObject:   "an attachment entry must be a mapping",
	KeyMessageSHABad:         "bad attachment sha256: %s",
	KeyMessageSHAFormat:      "bad sha256",
	KeyMessageFileNotInPool:  "attachment %s is not in the pool yet, upload it first",
	KeyMessageFileTooBig:     "file exceeds the %d byte limit",
	KeyMessageDeclaredTooBig: "file is %d bytes, over the %d byte limit",
	KeyMessageFileGone:       "%s is gone from the attachment pool",
	KeyMessageNotReferenced:  "%s does not reference attachment %s",
	KeyMessageIDRequired:     "downloading an attachment needs ?id=<message id>",
	KeyMessageBodyNotJSON:    "request body is not JSON: %s",
	KeyMessageBodyNotObject:  "request body must be a JSON object",
	KeyMessageBrokenFile:     "cannot read %s: %s",
	KeyMessageFileNotObject:  "%s is not a JSON object",
	KeyMessageFileKind:       "%s has unknown kind=%s",
	KeyMessageFileFields:     "%s lacks text / isDeleted",
	KeyMessageFileKey:        "%s is a key message without a key",
	KeyMessageFileAtts:       "attachments of %s is not a list",
	KeyMessageFileAttEntry:   "%s has a bad attachment entry: %s",
	KeyMessageFileUpdatedAt:  "updatedAt of %s is not an integer",
	KeyMessageBodyTooBig:     "request body exceeds %d bytes",
	KeyMessageBodyType:       "field %s of the request body has the wrong type",

	KeyAppTitle:             "Scope Status",
	KeyHTTPNotFound:         "no such endpoint: %s",
	KeyHTTPNotReady:         "service is not ready yet",
	KeyHTTPNotReadyCause:    "not ready: %s",
	KeyHTTPTokenBad:         "bad token",
	KeyHTTPInternal:         "server error: %s",
	KeyHTTPNoFlush:          "this connection cannot stream",
	KeyHTTPBusy:             "server is busy, try again later",
	KeyHTTPMethodNotAllowed: "method not allowed: %s",
	KeyReloadOK:             "reloaded %d entries / %d services / %d sites",

	KeyCategoryFriend:  "Friends",
	KeyCategoryService: "Services",
	KeyCategoryServer:  "Servers",
	KeyCategoryNAS:     "NAS / storage",
	KeyCategoryRouter:  "Routers",
	KeyCategoryIoT:     "IoT / smart home",
	KeyCategoryWork:    "Work",
	KeyCategoryOther:   "Other",
}

var tables = map[Lang]map[Key]string{Zh: zh, En: en}

// T 渲染一条文案；语言或键不认识时原样返回键名。
func T(lang Lang, key Key, args ...any) string {
	text, ok := tables[lang][key]
	if !ok {
		return string(key)
	}
	if len(args) == 0 {
		return text
	}
	return fmt.Sprintf(text, args...)
}

// Negotiate 从 Accept-Language 里挑一个支持的语言：按 q 值取最高的 zh / en。
func Negotiate(header string) Lang {
	best, bestQ := Default, -1.0
	for _, part := range strings.Split(header, ",") {
		fields := strings.Split(part, ";")
		tag := strings.ToLower(strings.TrimSpace(fields[0]))
		if tag == "" {
			continue
		}
		q := 1.0
		for _, field := range fields[1:] {
			name, value, found := strings.Cut(strings.TrimSpace(field), "=")
			if !found || strings.TrimSpace(name) != "q" {
				continue
			}
			parsed, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
			if err != nil {
				continue
			}
			q = parsed
		}
		var lang Lang
		switch {
		case tag == "*":
			lang = Default
		case strings.HasPrefix(tag, "zh"):
			lang = Zh
		case strings.HasPrefix(tag, "en"):
			lang = En
		default:
			continue
		}
		if q > bestQ {
			best, bestQ = lang, q
		}
	}
	return best
}
