# CMD_AGENT 架构分析与重构方案

## 一、当前架构分析

### 1.1 CMD_AGENT 核心组件

#### 1.1.1 配置层 (`conf/conf.go`)
```go
type BaseConf struct {
    CmdAgentEnabled   bool   `json:"cmd_agent_enabled"`  // 启用标志
    // 通过环境变量配置:
    // - CMD_AGENT_ENABLED: true/false
    // - CMD_AGENT_COMMAND: agent 命令路径
    // - CMD_AGENT_TIMEOUT: 超时时间(秒)
}
```

#### 1.1.2 执行层 (`utils/agent.go`)
**核心结构:**
```go
type AgentConfig struct {
    Command      string        // agent 命令路径
    Args         []string      // 命令行参数
    Sandbox      bool          // 沙箱模式
    StreamOutput bool          // 流式输出
    Timeout      time.Duration // 超时时间
    WorkDir      string        // 工作目录
    Continue     bool          // 继续会话
    ResumeChatId string        // 恢复会话ID
    ChatId       string        // 当前会话ID
}

type AgentOutput struct {
    Type      string                   `json:"type"`       // thinking/assistant/result/error
    Subtype   string                   `json:"subtype,omitempty"`
    Message   *AgentMessage            `json:"message,omitempty"`
    SessionID string                   `json:"session_id,omitempty"`
    Text      string                   `json:"text,omitempty"`
    Result    string                   `json:"result,omitempty"`
    IsError   bool                     `json:"is_error,omitempty"`
    // ...
}
```

**核心函数:**
- `ExecuteAgentWithCallback()`: 执行 agent 命令，通过 callback 实时流式输出
- `LoadAgentSessionID()`: 加载会话ID，支持多会话管理
- 默认参数: `-p -f --sandbox enabled --stream-partial-output --output-format stream-json`

#### 1.1.3 调用层 (`robot/robot.go`)
```go
func (r *RobotInfo) ExecLLM(content string, msgChan *MsgChan) {
    // 如果启用了 CMD_AGENT_ENABLED，直接使用 agent 命令处理
    if conf.BaseConfInfo.CmdAgentEnabled {
        r.execAgentCmd(content, msgChan)
        return
    }
    
    // 否则使用正常的 LLM 流程
    llmClient := llm.NewLLM(...)
    err := llmClient.CallLLM()
}

func (r *RobotInfo) execAgentCmd(content string, msgChan *MsgChan) {
    // 1. 创建用户专属工作目录: data/agent_sessions/{userId}/
    // 2. 加载或创建会话
    // 3. 构建 AgentConfig
    // 4. 通过 callback 实时流式输出到 msgChan
    // 5. 会话管理(保存 session_id 映射)
}
```

### 1.2 当前工作流程

```
用户输入
    ↓
[robot.ExecLLM] 判断是否启用 CMD_AGENT
    ↓
    ├─→ [是] execAgentCmd()
    │       ↓
    │   [构建 AgentConfig]
    │       ↓
    │   [ExecuteAgentWithCallback]
    │       ↓
    │   [exec.Command 执行外部 agent 命令]
    │       ↓
    │   [实时解析 JSON 流输出]
    │       ↓
    │   [callback → msgChan → 用户]
    │
    └─→ [否] 正常 LLM 流程
            ↓
        [llm.NewLLM]
            ↓
        [llmClient.CallLLM]
            ↓
        [根据类型选择 LLMClient]
            ↓
        [OpenAIReq/OllamaReq/GeminiReq...]
            ↓
        [Send() → 流式输出]
```

### 1.3 当前 LLM 架构

#### 1.3.1 LLM 核心接口 (`llm/llm.go`)
```go
type LLM struct {
    MessageChan      chan *param.MsgInfo
    HTTPMsgChan      chan string
    Content          string
    Images           [][]byte
    Model            string
    LLMClient        LLMClient     // 策略模式: 多种 LLM 实现
    DeepseekTools    []godeepseek.Tool
    OpenAITools      []openai.Tool
    GeminiTools      []*genai.Tool
    // ...
}

type LLMClient interface {
    Send(ctx context.Context, l *LLM) error
    SyncSend(ctx context.Context, l *LLM) (string, error)
    GetMessage(role, msg string)
    GetImageMessage(image [][]byte, msg string)
    GetAudioMessage(audio []byte, msg string)
    AppendMessages(client LLMClient)
    GetModel(l *LLM)
}
```

