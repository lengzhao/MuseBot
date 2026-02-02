package main

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"

	"github.com/AlecAivazis/survey/v2"
)

type Config struct {
	Env map[string]string
}

func (c *Config) Set(key, value string) {
	if value != "" {
		c.Env[key] = value
	}
}

func (c *Config) Get(key string) string {
	if val, ok := c.Env[key]; ok {
		return val
	}
	return ""
}

func (c *Config) SetBool(key string, val bool) {
	if val {
		c.Env[key] = "true"
	} else {
		c.Env[key] = "false"
	}
}

func (c *Config) PromptInput(key, label, defaultValue string) {
	// 如果当前已有值，使用当前值作为默认值
	if currentVal := c.Get(key); currentVal != "" && defaultValue == "" {
		defaultValue = currentVal
	}

	displayDefault := defaultValue
	if defaultValue != "" && isSensitive(key) {
		displayDefault = maskString(defaultValue)
	}

	var res string
	prompt := &survey.Input{
		Message: label,
		Default: displayDefault,
	}
	survey.AskOne(prompt, &res)

	// 如果用户直接回车（res == displayDefault），说明使用的是默认值（已脱敏展示）
	// 此时应该保留原始的 defaultValue
	if res == displayDefault {
		res = defaultValue
	}

	c.Set(key, res)
}

func isSensitive(key string) bool {
	key = strings.ToUpper(key)
	return strings.Contains(key, "TOKEN") ||
		strings.Contains(key, "SECRET") ||
		strings.Contains(key, "KEY") ||
		strings.Contains(key, "PASSWORD")
}

func maskString(s string) string {
	if len(s) <= 8 {
		return "****"
	}
	return s[:4] + "****" + s[len(s)-4:]
}

func (c *Config) PromptSelect(key, label string, options []string) string {
	// 如果当前已有值，且该值在选项列表中，设置为默认选项
	var defaultVal string
	if currentVal := c.Get(key); currentVal != "" {
		// 检查当前值是否在选项列表中
		for _, opt := range options {
			if opt == currentVal {
				defaultVal = currentVal
				break
			}
		}
	}

	var res string
	prompt := &survey.Select{
		Message: label,
		Options: options,
	}
	// 只有当 defaultVal 非空时才设置 Default
	if defaultVal != "" {
		prompt.Default = defaultVal
	}
	survey.AskOne(prompt, &res)
	c.Set(key, res)
	return res
}

func (c *Config) PromptMultiSelect(label string, options []string, defaults []string) []string {
	var selected []string
	prompt := &survey.MultiSelect{
		Message: label,
		Options: options,
		Default: defaults,
	}
	survey.AskOne(prompt, &selected)
	return selected
}

