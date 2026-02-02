package llm

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/sashabaranov/go-openai"
	"github.com/yincongcyincong/MuseBot/conf"
	"github.com/yincongcyincong/MuseBot/db"
	"github.com/yincongcyincong/MuseBot/logger"
	"github.com/yincongcyincong/MuseBot/metrics"
	"github.com/yincongcyincong/MuseBot/param"
	"github.com/yincongcyincong/MuseBot/utils"
)

// AgentType 定义支持的 Agent 类型
type AgentType string

const (
	AgentTypeQoder    AgentType = "qoder"    // Qoder Agent
	AgentTypeOpenCode AgentType = "opencode" // OpenCode Agent
	AgentTypeCursor   AgentType = "cursor"   // Cursor Agent
	AgentTypeCustom   AgentType = "custom"   // 自定义 Agent
)

// AgentProfile 定义 Agent 的配置模板
type AgentProfile struct {
	Type        AgentType // Agent 类型
	Command     string    // 命令路径
	DefaultArgs []string  // 默认参数
	StreamArgs  []string  // 流式输出参数
	ResumeFlag  string    // 恢复会话的参数标志（如 --resume）
	WorkDirFlag string    // 工作目录参数标志（如 --workdir）
}

// BuiltinAgentProfiles 内置的 Agent 配置
var BuiltinAgentProfiles = map[AgentType]*AgentProfile{
	AgentTypeQoder: {
		Type:        AgentTypeQoder,
		Command:     "qodercli",
		DefaultArgs: []string{"-p", "--yolo"},
		StreamArgs:  []string{"-f=stream-json"},
		ResumeFlag:  "-r",
		WorkDirFlag: "-w",
	},
	AgentTypeOpenCode: {
		Type:        AgentTypeOpenCode,
		Command:     "opencode",
		DefaultArgs: []string{"run"},
		StreamArgs:  []string{}, // opencode run 默认通常就是流式的
		ResumeFlag:  "-s",
		WorkDirFlag: "", // 通过 cmd.Dir 处理
	},
	AgentTypeCursor: {
		Type:        AgentTypeCursor,
		Command:     "agent",
		DefaultArgs: []string{"--print", "--output-format", "json", "--sandbox", "enabled", "-f"},
		StreamArgs:  []string{"--output-format", "stream-json", "--stream-partial-output"},
		ResumeFlag:  "--resume",
		WorkDirFlag: "--workspace",
	},
}

// AgentMessage 表示 agent 消息
type AgentMessage struct {
	Role    string `json:"role"` // user/assistant/system
	Content string `json:"content"`
	Type    string `json:"type"` // text/thinking/result
}

// AgentReq 实现 LLMClient 接口
type AgentReq struct {
	// Agent 类型和配置
	AgentType AgentType     // Agent 类型
	Profile   *AgentProfile // Agent 配置模板

	// 会话管理
	AgentMsgs []AgentMessage // 消息历史
	SessionID string         // 当前会话ID
	WorkDir   string         // 工作目录

	// 运行时配置
	Command       string        // 实际命令路径（可覆盖 Profile.Command）
	ExtraArgs     []string      // 额外的命令参数
	Timeout       time.Duration // 超时时间
	StreamEnabled bool          // 是否启用流式输出
}

// Send 实现流式输出
func (a *AgentReq) Send(ctx context.Context, l *LLM) error {
	if l.OverLoop() {
		return errors.New("too many loops")
	}

	start := time.Now()

	// 构建命令
	cmd := a.buildCommand(ctx, l)

	// 执行命令并流式解析
	err := a.executeStreamCommand(ctx, cmd, l)

	metrics.APIRequestDuration.WithLabelValues(l.Model).Observe(time.Since(start).Seconds())

	return err
}

// SyncSend 实现同步输出
func (a *AgentReq) SyncSend(ctx context.Context, l *LLM) (string, error) {
	// 创建一个临时的消息收集器
	var result strings.Builder

	// 构建命令
	cmd := a.buildCommand(ctx, l)

	// 执行命令
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("agent command failed: %w", err)
	}

	// 解析输出
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}

		var agentOutput utils.AgentOutput
		if err := json.Unmarshal([]byte(line), &agentOutput); err != nil {
			continue
		}

		// 提取文本内容
		content := a.extractContent(&agentOutput)
		if content != "" {
			result.WriteString(content)
		}
	}

	return result.String(), nil
}

