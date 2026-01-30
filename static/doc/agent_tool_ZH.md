# Agent 命令行工具集成

MuseBot 支持集成 agent 命令行工具，该工具使用 LLM 来处理用户输入消息。当启用 agent 模式后，所有用户输入将直接发送给 agent 命令处理，而不是通过 LLM 的工具调用机制。

## 配置

### 环境变量

通过环境变量启用和配置 agent 模式：

```bash
# 启用 agent 模式（必需）
CMD_AGENT_ENABLED=true

# agent 命令路径（可选，默认为 "agent"）
CMD_AGENT_COMMAND=agent

# agent 命令超时时间（可选，秒，默认 300，设置为 0 表示无超时限制）
CMD_AGENT_TIMEOUT=300
```

### 配置说明

- **CMD_AGENT_ENABLED**: 设置为 `true` 启用 agent 模式。启用后，所有用户输入将直接发送给 agent 命令处理
- **CMD_AGENT_COMMAND**: agent 命令的路径，默认为 "agent"。如果 agent 命令不在系统 PATH 中，可以指定完整路径
- **CMD_AGENT_TIMEOUT**: agent 命令的超时时间（秒），默认 300 秒。设置为 0 表示无超时限制（适用于执行非常复杂的任务）

## 使用方法

启用 agent 模式后，直接发送消息即可，MuseBot 会自动将消息发送给 agent 命令处理：

```
你好，帮我分析一下这个代码的性能问题
```

## 工作原理

1. **模式启用**：当 `CMD_AGENT_ENABLED=true` 时，MuseBot 进入 agent 模式
2. **直接处理**：用户输入直接发送给 agent 命令，不经过 LLM 的工具选择机制
3. **会话管理**：每个用户有独立的工作目录（`data/agent_sessions/{userId}/`），自动保存和恢复会话
4. **命令执行**：MuseBot 执行 agent 命令，传入用户输入和必要的参数
5. **流式输出**：实时解析 agent 命令的 JSON 流输出，包括思考过程和最终结果
6. **结果返回**：将结果实时流式返回给用户

## Agent 命令格式

agent 工具会执行以下命令：

```bash
# 新会话
agent -p -f --sandbox enabled --stream-partial-output --output-format stream-json <用户输入>

# 继续会话（如果存在已保存的 session_id）
agent -p -f --sandbox enabled --stream-partial-output --output-format stream-json --resume <session_id> <用户输入>
```

## 会话管理

### 会话隔离

- 每个用户有独立的工作目录：`data/agent_sessions/{userId}/`
- 每个用户目录下保存 `.agent_session_id` 文件，记录当前会话的 session_id
- 不同用户的会话完全隔离，互不干扰

### 会话恢复

- 首次对话：agent 生成新的 session_id，保存到用户目录
- 后续对话：自动从用户目录加载 session_id，使用 `--resume` 参数恢复会话
- 会话持久化：session_id 保存在用户目录的 `.agent_session_id` 文件中

## 输出格式

agent 工具输出 JSON 流格式，包含以下类型：

- `thinking`: 思考过程（会以特殊格式显示，与正文区分）
- `assistant`: 助手回复
- `result`: 最终结果
- `error`: 错误信息

MuseBot 会自动解析这些输出：
- `thinking` 内容会以特殊格式显示（灰色背景，带"💭 思考过程"标题）
- `assistant` 和 `result` 内容会作为主要回复显示
- 所有内容都会实时流式输出

## 注意事项

1. **命令可用性**：确保 `agent` 命令在系统 PATH 中可用，或通过 `CMD_AGENT_COMMAND` 指定完整路径
2. **超时设置**：对于复杂任务，可以设置 `CMD_AGENT_TIMEOUT=0` 来禁用超时限制
3. **会话管理**：每个用户的会话会自动保存和恢复，无需手动管理
4. **模式切换**：启用 agent 模式后，所有消息都会直接发送给 agent，不会使用 LLM 的工具调用机制
5. **配置生效**：修改环境变量后需要重启 MuseBot 使配置生效
