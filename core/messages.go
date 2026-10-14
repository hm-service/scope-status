package core

import (
	"crypto/rand"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/hm-service/scope-status/i18n"
)

// MaxUploadBytes 是单个附件的上限：1 GiB。消息条数、正文长度、附件个数都不设限。
const MaxUploadBytes int64 = 1024 * 1024 * 1024

const (
	MessagePrefix = "message-"
	MessageSuffix = ".json"
)

var (
	messageIDRe = regexp.MustCompile(`^message-(\d{4})-(\d{2})-(\d{2})-(\d{2})-(\d{2})-(\d{2})-(\d{4})-([a-z0-9]{6})$`)
	sha256Re    = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

const suffixAlphabet = "abcdefghijklmnopqrstuvwxyz0123456789"

func ValidMessageID(id string) bool { return messageIDRe.MatchString(id) }

func ValidSHA256(value string) bool { return sha256Re.MatchString(value) }

// ParseMessageID 从 id 里取出创建时间：文件名是创建时间的唯一真相来源。
func ParseMessageID(id string) (time.Time, error) {
	match := messageIDRe.FindStringSubmatch(id)
	if match == nil {
		return time.Time{}, BadRequest(i18n.KeyMessageBadID)
	}
	var year, month, day, hour, minute, second, millis int
	fmt.Sscanf(strings.Join(match[1:8], " "), "%d %d %d %d %d %d %d", &year, &month, &day, &hour, &minute, &second, &millis)
	created := time.Date(year, time.Month(month), day, hour, minute, second, millis*int(time.Millisecond), time.Local)
	if created.Year() != year || int(created.Month()) != month || created.Day() != day ||
		created.Hour() != hour || created.Minute() != minute || created.Second() != second {
		return time.Time{}, BadRequest(i18n.KeyMessageBadID)
	}
	return created, nil
}

// NewMessageID 生成 message-yyyy-MM-dd-HH-mm-ss-ms(4)-随机6位；撞了就换随机后缀。
func NewMessageID(now time.Time, taken func(string) bool) (string, error) {
	stamp := now.Format("2006-01-02-15-04-05")
	prefix := fmt.Sprintf("%s%s-%04d-", MessagePrefix, stamp, now.Nanosecond()/int(time.Millisecond))
	for attempt := 0; attempt < 32; attempt++ {
		suffix, err := randomSuffix(6)
		if err != nil {
			return "", err
		}
		id := prefix + suffix
		if !taken(id) {
			return id, nil
		}
	}
	return "", ServerError(i18n.KeyHTTPInternal, "message id collision")
}

func randomSuffix(length int) (string, error) {
	buf := make([]byte, length)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	out := make([]byte, length)
	for index, value := range buf {
		out[index] = suffixAlphabet[int(value)%len(suffixAlphabet)]
	}
	return string(out), nil
}

type Attachment struct {
	SHA256 string
	Name   string
	Size   int64
}

// Message 是一条消息的领域形状：只有正文与附件，没有可见性之说。
type Message struct {
	ID          string
	Text        string
	UpdatedAt   *int64
	IsDeleted   bool
	Attachments []Attachment
	CreatedAt   time.Time
}

type AttachmentView struct {
	SHA256 string `json:"sha256"`
	Name   string `json:"name"`
	Size   int64  `json:"size"`
	URL    string `json:"url"`
}

// MessageView 是消息对外的形状：正文、附件、编辑时间都在位。
type MessageView struct {
	ID          string           `json:"id"`
	CreatedAt   int64            `json:"createdAt"`
	Text        string           `json:"text"`
	UpdatedAt   *int64           `json:"updatedAt"`
	Attachments []AttachmentView `json:"attachments"`
}

type MessageSummary struct {
	Total int `json:"total"`
}

type MessagesPayload struct {
	Messages []MessageView  `json:"messages"`
	Summary  MessageSummary `json:"summary"`
}

func (m *Message) View() MessageView {
	attachments := make([]AttachmentView, 0, len(m.Attachments))
	for _, attachment := range m.Attachments {
		attachments = append(attachments, AttachmentView{
			SHA256: attachment.SHA256,
			Name:   attachment.Name,
			Size:   attachment.Size,
			URL:    fmt.Sprintf("/api/messages/files/%s?id=%s", attachment.SHA256, m.ID),
		})
	}
	return MessageView{
		ID:          m.ID,
		CreatedAt:   m.CreatedAt.UnixMilli(),
		Text:        m.Text,
		UpdatedAt:   m.UpdatedAt,
		Attachments: attachments,
	}
}

// CleanFileName 只取最后一段并去掉控制字符，附件名不当路径用。
func CleanFileName(name string) (string, error) {
	if strings.TrimSpace(name) == "" {
		return "", BadRequest(i18n.KeyMessageNameMissing)
	}
	raw := strings.ReplaceAll(name, "\\", "/")
	// 只取最后一段："/" 与 "a/" 都要落到 Python 里 PurePosixPath.name 的语义上。
	if index := strings.LastIndex(raw, "/"); index >= 0 {
		raw = raw[index+1:]
	}
	cleaned := raw
	var builder strings.Builder
	for _, r := range cleaned {
		if r >= ' ' && r != 0x7f {
			builder.WriteRune(r)
		}
	}
	cleaned = strings.TrimSpace(builder.String())
	if cleaned == "" || cleaned == "." || cleaned == ".." {
		return "", BadRequest(i18n.KeyMessageNameInvalid)
	}
	return cleaned, nil
}
