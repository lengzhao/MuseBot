# Agent Command Line Tool Integration

MuseBot supports integrating the agent command line tool, which uses LLM to process user input messages. When agent mode is enabled, all user input is sent directly to the agent command for processing, rather than through LLM's tool calling mechanism.

## Configuration

### Environment Variables

Enable and configure agent mode through environment variables:

```bash
# Enable agent mode (required)
CMD_AGENT_ENABLED=true

# Agent command path (optional, default: "agent")
CMD_AGENT_COMMAND=agent

# Agent command timeout in seconds (optional, default: 300, set to 0 for no timeout)
CMD_AGENT_TIMEOUT=300
```

### Configuration Options

- **CMD_AGENT_ENABLED**: Set to `true` to enable agent mode. When enabled, all user input is sent directly to the agent command
- **CMD_AGENT_COMMAND**: Path to the agent command, default is "agent". If the agent command is not in system PATH, specify the full path
- **CMD_AGENT_TIMEOUT**: Timeout for agent command execution in seconds, default is 300 seconds. Set to 0 for no timeout (suitable for very complex tasks)

## Usage

After enabling agent mode, simply send messages directly. MuseBot will automatically send messages to the agent command:

```
Hello, help me analyze the performance issues of this code
```

## How It Works

1. **Mode Activation**: When `CMD_AGENT_ENABLED=true`, MuseBot enters agent mode
2. **Direct Processing**: User input is sent directly to the agent command, bypassing LLM's tool selection mechanism
3. **Session Management**: Each user has an independent working directory (`data/agent_sessions/{userId}/`), with automatic session save and restore
4. **Command Execution**: MuseBot executes the agent command with user input and necessary parameters
5. **Streaming Output**: Real-time parsing of agent command's JSON stream output, including thinking process and final results
6. **Result Return**: Results are streamed back to users in real-time

## Agent Command Format

The agent tool executes the following commands:

```bash
# New session
agent -p -f --sandbox enabled --stream-partial-output --output-format stream-json <user input>

# Continue session (if saved session_id exists)
agent -p -f --sandbox enabled --stream-partial-output --output-format stream-json --resume <session_id> <user input>
```

## Session Management

### Session Isolation

- Each user has an independent working directory: `data/agent_sessions/{userId}/`
- Each user directory contains a `.agent_session_id` file that records the current session's session_id
- Sessions from different users are completely isolated and do not interfere with each other

### Session Recovery

- First conversation: Agent generates a new session_id and saves it to the user directory
- Subsequent conversations: Automatically loads session_id from user directory and uses `--resume` parameter to restore the session
- Session persistence: session_id is saved in the `.agent_session_id` file in the user directory

## Output Format

Agent tool outputs JSON stream format, including the following types:

- `thinking`: Thinking process (displayed in special format, separated from main text)
- `assistant`: Assistant reply
- `result`: Final result
- `error`: Error information

MuseBot automatically parses these outputs:
- `thinking` content is displayed in special format (gray background with "💭 Thinking Process" header)
- `assistant` and `result` content are displayed as main replies
- All content is streamed in real-time

## Notes

1. **Command Availability**: Ensure the `agent` command is available in system PATH, or specify the full path via `CMD_AGENT_COMMAND`
2. **Timeout Settings**: For complex tasks, you can set `CMD_AGENT_TIMEOUT=0` to disable timeout limits
3. **Session Management**: Each user's session is automatically saved and restored, no manual management required
4. **Mode Switching**: After enabling agent mode, all messages are sent directly to the agent and will not use LLM's tool calling mechanism
5. **Configuration Effect**: After modifying environment variables, restart MuseBot for the configuration to take effect
