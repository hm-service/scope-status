package core

import (
	"time"

	"github.com/hm-service/scope-status/i18n"
)

type Summary struct {
	Total   int `json:"total"`
	Online  int `json:"online"`
	Offline int `json:"offline"`
}

func NewSummary(total, online int) Summary {
	return Summary{Total: total, Online: online, Offline: total - online}
}

type FriendStatus struct {
	Friend    Friend
	Online    bool
	LatencyMs *float64
	Addresses []ProbeResult
}

type ServiceStatus struct {
	Service   Service
	Online    bool
	LatencyMs *float64
	Detail    *Text
	Status    string
}

type LinkStatus struct {
	Link       Link
	Online     bool
	LatencyMs  *float64
	Detail     *Text
	Status     string
	FaviconURL *string
}

// Group 是同一分类下的成员；Members 存的是成员在扁平列表里的下标。
type Group struct {
	Category Category
	Total    int
	Online   int
	Members  []int
}

type Snapshot struct {
	Title           string
	TitleKey        i18n.Key
	Method          string
	GeneratedAt     time.Time
	DurationMs      float64
	Reloadable      bool
	ReloadCount     int
	StreamInterval  float64
	Summary         Summary
	Groups          []Group
	Friends         []FriendStatus
	Services        []ServiceStatus
	ServicesGroups  []Group
	ServicesSummary Summary
	Links           []LinkStatus
	LinksSummary    Summary
}

func (s *Snapshot) TitleText(lang i18n.Lang) string {
	if s.Title != "" {
		return s.Title
	}
	if s.TitleKey != "" {
		return i18n.T(lang, s.TitleKey)
	}
	return i18n.T(lang, i18n.KeyAppTitle)
}

// BestLatency 取所有通了的地址里最小的延迟。
func BestLatency(results []ProbeResult) *float64 {
	var best *float64
	for _, result := range results {
		if !result.Online() || result.LatencyMs == nil {
			continue
		}
		if best == nil || *result.LatencyMs < *best {
			value := *result.LatencyMs
			best = &value
		}
	}
	return best
}

func GroupFriends(source *Source, friends []FriendStatus) []Group {
	ids := make([]string, len(friends))
	online := make([]bool, len(friends))
	for index, friend := range friends {
		ids[index] = friend.Friend.Category
		online[index] = friend.Online
	}
	return groupBy(source, ids, online)
}

func GroupServices(source *Source, services []ServiceStatus) []Group {
	ids := make([]string, len(services))
	online := make([]bool, len(services))
	for index, service := range services {
		ids[index] = service.Service.Category
		online[index] = service.Online
	}
	return groupBy(source, ids, online)
}

func groupBy(source *Source, ids []string, online []bool) []Group {
	order := make([]string, 0, len(ids))
	members := make(map[string][]int, len(ids))
	for index, id := range ids {
		if _, seen := members[id]; !seen {
			order = append(order, id)
		}
		members[id] = append(members[id], index)
	}
	groups := make([]Group, 0, len(order))
	for _, id := range order {
		indexes := members[id]
		up := 0
		for _, index := range indexes {
			if online[index] {
				up++
			}
		}
		groups = append(groups, Group{Category: source.Category(id), Total: len(indexes), Online: up, Members: indexes})
	}
	sortGroups(groups)
	return groups
}

func sortGroups(groups []Group) {
	for i := 1; i < len(groups); i++ {
		for j := i; j > 0; j-- {
			left, right := groups[j-1], groups[j]
			if left.Category.Order < right.Category.Order ||
				(left.Category.Order == right.Category.Order && left.Category.ID <= right.Category.ID) {
				break
			}
			groups[j-1], groups[j] = right, left
		}
	}
}
