// Package core 是纯逻辑层：配置的解析与校验、探测、消息的标识与校验规则。
// 它不 import 任何上层包。
package core

import (
	"bytes"
	"encoding/json"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/hm-service/scope-status/i18n"

	"gopkg.in/yaml.v3"
)

const (
	ServiceCategory = "service"

	ProtocolUDP = "udp"
	ProtocolTCP = "tcp"
	ProtocolA2S = "a2s"

	MethodICMP = "icmp"
	MethodTCP  = "tcp"
	MethodHTTP = "http"

	StatusUp   = "up"
	StatusDown = "down"

	// 条目类型：peers 只能写 friend / server；cloud 是 cloud_servers 块专用的值，
	// 解析时会并进 peers 列表，前端按它认「中继基建」里的机器。
	KindFriend = "friend"
	KindServer = "server"
	KindCloud  = "cloud"
)

// Property 是端点属性的一项，保留配置文件里的书写顺序。
type Property struct {
	Name  string
	Value string
}

type Properties []Property

func (p Properties) Get(name string) (string, bool) {
	for _, item := range p {
		if item.Name == name {
			return item.Value, true
		}
	}
	return "", false
}

// MarshalJSON 按书写顺序输出成对象：map 会把顺序丢掉。
func (p Properties) MarshalJSON() ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteByte('{')
	for index, item := range p {
		if index > 0 {
			buf.WriteByte(',')
		}
		name, err := json.Marshal(item.Name)
		if err != nil {
			return nil, err
		}
		value, err := json.Marshal(item.Value)
		if err != nil {
			return nil, err
		}
		buf.Write(name)
		buf.WriteByte(':')
		buf.Write(value)
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

// Category 是一个展示分组。内置分类的组名走文案表，配置里写了 label 就用配置的。
type Category struct {
	ID       string
	Label    string
	LabelKey i18n.Key
	Order    int
}

func (c Category) Render(lang i18n.Lang) string {
	if c.Label != "" {
		return c.Label
	}
	if c.LabelKey != "" {
		return i18n.T(lang, c.LabelKey)
	}
	return c.ID
}

// 分组顺序写死在这儿：配置里不用再维护 categories 块。
var builtinOrders = map[string]int{
	"friend": 10, "server": 20, "cloud": 30, "service": 40,
	"nas": 50, "router": 60, "iot": 70, "work": 80, "other": 90,
}

func builtinCategory(id string) Category {
	order, ok := builtinOrders[id]
	if !ok {
		order = 100
	}
	return Category{ID: id, LabelKey: builtinLabels[id], Order: order}
}

var builtinLabels = map[string]i18n.Key{
	"friend":  i18n.KeyCategoryFriend,
	"service": i18n.KeyCategoryService,
	"server":  i18n.KeyCategoryServer,
	"cloud":   i18n.KeyCategoryCloud,
	"nas":     i18n.KeyCategoryNAS,
	"router":  i18n.KeyCategoryRouter,
	"iot":     i18n.KeyCategoryIoT,
	"work":    i18n.KeyCategoryWork,
	"other":   i18n.KeyCategoryOther,
}

type ProbeConfig struct {
	Method      string
	Timeout     time.Duration
	Port        int
	Concurrency int
	Count       int
}

func DefaultProbeConfig() ProbeConfig {
	return ProbeConfig{Method: MethodICMP, Timeout: 2 * time.Second, Port: 80, Concurrency: 16, Count: 1}
}

// Peer 是「会被探测的一个条目」：peers 块和 cloud_servers 块解析完都长这样。
// Kind 区分 friend / server / cloud；Category 直接等于 Kind（分组走内置名字与顺序）。
type Peer struct {
	Name       string
	Note       *string
	Kind       string
	Category   string
	Addresses  []string
	Properties Properties
	Index      int

	// 只有 kind = cloud 才有：core 是 WG 主干，private / public 分开记，便于展示与探测。
	Core    bool
	Private []string
	Public  []string
}

type Service struct {
	Name       string
	Host       string
	Port       int
	Protocol   string
	ConnectCmd *string
	Category   string
	Properties Properties
	Index      int
}

func (s Service) Endpoint() string { return net.JoinHostPort(s.Host, strconv.Itoa(s.Port)) }

type Link struct {
	Name         string
	URL          string
	ProbeURL     *string
	ExpectStatus int
	Properties   Properties
	Index        int
}

func (l Link) Target() string {
	if l.ProbeURL != nil {
		return *l.ProbeURL
	}
	return l.URL
}

type Source struct {
	Title      string
	TitleKey   i18n.Key
	Probe      ProbeConfig
	Categories []Category
	Peers      []Peer
	Services   []Service
	Links      []Link
}

func (s *Source) TitleText(lang i18n.Lang) string {
	if s.Title != "" {
		return s.Title
	}
	if s.TitleKey != "" {
		return i18n.T(lang, s.TitleKey)
	}
	return i18n.T(lang, i18n.KeyAppTitle)
}

func (s *Source) Category(id string) Category {
	for _, category := range s.Categories {
		if category.ID == id {
			return category
		}
	}
	return builtinCategory(id)
}

// ParseSource 解析配置文件的内容；顶层数组等价于 peers 列表。
func ParseSource(name string, data []byte) (*Source, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, ConfigError(i18n.KeyConfigSyntax, name, err.Error())
	}
	root := &doc
	if root.Kind == yaml.DocumentNode {
		if len(root.Content) == 0 {
			return nil, ConfigError(i18n.KeyConfigEmpty, name)
		}
		root = root.Content[0]
	}
	if root.Kind == 0 || (root.Kind == yaml.ScalarNode && root.Tag == "!!null") {
		return nil, ConfigError(i18n.KeyConfigEmpty, name)
	}
	if root.Kind == yaml.SequenceNode {
		root = &yaml.Node{Kind: yaml.MappingNode, Content: []*yaml.Node{
			{Kind: yaml.ScalarNode, Tag: "!!str", Value: "peers"},
			root,
		}}
	}
	if root.Kind != yaml.MappingNode {
		return nil, ConfigError(i18n.KeyConfigNotMapping, name)
	}

	pairs, err := mapPairs(root, name)
	if err != nil {
		return nil, err
	}
	source := &Source{TitleKey: i18n.KeyAppTitle, Probe: DefaultProbeConfig()}

	if node := pairs.get("title"); node != nil {
		title, err := wantString(node, i18n.KeyConfigTitle)
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(title) == "" {
			return nil, ConfigError(i18n.KeyConfigTitle)
		}
		source.Title, source.TitleKey = strings.TrimSpace(title), ""
	}

	if node := pairs.get("probe"); node != nil {
		probe, err := parseProbe(node)
		if err != nil {
			return nil, err
		}
		source.Probe = probe
	}

	if node := pairs.get("peers"); !isNull(node) {
		peers, err := parsePeers(node)
		if err != nil {
			return nil, err
		}
		source.Peers = peers
	}

	// 云服务器并进 peers 列表（kind = cloud）：探测、分组、摘要都走同一条路。
	if node := pairs.get("cloud_servers"); !isNull(node) {
		clouds, err := parseCloudServers(node, len(source.Peers))
		if err != nil {
			return nil, err
		}
		source.Peers = append(source.Peers, clouds...)
	}

	if node := pairs.get("services"); !isNull(node) {
		services, err := parseServices(node)
		if err != nil {
			return nil, err
		}
		source.Services = services
	}

	if node := pairs.get("links"); !isNull(node) {
		links, err := parseLinks(node)
		if err != nil {
			return nil, err
		}
		source.Links = links
	}

	source.Categories = usedCategories(source.Peers, source.Services)
	return source, nil
}

