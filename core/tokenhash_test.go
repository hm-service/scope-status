package core

import (
	"strings"
	"sync"
	"testing"
)

func TestHashTokenRoundTrip(t *testing.T) {
	hashed, err := HashToken("服务端的令牌")
	if err != nil {
		t.Fatalf("哈希失败：%v", err)
	}
	if !strings.HasPrefix(hashed, "$argon2id$v=19$m=19456,t=2,p=1$") {
		t.Fatalf("PHC 串形状不对：%q", hashed)
	}
	if strings.Contains(hashed, "服务端的令牌") {
		t.Fatal("摘要里不该出现明文")
	}
	if !ValidTokenHash(hashed) {
		t.Fatal("自己生成的摘要应该能解析")
	}
	matched, err := VerifyToken("服务端的令牌", hashed)
	if err != nil || !matched {
		t.Fatalf("token 对了没验过：%v / %v", matched, err)
	}
	matched, err = VerifyToken("别的令牌", hashed)
	if err != nil || matched {
		t.Fatalf("token 错了却验过：%v / %v", matched, err)
	}
	matched, err = VerifyToken("服务端的令", hashed)
	if err != nil || matched {
		t.Fatalf("少一个字符也该拒绝：%v / %v", matched, err)
	}
	// 盐是随机的：同一个 token 两次哈希必须不同
	again, err := HashToken("服务端的令牌")
	if err != nil || again == hashed {
		t.Fatalf("盐没随机：%v", err)
	}
}

func TestVerifyTokenRejectsBrokenHash(t *testing.T) {
	cases := map[string]string{
		"不是 PHC": "s3cret",
		"算法不对":   "$argon2i$v=19$m=19456,t=2,p=1$c2FsdHNhbHQ$aGFzaGhhc2hoYXNoaGFzaA",
		"版本不对":   "$argon2id$v=13$m=19456,t=2,p=1$c2FsdHNhbHQ$aGFzaGhhc2hoYXNoaGFzaA",
		"参数被放大":  "$argon2id$v=19$m=999999999,t=2,p=1$c2FsdHNhbHQ$aGFzaGhhc2hoYXNoaGFzaA",
		"迭代次数太多": "$argon2id$v=19$m=19456,t=99,p=1$c2FsdHNhbHQ$aGFzaGhhc2hoYXNoaGFzaA",
		"盐太短":    "$argon2id$v=19$m=19456,t=2,p=1$c2FsdA$aGFzaGhhc2hoYXNoaGFzaA",
		"摘要太短":   "$argon2id$v=19$m=19456,t=2,p=1$c2FsdHNhbHQ$aGFzaA",
		"字段数不对":  "$argon2id$v=19$m=19456,t=2,p=1$c2FsdHNhbHQ",
		"空串":     "",
	}
	for name, encoded := range cases {
		t.Run(name, func(t *testing.T) {
			if ValidTokenHash(encoded) {
				t.Fatalf("%q 不该被当成合法摘要", encoded)
			}
			matched, err := VerifyToken("s3cret", encoded)
			if err == nil {
				t.Fatal("坏摘要应该报错，不能当成 token 不对")
			}
			if matched {
				t.Fatal("坏摘要不能算验过")
			}
		})
	}
}

// 并发校验：4 个哈希槽位不能被抢占成死锁。
func TestVerifyTokenConcurrent(t *testing.T) {
	hashed, err := HashToken("并发用的令牌")
	if err != nil {
		t.Fatalf("哈希失败：%v", err)
	}
	var wait sync.WaitGroup
	for index := 0; index < 8; index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			matched, err := VerifyToken("并发用的令牌", hashed)
			if err != nil || !matched {
				t.Errorf("并发校验失败：%v / %v", matched, err)
			}
		}()
	}
	wait.Wait()
}