#### 1.3.2 现有 LLMClient 实现
- **OpenAIReq**: OpenAI/Deepseek/Aliyun/Vol/AI302/OpenRouter/ChatAnyWhere
- **OllamaReq**: Ollama 本地模型
- **GeminiReq**: Google Gemini
- 每个实现都支持:
  - 流式输出 (Send)
  - 同步输出 (SyncSend)
  - 工具调用 (Tool Calling/Function Calling)
  - 多模态 (文本/图片/音频)

---

## 二、问题分析

### 2.1 架构层面的问题

1. **路由逻辑分离**: CMD_AGENT 通过 if 判断独立处理，与 LLM 体系完全隔离
2. **重复逻辑**: execAgentCmd 和 LLM.CallLLM 都处理消息流、会话管理、错误处理
3. **不一致的接口**: Agent 使用 callback 函数，LLM 使用 MessageChan
4. **扩展性差**: 如果要添加更多 agent 类型，需要修改多处代码
5. **工具调用割裂**: Agent 有自己的工具系统，LLM 也有 MCP 工具系统

### 2.2 代码质量问题

1. **硬编码**: agent 命令参数硬编码在 DefaultAgentConfig 中
2. **会话管理分散**: Agent 有自己的会话管理(session_id)，LLM 有自己的上下文管理
3. **错误处理不统一**: Agent 和 LLM 的错误处理逻辑不同
4. **测试困难**: 依赖外部命令，难以进行单元测试

---

## 三、重构方案：将 CMD_AGENT 虚拟为特殊的 LLM Client

### 3.1 设计思路

**核心理念**: 将 Agent 视为一种特殊的 LLM Provider，遵循统一的 LLMClient 接口

```
                    LLMClient Interface
                           ↑
        ┌──────────────────┼──────────────────┐
        │                  │                  │
    OpenAIReq          AgentReq          GeminiReq
   (API-based)     (Command-based)      (API-based)
```

### 3.2 新架构设计

#### 3.2.1 创建 AgentReq 实现 LLMClient 接口

