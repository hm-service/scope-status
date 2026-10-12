package api

import (
	"time"

	"github.com/hm-service/scope-status/core"
	"github.com/hm-service/scope-status/i18n"
)

type addressView struct {
	Address   string   `json:"address"`
	Status    string   `json:"status"`
	LatencyMs *float64 `json:"latencyMs"`
	Detail    *string  `json:"detail"`
	Method    string   `json:"method"`
}

type friendView struct {
	Name       string          `json:"name"`
	Note       *string         `json:"note"`
	Category   string          `json:"category"`
	Online     bool            `json:"online"`
	LatencyMs  *float64        `json:"latencyMs"`
	Addresses  []addressView   `json:"addresses"`
	Properties core.Properties `json:"properties"`
}

type serviceView struct {
	Name       string          `json:"name"`
	Host       string          `json:"host"`
	Port       int             `json:"port"`
	Address    string          `json:"address"`
	Protocol   string          `json:"protocol"`
	ConnectCmd *string         `json:"connectCmd"`
	Category   string          `json:"category"`
	Properties core.Properties `json:"properties"`
	Online     bool            `json:"online"`
	LatencyMs  *float64        `json:"latencyMs"`
	Detail     *string         `json:"detail"`
	Method     string          `json:"method"`
	Status     string          `json:"status"`
}

type linkView struct {
	Name         string          `json:"name"`
	URL          string          `json:"url"`
	ProbeURL     string          `json:"probeUrl"`
	ExpectStatus int             `json:"expectStatus"`
	Properties   core.Properties `json:"properties"`
	Online       bool            `json:"online"`
	LatencyMs    *float64        `json:"latencyMs"`
	Detail       *string         `json:"detail"`
	Status       string          `json:"status"`
	FaviconURL   *string         `json:"faviconUrl"`
}

type groupView struct {
	ID       string        `json:"id"`
	Label    string        `json:"label"`
	Order    int           `json:"order"`
	Total    int           `json:"total"`
	Online   int           `json:"online"`
	Friends  []friendView  `json:"friends,omitempty"`
	Services []serviceView `json:"services,omitempty"`
}

type snapshotView struct {
	Title           string        `json:"title"`
	Method          string        `json:"method"`
	GeneratedAt     string        `json:"generatedAt"`
	DurationMs      float64       `json:"durationMs"`
	Reloadable      bool          `json:"reloadable"`
	ReloadCount     int           `json:"reloadCount"`
	StreamInterval  float64       `json:"streamInterval"`
	Reused          bool          `json:"reused"`
	Summary         core.Summary  `json:"summary"`
	Groups          []groupView   `json:"groups"`
	Friends         []friendView  `json:"friends"`
	Services        []serviceView `json:"services"`
	ServicesGroups  []groupView   `json:"servicesGroups"`
	ServicesSummary core.Summary  `json:"servicesSummary"`
	Links           []linkView    `json:"links"`
	LinksSummary    core.Summary  `json:"linksSummary"`
}

// SnapshotView 把快照转成前端消费的形状：文案在这里按请求的语言渲染。
func SnapshotView(snapshot *core.Snapshot, lang i18n.Lang, reused bool) any {
	friends := make([]friendView, 0, len(snapshot.Friends))
	for _, friend := range snapshot.Friends {
		addresses := make([]addressView, 0, len(friend.Addresses))
		for _, result := range friend.Addresses {
			addresses = append(addresses, addressView{
				Address:   result.Address,
				Status:    result.Status,
				LatencyMs: result.LatencyMs,
				Detail:    detailText(result.Detail, lang),
				Method:    result.Method,
			})
		}
		friends = append(friends, friendView{
			Name:       friend.Friend.Name,
			Note:       friend.Friend.Note,
			Category:   friend.Friend.Category,
			Online:     friend.Online,
			LatencyMs:  friend.LatencyMs,
			Addresses:  addresses,
			Properties: friend.Friend.Properties,
		})
	}

	services := make([]serviceView, 0, len(snapshot.Services))
	for _, service := range snapshot.Services {
		services = append(services, serviceView{
			Name:       service.Service.Name,
			Host:       service.Service.Host,
			Port:       service.Service.Port,
			Address:    service.Service.Endpoint(),
			Protocol:   service.Service.Protocol,
			ConnectCmd: service.Service.ConnectCmd,
			Category:   service.Service.Category,
			Properties: service.Service.Properties,
			Online:     service.Online,
			LatencyMs:  service.LatencyMs,
			Detail:     detailText(service.Detail, lang),
			Method:     service.Service.Protocol,
			Status:     service.Status,
		})
	}

	links := make([]linkView, 0, len(snapshot.Links))
	for _, link := range snapshot.Links {
		links = append(links, linkView{
			Name:         link.Link.Name,
			URL:          link.Link.URL,
			ProbeURL:     link.Link.Target(),
			ExpectStatus: link.Link.ExpectStatus,
			Properties:   link.Link.Properties,
			Online:       link.Online,
			LatencyMs:    link.LatencyMs,
			Detail:       detailText(link.Detail, lang),
			Status:       link.Status,
			FaviconURL:   link.FaviconURL,
		})
	}

	groups := make([]groupView, 0, len(snapshot.Groups))
	for _, group := range snapshot.Groups {
		members := make([]friendView, 0, len(group.Members))
		for _, index := range group.Members {
			members = append(members, friends[index])
		}
		groups = append(groups, groupView{
			ID:      group.Category.ID,
			Label:   group.Category.Render(lang),
			Order:   group.Category.Order,
			Total:   group.Total,
			Online:  group.Online,
			Friends: members,
		})
	}

	serviceGroups := make([]groupView, 0, len(snapshot.ServicesGroups))
	for _, group := range snapshot.ServicesGroups {
		members := make([]serviceView, 0, len(group.Members))
		for _, index := range group.Members {
			members = append(members, services[index])
		}
		serviceGroups = append(serviceGroups, groupView{
			ID:       group.Category.ID,
			Label:    group.Category.Render(lang),
			Order:    group.Category.Order,
			Total:    group.Total,
			Online:   group.Online,
			Services: members,
		})
	}

	return snapshotView{
		Title:           snapshot.TitleText(lang),
		Method:          snapshot.Method,
		GeneratedAt:     snapshot.GeneratedAt.Format(time.RFC3339),
		DurationMs:      snapshot.DurationMs,
		Reloadable:      snapshot.Reloadable,
		ReloadCount:     snapshot.ReloadCount,
		StreamInterval:  snapshot.StreamInterval,
		Reused:          reused,
		Summary:         snapshot.Summary,
		Groups:          groups,
		Friends:         friends,
		Services:        services,
		ServicesGroups:  serviceGroups,
		ServicesSummary: snapshot.ServicesSummary,
		Links:           links,
		LinksSummary:    snapshot.LinksSummary,
	}
}

func detailText(detail *core.Text, lang i18n.Lang) *string {
	if detail == nil {
		return nil
	}
	text := detail.Render(lang)
	return &text
}
