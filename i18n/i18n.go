package i18n

import (
	"embed"
	"encoding/json"
	"io/fs"
	"os"

	"github.com/nicksnyder/go-i18n/v2/i18n"
	"github.com/yincongcyincong/MuseBot/conf"
	"github.com/yincongcyincong/MuseBot/logger"
	botUtils "github.com/yincongcyincong/MuseBot/utils"
	"golang.org/x/text/language"
)

var (
	ruLocalizer *i18n.Localizer
	enLocalizer *i18n.Localizer
	zhLocalizer *i18n.Localizer
	staticFS    embed.FS
)

const (
	ru = "ru"
	en = "en"
	zh = "zh"
)

// SetStaticFS 设置静态文件的 embed.FS
func SetStaticFS(fsys embed.FS) {
	staticFS = fsys
}

func InitI18n() {
	// 1. Create a new i18n bundle with English as default language
	bundle := i18n.NewBundle(language.English)

	// 2. Register JSON unmarshal function (other formats like TOML are also supported)
	bundle.RegisterUnmarshalFunc("json", json.Unmarshal)

	// 3. Load translation files
	// 尝试从 embed.FS 加载，如果失败则从文件系统加载
	// Russian translations
	if err := loadMessageFile(bundle, "conf/i18n/i18n.ru.json"); err != nil {
		logger.Error("Failed to load Russian translation file", "err", err)
	}
	// English translations
	if err := loadMessageFile(bundle, "conf/i18n/i18n.en.json"); err != nil {
		logger.Error("Failed to load English translation file", "err", err)
	}
	// Chinese translations
	if err := loadMessageFile(bundle, "conf/i18n/i18n.zh.json"); err != nil {
		logger.Error("Failed to load Chinese translation file", "err", err)
	}

	// 4. Create localizers for each language
	ruLocalizer = i18n.NewLocalizer(bundle, ru)
	enLocalizer = i18n.NewLocalizer(bundle, en)
	zhLocalizer = i18n.NewLocalizer(bundle, zh)
}

// loadMessageFile 从 embed.FS 或文件系统加载消息文件
func loadMessageFile(bundle *i18n.Bundle, path string) error {
	// 首先尝试从 embed.FS 读取
	if data, err := fs.ReadFile(staticFS, path); err == nil {
		_, err := bundle.ParseMessageFileBytes(data, path)
		return err
	}

	// 如果 embed 失败，尝试从文件系统读取（用于开发调试）
	absPath := botUtils.GetAbsPath(path)
	if data, err := os.ReadFile(absPath); err == nil {
		_, err := bundle.ParseMessageFileBytes(data, path)
		return err
	}

	// 使用原始方法作为最后的备选
	_, err := bundle.LoadMessageFile(botUtils.GetAbsPath(path))
	return err
}

// GetMessage function to get localized message
func GetMessage(messageID string, templateData map[string]interface{}) string {
	var localizer *i18n.Localizer
	switch conf.BaseConfInfo.Lang {
	case ru:
		localizer = ruLocalizer
	case zh:
		localizer = zhLocalizer
	default:
		localizer = enLocalizer
	}

	msg, err := localizer.Localize(&i18n.LocalizeConfig{
		MessageID:    messageID,
		TemplateData: templateData,
	})
	if err != nil {
		logger.Warn("Failed to localize message", "tag", conf.BaseConfInfo.Lang, "messageID", messageID, "err", err)
		return ""
	}
	return msg
}