// usedCategories 收集用到的分类，配内置名字与顺序（内置表里没有的原样当组名）。
func usedCategories(peers []Peer, services []Service) []Category {
	seen := make(map[string]bool, len(peers)+len(services))
	categories := make([]Category, 0, len(peers)+len(services))
	for _, id := range append(peerCategories(peers), serviceCategories(services)...) {
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		categories = append(categories, builtinCategory(id))
	}
	sortCategories(categories)
	return categories
}

func peerCategories(peers []Peer) []string {
	ids := make([]string, 0, len(peers))
	for _, peer := range peers {
		ids = append(ids, peer.Category)
	}
	return ids
}

func serviceCategories(services []Service) []string {
	ids := make([]string, 0, len(services))
	for _, service := range services {
		ids = append(ids, service.Category)
	}
	return ids
}

func sortCategories(categories []Category) {
	for i := 1; i < len(categories); i++ {
		for j := i; j > 0; j-- {
			left, right := categories[j-1], categories[j]
			if left.Order < right.Order || (left.Order == right.Order && left.ID <= right.ID) {
				break
			}
			categories[j-1], categories[j] = right, left
		}
	}
}

func parseProbe(node *yaml.Node) (ProbeConfig, error) {
	probe := DefaultProbeConfig()
	if node.Kind != yaml.MappingNode {
		return probe, ConfigError(i18n.KeyConfigProbeNotMapping)
	}
	pairs, err := mapPairs(node, "probe")
	if err != nil {
		return probe, err
	}
	if field := pairs.get("method"); field != nil {
		method, err := wantString(field, i18n.KeyConfigProbeMethod, "")
		if err != nil {
			return probe, err
		}
		method = strings.ToLower(strings.TrimSpace(method))
		switch method {
		case MethodICMP, MethodTCP, MethodHTTP:
			probe.Method = method
		default:
			return probe, ConfigError(i18n.KeyConfigProbeMethod, method)
		}
	}
	if field := pairs.get("timeout"); field != nil {
		timeout, err := wantNumber(field, i18n.KeyConfigProbeTimeout)
		if err != nil {
			return probe, err
		}
		probe.Timeout = time.Duration(timeout * float64(time.Second))
	}
	if field := pairs.get("port"); field != nil {
		port, err := wantInt(field, i18n.KeyConfigProbePort)
		if err != nil {
			return probe, err
		}
		probe.Port = port
	}
	if field := pairs.get("concurrency"); field != nil {
		value, err := wantInt(field, i18n.KeyConfigProbeConcurrency)
		if err != nil {
			return probe, err
		}
		probe.Concurrency = max(1, value)
	}
	if field := pairs.get("count"); field != nil {
		value, err := wantInt(field, i18n.KeyConfigProbeCount)
		if err != nil {
			return probe, err
		}
		probe.Count = max(1, value)
	}
	return probe, nil
}

