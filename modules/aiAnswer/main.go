package main

import (
	"context"
	"fmt"
	"math/rand"
	"os"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"calarbot2/botModules"
	"calarbot2/common"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
)

const AiConfigFile = "/aiConfig.yaml"
const DiceSize = 1000

type Module struct {
	order    int
	aiConfig AIConfig
}

type AIConfig struct {
	Name         string `yaml:"name"`
	Url          string `yaml:"url"`
	Token        string `yaml:"token"`
	AnswerLevel  int    `yaml:"answer_level"`
	ReplyWeight  int    `yaml:"reply_weight"`
	CallWeight   int    `yaml:"call_weight"`
	BotUsername  string `yaml:"bot_username"`
	SystemPrompt string `yaml:"system_prompt"`
	ModelName    string `yaml:"model_name"`
}

func (m Module) Order() int {
	return m.order
}

func (m Module) IsCalled(msg *tgbotapi.Message) bool {
	if msg == nil {
		return false
	}
	roll := rand.Intn(DiceSize + 1)
	if msg.ReplyToMessage != nil && msg.ReplyToMessage.From != nil && msg.ReplyToMessage.From.UserName == m.aiConfig.BotUsername {
		fmt.Printf("Reply to my message, roll: %d\n", roll)
		roll = roll + m.aiConfig.ReplyWeight
	}
	if msg.Entities != nil && common.Contains(common.ExtractMentions(msg), "@"+m.aiConfig.BotUsername) {
		fmt.Printf("Message contains @%s, roll: %d\n", m.aiConfig.BotUsername, roll)
		roll = roll + m.aiConfig.CallWeight
	}

	fmt.Printf("Total rolled: %d\n", roll)
	return roll >= m.aiConfig.AnswerLevel
}

func (m Module) Answer(payload *botModules.Payload) (string, error) {
	client := openai.NewClient(
		option.WithAPIKey(m.aiConfig.Token),
		option.WithBaseURL(m.aiConfig.Url),
	)
	chatName := "Unknown"
	if payload.Msg.Chat != nil && payload.Msg.Chat.Title != "" {
		chatName = payload.Msg.Chat.Title
	}

	chatCompletion, err := client.Chat.Completions.New(context.TODO(), openai.ChatCompletionNewParams{
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.SystemMessage(m.aiConfig.SystemPrompt),
			openai.UserMessage(fmt.Sprintf("Message from %s in %s:\n'%s'", payload.Msg.From.UserName, chatName, payload.Msg.Text)),
		},

		Model: m.aiConfig.ModelName,
	})
	if err != nil {
		return "", fmt.Errorf("error calling OpenAI API: %v", err)
	}
	return chatCompletion.Choices[0].Message.Content, nil
}

func main() {
	order := 1000
	if len(os.Args) > 1 {
		_, _ = fmt.Sscanf(os.Args[1], "%d", &order)
	}

	aiConfig := AIConfig{}
	err := common.ReadConfig(AiConfigFile, &aiConfig)
	if err != nil {
		fmt.Println("Configure error:", err)
		return
	}

	module := Module{order: order, aiConfig: aiConfig}

	if err := botModules.RunModuleServer(module, ":8080", 0); err != nil {
		fmt.Println(err)
	}
}
