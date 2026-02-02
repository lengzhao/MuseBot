# MuseBot 配置引导程序设计文档

## 1. 目标
为 MuseBot 提供一个交互式的命令行引导程序（Wizard），帮助用户快速生成 `.env` 配置文件，降低上手门槛。

## 2. 核心特性
- **多语言支持**：启动即选语言（中文/英文/俄文）。
- **交互式设置**：通过问答方式引导用户填写配置，并提供 Key 获取指南。
- **配置模式切换**：
    - **最简配置**：仅配置 LLM 和 机器人平台（如 Telegram），快速运行。
    - **自定义配置**：涵盖数据库、代理、多媒体（音频/图片/视频）、RAG 等高级选项。
- **自动保存**：完成引导后自动生成或更新 `.env` 文件。

## 3. 引导流程设计

### 第一步：语言选择
- 选项：1. 中文 (zh) / 2. English (en) / 3. Русский (ru)
- 影响后续所有提示文字。

### 第二步：配置模式选择
- 选项：
    1. **最简配置** (Simple)：推荐初次使用。
    2. **自定义配置** (Customized)：适合进阶用户。

### 第三步：核心配置（最简模式 & 自定义模式均包含）
1. **LLM 服务商选择**：
    - 选项：deepseek, gemini, openai, openrouter, vol, chatanywhere, cmd-agent 等。
    - **cmd-agent**：CMD Agent 类型，需配置 `CMD_AGENT_TYPE` (qoder/opencode/cursor/custom) 和 `CMD_AGENT_TIMEOUT`。
        - **自动检测**：选择 Agent 类型后，向导会自动检测对应的命令程序是否已安装。
        - **安装引导**：如果命令未找到，会根据操作系统和 Agent 类型显示详细的安装指引和下载链接。
        - 支持的 Agent 类型：
            - `qoder`: 使用 `qodercli` 命令
            - `opencode`: 使用 `opencode` 命令
            - `cursor`: 使用 `agent` 命令
            - `custom`: 自定义命令路径
    - 根据选择，引导填写对应的 Token（如 `DEEPSEEK_TOKEN`）。
    - **引导提示**：例如选择 DeepSeek 时，提示 "请前往 https://platform.deepseek.com/ 获取 API Key"。
2. **机器人平台选择**：
    - 选项：Web, Telegram, Discord, Slack, Lark, DingTalk, WeChat 等。
    - **Web**：本地 Web UI，基于 `HTTP_HOST` 运行。
    - 根据选择，引导填写对应的 Token 或 APP_ID/SECRET。
    - **引导提示**：例如选择 Telegram 时，提示 "请联系 @BotFather 创建机器人并获取 Token"。

### 第四步：全量配置引导（仅自定义模式）
自定义模式将引导用户完成 `.env.example` 中定义的所有配置项。为提高效率，将按功能模块分段询问，用户可选择跳过不使用的模块：

1.  **基础运行配置**：
    *   日志级别 (LOG_LEVEL)、机器人名称 (BOT_NAME)、监听地址 (HTTP_HOST)。
    *   数据库类型 (DB_TYPE) 与连接串 (DB_CONF)。
    *   安全与限制：允许的用户/群组 ID、Token 限制 (TOKEN_PER_USER)、最大并发聊天 (MAX_USER_CHAT)。
    *   运行特性：流式输出 (IS_STREAMING)、智能模式 (SMART_MODE)、上下文过期时间等。
2.  **多平台机器人配置**：
    *   逐一确认是否启用 Telegram, Discord, Slack, Lark, DingTalk, WeChat, QQ 等。
    *   针对启用的平台，引导填写对应凭证（Token/ID/Secret）。
3.  **LLM 与 多媒体服务 Token**：
    *   集中配置所有已启用的 LLM (DeepSeek, OpenAI, Gemini, etc.) 及多媒体服务 (Volcengine, Aliyun, 302.ai) 的访问凭证。
4.  **LLM 运行参数微调**：
    *   Temperature, Top_P, Max_Tokens, Penalty 参数等。
5.  **多媒体功能细节配置**：
    *   **音频 (Audio)**：TTS 类型选择、各服务商的具体参数（如语速、发音人等）。
    *   **图片 (Photo)**：生成模型选择、默认宽高、水印设置等。
    *   **视频 (Video)**：模型选择、时长、帧率、分辨率等。
6.  **RAG 知识库配置**：
    *   嵌入模型 (Embedding) 类型。
    *   向量数据库 (Vector DB) 选择（Chroma/Milvus/Weaviate）及其连接地址。
    *   知识库文件路径及分块策略 (Chunk Size/Overlap)。
7.  **高级组件配置**：
    *   **服务注册 (Register)**：Etcd 地址与凭证。
    *   **工具扩展 (MCP)**：MCP 配置文件路径。
    *   **管理后台 (Admin)**：端口设置与 Session Key。
    *   **CMD Agent**：Agent 类型选择及超时设置。
8.  **网络与代理**：
    *   LLM 代理、机器人代理、SSL 证书文件路径。

### 第五步：保存与完成
- 检查必填项。
- 写入 `.env` 文件。
- 提示启动命令。

## 4. 关键配置项与获取指南 (Key Guidance)

| 配置项 | 获取指南 (Key Guidance) |
| :--- | :--- |
| **TELEGRAM_BOT_TOKEN** | 在 Telegram 中联系 [@BotFather](https://t.me/botfather)，发送 `/newbot` 指令，按照提示设置名称后即可获取。 |
| **DISCORD_BOT_TOKEN** | 访问 [Discord Developer Portal](https://discord.com/developers/applications)，创建应用并在 Bot 页面生成 Token。 |
| **SLACK_BOT_TOKEN** | 在 [Slack API](https://api.slack.com/apps) 创建 App，开启 Socket Mode，在 OAuth & Permissions 页面获取 Bot User OAuth Token。 |
| **LARK_APP_ID/SECRET** | 访问 [飞书开放平台](https://open.feishu.cn/)，创建企业自建应用，在“凭证与基础信息”中查看。 |
| **DEEPSEEK_TOKEN** | 访问 [DeepSeek 开放平台](https://platform.deepseek.com/) 注册并创建 API Key。 |
| **OPENAI_TOKEN** | 访问 [OpenAI API Keys](https://platform.openai.com/api-keys) 创建 Secret Key。 |
| **GEMINI_TOKEN** | 访问 [Google AI Studio](https://aistudio.google.com/app/apikey) 获取。 |
| **OPEN_ROUTER_TOKEN** | 访问 [OpenRouter Keys](https://openrouter.ai/keys) 获取。 |
| **VOLC_AK/SK/TOKEN** | 访问 [火山引擎控制台](https://console.volcengine.com/)，在“访问控制-身份管理”中获取 AK/SK；在“豆包大模型”中获取模型对应的 Token/Endpoint。 |
| **ALIYUN_TOKEN** | 访问 [阿里云百炼控制台](https://bailian.console.aliyun.com/) 获取 API-KEY。 |
| **ERNIE_AK/SK** | 访问 [百度智能云控制台](https://console.bce.baidu.com/)，在“千帆大模型平台”的应用列表中获取。 |
| **AI_302_TOKEN** | 访问 [302.ai 官网](https://302.ai/) 获取 API Token。 |

## 5. 实现建议
- 使用 Go 语言编写，作为项目的一个子命令或独立工具（如 `go run main.go wizard`）。
- 引入交互式库（如 `github.com/manifoldco/promptui` 或 `github.com/alecthomas/survey/v2`）以提供更好的用户体验。
- 复用项目已有的 `i18n` 资源，或在引导程序内维护独立的简易多语言映射。