func parsePeers(node *yaml.Node) ([]Peer, error) {
	if node.Kind != yaml.SequenceNode {
		return nil, ConfigError(i18n.KeyConfigPeersNotList)
	}
	peers := make([]Peer, 0, len(node.Content))
	for index, item := range node.Content {
		if item.Kind != yaml.MappingNode {
			return nil, ConfigError(i18n.KeyConfigPeerNotObject, index)
		}
		pairs, err := mapPairs(item, "peers")
		if err != nil {
			return nil, err
		}
		nameNode := pairs.get("name")
		if nameNode == nil {
			return nil, ConfigError(i18n.KeyConfigPeerName, index)
		}
		name, err := wantString(nameNode, i18n.KeyConfigPeerName, index)
		if err != nil {
			return nil, err
		}
		name = strings.TrimSpace(name)
		if name == "" {
			return nil, ConfigError(i18n.KeyConfigPeerName, index)
		}

		addressNode := pairs.get("addresses")
		if addressNode == nil {
			return nil, ConfigError(i18n.KeyConfigAddressesMissing, name)
		}
		if addressNode.Kind != yaml.SequenceNode || len(addressNode.Content) == 0 {
			return nil, ConfigError(i18n.KeyConfigAddressesNotList, name)
		}
		addresses := make([]string, 0, len(addressNode.Content))
		for _, entry := range addressNode.Content {
			address, err := wantString(entry, i18n.KeyConfigAddressesNotList, name)
			if err != nil {
				return nil, err
			}
			address = strings.TrimSpace(address)
			if address == "" {
				continue
			}
			if !validHost(hostOf(address)) {
				return nil, ConfigError(i18n.KeyConfigAddressInvalid, name, "地址", address)
			}
			if !contains(addresses, address) {
				addresses = append(addresses, address)
			}
		}
		if len(addresses) == 0 {
			return nil, ConfigError(i18n.KeyConfigAddressesEmpty, name)
		}

		kindNode := pairs.get("kind")
		if kindNode == nil {
			return nil, ConfigError(i18n.KeyConfigPeerKind, name)
		}
		kind, err := parsePeerKind(kindNode, name)
		if err != nil {
			return nil, err
		}
		peer := Peer{Name: name, Addresses: addresses, Kind: kind, Category: kind, Index: index}
		if noteNode := pairs.get("note"); noteNode != nil {
			note, err := wantString(noteNode, i18n.KeyConfigPeerNote, name)
			if err != nil {
				return nil, err
			}
			if trimmed := strings.TrimSpace(note); trimmed != "" {
				peer.Note = &trimmed
			}
		}
		properties, err := parseProperties(pairs.get("properties"), name)
		if err != nil {
			return nil, err
		}
		peer.Properties = properties
		peers = append(peers, peer)
	}
	return peers, nil
}

