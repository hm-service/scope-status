package core

import (
	"strings"
	"testing"
)

func newSessions(t *testing.T) *Sessions {
	t.Helper()
	sessions, err := NewSessions()
	if err != nil {
		t.Fatalf("建会话表失败：%v", err)
	}
	return sessions
}

func TestSessionIssueAndValid(t *testing.T) {
	sessions := newSessions(t)
	token, err := sessions.Issue()
	if err != nil {
		t.Fatalf("签发失败：%v", err)
	}
	if token == "" || strings.ContainsAny(token, "+/=") {
		t.Fatalf("token 形状不对：%q", token)
	}
	if !sessions.Valid(token) {
		t.Fatal("刚签发的 token 应该认")
	}
	for name, candidate := range map[string]string{
		"空":          "",
		"乱写":         "nope",
		"长长的假 token": strings.Repeat("a", 43),
		"改一个字符":      token[:len(token)-1] + "x",
		"多一个字符":      token + "x",
	} {
		if sessions.Valid(candidate) {
			t.Errorf("%s 不该认：%q", name, candidate)
		}
	}

	// 每条会话的 token 都不一样
	seen := map[string]bool{}
	for index := 0; index < 8; index++ {
		issued := mustIssue(t, sessions)
		if seen[issued] {
			t.Fatalf("签发了重复的 token：%q", issued)
		}
		seen[issued] = true
	}
}

// 会话只活在内存里：换一个 Sessions（等同于重启）就全部失效。
func TestSessionsDieWithProcess(t *testing.T) {
	first := newSessions(t)
	token := mustIssue(t, first)
	second := newSessions(t)
	if second.Valid(token) {
		t.Fatal("重启后的会话表不该认旧 token")
	}
}

// 会话数有上限：签满以后丢最旧的，新的照常能用。
func TestSessionLimitEvictsOldest(t *testing.T) {
	sessions := newSessions(t)
	tokens := make([]string, 0, sessionLimit+1)
	for index := 0; index <= sessionLimit; index++ {
		tokens = append(tokens, mustIssue(t, sessions))
	}
	if sessions.Valid(tokens[0]) {
		t.Fatal("最旧的一条应该被挤掉")
	}
	for _, token := range tokens[1:] {
		if !sessions.Valid(token) {
			t.Fatalf("还在上限内的会话应该认：%q", token)
		}
	}
}

func mustIssue(t *testing.T, sessions *Sessions) string {
	t.Helper()
	token, err := sessions.Issue()
	if err != nil {
		t.Fatalf("签发失败：%v", err)
	}
	return token
}
