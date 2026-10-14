package api_test

import (
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/hm-service/scope-status/internal/testenv"
)

// GET /api/messages?since=<Unix 毫秒>：按「最近一次活动」筛，非法值直接 400。
func TestMessageListSince(t *testing.T) {
	env := testenv.New(t, testenv.DefaultSource)
	store := env.App.Messages.Store()
	base := time.Date(2026, 10, 14, 12, 0, 0, 0, time.Local)

	older, err := store.Create("旧的", nil, base)
	if err != nil {
		t.Fatalf("建消息失败：%v", err)
	}
	newer, err := store.Create("新的", nil, base.Add(time.Minute))
	if err != nil {
		t.Fatalf("建消息失败：%v", err)
	}
	// 编辑旧的那条之后，它才是「最近一次活动」最新的
	text := "旧的（刚改）"
	if _, err := store.Update(older.ID, &text, nil, false, base.Add(2*time.Minute).UnixMilli()); err != nil {
		t.Fatalf("改消息失败：%v", err)
	}

	cases := []struct {
		name      string
		query     string
		wantTotal int
		wantFirst string
	}{
		{"不带参数给全部", "", 2, newer.ID},
		{"只要改过的那条", "?since=" + strconv.FormatInt(base.Add(90*time.Second).UnixMilli(), 10), 1, older.ID},
		{"比谁都新就空列表", "?since=" + strconv.FormatInt(base.Add(3*time.Minute).UnixMilli(), 10), 0, ""},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			got := call(t, env, http.MethodGet, "/api/messages"+testCase.query, "")
			if got.status != http.StatusOK {
				t.Fatalf("状态 = %d（%s）", got.status, got.body)
			}
			payload := decode[struct {
				Messages []struct {
					ID string `json:"id"`
				} `json:"messages"`
				Summary struct {
					Total    int   `json:"total"`
					LatestAt int64 `json:"latestAt"`
				} `json:"summary"`
			}](t, got.body)
			if payload.Summary.Total != testCase.wantTotal || len(payload.Messages) != testCase.wantTotal {
				t.Fatalf("条数 = %d/%d，想要 %d", payload.Summary.Total, len(payload.Messages), testCase.wantTotal)
			}
			if testCase.wantTotal > 0 && payload.Messages[0].ID != testCase.wantFirst {
				t.Fatalf("第一条 = %s，想要 %s", payload.Messages[0].ID, testCase.wantFirst)
			}
			if want := base.Add(2 * time.Minute).UnixMilli(); payload.Summary.LatestAt != want {
				t.Fatalf("latestAt = %d，想要 %d", payload.Summary.LatestAt, want)
			}
		})
	}

	for _, bad := range []string{"?since=abc", "?since=-1", "?since=1.5"} {
		got := call(t, env, http.MethodGet, "/api/messages"+bad, "")
		if got.status != http.StatusBadRequest {
			t.Fatalf("%s 状态 = %d，想要 400", bad, got.status)
		}
		if !strings.Contains(string(got.body), "since") {
			t.Fatalf("%s 的报错里没提 since：%s", bad, got.body)
		}
	}
}