// parsePeerKind 只认 friend / server；没写或写错都直接报错，不猜。
func parsePeerKind(node *yaml.Node, name string) (string, error) {
	raw, err := wantString(node, i18n.KeyConfigPeerKind, name)
	if err != nil {
		return "", err
	}
	switch kind := slug(raw); kind {
	case KindFriend, KindServer:
		return kind, nil
	default:
		return "", ConfigError(i18n.KeyConfigPeerKind, name)
	}
}

// parseCloudServers 解析 cloud_servers：中继服务器。
// private / public 都是数组（至少一边非空），core 全文件最多一个。
func parseCloudServers(node *yaml.Node, offset int) ([]Peer, error) {
	if node.Kind != yaml.SequenceNode {
		return nil, ConfigError(i18n.KeyConfigCloudServersNotList)
	}
	servers := make([]Peer, 0, len(node.Content))
	coreName := ""
	for index, item := range node.Content {
		if item.Kind != yaml.MappingNode {
			return nil, ConfigError(i18n.KeyConfigCloudServerNotObject, index)
		}
		pairs, err := mapPairs(item, "cloud_servers")
		if err != nil {
			return nil, err
		}
		nameNode := pairs.get("name")
		if nameNode == nil {
			return nil, ConfigError(i18n.KeyConfigCloudServerName, index)
		}
		name, err := wantString(nameNode, i18n.KeyConfigCloudServerName, index)
		if err != nil {
			return nil, err
		}
		name = strings.TrimSpace(name)
		if name == "" {
			return nil, ConfigError(i18n.KeyConfigCloudServerName, index)
		}

		private, err := parseAddressList(pairs.get("private"), name, "private")
		if err != nil {
			return nil, err
		}
		public, err := parseAddressList(pairs.get("public"), name, "public")
		if err != nil {
			return nil, err
		}
		if len(private) == 0 && len(public) == 0 {
			return nil, ConfigError(i18n.KeyConfigCloudServerAddress, name)
		}

		server := Peer{
			Name: name, Kind: KindCloud, Category: KindCloud, Index: offset + index,
			Addresses: append(append([]string{}, private...), public...),
			Private:   private,
			Public:    public,
		}
		if coreNode := pairs.get("core"); coreNode != nil {
			core, err := wantBool(coreNode, i18n.KeyConfigCloudServerCore, name)
			if err != nil {
				return nil, err
			}
			if core {
				if coreName != "" {
					return nil, ConfigError(i18n.KeyConfigCloudServerCoreMultiple, coreName, name)
				}
				coreName = name
			}
			server.Core = core
		}
		if noteNode := pairs.get("note"); noteNode != nil {
			note, err := wantString(noteNode, i18n.KeyConfigPeerNote, name)
			if err != nil {
				return nil, err
			}
			if trimmed := strings.TrimSpace(note); trimmed != "" {
				server.Note = &trimmed
			}
		}
		properties, err := parseProperties(pairs.get("properties"), name)
		if err != nil {
			return nil, err
		}
		server.Properties = properties
		servers = append(servers, server)
	}
	return servers, nil
}

