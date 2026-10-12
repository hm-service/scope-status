package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/hm-service/scope-status/core"
	"github.com/hm-service/scope-status/httpx"
	"github.com/hm-service/scope-status/i18n"
	"github.com/hm-service/scope-status/store"
)

// maxJSONBody 是 JSON 请求体的上限；附件走裸文件流，不占这个额度。
const maxJSONBody = 1 << 20

type contextKey int

const langKey contextKey = 0

func withLang(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lang := i18n.Negotiate(r.Header.Get("Accept-Language"))
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), langKey, lang)))
	})
}

func langOf(r *http.Request) i18n.Lang {
	if lang, ok := r.Context().Value(langKey).(i18n.Lang); ok {
		return lang
	}
	return i18n.Negotiate(r.Header.Get("Accept-Language"))
}

// writeError 把业务错误按请求语言渲染成纯文本，状态码跟着错误走。
func writeError(w http.ResponseWriter, r *http.Request, err error) {
	lang := langOf(r)
	var business *core.Error
	if errors.As(err, &business) {
		httpx.Text(w, business.Status, i18n.T(lang, business.Key, business.Args...)+"\n")
		return
	}
	httpx.Text(w, http.StatusInternalServerError, i18n.T(lang, i18n.KeyHTTPInternal, err.Error())+"\n")
}

func readJSONBody(r *http.Request) (map[string]json.RawMessage, error) {
	body, err := io.ReadAll(io.LimitReader(r.Body, maxJSONBody+1))
	if err != nil {
		return nil, core.BadRequest(i18n.KeyMessageBodyNotJSON, err.Error())
	}
	if int64(len(body)) > maxJSONBody {
		return nil, core.TooLarge(i18n.KeyMessageBodyTooBig, maxJSONBody)
	}
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return map[string]json.RawMessage{}, nil
	}
	if trimmed[0] != '{' {
		return nil, core.BadRequest(i18n.KeyMessageBodyNotObject)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(trimmed, &raw); err != nil {
		return nil, core.BadRequest(i18n.KeyMessageBodyNotJSON, err.Error())
	}
	return raw, nil
}

// bodyString 取一个字符串字段；字段不在就是「没给」。
func bodyString(raw map[string]json.RawMessage, name string) (*string, error) {
	field, ok := raw[name]
	if !ok || string(bytes.TrimSpace(field)) == "null" {
		return nil, nil
	}
	var value string
	if err := json.Unmarshal(field, &value); err != nil {
		return nil, core.BadRequest(i18n.KeyMessageBodyType, name)
	}
	return &value, nil
}

func bodyKind(raw map[string]json.RawMessage) (core.Kind, error) {
	value, err := bodyString(raw, "kind")
	if err != nil {
		return "", err
	}
	if value == nil {
		return core.KindPublic, nil
	}
	kind := core.Kind(*value)
	if kind != core.KindPublic && kind != core.KindKey {
		return "", core.BadRequest(i18n.KeyMessageKindInvalid)
	}
	return kind, nil
}

// bodyAttachments 取附件引用；第二个返回值表示这个字段到底给没给。
func bodyAttachments(raw map[string]json.RawMessage) ([]store.AttachmentRef, bool, error) {
	field, ok := raw["attachments"]
	if !ok || string(bytes.TrimSpace(field)) == "null" {
		return nil, false, nil
	}
	var items []struct {
		SHA256 string `json:"sha256"`
		Name   string `json:"name"`
	}
	if err := json.Unmarshal(field, &items); err != nil {
		return nil, true, core.BadRequest(i18n.KeyMessageAttsNotArray)
	}
	refs := make([]store.AttachmentRef, 0, len(items))
	for _, item := range items {
		refs = append(refs, store.AttachmentRef{SHA256: item.SHA256, Name: item.Name})
	}
	return refs, true, nil
}
