package llm

import (
	"context"
	"testing"

	"github.com/yincongcyincong/MuseBot/param"
)

func TestAgentProfile(t *testing.T) {
	// 测试内置 Profile
	tests := []struct {
		name      string
		agentType AgentType
		wantCmd   string
	}{
		{"Qoder", AgentTypeQoder, "qoder"},
		{"OpenCode", AgentTypeOpenCode, "opencode"},
		{"Cursor", AgentTypeCursor, "cursor-agent"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			profile := getAgentProfile(tt.agentType)
			if profile == nil {
				t.Errorf("getAgentProfile(%v) = nil, want profile", tt.agentType)
				return
			}
			if profile.Command != tt.wantCmd {
				t.Errorf("getAgentProfile(%v).Command = %v, want %v", tt.agentType, profile.Command, tt.wantCmd)
			}
		})
	}
}

func TestBuildArgs(t *testing.T) {
	// 测试参数构建
	agent := &AgentReq{
		AgentType:     AgentTypeQoder,
		Profile:       BuiltinAgentProfiles[AgentTypeQoder],
		StreamEnabled: true,
		SessionID:     "test-session-123",
		WorkDir:       "/test/dir",
		ExtraArgs:     []string{"--verbose"},
	}

	args := agent.buildArgs()

	// 验证参数包含必要的元素
	hasResume := false
	for i, arg := range args {
		if arg == "--resume" && i+1 < len(args) && args[i+1] == "test-session-123" {
			hasResume = true
			break
		}
	}

	if !hasResume {
		t.Error("buildArgs() should include --resume flag with session ID")
	}
}

func TestAgentGetModel(t *testing.T) {
	agent := &AgentReq{
		AgentType: AgentTypeQoder,
	}

	llm := &LLM{}
	agent.GetModel(llm)

	expected := "agent-qoder"
	if llm.Model != expected {
		t.Errorf("GetModel() set Model = %v, want %v", llm.Model, expected)
	}
}

func TestGetConfiguredAgentType(t *testing.T) {
	// 测试默认类型
	ctx := context.Background()
	agentType := getConfiguredAgentType(ctx)

	// 默认应该是 qoder
	if agentType != AgentTypeQoder {
		t.Errorf("getConfiguredAgentType() = %v, want %v", agentType, AgentTypeQoder)
	}
}

func TestParseArgs(t *testing.T) {
	tests := []struct {
		input string
		want  []string
	}{
		{"", []string{}},
		{"--verbose", []string{"--verbose"}},
		{"--flag1 --flag2 value", []string{"--flag1", "--flag2", "value"}},
	}

	for _, tt := range tests {
		got := parseArgs(tt.input)
		if len(got) != len(tt.want) {
			t.Errorf("parseArgs(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestSanitizeSessionID(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"normal-id", "normal-id"},
		{"path/with/slashes", "path_with_slashes"},
		{"path\\with\\backslashes", "path_with_backslashes"},
		{"has:colon", "has_colon"},
		{"../../../etc/passwd", "..__.._..__etc_passwd"},
	}

	for _, tt := range tests {
		got := sanitizeSessionID(tt.input)
		if got != tt.want {
			t.Errorf("sanitizeSessionID(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestAgentTypeConstants(t *testing.T) {
	// 测试 Agent 类型常量
	if param.CmdAgent != "cmd-agent" {
		t.Errorf("param.CmdAgent = %v, want \"cmd-agent\"", param.CmdAgent)
	}

	// 验证内置 Profile 都存在
	agentTypes := []AgentType{
		AgentTypeQoder,
		AgentTypeOpenCode,
		AgentTypeCursor,
	}

	for _, at := range agentTypes {
		if _, ok := BuiltinAgentProfiles[at]; !ok {
			t.Errorf("BuiltinAgentProfiles missing entry for %v", at)
		}
	}
}