```go
// llm/agent.go

// AgentType 定义支持的 Agent 类型
type AgentType string

const (
    AgentTypeQoder     AgentType = "qoder"      // Qoder Agent
    AgentTypeOpenCode  AgentType = "opencode"   // OpenCode Agent
    AgentTypeCursor    AgentType = "cursor"     // Cursor Agent
    AgentTypeCustom    AgentType = "custom"     // 自定义 Agent
)

// AgentProfile 定义 Agent 的配置模板
type AgentProfile struct {
    Type         AgentType      // Agent 类型
    Command      string         // 命令路径
    DefaultArgs  []string       // 默认参数
    StreamArgs   []string       // 流式输出参数
    ResumeFlag   string         // 恢复会话的参数标志（如 --resume）
    WorkDirFlag  string         // 工作目录参数标志（如 --workdir）
}

// 内置的 Agent 配置
var BuiltinAgentProfiles = map[AgentType]*AgentProfile{
    AgentTypeQoder: {
        Type:        AgentTypeQoder,
        Command:     "qoder",
        DefaultArgs: []string{"-p", "-f", "--sandbox", "enabled"},
        StreamArgs:  []string{"--stream-partial-output", "--output-format", "stream-json"},
        ResumeFlag:  "--resume",
        WorkDirFlag: "--workdir",
    },
    AgentTypeOpenCode: {
        Type:        AgentTypeOpenCode,
        Command:     "opencode",
        DefaultArgs: []string{"--interactive", "--safe-mode"},
        StreamArgs:  []string{"--stream", "--format", "json"},
        ResumeFlag:  "--continue",
        WorkDirFlag: "--workspace",
    },
    AgentTypeCursor: {
        Type:        AgentTypeCursor,
        Command:     "cursor-agent",
        DefaultArgs: []string{"--mode", "chat"},
        StreamArgs:  []string{"--streaming", "--json-output"},
        ResumeFlag:  "--session",
        WorkDirFlag: "--cwd",
    },
}

type AgentReq struct {
    // Agent 类型和配置
    AgentType      AgentType            // Agent 类型
    Profile        *AgentProfile        // Agent 配置模板
    
    // 会话管理
    AgentMsgs      []AgentMessage       // 消息历史
    SessionID      string               // 当前会话ID
    WorkDir        string               // 工作目录
    
    // 运行时配置
    Command        string               // 实际命令路径（可覆盖 Profile.Command）
    ExtraArgs      []string             // 额外的命令参数
    Timeout        time.Duration        // 超时时间
    StreamEnabled  bool                 // 是否启用流式输出
}

type AgentMessage struct {
    Role    string      `json:"role"`    // user/assistant/system
    Content string      `json:"content"`
    Type    string      `json:"type"`    // text/thinking/result
}

// 实现 LLMClient 接口
func (a *AgentReq) Send(ctx context.Context, l *LLM) error {
    // 1. 构建命令
    cmd := a.buildCommand(l)
    
    // 2. 执行命令并流式解析
    return a.executeStreamCommand(ctx, cmd, l)
}

func (a *AgentReq) SyncSend(ctx context.Context, l *LLM) (string, error) {
    // 同步执行，等待完整结果
    var result strings.Builder
    
    err := a.Send(ctx, l)
    return result.String(), err
}

func (a *AgentReq) GetMessage(role, msg string) {
    a.AgentMsgs = append(a.AgentMsgs, AgentMessage{
        Role:    role,
        Content: msg,
        Type:    "text",
    })
}

func (a *AgentReq) GetImageMessage(image [][]byte, msg string) {
    // Agent 暂不支持图片，可以转为文本描述
    a.GetMessage(openai.ChatMessageRoleUser, 
        fmt.Sprintf("%s [Image attached]", msg))
}

func (a *AgentReq) GetAudioMessage(audio []byte, msg string) {
    // Agent 暂不支持音频
    a.GetMessage(openai.ChatMessageRoleUser, msg)
}

func (a *AgentReq) AppendMessages(client LLMClient) {
    if agentClient, ok := client.(*AgentReq); ok {
        a.AgentMsgs = append(a.AgentMsgs, agentClient.AgentMsgs...)
    }
}

func (a *AgentReq) GetModel(l *LLM) {
    // Agent 使用配置的命令作为 "model"
    l.Model = "cmd-agent"
}

// 私有辅助方法
func (a *AgentReq) buildCommand(l *LLM) *exec.Cmd {
    // 构建命令，包含会话恢复逻辑
    args := a.buildArgs(l)
    
    // 使用实际命令路径，如果没有设置则使用 Profile 的默认值
    command := a.Command
    if command == "" && a.Profile != nil {
        command = a.Profile.Command
    }
    
    return exec.CommandContext(l.Ctx, command, args...)
}

func (a *AgentReq) buildArgs(l *LLM) []string {
    if a.Profile == nil {
        // 如果没有 Profile，使用默认配置（向后兼容）
        return []string{"-p", "-f", "--sandbox", "enabled", 
                       "--stream-partial-output", "--output-format", "stream-json"}
    }
    
    // 从 Profile 构建参数
    args := make([]string, 0)
    
    // 1. 添加默认参数
    args = append(args, a.Profile.DefaultArgs...)
    
    // 2. 根据 StreamEnabled 添加流式输出参数
    if a.StreamEnabled {
        args = append(args, a.Profile.StreamArgs...)
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

func (a *AgentReq) executeStreamCommand(ctx context.Context, cmd *exec.Cmd, l *LLM) error {
    // 设置工作目录
    if a.WorkDir != "" {
        cmd.Dir = a.WorkDir
    }
    
    // 获取输出管道
    stdout, _ := cmd.StdoutPipe()
    stderr, _ := cmd.StderrPipe()
    
    // 启动命令
    if err := cmd.Start(); err != nil {
        return err
    }
    
    // 解析 JSON 流输出
    msgInfoContent := &param.MsgInfo{SendLen: FirstSendLen}
    scanner := bufio.NewScanner(stdout)
    
    for scanner.Scan() {
        line := scanner.Text()
        var output utils.AgentOutput
        
        if err := json.Unmarshal([]byte(line), &output); err != nil {
            continue
        }
        
        // 保存 session_id
        if output.SessionID != "" && a.SessionID == "" {
            a.SessionID = output.SessionID
            // 保存到文件
            a.saveSessionID(l.ChatId)
        }
        
        // 处理不同类型的输出
        switch output.Type {
        case "thinking":
            // thinking 内容特殊处理
            content := a.formatThinkingContent(output)
            msgInfoContent = l.SendMsg(msgInfoContent, content)
            
        case "assistant", "result":
            // 正常助手回复
            content := a.extractContent(output)
            msgInfoContent = l.SendMsg(msgInfoContent, content)
            
        case "error":
            // 错误处理
            return fmt.Errorf("agent error: %s", output.Text)
        }
    }
    
    // 等待命令完成
    return cmd.Wait()
}

func (a *AgentReq) formatThinkingContent(output utils.AgentOutput) string {
    // 格式化 thinking 内容
    content := output.Text
    if !strings.Contains(content, "<think>") {
        content = "<think>" + content
    }
    return content
}

func (a *AgentReq) extractContent(output utils.AgentOutput) string {
    // 从 output 中提取文本内容
    if output.Text != "" {
        return output.Text
    }
    if output.Result != "" {
        return output.Result
    }
    // 从 Message.Content 提取
    if output.Message != nil {
        return a.extractFromMessage(output.Message)
    }
    return ""
}

func (a *AgentReq) saveSessionID(chatId string) {
    // 保存 session_id 到工作目录
    sessionFile := filepath.Join(a.WorkDir, ".agent_session_map.json")
    // ... 实现会话保存逻辑
}

func (a *AgentReq) loadSessionID(chatId string) string {
    // 从工作目录加载 session_id
    sessionFile := filepath.Join(a.WorkDir, ".agent_session_map.json")
    // ... 实现会话加载逻辑
    return ""
}
```

