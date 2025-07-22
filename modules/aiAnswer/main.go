package main

import (
	"context"
	"fmt"
	"math/rand"
	"os"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"

	"calarbot2/botModules"
	"calarbot2/common"
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
}

//type PromptRef struct {
//	ID      string `json:"id"`
//	Version string `json:"version"`
//}
//type ChatMessage struct {
//	Role    string `json:"role"`
//	Content string `json:"content"`
//}
//type ResponseAPIResponse struct {
//	ID                string      `json:"id"`
//	Object            string      `json:"object"`
//	Created           int64       `json:"created"`
//	Model             string      `json:"model"`
//	Prompt            PromptRef   `json:"prompt"`
//	Message           ChatMessage `json:"message"` // <— reuse here
//	SystemFingerprint string      `json:"system_fingerprint"`
//}

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
	if msg.Entities != nil && common.Contains(extractMentions(msg), "@"+m.aiConfig.BotUsername) {
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

		Model: openai.ChatModelGPT4_1Mini,
	})
	if err != nil {
		return "", fmt.Errorf("error calling OpenAI API: %v", err)
	}
	return chatCompletion.Choices[0].Message.Content, nil
}

//func (m Module) callWithHttp(payload *botModules.Payload, chatName string) (string, error) {
//
//	requestBody := map[string]interface{}{
//		"prompt": map[string]string{
//			"id":      m.aiConfig.PromptId,
//			"version": m.aiConfig.PromptVersion,
//		},
//		"message": map[string]string{
//			"role":    "user",
//			"content": fmt.Sprintf("Message from %s in %s:\n'%s'", payload.Msg.From.UserName, chatName, payload.Msg.Text),
//		},
//	}
//
//	bodyBytes, _ := json.Marshal(requestBody)
//
//	req, _ := http.NewRequestWithContext(context.Background(), "POST", "https://api.openai.com/v1/responses", bytes.NewReader(bodyBytes))
//	req.Header.Set("Authorization", "Bearer "+m.aiConfig.Token)
//	req.Header.Set("Content-Type", "application/json")
//
//	resp, err := http.DefaultClient.Do(req)
//	if err != nil {
//		panic(err)
//	}
//	defer resp.Body.Close()
//
//	body, _ := io.ReadAll(resp.Body)
//	var result ResponseAPIResponse
//	if err := json.Unmarshal(body, &result); err != nil {
//		log.Fatalf("Failed to parse response: %v", err)
//	}
//	return result.Message.Content, nil
//}

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

func extractMentions(msg *tgbotapi.Message) []string {
	var mentions []string
	for _, entity := range msg.Entities {
		mentions = append(mentions, msg.Text[entity.Offset:entity.Offset+entity.Length])
	}
	return mentions
}
