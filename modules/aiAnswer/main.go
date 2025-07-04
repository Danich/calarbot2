package aiAnswer

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
	Name        string `yaml:"name"`
	Url         string `yaml:"url"`
	Token       string `yaml:"token"`
	AnswerLevel int    `yaml:"answer_level"`
}

func (m Module) Order() int {
	return m.order
}

func (m Module) IsCalled(msg *tgbotapi.Message) bool {
	if msg == nil {
		return false
	}
	roll := rand.Intn(DiceSize + 1)
	fmt.Printf("Dice rolled: %d\n", roll)
	return roll >= m.aiConfig.AnswerLevel
}

func (m Module) Answer(payload *botModules.Payload) (string, error) {
	client := openai.NewClient(
		option.WithAPIKey(m.aiConfig.Token),
		option.WithBaseURL(m.aiConfig.Url),
	)
	chatCompletion, err := client.Chat.Completions.New(context.TODO(), openai.ChatCompletionNewParams{
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.UserMessage("User asked for"),
		},
		Model: openai.ChatModelGPT4o,
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
	err = botModules.ServeModule(module, ":8080")
	if err != nil {
		fmt.Println("Error starting server:", err)
	}
}