#### 3.2.2 修改 NewLLM 工厂函数

```go
// llm/llm.go
func NewLLM(opts ...Option) *LLM {
    l := new(LLM)
    l.Cs = new(param.ContextState)
    for _, opt := range opts {
        opt(l)
    }
    
    // 根据用户配置选择 LLMClient
    txtType := utils.GetTxtType(db.GetCtxUserInfo(l.Ctx).LLMConfigRaw)
    
    switch txtType {
    case param.CmdAgent:  // 新增: Agent 类型
        // 从用户配置或环境变量获取 Agent 类型
        agentType := getConfiguredAgentType(l.Ctx)
        profile := getAgentProfile(agentType)
        
        l.LLMClient = &AgentReq{
            AgentType:     agentType,
            Profile:       profile,
            Command:       getAgentCommand(agentType),      // 允许覆盖命令路径
            ExtraArgs:     getAgentExtraArgs(agentType),   // 用户自定义参数
            Timeout:       getAgentTimeout(),
            WorkDir:       getAgentWorkDir(l.UserId),
            StreamEnabled: conf.BaseConfInfo.IsStreaming,
            AgentMsgs:     []AgentMessage{},
        }
        
        // 加载会话ID
        if agentReq, ok := l.LLMClient.(*AgentReq); ok {
            agentReq.SessionID = agentReq.loadSessionID(l.ChatId)
        }
        
    case param.Ollama:
        l.LLMClient = &OllamaReq{
            ToolCall:           []godeepseek.ToolCall{},
            ToolMessage:        []godeepseek.ChatCompletionMessage{},
            CurrentToolMessage: []godeepseek.ChatCompletionMessage{},
        }
        
    default:
        l.LLMClient = &OpenAIReq{
            ToolCall:           []openai.ToolCall{},
            ToolMessage:        []openai.ChatCompletionMessage{},
            CurrentToolMessage: []openai.ChatCompletionMessage{},
        }
    }
    
    return l
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
    // 例如：CMD_AGENT_CUSTOM_COMMAND, CMD_AGENT_CUSTOM_ARGS 等
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

// 辅助函数
func getAgentTimeout() time.Duration {
    if timeout := os.Getenv("CMD_AGENT_TIMEOUT"); timeout != "" {
        if t, err := strconv.Atoi(timeout); err == nil && t >= 0 {
            return time.Duration(t) * time.Second
        }
    }
    return 5 * time.Minute
}

func getAgentWorkDir(userId string) string {
    sanitizedUserId := sanitizeSessionID(userId)
    workDir := utils.GetAbsPath(filepath.Join("data", "agent_sessions", sanitizedUserId))
    os.MkdirAll(workDir, 0755)
    return workDir
}

func parseArgs(argsStr string) []string {
    if argsStr == "" {
        return []string{}
    }
    // 简单的参数解析，支持空格分隔
    return strings.Fields(argsStr)
}
```

#### 3.2.3 添加 Agent 类型到参数定义

```go
// param/param.go
const (
    DeepSeek     = "deepseek"
    Ollama       = "ollama"
    CmdAgent     = "cmd-agent"  // 新增
    // ...
)
```

