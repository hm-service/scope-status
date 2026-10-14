package store

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/hm-service/scope-status/core"
	"github.com/hm-service/scope-status/i18n"
)

// 盘上就两样东西，一视同仁，不分消息类型：
//
//   - message-yyyy-MM-dd-HH-mm-ss-ms(4)-随机6位.json —— 一条消息一个文件，本体按原样存；
//   - files/<sha256> —— 附件池，按内容寻址，同内容天然共享一份。
type MessageStore struct {
	root      string
	files     string
	maxUpload int64
}

type AttachmentRef struct {
	SHA256 string
	Name   string
}

type MessageStats struct {
	Live      int   `json:"live"`
	Deleted   int   `json:"deleted"`
	Files     int   `json:"files"`
	FileBytes int64 `json:"fileBytes"`
}

type messageFile struct {
	Text        string           `json:"text"`
	UpdatedAt   *int64           `json:"updatedAt,omitempty"`
	Attachments []attachmentFile `json:"attachments"`
	IsDeleted   bool             `json:"isDeleted"`
}

type attachmentFile struct {
	SHA256 string `json:"sha256"`
	Name   string `json:"name"`
	Size   int64  `json:"size"`
}

func NewMessageStore(root string, maxUpload int64) *MessageStore {
	return &MessageStore{root: root, files: filepath.Join(root, "files"), maxUpload: maxUpload}
}

func (s *MessageStore) Root() string          { return s.root }
func (s *MessageStore) FilesDir() string      { return s.files }
func (s *MessageStore) MaxUploadBytes() int64 { return s.maxUpload }

// Ensure 建目录并收紧权限：数据目录 0700，消息与附件 0600。
func (s *MessageStore) Ensure() error {
	for _, dir := range []string{s.root, s.files} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return err
		}
		if err := os.Chmod(dir, 0o700); err != nil {
			return err
		}
	}
	entries, err := os.ReadDir(s.root)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if err := os.Chmod(filepath.Join(s.root, entry.Name()), 0o600); err != nil {
			return err
		}
	}
	pool, err := os.ReadDir(s.files)
	if err != nil {
		return err
	}
	for _, entry := range pool {
		if err := os.Chmod(filepath.Join(s.files, entry.Name()), 0o600); err != nil {
			return err
		}
	}
	return nil
}

func (s *MessageStore) messagePath(id string) string {
	return filepath.Join(s.root, id+core.MessageSuffix)
}

func (s *MessageStore) poolPath(sha string) string {
	return filepath.Join(s.files, sha)
}

func (s *MessageStore) ids() ([]string, error) {
	entries, err := os.ReadDir(s.root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	ids := make([]string, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasPrefix(name, core.MessagePrefix) || !strings.HasSuffix(name, core.MessageSuffix) {
			continue
		}
		ids = append(ids, strings.TrimSuffix(name, core.MessageSuffix))
	}
	sort.Strings(ids)
	return ids, nil
}

func (s *MessageStore) poolFiles() ([]string, error) {
	entries, err := os.ReadDir(s.files)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !core.ValidSHA256(entry.Name()) {
			continue
		}
		names = append(names, entry.Name())
	}
	sort.Strings(names)
	return names, nil
}

// CheckPermissions 断言数据文件权限：消息 json 与附件池里的文件都必须是 0600。
func (s *MessageStore) CheckPermissions() error {
	for _, dir := range []string{s.root, s.files} {
		info, err := os.Stat(dir)
		if err != nil {
			return err
		}
		if !info.IsDir() || info.Mode().Perm() != 0o700 {
			return fmt.Errorf("%s mode is %v, want 0700", dir, info.Mode().Perm())
		}
	}
	ids, err := s.ids()
	if err != nil {
		return err
	}
	for _, id := range ids {
		if err := checkFileMode(s.messagePath(id)); err != nil {
			return err
		}
	}
	names, err := s.poolFiles()
	if err != nil {
		return err
	}
	for _, name := range names {
		if err := checkFileMode(s.poolPath(name)); err != nil {
			return err
		}
	}
	return nil
}

func checkFileMode(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if info.Mode().Perm() != 0o600 {
		return fmt.Errorf("%s mode is %v, want 0600", path, info.Mode().Perm())
	}
	return nil
}