// GetMessage 添加消息
func (a *AgentReq) GetMessage(role, msg string) {
	a.AgentMsgs = append(a.AgentMsgs, AgentMessage{
		Role:    role,
		Content: msg,
		Type:    "text",
	})
}

// GetImageMessage 处理图片消息（Agent 暂不支持，转为文本）
func (a *AgentReq) GetImageMessage(image [][]byte, msg string) {
	a.GetMessage(openai.ChatMessageRoleUser, fmt.Sprintf("%s [Image attached]", msg))
}

// GetAudioMessage 处理音频消息（Agent 暂不支持）
func (a *AgentReq) GetAudioMessage(audio []byte, msg string) {
	a.GetMessage(openai.ChatMessageRoleUser, msg)
}

// AppendMessages 追加消息
func (a *AgentReq) AppendMessages(client LLMClient) {
	if agentClient, ok := client.(*AgentReq); ok {
		a.AgentMsgs = append(a.AgentMsgs, agentClient.AgentMsgs...)
	}
}

// GetModel 获取模型信息
func (a *AgentReq) GetModel(l *LLM) {
	// Agent 使用 agent type 作为 model
	l.Model = fmt.Sprintf("agent-%s", a.AgentType)
}

// buildCommand 构建命令
func (a *AgentReq) buildCommand(ctx context.Context, l *LLM) *exec.Cmd {
	// 构建参数
	args := a.buildArgs()

	// 添加用户输入
	args = append(args, l.Content)

	// 使用实际命令路径，如果没有设置则使用 Profile 的默认值
	command := a.Command
	if command == "" && a.Profile != nil {
		command = a.Profile.Command
	}

	logger.InfoCtx(ctx, "agent command", "command", command, "args", args)

	cmd := exec.CommandContext(ctx, command, args...)

	// 设置工作目录
	if a.WorkDir != "" {
		cmd.Dir = a.WorkDir
	}

	return cmd
}

// buildArgs 构建命令参数
func (a *AgentReq) buildArgs() []string {
	if a.Profile == nil {
		// 如果没有 Profile，使用默认配置（向后兼容）
		return []string{"-p", "-f", "--sandbox", "enabled",
			"--stream-partial-output", "--output-format", "stream-json"}
	}

	args := make([]string, 0)

	// 1. 添加默认参数
	args = append(args, a.Profile.DefaultArgs...)

	// 2. 根据 StreamEnabled 添加流式输出参数，否则使用标准 json 格式
	if a.StreamEnabled {
		args = append(args, a.Profile.StreamArgs...)
	} else if a.AgentType == AgentTypeQoder {
		// 非流式模式下，qoder 使用 json 格式
		args = append(args, "-f=json")
	}

	// 3. 如果有会话ID，使用恢复参数
	if a.SessionID != "" && a.Profile.ResumeFlag != "" {
		args = append(args, a.Profile.ResumeFlag, a.SessionID)
	}

	// 4. 添加工作目录
	if a.WorkDir != "" && a.Profile.WorkDirFlag != "" {
		args = append(args, a.Profile.WorkDirFlag, a.WorkDir)
	}

	// 5. 添加额外参数
	args = append(args, a.ExtraArgs...)

	return args
}