#### 3.2.4 简化 robot.ExecLLM

```go
// robot/robot.go
func (r *RobotInfo) ExecLLM(content string, msgChan *MsgChan) {
    chatId, msgId, userId := r.GetChatIdAndMsgIdAndUserID()
    
    if len(content) == 0 {
        // ... 处理空内容
        return
    }
    
    // 不再需要 CMD_AGENT 的特殊判断
    // 统一通过 LLM 架构处理
    r.InsertRecord()
    perMsgLen := r.Robot.getPerMsgLen()
    
    images := make([][]byte, 0)
    if len(r.Robot.getImage()) > 0 {
        images = append(images, r.Robot.getImage())
    }
    
    llmClient := llm.NewLLM(
        llm.WithChatId(chatId),
        llm.WithUserId(userId),
        llm.WithMsgId(msgId),
        llm.WithMessageChan(msgChan.NormalMessageChan),
        llm.WithHTTPMsgChan(msgChan.StrMessageChan),
        llm.WithContent(content),
        llm.WithPerMsgLen(perMsgLen),
        llm.WithCS(r.cs),
        llm.WithContext(r.Ctx),
        llm.WithImages(images),
        // ... 其他选项
    )
    
    err := llmClient.CallLLM()
    if err != nil {
        logger.ErrorCtx(r.Ctx, "get content fail", "err", err)
        r.SendMsg(chatId, err.Error(), msgId, "", nil)
    }
}

// 可以删除 execAgentCmd 方法
```

#### 3.2.5 更新用户配置逻辑

```go
// db/user.go 或相关配置文件
type LLMConfig struct {
    TxtModel  string `json:"txt_model"`
    TxtType   string `json:"txt_type"`   // "openai"/"deepseek"/"cmd-agent"
    // ...
}

// 当 CMD_AGENT_ENABLED=true 时，自动设置用户的 TxtType 为 "cmd-agent"
func InitUserConfig(userId string) {
    config := &LLMConfig{}
    
    if conf.BaseConfInfo.CmdAgentEnabled {
        config.TxtType = param.CmdAgent
        config.TxtModel = "agent"
    } else {
        // 使用默认配置
        config.TxtType = param.OpenAi
        config.TxtModel = openai.GPT3Dot5Turbo
    }
    
    // 保存配置
    SaveUserConfig(userId, config)
}
```

### 3.3 迁移路径

#### 阶段 1: 创建 AgentReq (新增代码)
1. 创建 `llm/agent.go` 文件
2. 实现 `AgentReq` 结构体和所有接口方法
3. 编写单元测试

#### 阶段 2: 集成到 LLM 框架 (修改现有代码)
1. 在 `param/param.go` 添加 `CmdAgent` 常量
2. 修改 `llm/llm.go` 的 `NewLLM()` 函数
3. 修改 `utils/llm.go` 的相关工具函数

#### 阶段 3: 简化调用层 (重构)
1. 简化 `robot/robot.go` 的 `ExecLLM()` 方法
2. 删除 `execAgentCmd()` 方法
3. 删除或标记废弃 `utils/agent.go` 中的旧代码

#### 阶段 4: 配置和文档更新
1. 更新环境变量配置逻辑
2. 更新用户文档
3. 添加迁移指南

### 3.4 配置更新

#### 3.4.1 环境变量配置（新版）

```bash
# .env 文件配置示例

# ===== Agent 基础配置 =====
# 启用 Agent 模式
CMD_AGENT_ENABLED=true

# 选择 Agent 类型（qoder/opencode/cursor/custom）
CMD_AGENT_TYPE=qoder

# 超时时间（秒，0 表示无限制）
CMD_AGENT_TIMEOUT=300

# 是否启用流式输出
IS_STREAMING=true

# ===== 特定 Agent 配置 =====
# Qoder Agent
CMD_AGENT_QODER_COMMAND=/usr/local/bin/qoder
CMD_AGENT_QODER_EXTRA_ARGS="--verbose"

# OpenCode Agent
CMD_AGENT_OPENCODE_COMMAND=/usr/local/bin/opencode
CMD_AGENT_OPENCODE_EXTRA_ARGS="--config /path/to/config.json"

# Cursor Agent
CMD_AGENT_CURSOR_COMMAND=/usr/local/bin/cursor-agent
CMD_AGENT_CURSOR_EXTRA_ARGS="--api-key YOUR_KEY"

# ===== 自定义 Agent =====
CMD_AGENT_TYPE=custom
CMD_AGENT_CUSTOM_COMMAND=/path/to/my-agent
CMD_AGENT_CUSTOM_DEFAULT_ARGS="--mode interactive"
CMD_AGENT_CUSTOM_STREAM_ARGS="--stream --json"
CMD_AGENT_CUSTOM_RESUME_FLAG="--continue"
CMD_AGENT_CUSTOM_WORKDIR_FLAG="--workspace"
```

