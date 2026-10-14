package core

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"sync"
)

// sessionLimit 是最多同时存活的会话数：每个浏览器登录一次签一条，超了丢最旧的。
const sessionLimit = 32

// Sessions 是内存里的会话表：启动时随机生成一把 HMAC 密钥，签出去的 token 只留摘要，
// 密钥与摘要都随进程消失，所以重启即全部失效，盘上没有任何会话痕迹。
type Sessions struct {
	key     []byte
	mu      sync.Mutex
	digests [][sha256.Size]byte
}

func NewSessions() (*Sessions, error) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	return &Sessions{key: key, digests: make([][sha256.Size]byte, 0, sessionLimit)}, nil
}

// Issue 签一条会话：token = HMAC(启动时的随机密钥, 随机 nonce)，内存里只留它的 SHA-256。
func (s *Sessions) Issue() (string, error) {
	nonce := make([]byte, 32)
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	mac := hmac.New(sha256.New, s.key)
	mac.Write(nonce)
	token := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	digest := sha256.Sum256([]byte(token))

	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.digests) == sessionLimit {
		copy(s.digests, s.digests[1:])
		s.digests = s.digests[:sessionLimit-1]
	}
	s.digests = append(s.digests, digest)
	return token, nil
}

// Valid 只认签发过的会话：比的是摘要，恒定时间，且不因为提前命中就少比几条。
func (s *Sessions) Valid(token string) bool {
	if token == "" {
		return false
	}
	digest := sha256.Sum256([]byte(token))
	s.mu.Lock()
	defer s.mu.Unlock()
	matched := 0
	for _, known := range s.digests {
		matched |= subtle.ConstantTimeCompare(known[:], digest[:])
	}
	return matched == 1
}