// parseAddressList 解析 private / public：不写没关系，写了就必须是合法的非空字符串数组。
func parseAddressList(node *yaml.Node, name, field string) ([]string, error) {
	if isNull(node) {
		return nil, nil
	}
	if node.Kind != yaml.SequenceNode {
		return nil, ConfigError(i18n.KeyConfigAddressesNotList, name)
	}
	addresses := make([]string, 0, len(node.Content))
	for _, entry := range node.Content {
		address, err := wantString(entry, i18n.KeyConfigAddressesNotList, name)
		if err != nil {
			return nil, err
		}
		address = strings.TrimSpace(address)
		if address == "" {
			return nil, ConfigError(i18n.KeyConfigCloudServerAddress, name)
		}
		if !validHost(hostOf(address)) {
			return nil, ConfigError(i18n.KeyConfigAddressInvalid, name, field, address)
		}
		if !contains(addresses, address) {
			addresses = append(addresses, address)
		}
	}
	return addresses, nil
}

func parseServices(node *yaml.Node) ([]Service, error) {
	if node.Kind != yaml.SequenceNode {
		return nil, ConfigError(i18n.KeyConfigServicesNotList)
	}
	services := make([]Service, 0, len(node.Content))
	for index, item := range node.Content {
		if item.Kind != yaml.MappingNode {
			return nil, ConfigError(i18n.KeyConfigServiceNotObject, index)
		}
		pairs, err := mapPairs(item, "services")
		if err != nil {
			return nil, err
		}
		nameNode := pairs.get("name")
		if nameNode == nil {
			return nil, ConfigError(i18n.KeyConfigServiceName, index)
		}
		name, err := wantString(nameNode, i18n.KeyConfigServiceName, index)
		if err != nil {
			return nil, err
		}
		name = strings.TrimSpace(name)
		if name == "" {
			return nil, ConfigError(i18n.KeyConfigServiceName, index)
		}

		hostNode := pairs.get("host")
		if hostNode == nil {
			return nil, ConfigError(i18n.KeyConfigServiceHost, name)
		}
		host, err := wantString(hostNode, i18n.KeyConfigServiceHost, name)
		if err != nil {
			return nil, err
		}
		host = strings.TrimSpace(host)
		if host == "" {
			return nil, ConfigError(i18n.KeyConfigServiceHost, name)
		}
		if !validHost(host) {
			return nil, ConfigError(i18n.KeyConfigHostInvalid, name, host)
		}

		portNode := pairs.get("port")
		if portNode == nil {
			return nil, ConfigError(i18n.KeyConfigServicePort, name)
		}
		port, err := wantInt(portNode, i18n.KeyConfigServicePort, name)
		if err != nil {
			return nil, err
		}
		if port < 1 || port > 65535 {
			return nil, ConfigError(i18n.KeyConfigServicePortRange, name)
		}

		service := Service{Name: name, Host: host, Port: port, Protocol: ProtocolUDP, Category: ServiceCategory, Index: index}
		if protocolNode := pairs.get("protocol"); protocolNode != nil {
			protocol, err := wantString(protocolNode, i18n.KeyConfigServiceProtocol, name, "")
			if err != nil {
				return nil, err
			}
			protocol = strings.ToLower(strings.TrimSpace(protocol))
			switch protocol {
			case ProtocolUDP, ProtocolTCP, ProtocolA2S:
				service.Protocol = protocol
			default:
				return nil, ConfigError(i18n.KeyConfigServiceProtocol, name, protocol)
			}
		}
		if cmdNode := pairs.get("connect_cmd"); cmdNode != nil {
			cmd, err := wantString(cmdNode, i18n.KeyConfigServiceConnectCmd, name)
			if err != nil {
				return nil, err
			}
			if trimmed := strings.TrimSpace(cmd); trimmed != "" {
				service.ConnectCmd = &trimmed
			}
		}
		if categoryNode := pairs.get("category"); categoryNode != nil {
			category, err := wantString(categoryNode, i18n.KeyConfigCategoryType, name)
			if err != nil {
				return nil, err
			}
			if slug := slug(category); slug != "" {
				service.Category = slug
			}
		}
		properties, err := parseProperties(pairs.get("properties"), name)
		if err != nil {
			return nil, err
		}
		service.Properties = properties
		services = append(services, service)
	}
	return services, nil
}

