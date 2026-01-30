package utils

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/yincongcyincong/MuseBot/logger"
)

// AgentOutput 表示 agent 命令的输出结构
type AgentOutput struct {
	Type      string                   `json:"type"`
	Subtype   string                   `json:"subtype,omitempty"`
	Message   *AgentMessage            `json:"message,omitempty"`
	SessionID string                   `json:"session_id,omitempty"`
	Timestamp int64                    `json:"timestamp_ms,omitempty"`
	Text      string                   `json:"text,omitempty"`
	Result    string                   `json:"result,omitempty"`
	IsError   bool                     `json:"is_error,omitempty"`
	Duration  int64                    `json:"duration_ms,omitempty"`
	RequestID string                   `json:"request_id,omitempty"`
	Content   []map[string]interface{} `json:"content,omitempty"`
}

// AgentMessage 表示 agent 消息结构
type AgentMessage struct {
	Role    string                   `json:"role"`
	Content []map[string]interface{} `json:"content"`
}

// AgentConfig agent 工具配置
type AgentConfig struct {
	Command      string        // agent 命令路径，默认为 "agent"
	Args         []string      // 额外的命令行参数
	Sandbox      bool          // 是否启用沙箱
	StreamOutput bool          // 是否流式输出
	Timeout      time.Duration // 超时时间
	WorkDir      string        // 工作目录（session workdir）
	Continue     bool          // 是否继续会话
	ResumeChatId string        // 要恢复的会话 chatId（使用 --resume 参数）
	ChatId       string        // 当前会话的 chat_id，用于保存 session_id 映射
}

// DefaultAgentConfig 返回默认的 agent 配置
func DefaultAgentConfig() *AgentConfig {
	return &AgentConfig{
		Command:      "agent",
		Args:         []string{"-p", "-f", "--sandbox", "enabled", "--stream-partial-output", "--output-format", "stream-json"},
		Sandbox:      true,
		StreamOutput: true,
		Timeout:      5 * time.Minute,
		Continue:     false,
	}
}

// BuildAgentArgs 构建 agent 命令参数
func (c *AgentConfig) BuildAgentArgs(input string) []string {
	args := make([]string, 0)
	args = append(args, c.Args...)

	// 如果指定了 ResumeChatId，使用 --resume 参数恢复特定会话
	if c.ResumeChatId != "" {
		args = append(args, "--resume", c.ResumeChatId)
	} else if c.Continue {
		// 否则，如果继续会话，添加 --continue 参数（恢复最后一个会话）
		args = append(args, "--continue")
	}

	// 直接添加用户输入，不要加引号
	// exec.Command 不会解析引号，引号会被当作参数的一部分
	args = append(args, input)

	return args
}