#### 3.4.2 用户数据库配置
```go
// db/user.go
type LLMConfig struct {
    TxtModel  string `json:"txt_model"`
    TxtType   string `json:"txt_type"`    // 新增: "cmd-agent"
    AgentType string `json:"agent_type"`  // 新增: "qoder"/"opencode"/"cursor"/"custom"
    // ... 其他字段
}

// 初始化用户配置
func InitUserConfig(userId string) {
    config := &LLMConfig{}
    
    if conf.BaseConfInfo.CmdAgentEnabled {
        config.TxtType = param.CmdAgent
        config.AgentType = string(getConfiguredAgentType(context.Background()))
    } else {
        config.TxtType = param.OpenAi
        config.TxtModel = openai.GPT3Dot5Turbo
    }
    
    SaveUserConfig(userId, config)
}
```

---

## 四、重构优势

### 4.1 架构层面

1. **统一接口**: Agent 和其他 LLM 共享相同的接口和调用方式
2. **解耦**: 移除 robot 层的 if 判断，业务逻辑更清晰
3. **可扩展**: 添加新的 Agent 类型只需实现 LLMClient 接口
4. **可测试**: 可以为 AgentReq 编写单元测试，mock 外部命令

### 4.2 代码质量

1. **减少重复**: 消息流、错误处理、会话管理统一在 LLM 框架中
2. **配置统一**: Agent 配置和其他 LLM 配置使用相同的机制
3. **日志统一**: 使用统一的 metrics 和 logger
4. **维护性**: 一处修改，所有 LLM 类型受益

### 4.3 功能增强

1. **多 Agent 支持**: 可以轻松支持多个不同的 Agent 命令
2. **工具集成**: 可以让 Agent 也支持 MCP 工具系统
3. **多模态**: 为 Agent 添加图片、音频支持变得容易
4. **混合模式**: 可以在一个对话中切换不同的 LLM/Agent

---

## 五、实施建议

### 5.1 优先级

**P0 - 核心功能**:
- 创建 `AgentReq` 实现基本的 Send/SyncSend
- 修改 `NewLLM()` 集成 Agent 类型
- 确保会话管理正常工作

**P1 - 完善功能**:
- 实现完整的消息历史管理
- 添加错误重试机制
- 优化流式输出性能
- 清理删除 `utils/agent.go` 和 `robot.execAgentCmd()` 旧代码

**P2 - 增强功能**:
- 多 Agent 支持
- Agent 工具集成
- 多模态支持

### 5.2 风险控制

1. **特性开关**: 使用环境变量控制是否启用 Agent 模式
   ```bash
   CMD_AGENT_ENABLED=true   # 启用 Agent
   CMD_AGENT_ENABLED=false  # 禁用 Agent，回退到普通 LLM
   ```

2. **渐进式迁移**: 先在测试环境验证，再逐步推广到生产
3. **用户级控制**: 允许单个用户选择使用 Agent 或 LLM

### 5.3 测试策略

1. **单元测试**: 为 `AgentReq` 的每个方法编写测试
2. **集成测试**: 测试完整的用户对话流程
3. **性能测试**: 对比新旧实现的性能差异
4. **兼容性测试**: 确保会话迁移正常

---

## 六、后续扩展方向

### 6.1 环境变量配置

#### 6.1.1 通用配置
```bash
# 启用 Agent 模式
CMD_AGENT_ENABLED=true

# 选择 Agent 类型（qoder/opencode/cursor/custom）
CMD_AGENT_TYPE=qoder

# 超时时间（秒）
CMD_AGENT_TIMEOUT=300

# 是否启用流式输出
IS_STREAMING=true
```

#### 6.1.2 特定 Agent 类型配置

**Qoder Agent:**
```bash
# 覆盖 qoder 命令路径
CMD_AGENT_QODER_COMMAND=/usr/local/bin/qoder

# 额外参数
CMD_AGENT_QODER_EXTRA_ARGS="--verbose --debug"
```