func (c *Config) RunSimple(trans *WizardTranslation) {
	fmt.Println("\n" + trans.SimpleMode)

	// LLM
	if confirm(trans.LLMConfig) {
		llm := c.PromptSelect("TYPE", trans.LLMProvider, []string{"deepseek", "gemini", "openai", "openrouter", "vol", "chatanywhere", "cmd-agent"})

		if llm == "cmd-agent" {
			agentType := c.PromptSelect("CMD_AGENT_TYPE", trans.CmdAgentType, []string{"qoder", "opencode", "cursor", "custom"})
			c.PromptInput("CMD_AGENT_TIMEOUT", trans.CmdAgentTimeout, "300")
			if agentType == "custom" {
				c.PromptInput("CMD_AGENT_CUSTOM_COMMAND", "Custom Command Path", "")
				c.PromptInput("CMD_AGENT_CUSTOM_DEFAULT_ARGS", "Custom Default Args (space separated)", "")
				c.PromptInput("CMD_AGENT_CUSTOM_STREAM_ARGS", "Custom Stream Args (space separated)", "")
				c.PromptInput("CMD_AGENT_CUSTOM_RESUME_FLAG", "Custom Resume Flag (e.g. --resume)", "")
				c.PromptInput("CMD_AGENT_CUSTOM_WORKDIR_FLAG", "Custom WorkDir Flag (e.g. --workdir)", "")
			}
		} else {
			switch llm {
			case "deepseek":
				c.PromptInput("DEEPSEEK_TOKEN", "DEEPSEEK_TOKEN", "")
			case "gemini":
				c.PromptInput("GEMINI_TOKEN", "GEMINI_TOKEN", "")
			case "openai":
				c.PromptInput("OPENAI_TOKEN", "OPENAI_TOKEN", "")
			case "openrouter":
				c.PromptInput("OPEN_ROUTER_TOKEN", "OPEN_ROUTER_TOKEN", "")
			case "vol":
				c.PromptInput("VOL_TOKEN", "VOL_TOKEN", "")
			case "chatanywhere":
				c.PromptInput("CHAT_ANY_WHERE_TOKEN", "CHAT_ANY_WHERE_TOKEN", "")
			}
		}
	}

	// Bot Platforms (Multi-select)
	allBots := []string{"Web", "Telegram", "Discord", "Slack", "Lark", "DingTalk", "WeChat", "QQ"}
	currentBots := c.detectCurrentBots()
	selectedBots := c.PromptMultiSelect(trans.BotPlatform, allBots, currentBots)

	for _, bot := range selectedBots {
		switch bot {
		case "Web":
			fmt.Println("🌐 Web UI will be available at your HTTP_HOST (recommended 127.0.0.1:8080)")
			if c.Get("HTTP_HOST") == "" {
				c.Set("HTTP_HOST", "127.0.0.1:8080")
			}
		case "Telegram":
			fmt.Println("\n📱 Telegram Bot Configuration")
			fmt.Println("💡 Guide: Contact @BotFather on Telegram, send /newbot and follow instructions")
			c.PromptInput("TELEGRAM_BOT_TOKEN", "TELEGRAM_BOT_TOKEN", "")
		case "Discord":
			fmt.Println("\n💬 Discord Bot Configuration")
			fmt.Println("💡 Guide: Visit https://discord.com/developers/applications")
			fmt.Println("   1. Create New Application")
			fmt.Println("   2. Go to Bot section and click 'Add Bot'")
			fmt.Println("   3. Copy the Token")
			c.PromptInput("DISCORD_BOT_TOKEN", "DISCORD_BOT_TOKEN", "")
		case "Slack":
			fmt.Println("\n💼 Slack Bot Configuration")
			fmt.Println("💡 Guide: Visit https://api.slack.com/apps")
			fmt.Println("   1. Create New App")
			fmt.Println("   2. Enable Socket Mode")
			fmt.Println("   3. Get Bot User OAuth Token from OAuth & Permissions")
			fmt.Println("   4. Get App-Level Token from Basic Information")
			c.PromptInput("SLACK_BOT_TOKEN", "SLACK_BOT_TOKEN", "")
			c.PromptInput("SLACK_APP_TOKEN", "SLACK_APP_TOKEN", "")
		case "Lark":
			fmt.Println("\n🚀 Lark (Feishu) Bot Configuration")
			fmt.Println("💡 Guide: Visit https://open.feishu.cn/")
			fmt.Println("   1. Create Enterprise Self-built App (企业自建应用)")
			fmt.Println("   2. Find App ID and App Secret in 'Credentials & Basic Info' (凭证与基础信息)")
			c.PromptInput("LARK_APP_ID", "LARK_APP_ID", "")
			c.PromptInput("LARK_APP_SECRET", "LARK_APP_SECRET", "")
		case "DingTalk":
			fmt.Println("\n📊 DingTalk Bot Configuration")
			fmt.Println("💡 Guide: Visit https://open.dingtalk.com/")
			fmt.Println("   1. Create Enterprise Internal App (企业内部应用)")
			fmt.Println("   2. Get Client ID and Client Secret from App Credentials")
			c.PromptInput("DING_CLIENT_ID", "DING_CLIENT_ID", "")
			c.PromptInput("DING_CLIENT_SECRET", "DING_CLIENT_SECRET", "")
		case "WeChat":
			fmt.Println("\n💚 WeChat Official Account Configuration")
			fmt.Println("💡 Guide: Visit https://mp.weixin.qq.com/")
			fmt.Println("   1. Register Official Account (注册公众号)")
			fmt.Println("   2. Get App ID and App Secret from Development > Basic Configuration")
			c.PromptInput("WECHAT_APP_ID", "WECHAT_APP_ID", "")
			c.PromptInput("WECHAT_APP_SECRET", "WECHAT_APP_SECRET", "")
		case "QQ":
			fmt.Println("\n🐧 QQ Bot Configuration")
			fmt.Println("💡 Guide: Visit https://q.qq.com/")
			fmt.Println("   1. Create QQ Bot Application")
			fmt.Println("   2. Get App ID and App Secret")
			fmt.Println("   3. Configure OneBot HTTP Server (e.g., LLOneBot)")
			c.PromptInput("QQ_APP_ID", "QQ_APP_ID", "")
			c.PromptInput("QQ_APP_SECRET", "QQ_APP_SECRET", "")
		}
	}
}