// executeStreamCommand 执行流式命令
func (a *AgentReq) executeStreamCommand(ctx context.Context, cmd *exec.Cmd, l *LLM) error {
	// 获取输出管道
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	// 关闭 stdin，防止命令等待输入
	cmd.Stdin = nil

	// 启动命令
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start agent command: %w", err)
	}

	// 解析 JSON 流输出
	msgInfoContent := &param.MsgInfo{SendLen: FirstSendLen}
	scanner := bufio.NewScanner(stdout)

	// 读取 stderr
	var stderrOutput strings.Builder
	go func() {
		stderrScanner := bufio.NewScanner(stderr)
		for stderrScanner.Scan() {
			line := stderrScanner.Text()
			logger.WarnCtx(ctx, "agent stderr", "output", line)
			stderrOutput.WriteString(line)
			stderrOutput.WriteString("\n")
		}
	}()

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		line := scanner.Text()
		if line == "" {
			continue
		}

		logger.Debug("agent output", "line", line)

		var output utils.AgentOutput
		if err := json.Unmarshal([]byte(line), &output); err != nil {
			logger.WarnCtx(ctx, "failed to parse agent output", "line", line, "err", err)
			continue
		}

		// 保存 session_id
		if output.SessionID != "" && a.SessionID == "" {
			a.SessionID = output.SessionID
			a.saveSessionID(l.ChatId)
			logger.InfoCtx(ctx, "saved agent session", "chat_id", l.ChatId, "session_id", a.SessionID)
		}

		// 处理不同类型的输出
		switch output.Type {
		case "thinking":
			// thinking 内容特殊格式化
			content := a.formatThinkingContent(&output)
			if content != "" {
				msgInfoContent = l.SendMsg(msgInfoContent, content)
			}

		case "assistant", "result":
			// 正常助手回复
			content := a.extractContent(&output)
			if content != "" {
				msgInfoContent = l.SendMsg(msgInfoContent, content)
			}

		case "error":
			// 错误处理
			return fmt.Errorf("agent error: %s", output.Text)
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("scanner error: %w", err)
	}

	// 发送最后的消息
	if l.MessageChan != nil && len(strings.TrimSpace(msgInfoContent.Content)) > 0 {
		if conf.BaseConfInfo.Powered != "" {
			msgInfoContent.Content = msgInfoContent.Content + "\n\n" + conf.BaseConfInfo.Powered
		}
		l.MessageChan <- msgInfoContent
	}

	// 等待命令完成
	if err := cmd.Wait(); err != nil {
		// 如果 stderr 有内容，包含在错误信息中
		errMsg := fmt.Sprintf("agent command failed: %v", err)
		if stderrContent := stderrOutput.String(); stderrContent != "" {
			errMsg += "\nstderr: " + stderrContent
		}
		return fmt.Errorf("%s", errMsg)
	}

	return nil
}

// formatThinkingContent 格式化 thinking 内容
func (a *AgentReq) formatThinkingContent(output *utils.AgentOutput) string {
	content := output.Text
	if content == "" && output.Message != nil {
		content = a.extractFromMessage(output.Message)
	}

	// 如果内容不为空且不包含 <think> 标签，添加标签
	if content != "" && !strings.Contains(content, "<think>") {
		content = "<think>" + content
	}

	return content
}

// extractContent 从 output 中提取文本内容
func (a *AgentReq) extractContent(output *utils.AgentOutput) string {
	if output.Text != "" {
		return output.Text
	}
	if output.Result != "" {
		return output.Result
	}
	if output.Message != nil {
		return a.extractFromMessage(output.Message)
	}
	return ""
}

// extractFromMessage 从 Message 中提取内容
func (a *AgentReq) extractFromMessage(msg *utils.AgentMessage) string {
	if msg == nil {
		return ""
	}

	var result strings.Builder
	for _, content := range msg.Content {
		if text, ok := content["text"].(string); ok {
			result.WriteString(text)
		}
	}
	return result.String()
}

// saveSessionID 保存会话ID
func (a *AgentReq) saveSessionID(chatId string) {
	if a.WorkDir == "" || chatId == "" {
		return
	}

	sessionFile := filepath.Join(a.WorkDir, ".agent_session_map.json")

	// 读取现有映射
	sessionMap := make(map[string]string)
	if data, err := os.ReadFile(sessionFile); err == nil {
		json.Unmarshal(data, &sessionMap)
	}

	// 更新映射
	sessionMap[chatId] = a.SessionID

	// 保存到文件
	if data, err := json.MarshalIndent(sessionMap, "", "  "); err == nil {
		os.WriteFile(sessionFile, data, 0644)
	}
}

