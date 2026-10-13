package core

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

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

// 口令哈希是 CPU 密集的，一次最多算 4 个。
var hashSlots = make(chan struct{}, 4)

// HashKey 生成 PHC 形式的 Argon2id 口令串：$argon2id$v=19$m=…,t=…,p=…$salt$hash。
func HashKey(key string) (string, error) {
	salt := make([]byte, argonSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	digest := argonKey(key, salt, argonMemory, argonTime, argonThreads, argonKeyLen)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, argonMemory, argonTime, argonThreads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(digest),
	), nil
}

// VerifyKey 校验口令，成功与否都至少花掉 floor（认证步骤的时间下限）。
func VerifyKey(key, encoded string, floor time.Duration) (bool, error) {
	started := time.Now()
	salt, digest, memory, iterations, threads, err := parseKeyHash(encoded)
	if err != nil {
		return false, err
	}
	candidate := argonKey(key, salt, memory, iterations, threads, uint32(len(digest)))
	matched := subtle.ConstantTimeCompare(candidate, digest) == 1
	if wait := floor - time.Since(started); wait > 0 {
		time.Sleep(wait)
	}
	return matched, nil
}

// ValidKeyHash 只判断这个串能不能解析，用在读盘校验上。
func ValidKeyHash(encoded string) bool {
	_, _, _, _, _, err := parseKeyHash(encoded)
	return err == nil
}

func argonKey(key string, salt []byte, memory, iterations uint32, threads uint8, length uint32) []byte {
	hashSlots <- struct{}{}
	defer func() { <-hashSlots }()
	return argon2.IDKey([]byte(key), salt, iterations, memory, threads, length)
}

func parseKeyHash(encoded string) (salt []byte, digest []byte, memory uint32, iterations uint32, threads uint8, err error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" {
		return nil, nil, 0, 0, 0, fmt.Errorf("key hash is not an argon2id PHC string")
	}
	var version int
	if _, scanErr := fmt.Sscanf(parts[2], "v=%d", &version); scanErr != nil || version != argon2.Version {
		return nil, nil, 0, 0, 0, fmt.Errorf("unsupported argon2 version %q", parts[2])
	}
	if _, scanErr := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &iterations, &threads); scanErr != nil {
		return nil, nil, 0, 0, 0, fmt.Errorf("bad argon2 parameters %q", parts[3])
	}
	// 参数上限挡住"改大参数把内存/CPU 放大"的坏文件
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