// List 返回所有未删除的消息，按时间戳倒序（新的在前）。
func (s *MessageStore) List() ([]core.Message, error) {
	ids, err := s.ids()
	if err != nil {
		return nil, err
	}
	messages := make([]core.Message, 0, len(ids))
	for _, id := range ids {
		message, deleted, err := s.read(id)
		if err != nil {
			return nil, err
		}
		if deleted {
			continue
		}
		messages = append(messages, *message)
	}
	sort.Slice(messages, func(i, j int) bool {
		if !messages[i].CreatedAt.Equal(messages[j].CreatedAt) {
			return messages[i].CreatedAt.After(messages[j].CreatedAt)
		}
		return messages[i].ID > messages[j].ID
	})
	return messages, nil
}

// Get 看一条消息：在、没被删就给。
func (s *MessageStore) Get(id string) (*core.Message, error) {
	message, deleted, err := s.read(id)
	if err != nil {
		return nil, err
	}
	if deleted {
		return nil, core.NotFound(i18n.KeyMessageDeleted, id)
	}
	return message, nil
}

// ReadFile 下载附件前先确认：消息在、且引用过这个 sha。
func (s *MessageStore) ReadFile(id, sha string) (string, string, error) {
	if !core.ValidSHA256(sha) {
		return "", "", core.BadRequest(i18n.KeyMessageSHAFormat)
	}
	message, err := s.Get(id)
	if err != nil {
		return "", "", err
	}
	for _, attachment := range message.Attachments {
		if attachment.SHA256 != sha {
			continue
		}
		path := s.poolPath(sha)
		if info, err := os.Stat(path); err != nil || info.IsDir() {
			return "", "", core.NotFound(i18n.KeyMessageFileGone, sha)
		}
		return path, attachment.Name, nil
	}
	return "", "", core.NotFound(i18n.KeyMessageNotReferenced, id, sha)
}

func (s *MessageStore) Stats() (MessageStats, error) {
	ids, err := s.ids()
	if err != nil {
		return MessageStats{}, err
	}
	stats := MessageStats{}
	for _, id := range ids {
		_, deleted, err := s.read(id)
		if err != nil {
			return MessageStats{}, err
		}
		if deleted {
			stats.Deleted++
		} else {
			stats.Live++
		}
	}
	names, err := s.poolFiles()
	if err != nil {
		return MessageStats{}, err
	}
	stats.Files = len(names)
	for _, name := range names {
		info, err := os.Stat(s.poolPath(name))
		if err != nil {
			return MessageStats{}, err
		}
		stats.FileBytes += info.Size()
	}
	return stats, nil
}

// Create 新建一条消息，返回它的完整内容。
func (s *MessageStore) Create(text string, refs []AttachmentRef, now time.Time) (*core.Message, error) {
	attachments, err := s.resolveAttachments(refs)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(text) == "" && len(attachments) == 0 {
		return nil, core.BadRequest(i18n.KeyMessageEmpty)
	}
	id, err := core.NewMessageID(now, func(candidate string) bool {
		_, err := os.Stat(s.messagePath(candidate))
		return err == nil
	})
	if err != nil {
		return nil, err
	}
	data := messageFile{Text: text, Attachments: attachments, IsDeleted: false}
	if err := s.write(id, &data); err != nil {
		return nil, err
	}
	return s.Get(id)
}

// Update 改正文 / 附件。
func (s *MessageStore) Update(id string, text *string, refs []AttachmentRef, replaceAttachments bool, nowMs int64) (*core.Message, error) {
	data, err := s.locate(id)
	if err != nil {
		return nil, err
	}
	if text != nil {
		data.Text = *text
	}
	if replaceAttachments {
		attachments, err := s.resolveAttachments(refs)
		if err != nil {
			return nil, err
		}
		data.Attachments = attachments
	}
	if strings.TrimSpace(data.Text) == "" && len(data.Attachments) == 0 {
		return nil, core.BadRequest(i18n.KeyMessageEmpty)
	}
	data.UpdatedAt = &nowMs
	if err := s.write(id, data); err != nil {
		return nil, err
	}
	return s.Get(id)
}

// Delete 打删除标志位，然后扫描引用回收没人用的池内文件。
func (s *MessageStore) Delete(id string) ([]string, error) {
	data, err := s.locate(id)
	if err != nil {
		return nil, err
	}
	data.IsDeleted = true
	if err := s.write(id, data); err != nil {
		return nil, err
	}
	return s.Sweep()
}