func (c *Config) detectCurrentBots() []string {
	var bots []string
	if c.Get("TELEGRAM_BOT_TOKEN") != "" {
		bots = append(bots, "Telegram")
	}
	if c.Get("DISCORD_BOT_TOKEN") != "" {
		bots = append(bots, "Discord")
	}
	if c.Get("SLACK_BOT_TOKEN") != "" {
		bots = append(bots, "Slack")
	}
	if c.Get("LARK_APP_ID") != "" {
		bots = append(bots, "Lark")
	}
	if c.Get("DING_CLIENT_ID") != "" {
		bots = append(bots, "DingTalk")
	}
	if c.Get("WECHAT_APP_ID") != "" {
		bots = append(bots, "WeChat")
	}
	if c.Get("QQ_APP_ID") != "" {
		bots = append(bots, "QQ")
	}
	if c.Get("HTTP_HOST") != "" {
		bots = append(bots, "Web")
	}
	return bots
}

func (c *Config) RunCustom(trans *WizardTranslation) {
	fmt.Println("\n" + trans.CustomMode)

	// 1. Base Config
	if confirm(trans.BaseConfig) {
		c.PromptInput("BOT_NAME", "BOT_NAME", "MuseBot")
		c.PromptInput("LOG_LEVEL", "LOG_LEVEL (debug, info, warn, error)", "info")
		c.PromptInput("HTTP_HOST", "HTTP_HOST", "127.0.0.1:8080")
		c.PromptSelect("DB_TYPE", "DB_TYPE", []string{"sqlite3", "mysql", "postgres"})
		c.PromptInput("DB_CONF", "DB_CONF", "")
		c.PromptInput("ALLOWED_USER_IDS", "ALLOWED_USER_IDS", "")
		c.PromptInput("ALLOWED_GROUP_IDS", "ALLOWED_GROUP_IDS", "")
	}

	// 2. Bot Config (Multiple)
	if confirm(trans.BotConfig) {
		allBots := []string{"Web", "Telegram", "Discord", "Slack", "Lark", "DingTalk", "WeChat", "QQ"}
		currentBots := c.detectCurrentBots()
		selectedBots := c.PromptMultiSelect(trans.BotPlatform, allBots, currentBots)

		for _, bot := range selectedBots {
			switch bot {
			case "Web":
				fmt.Println("🌐 Web UI enabled (uses HTTP_HOST)")
				if c.Get("HTTP_HOST") == "" {
					c.Set("HTTP_HOST", "127.0.0.1:8080")
				}
			case "Telegram":
				fmt.Println("\n📱 Telegram Bot Configuration")
				fmt.Println("💡 Guide: Contact @BotFather on Telegram, send /newbot and follow instructions")
				c.PromptInput("TELEGRAM_BOT_TOKEN", "TELEGRAM_BOT_TOKEN", "")
			case "Discord":
				fmt.Println("\n💬 Discord Bot Configuration")
				fmt.Println("💡 Guide: Visit https://discord.com/developers/applications")
				fmt.Println("   1. Create New Application")
				fmt.Println("   2. Go to Bot section and click 'Add Bot'")
				fmt.Println("   3. Copy the Token")
				c.PromptInput("DISCORD_BOT_TOKEN", "DISCORD_BOT_TOKEN", "")
			case "Slack":
				fmt.Println("\n💼 Slack Bot Configuration")
				fmt.Println("💡 Guide: Visit https://api.slack.com/apps")
				fmt.Println("   1. Create New App")
				fmt.Println("   2. Enable Socket Mode")
				fmt.Println("   3. Get Bot User OAuth Token from OAuth & Permissions")
				fmt.Println("   4. Get App-Level Token from Basic Information")
				c.PromptInput("SLACK_BOT_TOKEN", "SLACK_BOT_TOKEN", "")
				c.PromptInput("SLACK_APP_TOKEN", "SLACK_APP_TOKEN", "")
			case "Lark":
				fmt.Println("\n🚀 Lark (Feishu) Bot Configuration")
				fmt.Println("💡 Guide: Visit https://open.feishu.cn/")
				fmt.Println("   1. Create Enterprise Self-built App (企业自建应用)")
				fmt.Println("   2. Find App ID and App Secret in 'Credentials & Basic Info' (凭证与基础信息)")
				c.PromptInput("LARK_APP_ID", "LARK_APP_ID", "")
				c.PromptInput("LARK_APP_SECRET", "LARK_APP_SECRET", "")
			case "DingTalk":
				fmt.Println("\n📊 DingTalk Bot Configuration")
				fmt.Println("💡 Guide: Visit https://open.dingtalk.com/")
				fmt.Println("   1. Create Enterprise Internal App (企业内部应用)")
				fmt.Println("   2. Get Client ID and Client Secret from App Credentials")
				c.PromptInput("DING_CLIENT_ID", "DING_CLIENT_ID", "")
				c.PromptInput("DING_CLIENT_SECRET", "DING_CLIENT_SECRET", "")
			case "WeChat":
				fmt.Println("\n💚 WeChat Official Account Configuration")
				fmt.Println("💡 Guide: Visit https://mp.weixin.qq.com/")
				fmt.Println("   1. Register Official Account (注册公众号)")
				fmt.Println("   2. Get App ID and App Secret from Development > Basic Configuration")
				c.PromptInput("WECHAT_APP_ID", "WECHAT_APP_ID", "")
				c.PromptInput("WECHAT_APP_SECRET", "WECHAT_APP_SECRET", "")
			case "QQ":
				fmt.Println("\n🐧 QQ Bot Configuration")
				fmt.Println("💡 Guide: Visit https://q.qq.com/")
				fmt.Println("   1. Create QQ Bot Application")
				fmt.Println("   2. Get App ID and App Secret")
				fmt.Println("   3. Configure OneBot HTTP Server (e.g., LLOneBot)")
				c.PromptInput("QQ_APP_ID", "QQ_APP_ID", "")
				c.PromptInput("QQ_APP_SECRET", "QQ_APP_SECRET", "")
			}
		}
	}

	// 3. LLM Tokens (Multi-select)
	if confirm(trans.LLMConfig) {
		llm := c.PromptSelect("TYPE", trans.LLMProvider, []string{"deepseek", "gemini", "openai", "openrouter", "vol", "chatanywhere", "cmd-agent"})

		// CMD Agent 配置
		if llm == "cmd-agent" {
			agentType := c.PromptSelect("CMD_AGENT_TYPE", trans.CmdAgentType, []string{"qoder", "opencode", "cursor", "custom"})
			c.PromptInput("CMD_AGENT_TIMEOUT", trans.CmdAgentTimeout, "300")

			// 检查命令是否存在
			if agentType == "custom" {
				c.PromptInput("CMD_AGENT_CUSTOM_COMMAND", "Custom Command Path", "")
				c.PromptInput("CMD_AGENT_CUSTOM_DEFAULT_ARGS", "Custom Default Args (space separated)", "")
				c.PromptInput("CMD_AGENT_CUSTOM_STREAM_ARGS", "Custom Stream Args (space separated)", "")
				c.PromptInput("CMD_AGENT_CUSTOM_RESUME_FLAG", "Custom Resume Flag (e.g. --resume)", "")
				c.PromptInput("CMD_AGENT_CUSTOM_WORKDIR_FLAG", "Custom WorkDir Flag (e.g. --workdir)", "")
				// 检查自定义命令
				if customCmd := c.Get("CMD_AGENT_CUSTOM_COMMAND"); customCmd != "" {
					checkAndGuideCmdInstallation(agentType, customCmd, trans)
				}
			} else {
				// 检查内置 Agent 命令
				checkAndGuideCmdInstallation(agentType, getDefaultCommand(agentType), trans)
			}
		}

		// 如果不是 cmd-agent，或者用户想要额外配置标准 LLM
		shouldConfigureTokens := true
		if llm == "cmd-agent" {
			shouldConfigureTokens = confirm(trans.LLMTokensAdditional)
		}

		if shouldConfigureTokens {
			// LLM Token 多选配置
			allLLMs := []string{"DeepSeek", "OpenAI", "Gemini", "OpenRouter", "Volcengine", "Aliyun"}
			currentLLMs := c.detectCurrentLLMs()
			selectedLLMs := c.PromptMultiSelect(trans.LLMTokens, allLLMs, currentLLMs)

			for _, llm := range selectedLLMs {
				switch llm {
				case "DeepSeek":
					c.PromptInput("DEEPSEEK_TOKEN", "DEEPSEEK_TOKEN", "")
				case "OpenAI":
					c.PromptInput("OPENAI_TOKEN", "OPENAI_TOKEN", "")
				case "Gemini":
					c.PromptInput("GEMINI_TOKEN", "GEMINI_TOKEN", "")
				case "OpenRouter":
					c.PromptInput("OPEN_ROUTER_TOKEN", "OPEN_ROUTER_TOKEN", "")
				case "Volcengine":
					c.PromptInput("VOL_TOKEN", "VOL_TOKEN", "")
				case "Aliyun":
					c.PromptInput("ALIYUN_TOKEN", "ALIYUN_TOKEN", "")
				}
			}
		}
	}

	// 4. Media
	if confirm(trans.MediaConfig) {
		c.PromptSelect("MEDIA_TYPE", "MEDIA_TYPE", []string{"vol", "gemini", "openai", "aliyun", "302-ai", "openrouter"})
		c.PromptSelect("TTS_TYPE", "TTS_TYPE", []string{"vol", "gemini", "openai", "aliyun"})
	}

	// 5. RAG
	if confirm(trans.RAGConfig) {
		c.PromptSelect("EMBEDDING_TYPE", "EMBEDDING_TYPE", []string{"openai", "gemini", "ernie"})
		c.PromptSelect("VECTOR_DB_TYPE", "VECTOR_DB_TYPE", []string{"chroma", "weaviate", "milvus"})
		c.PromptInput("KNOWLEDGE_PATH", "KNOWLEDGE_PATH", "")
	}

	// 6. Proxy
	if confirm(trans.ProxyConfig) {
		c.PromptInput("LLM_PROXY", "LLM_PROXY", "")
		c.PromptInput("ROBOT_PROXY", "ROBOT_PROXY", "")
	}
}

