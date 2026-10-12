package api

import (
	"net/http"
	"os"
	"strconv"

	"github.com/hm-service/scope-status/core"
	"github.com/hm-service/scope-status/httpx"
	"github.com/hm-service/scope-status/i18n"
)

type attachmentUploadView struct {
	SHA256 string `json:"sha256"`
	Name   string `json:"name"`
	Size   int64  `json:"size"`
}

type messageDeleteView struct {
	OK           bool     `json:"ok"`
	ID           string   `json:"id"`
	RemovedFiles []string `json:"removedFiles"`
}

// GET /api/messages
func (s *Server) handleMessageList(w http.ResponseWriter, r *http.Request) {
	payload, err := s.opts.App.Messages.Payload()
	if err != nil {
		writeError(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, payload)
}

// POST /api/messages
func (s *Server) handleMessageCreate(w http.ResponseWriter, r *http.Request) {
	raw, err := readJSONBody(r)
	if err != nil {
		writeError(w, r, err)
		return
	}
	kind, err := bodyKind(raw)
	if err != nil {
		writeError(w, r, err)
		return
	}
	text, err := bodyString(raw, "text")
	if err != nil {
		writeError(w, r, err)
		return
	}
	key, err := bodyString(raw, "key")
	if err != nil {
		writeError(w, r, err)
		return
	}
	refs, _, err := bodyAttachments(raw)
	if err != nil {
		writeError(w, r, err)
		return
	}
	message, err := s.opts.App.Messages.Create(kind, deref(text), deref(key), refs)
	if err != nil {
		writeError(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, message.View())
}

// GET /api/messages/{message_id}
func (s *Server) handleMessageGet(w http.ResponseWriter, r *http.Request) {
	message, err := s.opts.App.Messages.Get(r.PathValue("message_id"), r.URL.Query().Get("key"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, message.View())
}

// POST /api/messages/{message_id}/unlock
func (s *Server) handleMessageUnlock(w http.ResponseWriter, r *http.Request) {
	raw, err := readJSONBody(r)
	if err != nil {
		writeError(w, r, err)
		return
	}
	key, err := bodyString(raw, "key")
	if err != nil {
		writeError(w, r, err)
		return
	}
	message, err := s.opts.App.Messages.Get(r.PathValue("message_id"), deref(key))
	if err != nil {
		writeError(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, message.View())
}

// PATCH /api/messages/{message_id}
func (s *Server) handleMessageUpdate(w http.ResponseWriter, r *http.Request) {
	raw, err := readJSONBody(r)
	if err != nil {
		writeError(w, r, err)
		return
	}
	text, err := bodyString(raw, "text")
	if err != nil {
		writeError(w, r, err)
		return
	}
	key, err := bodyString(raw, "key")
	if err != nil {
		writeError(w, r, err)
		return
	}
	refs, replace, err := bodyAttachments(raw)
	if err != nil {
		writeError(w, r, err)
		return
	}
	message, err := s.opts.App.Messages.Update(r.PathValue("message_id"), text, refs, replace, deref(key))
	if err != nil {
		writeError(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, message.View())
}

// POST /api/messages/{message_id}/delete
func (s *Server) handleMessageDelete(w http.ResponseWriter, r *http.Request) {
	raw, err := readJSONBody(r)
	if err != nil {
		writeError(w, r, err)
		return
	}
	key, err := bodyString(raw, "key")
	if err != nil {
		writeError(w, r, err)
		return
	}
	id := r.PathValue("message_id")
	removed, err := s.opts.App.Messages.Delete(id, deref(key))
	if err != nil {
		writeError(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusOK, messageDeleteView{OK: true, ID: id, RemovedFiles: removed})
}

// PUT /api/messages/files?name=<文件名>：请求体就是裸文件流，不拼 multipart。
func (s *Server) handleFileUpload(w http.ResponseWriter, r *http.Request) {
	declared := int64(-1)
	if value := r.Header.Get("Content-Length"); value != "" {
		if parsed, err := strconv.ParseInt(value, 10, 64); err == nil {
			declared = parsed
		}
	}
	attachment, err := s.opts.App.Messages.Store().SaveFile(r.URL.Query().Get("name"), r.Body, declared)
	if err != nil {
		writeError(w, r, err)
		return
	}
	httpx.JSON(w, http.StatusCreated, attachmentUploadView{SHA256: attachment.SHA256, Name: attachment.Name, Size: attachment.Size})
}

// GET /api/messages/files/{sha256}?id=&key=：附件池共享，必须说明是哪个消息引用的它。
func (s *Server) handleFileDownload(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		writeError(w, r, core.BadRequest(i18n.KeyMessageIDRequired))
		return
	}
	path, name, err := s.opts.App.Messages.Store().ReadFile(id, r.PathValue("sha256"), r.URL.Query().Get("key"))
	if err != nil {
		writeError(w, r, err)
		return
	}
	file, err := os.Open(path)
	if err != nil {
		writeError(w, r, core.NotFound(i18n.KeyMessageFileGone, path))
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		writeError(w, r, core.NotFound(i18n.KeyMessageFileGone, path))
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", httpx.ContentDisposition(name))
	w.Header().Set("Cache-Control", "no-store")
	http.ServeContent(w, r, name, info.ModTime(), file)
}

func deref(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
