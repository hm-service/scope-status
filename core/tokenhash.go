package core

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// Argon2id 参数取默认档 m=19 MiB, t=2, p=1。
const (
	argonMemory  = 19456
	argonTime    = 2
	argonThreads = 1
	argonKeyLen  = 32
	argonSaltLen = 16
)

// 一次哈希要 19 MiB，同时最多算 4 个：并发请求不能把内存顶穿。
var tokenHashSlots = make(chan struct{}, 4)

// HashToken 把 token 变成 PHC 形式的 Argon2id 串：$argon2id$v=19$m=…,t=…,p=…$salt$hash。
// 配置里那份 token 读完立刻走这里，进程里只留摘要。
func HashToken(token string) (string, error) {
	salt := make([]byte, argonSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	digest := argonToken(token, salt, argonMemory, argonTime, argonThreads, argonKeyLen)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, argonMemory, argonTime, argonThreads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(digest),
	), nil
}

// VerifyToken 把候选 token 按摘要里记着的参数与盐再哈希一遍，恒定时间比较。
func VerifyToken(token, encoded string) (bool, error) {
	salt, digest, memory, iterations, threads, err := parseTokenHash(encoded)
	if err != nil {
		return false, err
	}
	candidate := argonToken(token, salt, memory, iterations, threads, uint32(len(digest)))
	return subtle.ConstantTimeCompare(candidate, digest) == 1, nil
}

// ValidTokenHash 只判断这个串能不能解析。
func ValidTokenHash(encoded string) bool {
	_, _, _, _, _, err := parseTokenHash(encoded)
	return err == nil
}

func argonToken(token string, salt []byte, memory, iterations uint32, threads uint8, length uint32) []byte {
	tokenHashSlots <- struct{}{}
	defer func() { <-tokenHashSlots }()
	return argon2.IDKey([]byte(token), salt, iterations, memory, threads, length)
}

func parseTokenHash(encoded string) (salt []byte, digest []byte, memory uint32, iterations uint32, threads uint8, err error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" {
		return nil, nil, 0, 0, 0, fmt.Errorf("token hash is not an argon2id PHC string")
	}
	var version int
	if _, scanErr := fmt.Sscanf(parts[2], "v=%d", &version); scanErr != nil || version != argon2.Version {
		return nil, nil, 0, 0, 0, fmt.Errorf("unsupported argon2 version %q", parts[2])
	}
	if _, scanErr := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &iterations, &threads); scanErr != nil {
		return nil, nil, 0, 0, 0, fmt.Errorf("bad argon2 parameters %q", parts[3])
	}
	// 参数上限挡住"把参数改大"的坏串：内存与 CPU 都不能被放大
	if memory < 8*1024 || memory > 1024*1024 || iterations < 1 || iterations > 10 || threads < 1 || threads > 8 {
		return nil, nil, 0, 0, 0, fmt.Errorf("argon2 parameters out of range: m=%d,t=%d,p=%d", memory, iterations, threads)
	}
	salt, decodeErr := base64.RawStdEncoding.DecodeString(parts[4])
	if decodeErr != nil || len(salt) < 8 {
		return nil, nil, 0, 0, 0, fmt.Errorf("bad argon2 salt")
	}
	digest, decodeErr = base64.RawStdEncoding.DecodeString(parts[5])
	if decodeErr != nil || len(digest) < 16 {
		return nil, nil, 0, 0, 0, fmt.Errorf("bad argon2 digest")
	}
	return salt, digest, memory, iterations, threads, nil
}