func (c *Config) detectCurrentLLMs() []string {
	var llms []string
	if c.Get("DEEPSEEK_TOKEN") != "" {
		llms = append(llms, "DeepSeek")
	}
	if c.Get("OPENAI_TOKEN") != "" {
		llms = append(llms, "OpenAI")
	}
	if c.Get("GEMINI_TOKEN") != "" {
		llms = append(llms, "Gemini")
	}
	if c.Get("OPEN_ROUTER_TOKEN") != "" {
		llms = append(llms, "OpenRouter")
	}
	if c.Get("VOL_TOKEN") != "" {
		llms = append(llms, "Volcengine")
	}
	if c.Get("ALIYUN_TOKEN") != "" {
		llms = append(llms, "Aliyun")
	}
	return llms
}

func confirm(label string) bool {
	res := false
	prompt := &survey.Confirm{
		Message: label + " - " + "Configure this module?",
		Default: true,
	}
	survey.AskOne(prompt, &res)
	return res
}

// getDefaultCommand 返回内置 Agent 类型的默认命令名
func getDefaultCommand(agentType string) string {
	switch agentType {
	case "qoder":
		return "qodercli"
	case "opencode":
		return "opencode"
	case "cursor":
		return "agent"
	default:
		return ""
	}
}

// checkAndGuideCmdInstallation 检查命令是否存在，如果不存在则引导安装
func checkAndGuideCmdInstallation(agentType, command string, trans *WizardTranslation) {
	if command == "" {
		return
	}

	// 检查命令是否存在
	_, err := exec.LookPath(command)
	if err == nil {
		fmt.Printf("\n✅ %s\n", trans.CmdAgentFound)
		return
	}

	// 命令不存在，显示安装引导
	fmt.Printf("\n⚠️  %s '%s'\n", trans.CmdAgentNotFound, command)

	// 根据 Agent 类型显示安装指引
	guide := getCmdAgentInstallGuide(agentType, trans)
	if guide != "" {
		fmt.Println(guide)
	}
}