// loadSessionID 加载会话ID
func (a *AgentReq) loadSessionID(chatId string) string {
	if a.WorkDir == "" || chatId == "" {
		return ""
	}

	sessionFile := filepath.Join(a.WorkDir, ".agent_session_map.json")
	data, err := os.ReadFile(sessionFile)
	if err != nil {
		return ""
	}

	var sessionMap map[string]string
	if err := json.Unmarshal(data, &sessionMap); err != nil {
		return ""
	}

	return sessionMap[chatId]
}

// 获取配置的 Agent 类型
func getConfiguredAgentType(ctx context.Context) AgentType {
	// 1. 优先从用户配置读取
	userInfo := db.GetCtxUserInfo(ctx)
	if userInfo != nil && userInfo.LLMConfigRaw != nil {
		if agentType := userInfo.LLMConfigRaw.AgentType; agentType != "" {
			return AgentType(agentType)
		}
	}

	// 2. 从环境变量读取
	if envType := os.Getenv("CMD_AGENT_TYPE"); envType != "" {
		return AgentType(envType)
	}

	// 3. 默认使用 qoder
	return AgentTypeQoder
}

// 获取 Agent Profile（支持自定义配置）
func getAgentProfile(agentType AgentType) *AgentProfile {
	// 1. 查找内置配置
	if profile, ok := BuiltinAgentProfiles[agentType]; ok {
		return profile
	}

	// 2. 如果是自定义类型，从配置文件加载
	if agentType == AgentTypeCustom {
		return loadCustomAgentProfile()
	}

	// 3. 默认返回 qoder 配置
	return BuiltinAgentProfiles[AgentTypeQoder]
}

// 加载自定义 Agent 配置
func loadCustomAgentProfile() *AgentProfile {
	// 从配置文件或环境变量加载自定义 Agent 配置
	return &AgentProfile{
		Type:        AgentTypeCustom,
		Command:     os.Getenv("CMD_AGENT_CUSTOM_COMMAND"),
		DefaultArgs: parseArgs(os.Getenv("CMD_AGENT_CUSTOM_DEFAULT_ARGS")),
		StreamArgs:  parseArgs(os.Getenv("CMD_AGENT_CUSTOM_STREAM_ARGS")),
		ResumeFlag:  os.Getenv("CMD_AGENT_CUSTOM_RESUME_FLAG"),
		WorkDirFlag: os.Getenv("CMD_AGENT_CUSTOM_WORKDIR_FLAG"),
	}
}

// 获取 Agent 命令路径（允许覆盖）
func getAgentCommand(agentType AgentType) string {
	// 从环境变量读取特定类型的命令路径
	envKey := fmt.Sprintf("CMD_AGENT_%s_COMMAND", strings.ToUpper(string(agentType)))
	if cmd := os.Getenv(envKey); cmd != "" {
		return cmd
	}

	// 返回空字符串，使用 Profile 的默认命令
	return ""
}

// 获取额外的命令参数
func getAgentExtraArgs(agentType AgentType) []string {
	// 从环境变量读取额外参数
	envKey := fmt.Sprintf("CMD_AGENT_%s_EXTRA_ARGS", strings.ToUpper(string(agentType)))
	if args := os.Getenv(envKey); args != "" {
		return parseArgs(args)
	}
	return []string{}
}

// 获取超时时间
func getAgentTimeout() time.Duration {
	if timeout := os.Getenv("CMD_AGENT_TIMEOUT"); timeout != "" {
		if t, err := strconv.Atoi(timeout); err == nil && t >= 0 {
			return time.Duration(t) * time.Second
		}
	}
	return 5 * time.Minute
}

// 获取工作目录
func getAgentWorkDir(userId string) string {
	sanitizedUserId := sanitizeSessionID(userId)
	workDir := utils.GetAbsPath(filepath.Join("data", "agent_sessions", sanitizedUserId))
	os.MkdirAll(workDir, 0755)
	return workDir
}

// 清理 sessionID 中的特殊字符
func sanitizeSessionID(sessionID string) string {
	replacer := strings.NewReplacer(
		"/", "_",
		"\\", "_",
		"..", "__",
		":", "_",
	)
	return replacer.Replace(sessionID)
}

// 解析参数字符串
func parseArgs(argsStr string) []string {
	if argsStr == "" {
		return []string{}
	}
	// 简单的参数解析，支持空格分隔
	return strings.Fields(argsStr)
}
