package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// WizardTranslation holds all wizard-specific translations
type WizardTranslation struct {
	Welcome        string
	SelectLanguage string
	ConfigMode     string
	SimpleMode     string
	CustomMode     string
	LLMProvider    string
	BotPlatform    string
	LLMTokens      string
	BaseConfig     string
	BotConfig      string
	LLMConfig      string
	MediaConfig    string
	RAGConfig      string
	ProxyConfig    string
	SaveSuccess    string
	// Agent
	LLMTokensAdditional string
	CmdAgentType        string
	CmdAgentTimeout     string
	// Agent command detection
	CmdAgentFound          string
	CmdAgentNotFound       string
	CmdAgentQoderGuide     string
	CmdAgentOpenCodeGuide  string
	CmdAgentCursorGuide    string
	CmdAgentCustomGuide    string
	CmdAgentInstallMac     string
	CmdAgentInstallLinux   string
	CmdAgentInstallWindows string
	// Bot guides
	GuideTelegram string
	GuideDiscord  string
	GuideSlack    string
	GuideLark     string
	GuideDingTalk string
	GuideWeChat   string
	GuideQQ       string
}

// loadTranslations loads translations from i18n JSON files
func loadTranslations(lang string) (*WizardTranslation, error) {
	// Get the path to conf/i18n directory
	confPath := filepath.Join("..", "conf", "i18n", fmt.Sprintf("i18n.%s.json", lang))

	// Try relative path first
	data, err := os.ReadFile(confPath)
	if err != nil {
		// Try from current directory
		confPath = filepath.Join("conf", "i18n", fmt.Sprintf("i18n.%s.json", lang))
		data, err = os.ReadFile(confPath)
		if err != nil {
			return nil, fmt.Errorf("failed to load translation file: %w", err)
		}
	}

	var translations map[string]string
	if err := json.Unmarshal(data, &translations); err != nil {
		return nil, fmt.Errorf("failed to parse translation file: %w", err)
	}

	return &WizardTranslation{
		Welcome:                translations["wizard.welcome"],
		SelectLanguage:         translations["wizard.select_language"],
		ConfigMode:             translations["wizard.config_mode"],
		SimpleMode:             translations["wizard.simple_mode"],
		CustomMode:             translations["wizard.custom_mode"],
		LLMProvider:            translations["wizard.llm_provider"],
		BotPlatform:            translations["wizard.bot_platform"],
		LLMTokens:              translations["wizard.llm_tokens"],
		BaseConfig:             translations["wizard.base_config"],
		BotConfig:              translations["wizard.bot_config"],
		LLMConfig:              translations["wizard.llm_config"],
		MediaConfig:            translations["wizard.media_config"],
		RAGConfig:              translations["wizard.rag_config"],
		ProxyConfig:            translations["wizard.proxy_config"],
		SaveSuccess:            translations["wizard.save_success"],
		LLMTokensAdditional:    translations["wizard.llm_tokens_additional"],
		CmdAgentType:           translations["wizard.cmd_agent_type"],
		CmdAgentTimeout:        translations["wizard.cmd_agent_timeout"],
		CmdAgentFound:          translations["wizard.cmd_agent_found"],
		CmdAgentNotFound:       translations["wizard.cmd_agent_not_found"],
		CmdAgentQoderGuide:     translations["wizard.cmd_agent_qoder_guide"],
		CmdAgentOpenCodeGuide:  translations["wizard.cmd_agent_opencode_guide"],
		CmdAgentCursorGuide:    translations["wizard.cmd_agent_cursor_guide"],
		CmdAgentCustomGuide:    translations["wizard.cmd_agent_custom_guide"],
		CmdAgentInstallMac:     translations["wizard.cmd_agent_install_mac"],
		CmdAgentInstallLinux:   translations["wizard.cmd_agent_install_linux"],
		CmdAgentInstallWindows: translations["wizard.cmd_agent_install_windows"],
		GuideTelegram:          translations["wizard.guide.telegram"],
		GuideDiscord:           translations["wizard.guide.discord"],
		GuideSlack:             translations["wizard.guide.slack"],
		GuideLark:              translations["wizard.guide.lark"],
		GuideDingTalk:          translations["wizard.guide.dingtalk"],
		GuideWeChat:            translations["wizard.guide.wechat"],
		GuideQQ:                translations["wizard.guide.qq"],
	}, nil
}