// getCmdAgentInstallGuide 获取 Agent 安装指引
func getCmdAgentInstallGuide(agentType string, trans *WizardTranslation) string {
	os := runtime.GOOS
	switch agentType {
	case "qoder":
		return getQoderInstallGuide(os, trans)
	case "opencode":
		return getOpenCodeInstallGuide(os, trans)
	case "cursor":
		return getCursorInstallGuide(os, trans)
	default:
		return trans.CmdAgentCustomGuide
	}
}

func getQoderInstallGuide(os string, trans *WizardTranslation) string {
	guide := trans.CmdAgentQoderGuide + "\n"
	switch os {
	case "darwin":
		guide += trans.CmdAgentInstallMac
	case "linux":
		guide += trans.CmdAgentInstallLinux
	case "windows":
		guide += trans.CmdAgentInstallWindows
	}
	return guide
}

func getOpenCodeInstallGuide(os string, trans *WizardTranslation) string {
	guide := trans.CmdAgentOpenCodeGuide + "\n"
	switch os {
	case "darwin":
		guide += trans.CmdAgentInstallMac
	case "linux":
		guide += trans.CmdAgentInstallLinux
	case "windows":
		guide += trans.CmdAgentInstallWindows
	}
	return guide
}

func getCursorInstallGuide(os string, trans *WizardTranslation) string {
	guide := trans.CmdAgentCursorGuide + "\n"
	switch os {
	case "darwin":
		guide += trans.CmdAgentInstallMac
	case "linux":
		guide += trans.CmdAgentInstallLinux
	case "windows":
		guide += trans.CmdAgentInstallWindows
	}
	return guide
}