**OpenCode Agent:**
```bash
CMD_AGENT_OPENCODE_COMMAND=/usr/local/bin/opencode
CMD_AGENT_OPENCODE_EXTRA_ARGS="--config /path/to/config.json"
```

**Cursor Agent:**
```bash
CMD_AGENT_CURSOR_COMMAND=/usr/local/bin/cursor-agent
CMD_AGENT_CURSOR_EXTRA_ARGS="--api-key YOUR_KEY"
```

#### 6.1.3 自定义 Agent 配置
```bash
# 使用自定义 Agent
CMD_AGENT_TYPE=custom

# 自定义 Agent 的命令
CMD_AGENT_CUSTOM_COMMAND=/path/to/my-agent

# 默认参数
CMD_AGENT_CUSTOM_DEFAULT_ARGS="--mode interactive --safe"

# 流式输出参数
CMD_AGENT_CUSTOM_STREAM_ARGS="--stream --json"

# 恢复会话参数标志
CMD_AGENT_CUSTOM_RESUME_FLAG="--continue-session"

# 工作目录参数标志
CMD_AGENT_CUSTOM_WORKDIR_FLAG="--workspace"
```

### 6.2 用户配置示例

#### 6.2.1 数据库配置结构
```go
// db/user.go
type LLMConfig struct {
    TxtModel  string `json:"txt_model"`
    TxtType   string `json:"txt_type"`    // "openai"/"deepseek"/"cmd-agent"
    AgentType string `json:"agent_type"`  // "qoder"/"opencode"/"cursor"/"custom"
    // ...
}
```

#### 6.2.2 用户切换 Agent
用户可以通过命令或 UI 切换不同的 Agent：

```go
// 切换到 Qoder Agent
/set_agent qoder

// 切换到 OpenCode Agent
/set_agent opencode

// 切换到 Cursor Agent
/set_agent cursor

// 切换回普通 LLM
/set_llm openai
```

### 6.3 多 Agent 架构扩展

通过 AgentProfile 机制，可以轻松扩展支持更多 Agent：

```go
// 添加新的 Agent 类型
const (
    AgentTypeAider     AgentType = "aider"      // Aider Agent
    AgentTypeDevika    AgentType = "devika"     // Devika Agent
)

// 添加到内置配置
BuiltinAgentProfiles[AgentTypeAider] = &AgentProfile{
    Type:        AgentTypeAider,
    Command:     "aider",
    DefaultArgs: []string{"--yes", "--no-git"},
    StreamArgs:  []string{"--stream"},
    ResumeFlag:  "--resume",
    WorkDirFlag: "--workdir",
}
```

### 6.4 Agent 工具集成

```go
// Agent 可以调用 MCP 工具
func (a *AgentReq) executeWithTools(ctx context.Context, l *LLM) error {
    // 1. 将 MCP 工具转换为 Agent 可识别的格式
    tools := a.convertMCPToolsToAgentFormat(l.OpenAITools)
    
    // 2. 通过命令参数传递工具定义
    args := append(a.Args, "--tools", string(tools))
    
    // 3. 解析 Agent 的工具调用请求
    // 4. 调用 MCP 工具
    // 5. 将结果返回给 Agent
}
```

### 6.5 混合推理模式

```go
// 用户可以在对话中动态切换 LLM
func (r *RobotInfo) SwitchLLMType(newType string) {
    // 更新用户配置
    db.UpdateUserLLMConfig(r.GetUserID(), &LLMConfig{
        TxtType: newType,
    })
    
    // 下次对话自动使用新的 LLM 类型
}
```

---

## 七、总结

将 CMD_AGENT 重构为特殊的 LLM Client 是一个优雅的架构改进，它带来了:

✅ **统一的抽象**: 所有 LLM 和 Agent 共享相同的接口
✅ **更好的扩展性**: 轻松添加新的 Agent 类型
✅ **减少重复代码**: 消息流、会话管理、错误处理统一
✅ **提升可测试性**: 可以为 Agent 编写单元测试
✅ **保持向后兼容**: 通过配置无缝迁移

这个重构遵循了 SOLID 原则中的**开闭原则**(对扩展开放，对修改关闭)和**依赖倒置原则**(依赖抽象而非具体实现)，使得整个 LLM 架构更加健壮和灵活。
