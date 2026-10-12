// Package store 是盘上存储：SOURCE_FILE 的读取，以及 data 目录下消息与附件池的读写。
package store

import (
	"errors"
	"io/fs"
	"os"

	"github.com/hm-service/scope-status/core"
	"github.com/hm-service/scope-status/i18n"
)

// LoadSource 读并解析 SOURCE_FILE；文件不在、读不了、解析不过都直接报错，不做兜底。
func LoadSource(path string) (*core.Source, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, core.ConfigError(i18n.KeyConfigMissing, path)
		}
		return nil, core.ConfigError(i18n.KeyConfigUnreadable, path, err.Error())
	}
	return core.ParseSource(path, data)
}
