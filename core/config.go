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
	DefaultCategory = "friend"
	ServiceCategory = "service"

	ProtocolUDP = "udp"
	ProtocolTCP = "tcp"
	ProtocolA2S = "a2s"

	MethodICMP = "icmp"
	MethodTCP  = "tcp"
	MethodHTTP = "http"

	StatusUp   = "up"
	StatusDown = "down"
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

var builtinLabels = map[string]i18n.Key{
	"friend":  i18n.KeyCategoryFriend,
	"service": i18n.KeyCategoryService,
	"server":  i18n.KeyCategoryServer,
	"nas":     i18n.KeyCategoryNAS,
	"router":  i18n.KeyCategoryRouter,
	"iot":     i18n.KeyCategoryIoT,
	"work":    i18n.KeyCategoryWork,
	"other":   i18n.KeyCategoryOther,
}

func builtinCategory(id string) Category {
	return Category{ID: id, LabelKey: builtinLabels[id], Order: 100}
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

type Friend struct {
	Name       string
	Note       *string
	Category   string
	Addresses  []string
	Properties Properties
	Index      int
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
	Friends    []Friend
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

// ParseSource 解析配置文件的内容；顶层数组等价于 friends 列表。
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
			{Kind: yaml.ScalarNode, Tag: "!!str", Value: "friends"},
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

	if node := pairs.get("categories"); !isNull(node) {
		categories, err := parseCategories(node)
		if err != nil {
			return nil, err
		}
		source.Categories = categories
	}

	if node := pairs.get("friends", "Friends"); !isNull(node) {
		friends, err := parseFriends(node)
		if err != nil {
			return nil, err
		}
		source.Friends = friends
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

	source.Categories = completeCategories(source.Categories, source.Friends, source.Services)
	return source, nil
}

// completeCategories 给配置里没声明、但被用到的分类补上内置名字，避免前端丢组。
func completeCategories(categories []Category, friends []Friend, services []Service) []Category {
	declared := make(map[string]bool, len(categories))
	for _, category := range categories {
		declared[category.ID] = true
	}
	used := make([]string, 0, len(friends)+len(services))
	for _, friend := range friends {
		used = append(used, friend.Category)
	}
	for _, service := range services {
		used = append(used, service.Category)
	}
	for _, id := range used {
		if declared[id] {
			continue
		}
		declared[id] = true
		categories = append(categories, builtinCategory(id))
	}
	sortCategories(categories)
	return categories
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

func parseCategories(node *yaml.Node) ([]Category, error) {
	if node.Kind != yaml.SequenceNode {
		return nil, ConfigError(i18n.KeyConfigCategoriesNotList)
	}
	categories := make([]Category, 0, len(node.Content))
	for index, item := range node.Content {
		if item.Kind != yaml.MappingNode {
			return nil, ConfigError(i18n.KeyConfigCategoryNotObject, index)
		}
		pairs, err := mapPairs(item, "categories")
		if err != nil {
			return nil, err
		}
		idNode := pairs.get("id", "name", "key")
		if idNode == nil {
			return nil, ConfigError(i18n.KeyConfigCategoryID, index)
		}
		id, err := wantString(idNode, i18n.KeyConfigCategoryID, index)
		if err != nil {
			return nil, err
		}
		id = slug(id)
		if id == "" {
			return nil, ConfigError(i18n.KeyConfigCategoryID, index)
		}
		category := Category{ID: id, Order: 100 + index}
		if labelNode := pairs.get("label", "title"); labelNode != nil {
			label, err := wantString(labelNode, i18n.KeyConfigCategoryLabel, index)
			if err != nil {
				return nil, err
			}
			if strings.TrimSpace(label) != "" {
				category.Label = strings.TrimSpace(label)
			}
		}
		if orderNode := pairs.get("order"); orderNode != nil {
			order, err := wantInt(orderNode, i18n.KeyConfigCategoryOrder, index)
			if err != nil {
				return nil, err
			}
			category.Order = order
		}
		categories = append(categories, category)
	}
	return categories, nil
}

func parseFriends(node *yaml.Node) ([]Friend, error) {
	if node.Kind != yaml.SequenceNode {
		return nil, ConfigError(i18n.KeyConfigFriendsNotList)
	}
	friends := make([]Friend, 0, len(node.Content))
	for index, item := range node.Content {
		if item.Kind != yaml.MappingNode {
			return nil, ConfigError(i18n.KeyConfigFriendNotObject, index)
		}
		pairs, err := mapPairs(item, "friends")
		if err != nil {
			return nil, err
		}
		nameNode := pairs.get("name")
		if nameNode == nil {
			return nil, ConfigError(i18n.KeyConfigFriendName, index)
		}
		name, err := wantString(nameNode, i18n.KeyConfigFriendName, index)
		if err != nil {
			return nil, err
		}
		name = strings.TrimSpace(name)
		if name == "" {
			return nil, ConfigError(i18n.KeyConfigFriendName, index)
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

		friend := Friend{Name: name, Addresses: addresses, Category: DefaultCategory, Index: index}
		if noteNode := pairs.get("note"); noteNode != nil {
			note, err := wantString(noteNode, i18n.KeyConfigFriendNote, name)
			if err != nil {
				return nil, err
			}
			if trimmed := strings.TrimSpace(note); trimmed != "" {
				friend.Note = &trimmed
			}
		}
		if categoryNode := pairs.get("category", "group"); categoryNode != nil {
			category, err := wantString(categoryNode, i18n.KeyConfigCategoryType, name)
			if err != nil {
				return nil, err
			}
			if slug := slug(category); slug != "" {
				friend.Category = slug
			}
		}
		properties, err := parseProperties(pairs.get("properties"), name)
		if err != nil {
			return nil, err
		}
		friend.Properties = properties
		friends = append(friends, friend)
	}
	return friends, nil
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
		if cmdNode := pairs.get("connect_cmd", "connectCmd"); cmdNode != nil {
			cmd, err := wantString(cmdNode, i18n.KeyConfigServiceConnectCmd, name)
			if err != nil {
				return nil, err
			}
			if trimmed := strings.TrimSpace(cmd); trimmed != "" {
				service.ConnectCmd = &trimmed
			}
		}
		if categoryNode := pairs.get("category", "group"); categoryNode != nil {
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
		if probeNode := pairs.get("probe_url", "probeUrl"); probeNode != nil {
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
		if statusNode := pairs.get("expect_status", "expectStatus"); statusNode != nil {
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

// isNull 认「字段写了但没给值」（friends: 这种），当成空列表。
func isNull(node *yaml.Node) bool {
	return node == nil || (node.Kind == yaml.ScalarNode && node.Tag == "!!null")
}

func wantString(node *yaml.Node, key i18n.Key, args ...any) (string, error) {
	if node.Kind != yaml.ScalarNode || node.Tag != "!!str" {
		return "", ConfigError(key, args...)
	}
	return node.Value, nil
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