// ExecuteAgentWithCallback 执行 agent 命令并通过回调函数实时返回结果
func ExecuteAgentWithCallback(ctx context.Context, input string, config *AgentConfig, callback func(string)) (string, error) {
	if config == nil {
		config = DefaultAgentConfig()
	}

	// 构建命令参数
	args := config.BuildAgentArgs(input)

	// 创建命令上下文，支持超时
	// 如果超时时间为0，表示无超时限制，直接使用原始 context
	var cmdCtx context.Context
	var cancel context.CancelFunc
	if config.Timeout > 0 {
		cmdCtx, cancel = context.WithTimeout(ctx, config.Timeout)
		defer cancel()
	} else {
		cmdCtx = ctx
	}

	// 确保 Command 只包含可执行文件名，不包含参数
	// 如果 CMD_AGENT_COMMAND 包含了完整命令，需要解析
	command := config.Command
	if strings.Contains(command, " ") {
		// 如果命令包含空格，可能是完整命令，需要解析
		parts := strings.Fields(command)
		command = parts[0]
		// 将剩余部分添加到 args 前面
		if len(parts) > 1 {
			args = append(parts[1:], args...)
		}
	}

	cmd := exec.CommandContext(cmdCtx, command, args...)

	logger.Debug("agent command", "command", command, "args", args)

	// 设置工作目录
	if config.WorkDir != "" {
		cmd.Dir = config.WorkDir
	} else {
		cmd.Dir = "."
	}

	// 获取标准输出管道
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	// 获取标准错误管道
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return "", fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	// 启动命令
	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("failed to start agent command: %w", err)
	}

	// 读取标准输出
	var result strings.Builder
	var finalResult string
	var hasError bool
	var thinkingStarted bool  // 标记是否已经开始输出 thinking 内容
	var lastSentText string   // 记录上次发送的文本，避免重复发送
	var agentSessionID string // 保存 agent 返回的 session_id

	// 使用 goroutine 读取标准输出
	outputDone := make(chan error, 1)
	go func() {
		defer func() {
			// 确保在 context 取消时也能退出
			if ctx.Err() != nil {
				outputDone <- ctx.Err()
				return
			}
		}()
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			// 检查 context 是否已取消
			select {
			case <-cmdCtx.Done():
				outputDone <- cmdCtx.Err()
				return
			default:
			}

			line := scanner.Text()
			if line == "" {
				continue
			}
			logger.Debug("agent output", "line", line)

			// 解析 JSON 行
			var output AgentOutput
			if err := json.Unmarshal([]byte(line), &output); err != nil {
				logger.Warn("failed to parse agent output line", "line", line, "err", err)
				continue
			}

			// 保存 session_id（从任何包含 session_id 的输出中提取）
			if output.SessionID != "" && agentSessionID == "" {
				agentSessionID = output.SessionID
				// 保存 session_id 到文件，以便后续恢复
				if config.WorkDir != "" && config.ChatId != "" {
					saveAgentSessionID(config.WorkDir, config.ChatId, agentSessionID)
					logger.Debug("saved agent session_id", "chat_id", config.ChatId, "session_id", agentSessionID, "workDir", config.WorkDir)
				}
			}

			// 处理不同类型的输出
			switch output.Type {
			case "thinking":
				if callback == nil {
					break
				}

				// 如果之前没有输出过 thinking 内容，先输出开始标签
				if !thinkingStarted {
					callback("<think>")
					thinkingStarted = true
					logger.Debug("thinking started, sent opening tag")
				}
				if output.Text != "" {
					callback(output.Text)
				}
				if output.Subtype == "completed" && thinkingStarted {
					callback("</think>")
					thinkingStarted = false
					logger.Debug("thinking completed, sent closing tag")
				}
			case "assistant":
				if output.Message != nil && len(output.Message.Content) > 0 {
					if text, ok := output.Message.Content[0]["text"].(string); ok {
						textPreview := text
						if len(text) > 100 {
							textPreview = text[:100] + "..."
						}
						lastSentPreview := lastSentText
						if len(lastSentText) > 100 {
							lastSentPreview = lastSentText[:100] + "..."
						}
						logger.Debug("assistant message received", "text", textPreview, "length", len(text), "lastSentText", lastSentPreview, "equal", text == lastSentText)
						// 避免重复发送相同的内容
						if text != lastSentText {
							if callback != nil {
								logger.Debug("sending assistant message via callback", "text", textPreview)
								callback(text)
							}
							lastSentText = text
						} else {
							logger.Debug("skipping duplicate assistant message", "text", textPreview)
						}
						result.WriteString(text)
					}
				}
			case "result":
				if output.Result != "" {
					finalResult = output.Result
					// result 类型通常包含最终结果，如果已经有 assistant 消息，不需要再次通过 callback 发送
					// 只在没有 assistant 消息时才通过 callback 发送
					if result.Len() == 0 && callback != nil {
						callback(output.Result)
					}
				}
				if output.IsError {
					hasError = true
				}
			case "error":
				hasError = true
				if output.Text != "" {
					errorMsg := fmt.Sprintf("[错误] %s", output.Text)
					if callback != nil {
						callback(errorMsg)
					}
					result.WriteString(errorMsg)
				}
			}
		}
		outputDone <- scanner.Err()
	}()

	// 读取标准错误
	stderrDone := make(chan error, 1)
	var stderrText strings.Builder
	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			stderrText.WriteString(scanner.Text())
			stderrText.WriteString("\n")
		}
		if stderrText.Len() > 0 {
			logger.Warn("agent stderr", "output", stderrText.String())
		}
		stderrDone <- scanner.Err()
	}()

	// 等待命令完成
	if err := cmd.Wait(); err != nil {
		<-outputDone
		<-stderrDone
		errMsg := fmt.Sprintf("agent command failed: %v", err)
		if stderrText.Len() > 0 {
			errMsg += fmt.Sprintf("; stderr: %s", stderrText.String())
		}
		return "", fmt.Errorf("%s", errMsg)
	}

	// 等待输出读取完成
	<-outputDone
	<-stderrDone

	// 返回最终结果
	if finalResult != "" {
		if hasError {
			return finalResult, fmt.Errorf("agent execution failed")
		}
		return finalResult, nil
	}

	resultStr := result.String()
	if resultStr == "" {
		return "", fmt.Errorf("no output from agent command")
	}

	if hasError {
		return resultStr, fmt.Errorf("agent execution completed with errors")
	}

	return resultStr, nil
}

