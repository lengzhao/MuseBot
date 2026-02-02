package main

import (
	"context"
	"os"
	"testing"
	
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/stretchr/testify/assert"
	"github.com/yincongcyincong/MuseBot/conf"
	"github.com/yincongcyincong/MuseBot/db"
	"github.com/yincongcyincong/MuseBot/i18n"
	"github.com/yincongcyincong/MuseBot/llm"
	"github.com/yincongcyincong/MuseBot/param"
	"github.com/yincongcyincong/MuseBot/robot"
)

func TestMain(m *testing.M) {
	setup()
	
	code := m.Run()
	
	os.Exit(code)
}

func setup() {
	conf.InitConf()
	db.InitTable()
	i18n.InitI18n()
}

type mockLLMClient struct{}

func (m *mockLLMClient) Send(ctx context.Context, l *llm.LLM) error {
	l.MessageChan <- &param.MsgInfo{Content: "mock response", Finished: true}
	return nil
}
func (m *mockLLMClient) GetMessage(role, msg string)               {}
func (m *mockLLMClient) GetImageMessage(image [][]byte, msg string) {}
func (m *mockLLMClient) GetAudioMessage(audio []byte, msg string)  {}
func (m *mockLLMClient) AppendMessages(client llm.LLMClient)       {}
func (m *mockLLMClient) SyncSend(ctx context.Context, l *llm.LLM) (string, error) {
	return "mock response", nil
}
func (m *mockLLMClient) GetModel(l *llm.LLM) { l.Model = "mock" }

func TestSendTelegramMsg(t *testing.T) {
	messageChan := make(chan *param.MsgInfo)

	go func() {
		bot := robot.CreateBot(context.Background())
		tr := robot.NewTelegramRobot(tgbotapi.Update{
			Message: &tgbotapi.Message{
				MessageID: 1,
				From: &tgbotapi.User{
					ID: 5542540980,
				},
				Chat: &tgbotapi.Chat{
					ID: 5542540980,
				},
			},
		}, bot)
		tr.Robot = robot.NewRobot(robot.WithRobot(tr))
		tr.Robot.HandleUpdate(&robot.MsgChan{
			NormalMessageChan: messageChan,
		}, "")
	}()

	conf.BaseConfInfo.Type = param.DeepSeek

	ctx := context.WithValue(context.Background(), "user_info", &db.User{
		LLMConfig:    `{"type":"deepseek"}`,
		LLMConfigRaw: &param.LLMConfig{TxtType: param.DeepSeek},
	})

	callLLM := llm.NewLLM(llm.WithChatId("1"), llm.WithMsgId("2"), llm.WithUserId("3"),
		llm.WithMessageChan(messageChan), llm.WithContent("hi"), llm.WithContext(ctx))
	callLLM.LLMClient = &mockLLMClient{}
	callLLM.LLMClient.GetModel(callLLM)
	callLLM.GetMessages("3", "hi")
	err := callLLM.LLMClient.Send(ctx, callLLM)
	assert.Equal(t, nil, err)
}
