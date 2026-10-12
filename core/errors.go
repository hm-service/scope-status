package core

import "github.com/hm-service/scope-status/i18n"

// Error 是带文案键的业务错误：状态码与文案都留给 HTTP 层，业务层不拼句子。
type Error struct {
	Key    i18n.Key
	Args   []any
	Status int
}

func (e *Error) Error() string { return i18n.T(i18n.Default, e.Key, e.Args...) }

func NewError(status int, key i18n.Key, args ...any) *Error {
	return &Error{Key: key, Args: args, Status: status}
}

func BadRequest(key i18n.Key, args ...any) *Error { return NewError(400, key, args...) }
func Forbidden(key i18n.Key, args ...any) *Error  { return NewError(403, key, args...) }
func NotFound(key i18n.Key, args ...any) *Error   { return NewError(404, key, args...) }
func TooLarge(key i18n.Key, args ...any) *Error   { return NewError(413, key, args...) }
func ServerError(key i18n.Key, args ...any) *Error {
	return NewError(500, key, args...)
}

// ConfigError 是配置写坏了的错误：一律 500，把原文交给页面。
func ConfigError(key i18n.Key, args ...any) *Error { return NewError(500, key, args...) }