// AgentSessionMap 保存 chat_id 到 agent session_id 的映射
type AgentSessionMap struct {
	mu   sync.RWMutex
	Map  map[string]string `json:"map"` // chat_id -> agent_session_id
	File string            `json:"-"`   // 映射文件路径
}

// saveAgentSessionID 保存 chat_id 到 agent session_id 的映射
func saveAgentSessionID(workDir, chatId, sessionID string) {
	sessionFile := filepath.Join(workDir, ".agent_session_map.json")

	// 加载现有的映射
	sessionMap := &AgentSessionMap{
		Map:  make(map[string]string),
		File: sessionFile,
	}
	if data, err := os.ReadFile(sessionFile); err == nil {
		if err := json.Unmarshal(data, sessionMap); err != nil {
			logger.Warn("failed to parse agent session map", "err", err, "sessionFile", sessionFile)
			// 如果解析失败，使用空映射
			sessionMap.Map = make(map[string]string)
		}
	}

	// 更新映射
	sessionMap.mu.Lock()
	sessionMap.Map[chatId] = sessionID
	sessionMap.mu.Unlock()

	// 保存到文件
	data, err := json.MarshalIndent(sessionMap.Map, "", "  ")
	if err != nil {
		logger.Warn("failed to marshal agent session map", "err", err, "sessionFile", sessionFile)
		return
	}
	if err := os.WriteFile(sessionFile, data, 0644); err != nil {
		logger.Warn("failed to save agent session map", "err", err, "sessionFile", sessionFile)
	}
}

// LoadAgentSessionID 从文件加载指定 chat_id 的 agent session_id（导出函数，供外部调用）
func LoadAgentSessionID(workDir, chatId string) string {
	sessionFile := filepath.Join(workDir, ".agent_session_map.json")

	// 尝试加载新的映射格式
	data, err := os.ReadFile(sessionFile)
	if err != nil {
		// 如果文件不存在，尝试加载旧的单 session_id 格式（向后兼容）
		oldSessionFile := filepath.Join(workDir, ".agent_session_id")
		if oldData, err := os.ReadFile(oldSessionFile); err == nil {
			oldSessionID := strings.TrimSpace(string(oldData))
			if oldSessionID != "" {
				// 迁移到新格式
				sessionMap := map[string]string{
					chatId: oldSessionID,
				}
				if newData, err := json.MarshalIndent(sessionMap, "", "  "); err == nil {
					os.WriteFile(sessionFile, newData, 0644)
					// 删除旧文件
					os.Remove(oldSessionFile)
				}
				return oldSessionID
			}
		}
		return ""
	}

	var sessionMap map[string]string
	if err := json.Unmarshal(data, &sessionMap); err != nil {
		logger.Warn("failed to parse agent session map", "err", err, "sessionFile", sessionFile)
		return ""
	}

	return sessionMap[chatId]
}