// Sweep 扫描所有 message*.json 的引用，删掉没人引用的池内文件。
func (s *MessageStore) Sweep() ([]string, error) {
	ids, err := s.ids()
	if err != nil {
		return nil, err
	}
	referenced := make(map[string]bool)
	for _, id := range ids {
		data, err := s.decode(id)
		if err != nil {
			return nil, err
		}
		if data.IsDeleted {
			continue
		}
		for _, attachment := range data.Attachments {
			referenced[attachment.SHA256] = true
		}
	}
	names, err := s.poolFiles()
	if err != nil {
		return nil, err
	}
	removed := make([]string, 0)
	for _, name := range names {
		if referenced[name] {
			continue
		}
		if err := os.Remove(s.poolPath(name)); err != nil {
			return nil, err
		}
		removed = append(removed, name)
	}
	sort.Strings(removed)
	return removed, nil
}

// SaveFile 把请求体当裸文件流写进池子：边写边算 sha256，写完按内容改名。
func (s *MessageStore) SaveFile(name string, body io.Reader, declaredSize int64) (core.Attachment, error) {
	filename, err := core.CleanFileName(name)
	if err != nil {
		return core.Attachment{}, err
	}
	if declaredSize > s.maxUpload {
		return core.Attachment{}, core.TooLarge(i18n.KeyMessageDeclaredTooBig, declaredSize, s.maxUpload)
	}
	if err := os.MkdirAll(s.files, 0o700); err != nil {
		return core.Attachment{}, err
	}
	suffix, err := randomHex(8)
	if err != nil {
		return core.Attachment{}, err
	}
	temp := filepath.Join(s.files, ".upload-"+suffix+".part")
	handle, err := os.OpenFile(temp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return core.Attachment{}, err
	}
	digest := sha256.New()
	size, writeErr := copyLimited(handle, body, digest, s.maxUpload)
	closeErr := handle.Close()
	if writeErr == nil {
		writeErr = closeErr
	}
	if writeErr != nil {
		os.Remove(temp)
		return core.Attachment{}, writeErr
	}

	sha := hex.EncodeToString(digest.Sum(nil))
	target := s.poolPath(sha)
	if _, err := os.Stat(target); err == nil {
		if err := os.Remove(temp); err != nil {
			return core.Attachment{}, err
		}
	} else if err := os.Rename(temp, target); err != nil {
		os.Remove(temp)
		return core.Attachment{}, err
	}
	return core.Attachment{SHA256: sha, Name: filename, Size: size}, nil
}

func copyLimited(dst io.Writer, src io.Reader, digest io.Writer, limit int64) (int64, error) {
	buf := make([]byte, 64*1024)
	var total int64
	for {
		read, err := src.Read(buf)
		if read > 0 {
			total += int64(read)
			if total > limit {
				return total, core.TooLarge(i18n.KeyMessageFileTooBig, limit)
			}
			if _, err := digest.Write(buf[:read]); err != nil {
				return total, err
			}
			if _, err := dst.Write(buf[:read]); err != nil {
				return total, err
			}
		}
		if err == io.EOF {
			return total, nil
		}
		if err != nil {
			return total, err
		}
	}
}

func (s *MessageStore) resolveAttachments(refs []AttachmentRef) ([]attachmentFile, error) {
	attachments := make([]attachmentFile, 0, len(refs))
	for _, ref := range refs {
		if !core.ValidSHA256(ref.SHA256) {
			return nil, core.BadRequest(i18n.KeyMessageSHABad, ref.SHA256)
		}
		info, err := os.Stat(s.poolPath(ref.SHA256))
		if err != nil || info.IsDir() {
			return nil, core.BadRequest(i18n.KeyMessageFileNotInPool, ref.SHA256)
		}
		name := ref.Name
		if name == "" {
			name = ref.SHA256
		}
		cleaned, err := core.CleanFileName(name)
		if err != nil {
			return nil, err
		}
		attachments = append(attachments, attachmentFile{SHA256: ref.SHA256, Name: cleaned, Size: info.Size()})
	}
	return attachments, nil
}

