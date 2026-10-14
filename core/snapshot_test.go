package core

import (
	"testing"

	"github.com/hm-service/scope-status/i18n"
)

func peerStatuses(categories []string, online []bool) []PeerStatus {
	statuses := make([]PeerStatus, 0, len(categories))
	for index, category := range categories {
		statuses = append(statuses, PeerStatus{
			Peer:   Peer{Name: category, Category: category, Index: index},
			Online: online[index],
		})
	}
	return statuses
}

func TestGroupPeersOrderAndCounts(t *testing.T) {
	source := &Source{Categories: []Category{
		{ID: "server", Label: "服务器", Order: 2},
		{ID: "peer", Label: "对端", Order: 1},
	}}
	groups := GroupPeers(source, peerStatuses(
		[]string{"peer", "server", "peer", "peer"},
		[]bool{true, false, false, true},
	))
	if len(groups) != 2 {
		t.Fatalf("分组数 = %d", len(groups))
	}
	if groups[0].Category.ID != "peer" || groups[0].Total != 3 || groups[0].Online != 2 {
		t.Fatalf("第一组不对：%+v", groups[0])
	}
	if len(groups[0].Members) != 3 || groups[0].Members[0] != 0 || groups[0].Members[2] != 3 {
		t.Fatalf("成员下标不对：%+v", groups[0].Members)
	}
	if groups[1].Category.ID != "server" || groups[1].Online != 0 {
		t.Fatalf("第二组不对：%+v", groups[1])
	}
}

func TestGroupPeersUndefinedCategory(t *testing.T) {
	source := &Source{}
	groups := GroupPeers(source, peerStatuses([]string{"nas"}, []bool{true}))
	if len(groups) != 1 {
		t.Fatalf("分组数 = %d", len(groups))
	}
	if groups[0].Category.Render(i18n.Zh) != "NAS / 存储" || groups[0].Category.Order != 50 {
		t.Fatalf("没声明的分类应该落到内置名字：%+v", groups[0].Category)
	}
}

func TestBestLatency(t *testing.T) {
	one, two, three := 20.0, 5.5, 9.0
	results := []ProbeResult{
		{Status: StatusDown},
		{Status: StatusUp, LatencyMs: &one},
		{Status: StatusUp, LatencyMs: &two},
		{Status: StatusUp, LatencyMs: &three},
	}
	best := BestLatency(results)
	if best == nil || *best != two {
		t.Fatalf("最低延迟 = %v，想要 %v", best, two)
	}
	if BestLatency([]ProbeResult{{Status: StatusDown}}) != nil {
		t.Fatal("全不通时应该没有延迟")
	}
}

func TestSummary(t *testing.T) {
	summary := NewSummary(5, 2)
	if summary.Total != 5 || summary.Online != 2 || summary.Offline != 3 {
		t.Fatalf("概览不对：%+v", summary)
	}
}

func TestSnapshotTitle(t *testing.T) {
	withTitle := &Snapshot{Title: "我的看板"}
	if got := withTitle.TitleText(i18n.En); got != "我的看板" {
		t.Fatalf("配置写了 title 就该用它：%q", got)
	}
	fallback := &Snapshot{TitleKey: i18n.KeyAppTitle}
	if got := fallback.TitleText(i18n.En); got != "Scope Status" {
		t.Fatalf("默认标题的英文 = %q", got)
	}
	empty := &Snapshot{}
	if got := empty.TitleText(i18n.Zh); got != "Scope Status" {
		t.Fatalf("没键时的默认标题 = %q", got)
	}
}