func parseLinks(node *yaml.Node) ([]Link, error) {
	if node.Kind != yaml.SequenceNode {
		return nil, ConfigError(i18n.KeyConfigLinksNotList)
	}
	links := make([]Link, 0, len(node.Content))
	for index, item := range node.Content {
		if item.Kind != yaml.MappingNode {
			return nil, ConfigError(i18n.KeyConfigLinkNotObject, index)
		}
		pairs, err := mapPairs(item, "links")
		if err != nil {
			return nil, err
		}
		nameNode := pairs.get("name")
		if nameNode == nil {
			return nil, ConfigError(i18n.KeyConfigLinkName, index)
		}
		name, err := wantString(nameNode, i18n.KeyConfigLinkName, index)
		if err != nil {
			return nil, err
		}
		name = strings.TrimSpace(name)
		if name == "" {
			return nil, ConfigError(i18n.KeyConfigLinkName, index)
		}

		urlNode := pairs.get("url")
		if urlNode == nil {
			return nil, ConfigError(i18n.KeyConfigLinkURL, name)
		}
		rawURL, err := wantString(urlNode, i18n.KeyConfigLinkURL, name)
		if err != nil {
			return nil, err
		}
		rawURL = strings.TrimSpace(rawURL)
		if rawURL == "" {
			return nil, ConfigError(i18n.KeyConfigLinkURL, name)
		}
		if !validURL(rawURL) {
			return nil, ConfigError(i18n.KeyConfigLinkURLInvalid, name, "url", rawURL)
		}

		link := Link{Name: name, URL: rawURL, ExpectStatus: 200, Index: index}
		if probeNode := pairs.get("probe_url"); probeNode != nil {
			probeURL, err := wantString(probeNode, i18n.KeyConfigLinkProbeURL, name)
			if err != nil {
				return nil, err
			}
			probeURL = strings.TrimSpace(probeURL)
			if probeURL != "" {
				if !validURL(probeURL) {
					return nil, ConfigError(i18n.KeyConfigLinkURLInvalid, name, "probe_url", probeURL)
				}
				link.ProbeURL = &probeURL
			}
		}
		if statusNode := pairs.get("expect_status"); statusNode != nil {
			status, err := wantInt(statusNode, i18n.KeyConfigLinkExpectStatus, name)
			if err != nil {
				return nil, err
			}
			link.ExpectStatus = status
		}
		properties, err := parseProperties(pairs.get("properties"), name)
		if err != nil {
			return nil, err
		}
		link.Properties = properties
		links = append(links, link)
	}
	return links, nil
}

func parseProperties(node *yaml.Node, owner string) (Properties, error) {
	if node == nil {
		return nil, nil
	}
	if node.Kind != yaml.MappingNode {
		return nil, ConfigError(i18n.KeyConfigPropsNotMapping, owner)
	}
	properties := make(Properties, 0, len(node.Content)/2)
	seen := make(map[string]bool, len(node.Content)/2)
	for index := 0; index+1 < len(node.Content); index += 2 {
		keyNode, valueNode := node.Content[index], node.Content[index+1]
		if keyNode.Kind != yaml.ScalarNode || strings.TrimSpace(keyNode.Value) == "" {
			return nil, ConfigError(i18n.KeyConfigPropEmptyName, owner)
		}
		name := strings.TrimSpace(keyNode.Value)
		if seen[name] {
			return nil, ConfigError(i18n.KeyConfigDuplicateKey, owner, name)
		}
		seen[name] = true
		if valueNode.Kind != yaml.ScalarNode {
			return nil, ConfigError(i18n.KeyConfigPropNotScalar, owner, name)
		}
		properties = append(properties, Property{Name: name, Value: strings.TrimSpace(valueNode.Value)})
	}
	return properties, nil
}

