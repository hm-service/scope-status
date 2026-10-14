package core

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"strconv"
	"strings"
	"time"
)

// sessionVersion 是签名串的版本域：换格式就换它，老 token 自然全部失效。
const sessionVersion = "v1"

// sessionNonceLen 是每条 token 的随机部分：16 字节够唯一，token 也不会太长。
const sessionNonceLen = 16

// SessionSigner 签发与校验无状态的会话 token：
//
//	<base64url(exp|nonce)>.<base64url(HMAC-SHA256(key, "v1|"+前面那串))>
//
// 服务端不存任何会话：验签就是拿收到的原文重算一遍 MAC。密钥从 SESSION_SIGNING_KEY 来，
// 部署级共享，所以多个副本各验各的也能互通。
type SessionSigner struct {
	key []byte
	ttl time.Duration
}

func NewSessionSigner(key []byte, ttl time.Duration) *SessionSigner {
	return &SessionSigner{key: key, ttl: ttl}
}

// Issue 签一条会话：过期时间是「现在 + TTL」，秒级时间戳。
func (s *SessionSigner) Issue() (string, error) {
	nonce := make([]byte, sessionNonceLen)
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	plain := strconv.FormatInt(time.Now().Add(s.ttl).Unix(), 10) + "|" + base64.RawURLEncoding.EncodeToString(nonce)
	payload := base64.RawURLEncoding.EncodeToString([]byte(plain))
	return payload + "." + base64.RawURLEncoding.EncodeToString(s.sign(payload)), nil
}

// Valid 先验签、再信字段：MAC 不对、形状不对、过期了都只回 false，不解释原因。
func (s *SessionSigner) Valid(token string) bool {
	payload, encoded, found := strings.Cut(token, ".")
	if !found || payload == "" || encoded == "" || strings.Contains(encoded, ".") {
		return false
	}
	signature, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil || subtle.ConstantTimeCompare(signature, s.sign(payload)) != 1 {
		return false
	}
	plain, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		return false
	}
	rawExpires, rawNonce, found := strings.Cut(string(plain), "|")
	if !found {
		return false
	}
	expires, err := strconv.ParseInt(rawExpires, 10, 64)
	if err != nil {
		return false
	}
	nonce, err := base64.RawURLEncoding.DecodeString(rawNonce)
	if err != nil || len(nonce) != sessionNonceLen {
		return false
	}
	return time.Now().Before(time.Unix(expires, 0))
}

// sign 签的是 token 里那段原文，不是解码后的字段：这样不存在「解析再拼回去」的歧义。
func (s *SessionSigner) sign(payload string) []byte {
	mac := hmac.New(sha256.New, s.key)
	mac.Write([]byte(sessionVersion + "|" + payload))
	return mac.Sum(nil)
}
