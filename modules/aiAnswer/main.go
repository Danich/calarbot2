package main

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"os"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"calarbot2/botModules"
	"calarbot2/common"
	"calarbot2/modules/aiAnswer/handlers"
	"calarbot2/modules/aiAnswer/models"
	"calarbot2/modules/aiAnswer/router"
	"calarbot2/modules/aiAnswer/store"
)

const (
	AiConfigFile = "/aiConfig.yaml"
	DiceSize     = 1000
)

type AIConfig struct {
	BotUsername  string `yaml:"bot_username"`
	AnswerLevel  int    `yaml:"answer_level"`
	ReplyWeight  int    `yaml:"reply_weight"`
	CallWeight   int    `yaml:"call_weight"`
	SystemPrompt string `yaml:"system_prompt"`
	ContextSize  int    `yaml:"context_size"`
	TgBotToken   string `yaml:"tg_bot_token"`

	OpenRouterKey       string `yaml:"openrouter_key"`
	NebiusKey           string `yaml:"nebius_key"`
	NebiusURL           string `yaml:"nebius_url"`
	NebiusVisionModel   string `yaml:"nebius_vision_model"`
	NebiusImageGenModel string `yaml:"nebius_imagegen_model"`
	SQLitePath          string `yaml:"sqlite_path"`
}

type Module struct {
	order         int
	config        AIConfig
	store         *store.Store
	router        *router.Router
	textHandler   *handlers.TextHandler
	visionHandler *handlers.VisionHandler
	imageHandler  *handlers.ImageGenHandler
	cancelRefresh context.CancelFunc
}

type noopMeta struct{}

func (noopMeta) GetMeta(string) (string, bool, error) { return "", false, nil }
func (noopMeta) SetMeta(string, string) error         { return nil }

func metaBackend(s *store.Store) models.MetaStore {
	if s != nil {
		return s
	}
	return noopMeta{}
}

func NewModule(order int, config AIConfig) *Module {
	if config.ContextSize == 0 {
		config.ContextSize = 20
	}

	var s *store.Store
	if config.SQLitePath != "" {
		var err error
		s, err = store.New(config.SQLitePath)
		if err != nil {
			log.Printf("SQLite unavailable (%v), context will not persist across restarts", err)
		}
	}

	sel := models.NewModelSelector(metaBackend(s), "")
	ctx, cancel := context.WithCancel(context.Background())
	sel.StartRefresh(ctx)

	orClient := models.NewOpenRouterClient(config.OpenRouterKey, sel, "")
	nbClient := models.NewNebiusClient(config.NebiusKey, config.NebiusURL, config.NebiusVisionModel, config.NebiusImageGenModel)

	return &Module{
		order:         order,
		config:        config,
		store:         s,
		router:        router.New(orClient),
		textHandler:   handlers.NewTextHandler(orClient, config.SystemPrompt),
		visionHandler: handlers.NewVisionHandler(nbClient, config.TgBotToken),
		imageHandler:  handlers.NewImageGenHandler(nbClient),
		cancelRefresh: cancel,
	}
}

func (m *Module) Order() int { return m.order }

func (m *Module) IsCalled(msg *tgbotapi.Message) bool {
	if msg == nil {
		return false
	}
	if m.store != nil {
		if err := m.store.SaveMessage(msg); err != nil {
			log.Printf("store.SaveMessage: %v", err)
		}
	}
	if isDirectAddress(msg, m.config.BotUsername) {
		return true
	}
	roll := rand.Intn(DiceSize + 1)
	if msg.ReplyToMessage != nil && msg.ReplyToMessage.From != nil &&
		msg.ReplyToMessage.From.UserName == m.config.BotUsername {
		roll += m.config.ReplyWeight
	}
	if common.Contains(common.ExtractMentions(msg), "@"+m.config.BotUsername) {
		roll += m.config.CallWeight
	}
	return roll >= m.config.AnswerLevel
}

func (m *Module) Answer(payload *botModules.Payload) (botModules.RichAnswer, error) {
	ctx := context.Background()
	msg := payload.Msg

	if msg == nil || msg.Chat == nil {
		return botModules.RichAnswer{}, nil
	}

	var history []store.ContextMessage
	if m.store != nil {
		var err error
		history, err = m.store.GetContext(msg.Chat.ID, m.config.ContextSize)
		if err != nil {
			log.Printf("store.GetContext: %v", err)
		}
	}

	if isDirectAddress(msg, m.config.BotUsername) {
		route, err := m.router.Route(ctx, msg)
		if err != nil {
			log.Printf("router.Route error: %v", err)
			route = router.RouteChat
		}
		return m.dispatch(ctx, route, msg, history)
	}

	text, err := m.textHandler.Chat(ctx, msg, history)
	if err != nil {
		log.Printf("textHandler.Chat error: %v", err)
		return botModules.RichAnswer{}, nil
	}
	return botModules.RichAnswer{Text: text}, nil
}

func (m *Module) dispatch(ctx context.Context, route router.Route, msg *tgbotapi.Message, history []store.ContextMessage) (botModules.RichAnswer, error) {
	switch route {
	case router.RouteImageGen:
		result, err := m.imageHandler.Generate(ctx, msg.Text)
		if err != nil {
			log.Printf("imagegen error: %v", err)
			return botModules.RichAnswer{Text: "Не удалось сгенерировать изображение"}, nil
		}
		return result, nil

	case router.RouteVision:
		text, err := m.visionHandler.Describe(ctx, msg)
		if err != nil {
			log.Printf("vision error: %v", err)
			return botModules.RichAnswer{Text: "Не удалось обработать изображение"}, nil
		}
		return botModules.RichAnswer{Text: text}, nil

	case router.RouteTranslate:
		text, err := m.textHandler.Translate(ctx, msg, nil)
		if err != nil {
			log.Printf("translate error: %v", err)
			return botModules.RichAnswer{}, nil
		}
		return botModules.RichAnswer{Text: text}, nil

	case router.RouteQuestion:
		text, err := m.textHandler.Answer(ctx, msg, history)
		if err != nil {
			log.Printf("answer error: %v", err)
			return botModules.RichAnswer{}, nil
		}
		return botModules.RichAnswer{Text: text}, nil

	default: // RouteChat
		text, err := m.textHandler.Chat(ctx, msg, history)
		if err != nil {
			log.Printf("chat error: %v", err)
			return botModules.RichAnswer{}, nil
		}
		return botModules.RichAnswer{Text: text}, nil
	}
}

func isDirectAddress(msg *tgbotapi.Message, botUsername string) bool {
	if msg.ReplyToMessage != nil && msg.ReplyToMessage.From != nil &&
		msg.ReplyToMessage.From.UserName == botUsername {
		return true
	}
	return common.Contains(common.ExtractMentions(msg), "@"+botUsername)
}

func main() {
	order := 1000
	if len(os.Args) > 1 {
		_, _ = fmt.Sscanf(os.Args[1], "%d", &order)
	}

	var config AIConfig
	if err := common.ReadConfig(AiConfigFile, &config); err != nil {
		log.Fatalf("config error: %v", err)
	}

	module := NewModule(order, config)
	defer module.cancelRefresh()
	if module.store != nil {
		defer module.store.Close()
	}

	if err := botModules.RunModuleServer(module, ":8080", 0); err != nil {
		log.Println(err)
	}
}
