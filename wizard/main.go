package main

import (
	"bufio"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/AlecAivazis/survey/v2"
)

func main() {
	config := &Config{
		Env: make(map[string]string),
	}

	// 加载现有配置
	loadExistingConfig(config)

	// 1. Select Language
	var langStr string
	langPrompt := &survey.Select{
		Message: "Select Language / 请选择语言 / Выберите язык:",
		Options: []string{"中文 (zh)", "English (en)", "Русский (ru)"},
	}
	survey.AskOne(langPrompt, &langStr)

	var lang string
	switch langStr {
	case "中文 (zh)":
		lang = "zh"
	case "Русский (ru)":
		lang = "ru"
	default:
		lang = "en"
	}

	// Load translations
	trans, err := loadTranslations(lang)
	if err != nil {
		fmt.Printf("Error loading translations: %v\n", err)
		return
	}

	fmt.Println("\n" + trans.Welcome)
	config.Set("LANG", lang)

	// 2. Select Mode
	var mode string
	modePrompt := &survey.Select{
		Message: trans.ConfigMode,
		Options: []string{trans.SimpleMode, trans.CustomMode},
	}
	survey.AskOne(modePrompt, &mode)

	if mode == trans.SimpleMode {
		config.RunSimple(trans)
	} else {
		config.RunCustom(trans)
	}

	// 3. Save to .env
	saveToEnv(config.Env, trans)
}

// 加载现有配置（从 .env 或 .env.example）
func loadExistingConfig(config *Config) {
	// 先尝试从 .env 加载
	if err := loadEnvFile(config, ".env"); err == nil {
		fmt.Println("✅ Loaded existing .env configuration")
		return
	}

	// 如果 .env 不存在，从 .env.example 加载
	if err := loadEnvFile(config, ".env.example"); err == nil {
		fmt.Println("📋 Using .env.example as template")
		return
	}

	fmt.Println("⚠️  No existing configuration found, starting from scratch")
}

func loadEnvFile(config *Config, filename string) error {
	file, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// 跳过注释和空行
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// 解析 KEY=VALUE
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])
			config.Env[key] = value
		}
	}

	return scanner.Err()
}

func saveToEnv(env map[string]string, trans *WizardTranslation) {
	// 检查 .env 是否已存在
	_, err := os.Stat(".env")
	envExists := err == nil

	// 如果 .env 不存在，先从 .env.example 复制
	if !envExists {
		if err := copyFile(".env.example", ".env"); err != nil {
			fmt.Printf("⚠️  Warning: Could not copy .env.example to .env: %v\n", err)
			fmt.Println("Creating new .env file...")
		} else {
			fmt.Println("📋 Copied .env.example to .env")
		}
	}

	// 读取 .env 文件内容（保留注释和格式）
	lines, err := readEnvFileWithComments(".env")
	if err != nil {
		// 如果读取失败，使用传统方式写入
		fallbackSaveToEnv(env, trans)
		return
	}

	// 更新配置值
	updatedLines := updateEnvLines(lines, env)

	// 写回 .env 文件
	err = os.WriteFile(".env", []byte(strings.Join(updatedLines, "\n")+"\n"), 0644)
	if err != nil {
		fmt.Printf("Error writing .env file: %v\n", err)
		return
	}

	fmt.Println("\n" + trans.SaveSuccess)
}

// copyFile 复制文件
func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0644)
}

// readEnvFileWithComments 读取 .env 文件，保留所有行（包括注释）
func readEnvFileWithComments(filename string) ([]string, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	content := string(data)
	lines := strings.Split(content, "\n")
	// 移除最后的空行（如果有）
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines, nil
}

// updateEnvLines 更新配置行，保留注释和格式
func updateEnvLines(lines []string, env map[string]string) []string {
	updated := make([]string, 0, len(lines))
	updatedKeys := make(map[string]bool)

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// 保留注释和空行
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			updated = append(updated, line)
			continue
		}

		// 解析 KEY=VALUE
		parts := strings.SplitN(trimmed, "=", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			// 如果配置中有这个 key，使用新值
			if newValue, exists := env[key]; exists {
				updated = append(updated, fmt.Sprintf("%s=%s", key, newValue))
				updatedKeys[key] = true
			} else {
				// 保留原始行
				updated = append(updated, line)
			}
		} else {
			// 保留无法解析的行
			updated = append(updated, line)
		}
	}

	// 添加新增的配置项（不在原文件中的）
	newKeys := make([]string, 0)
	for key := range env {
		if !updatedKeys[key] {
			newKeys = append(newKeys, key)
		}
	}
	if len(newKeys) > 0 {
		sort.Strings(newKeys)
		updated = append(updated, "")
		updated = append(updated, "# ============================================")
		updated = append(updated, "# Additional Configuration (Added by Wizard)")
		updated = append(updated, "# ============================================")
		for _, key := range newKeys {
			updated = append(updated, fmt.Sprintf("%s=%s", key, env[key]))
		}
	}

	return updated
}

// fallbackSaveToEnv 传统保存方式（当无法读取 .env.example 时使用）
func fallbackSaveToEnv(env map[string]string, trans *WizardTranslation) {
	f, err := os.OpenFile(".env", os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		fmt.Printf("Error creating .env file: %v\n", err)
		return
	}
	defer f.Close()

	// Sort keys for consistent output
	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	f.WriteString("# MuseBot Generated Config\n")
	for _, k := range keys {
		f.WriteString(fmt.Sprintf("%s=%s\n", k, env[k]))
	}

	fmt.Println("\n" + trans.SaveSuccess)
}