type nodePair struct {
	key   string
	value *yaml.Node
}

type nodePairs []nodePair

func (p nodePairs) get(names ...string) *yaml.Node {
	for _, want := range names {
		for _, item := range p {
			if item.key == want {
				return item.value
			}
		}
	}
	return nil
}

func mapPairs(node *yaml.Node, owner string) (nodePairs, error) {
	pairs := make(nodePairs, 0, len(node.Content)/2)
	seen := make(map[string]bool, len(node.Content)/2)
	for index := 0; index+1 < len(node.Content); index += 2 {
		keyNode, valueNode := node.Content[index], node.Content[index+1]
		if keyNode.Kind != yaml.ScalarNode {
			return nil, ConfigError(i18n.KeyConfigNotMapping, owner)
		}
		key := keyNode.Value
		if seen[key] {
			return nil, ConfigError(i18n.KeyConfigDuplicateKey, owner, key)
		}
		seen[key] = true
		pairs = append(pairs, nodePair{key: key, value: valueNode})
	}
	return pairs, nil
}

// isNull 认「字段写了但没给值」（peers: 这种），当成空列表。
func isNull(node *yaml.Node) bool {
	return node == nil || (node.Kind == yaml.ScalarNode && node.Tag == "!!null")
}

func wantString(node *yaml.Node, key i18n.Key, args ...any) (string, error) {
	if node.Kind != yaml.ScalarNode || node.Tag != "!!str" {
		return "", ConfigError(key, args...)
	}
	return node.Value, nil
}

func wantBool(node *yaml.Node, key i18n.Key, args ...any) (bool, error) {
	if node.Kind != yaml.ScalarNode || node.Tag != "!!bool" {
		return false, ConfigError(key, args...)
	}
	return node.Value == "true", nil
}

func wantInt(node *yaml.Node, key i18n.Key, args ...any) (int, error) {
	if node.Kind != yaml.ScalarNode || node.Tag != "!!int" {
		return 0, ConfigError(key, args...)
	}
	value, err := strconv.Atoi(node.Value)
	if err != nil {
		return 0, ConfigError(key, args...)
	}
	return value, nil
}

func wantNumber(node *yaml.Node, key i18n.Key, args ...any) (float64, error) {
	if node.Kind != yaml.ScalarNode || (node.Tag != "!!int" && node.Tag != "!!float") {
		return 0, ConfigError(key, args...)
	}
	value, err := strconv.ParseFloat(node.Value, 64)
	if err != nil {
		return 0, ConfigError(key, args...)
	}
	return value, nil
}

func slug(value string) string {
	return strings.ReplaceAll(strings.ToLower(strings.TrimSpace(value)), " ", "-")
}

// hostOf 从「主机[:端口]」里取出主机部分；IPv6 字面量不做拆分。
func hostOf(address string) string {
	if strings.Count(address, ":") == 1 {
		host, _, _ := strings.Cut(address, ":")
		return host
	}
	return address
}

func validHost(host string) bool {
	if host == "" {
		return false
	}
	if net.ParseIP(host) != nil {
		return true
	}
	for _, part := range strings.Split(host, ".") {
		if part == "" {
			return false
		}
		for _, r := range part {
			if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '-' && r != '_' {
				return false
			}
		}
	}
	return true
}

func validURL(raw string) bool {
	parsed, err := url.Parse(raw)
	if err != nil {
		return false
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return false
	}
	return parsed.Host != ""
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
