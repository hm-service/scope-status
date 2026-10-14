package core

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"strings"
	"testing"
	"time"
)

const testSessionKey = "session-signing-key-0123456789"

func newSigner(ttl time.Duration) *SessionSigner {
	return NewSessionSigner([]byte(testSessionKey), ttl)
}

func TestSessionIssueAndValid(t *testing.T) {
	signer := newSigner(time.Hour)
	token, err := signer.Issue()
	if err != nil {
		t.Fatalf("签发失败：%v", err)
	}
	payload, signature, found := strings.Cut(token, ".")
	if !found || payload == "" || signature == "" {
		t.Fatalf("token 形状不对：%q", token)
	}
	if strings.ContainsAny(token, "&=/+") {
		t.Fatalf("token 该是纯 base64url + 点：%q", token)
	}
	plain, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		t.Fatalf("载荷不是 base64url：%v", err)
	}
	expires, nonce, found := strings.Cut(string(plain), "|")
	if !found || expires == "" || nonce == "" {
		t.Fatalf("载荷形状不对：%q", plain)
	}
	if !signer.Valid(token) {
		t.Fatal("刚签发的 token 应该认")
	}

	for name, candidate := range map[string]string{
		"空":           "",
		"没有点":         payload,
		"多一个点":        token + ".x",
		"乱写":          "nothing-here",
		"载荷被改":        flipLast(payload) + "." + signature,
		"签名被改":        payload + "." + flipLast(signature),
		"签名截断":        payload + "." + signature[:len(signature)-1],
		"签名不是 base64": payload + ".!!!!",
	} {
		if signer.Valid(candidate) {
			t.Errorf("%s 不该认：%q", name, candidate)
		}
	}

	// 每次签发都不一样（nonce 随机）
	seen := map[string]bool{}
	for index := 0; index < 8; index++ {
		issued, err := signer.Issue()
		if err != nil {
			t.Fatalf("签发失败：%v", err)
		}
		if seen[issued] {
			t.Fatalf("签发了重复的 token：%q", issued)
		}
		seen[issued] = true
	}
}

// 换一把密钥，老 token 全部不认：多副本共用同一把密钥才互通。
func TestSessionDiesWithKey(t *testing.T) {
	token, err := newSigner(time.Hour).Issue()
	if err != nil {
		t.Fatalf("签发失败：%v", err)
	}
	if !newSigner(time.Hour).Valid(token) {
		t.Fatal("同一把密钥应该认")
	}
	other := NewSessionSigner([]byte("another-signing-key-0123456789"), time.Hour)
	if other.Valid(token) {
		t.Fatal("换了密钥不该认")
	}
}

// TTL 是唯一的下线方式：负 TTL 签出来就已经过期，不用等。
func TestSessionExpiry(t *testing.T) {
	expired, err := newSigner(-time.Second).Issue()
	if err != nil {
		t.Fatalf("签发失败：%v", err)
	}
	if newSigner(time.Hour).Valid(expired) {
		t.Fatal("过期 token 不该认")
	}
	fresh, err := newSigner(time.Hour).Issue()
	if err != nil {
		t.Fatalf("签发失败：%v", err)
	}
	if !newSigner(time.Hour).Valid(fresh) {
		t.Fatal("没过期的 token 应该认")
	}
}

// 签名覆盖的是 "v1|载荷"：少了版本域算出来的 MAC 不该被接受。
func TestSessionVersionDomain(t *testing.T) {
	signer := newSigner(time.Hour)
	token, err := signer.Issue()
	if err != nil {
		t.Fatalf("签发失败：%v", err)
	}
	payload, _, _ := strings.Cut(token, ".")
	mac := hmac.New(sha256.New, []byte(testSessionKey))
	mac.Write([]byte(payload))
	withoutVersion := payload + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if signer.Valid(withoutVersion) {
		t.Fatal("不带版本域的签名不该认")
	}
	if !signer.Valid(token) {
		t.Fatal("正规签发的 token 应该认")
	}
}

func flipLast(value string) string {
	last := value[len(value)-1]
	if last == 'A' {
		return value[:len(value)-1] + "B"
	}
	return value[:len(value)-1] + "A"
}