// locate 定位一条消息：id 合法、文件在、没被删除。
func (s *MessageStore) locate(id string) (*messageFile, error) {
	_, deleted, err := s.read(id)
	if err != nil {
		return nil, err
	}
	if deleted {
		return nil, core.NotFound(i18n.KeyMessageDeleted, id)
	}
	data, err := s.decode(id)
	if err != nil {
		return nil, err
	}
	return data, nil
}

func (s *MessageStore) read(id string) (*core.Message, bool, error) {
	if !core.ValidMessageID(id) {
		return nil, false, core.BadRequest(i18n.KeyMessageBadID)
	}
	data, err := s.decode(id)
	if err != nil {
		return nil, false, err
	}
	created, err := core.ParseMessageID(id)
	if err != nil {
		return nil, false, err
	}
	attachments := make([]core.Attachment, 0, len(data.Attachments))
	for _, attachment := range data.Attachments {
		attachments = append(attachments, core.Attachment(attachment))
	}
	return &core.Message{
		ID:          id,
		Text:        data.Text,
		UpdatedAt:   data.UpdatedAt,
		IsDeleted:   data.IsDeleted,
		Attachments: attachments,
		CreatedAt:   created,
	}, data.IsDeleted, nil
}

// decode 读一条消息的 json 并逐字段校验；不合规就是 500，不修复、不跳过。
func (s *MessageStore) decode(id string) (*messageFile, error) {
	name := id + core.MessageSuffix
	body, err := os.ReadFile(s.messagePath(id))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, core.NotFound(i18n.KeyMessageNotFound, id)
		}
		return nil, core.ServerError(i18n.KeyMessageBrokenFile, name, err.Error())
	}
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return nil, core.ServerError(i18n.KeyMessageFileNotObject, name)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(trimmed, &raw); err != nil {
		return nil, core.ServerError(i18n.KeyMessageBrokenFile, name, err.Error())
	}

	var text string
	if err := decodeField(raw, "text", &text); err != nil {
		return nil, core.ServerError(i18n.KeyMessageFileFields, name)
	}
	var deleted bool
	if err := decodeField(raw, "isDeleted", &deleted); err != nil {
		return nil, core.ServerError(i18n.KeyMessageFileFields, name)
	}
	var updatedAt *int64
	if field, ok := raw["updatedAt"]; ok && string(field) != "null" {
		var value int64
		if err := json.Unmarshal(field, &value); err != nil {
			return nil, core.ServerError(i18n.KeyMessageFileUpdatedAt, name)
		}
		updatedAt = &value
	}

	field, ok := raw["attachments"]
	if !ok || string(bytes.TrimSpace(field)) == "null" {
		return nil, core.ServerError(i18n.KeyMessageFileAtts, name)
	}
	var entries []json.RawMessage
	if err := json.Unmarshal(field, &entries); err != nil {
		return nil, core.ServerError(i18n.KeyMessageFileAtts, name)
	}
	attachments := make([]attachmentFile, 0, len(entries))
	for _, entry := range entries {
		var item attachmentFile
		if err := json.Unmarshal(entry, &item); err != nil {
			return nil, core.ServerError(i18n.KeyMessageFileAttEntry, name, string(entry))
		}
		if !core.ValidSHA256(item.SHA256) || item.Name == "" {
			return nil, core.ServerError(i18n.KeyMessageFileAttEntry, name, string(entry))
		}
		attachments = append(attachments, item)
	}
	return &messageFile{
		Text:        text,
		UpdatedAt:   updatedAt,
		Attachments: attachments,
		IsDeleted:   deleted,
	}, nil
}

// write 先写临时文件再 rename，避免读到写了一半的 json。
func (s *MessageStore) write(id string, data *messageFile) error {
	if err := os.MkdirAll(s.root, 0o700); err != nil {
		return err
	}
	body, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	temp := filepath.Join(s.root, "."+id+core.MessageSuffix+".tmp")
	if err := os.WriteFile(temp, append(body, '\n'), 0o600); err != nil {
		return err
	}
	if err := os.Rename(temp, s.messagePath(id)); err != nil {
		os.Remove(temp)
		return err
	}
	return nil
}

func decodeField(raw map[string]json.RawMessage, name string, target any) error {
	field, ok := raw[name]
	if !ok {
		return fmt.Errorf("missing %s", name)
	}
	return json.Unmarshal(field, target)
}

func randomHex(size int) (string, error) {
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}
