package main

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"os"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"calarbot2/botModules"
	"calarbot2/common"
	"calarbot2/modules/aiAnswer/handlers"
	"calarbot2/modules/aiAnswer/lore"
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

	DefaultPersona string `yaml:"default_persona"`
	LoreWindow     int    `yaml:"lore_window"`
	LoreModel      string `yaml:"lore_model"`
	LoreNotify     bool   `yaml:"lore_notify"`
	NotifyURL      string `yaml:"notify_url"`

	OpenRouterKey     string `yaml:"openrouter_key"`
	NebiusKey         string `yaml:"nebius_key"`
	NebiusURL         string `yaml:"nebius_url"`
	NebiusVisionModel string `yaml:"nebius_vision_model"`
	// Рисование живёт у отдельного провайдера: Nebius генерацию убрал.
	ImageGenURL   string `yaml:"imagegen_url"`
	ImageGenKey   string `yaml:"imagegen_key"`
	ImageGenModel string `yaml:"imagegen_model"`
	PersonaModel  string `yaml:"persona_model"`
	SQLitePath    string `yaml:"sqlite_path"`
}

type Module struct {
	order         int
	config        AIConfig
	store         *store.Store
	router        *router.Router
	textHandler   *handlers.TextHandler
	visionHandler *handlers.VisionHandler
	imageHandler  *handlers.ImageGenHandler
	loreRunner    *lore.Runner
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
	if config.DefaultPersona == "" {
		config.DefaultPersona = "default"
	}
	if config.LoreWindow == 0 {
		config.LoreWindow = 20
	}

	var s *store.Store
	if config.SQLitePath != "" {
		var err error
		s, err = store.New(config.SQLitePath)
		if err != nil {
			log.Printf("SQLite unavailable (%v), context will not persist across restarts", err)
		}
	}
	if s != nil {
		if _, change, err := s.UpsertConfigPersona(
			config.DefaultPersona, config.DefaultPersona, config.SystemPrompt,
		); err != nil {
			log.Printf("persona seed: %v", err)
		} else if change != store.PersonaUnchanged {
			text := fmt.Sprintf("persona %q: %s", config.DefaultPersona, personaChangeText(change))
			log.Print(text)
			if config.LoreNotify && config.NotifyURL != "" {
				lore.NewHTTPNotifier(config.NotifyURL).Notify(text)
			}
		}
	}

	sel := models.NewModelSelector(metaBackend(s), "")
	ctx, cancel := context.WithCancel(context.Background())
	sel.StartRefresh(ctx)

	orClient := models.NewOpenRouterClient(config.OpenRouterKey, sel, "")
	nbClient := models.NewNebiusClient(config.NebiusKey, config.NebiusURL, config.NebiusVisionModel)
	imgClient := models.NewImageClient(config.ImageGenKey, config.ImageGenURL, config.ImageGenModel)

	// Текстом отвечает одна модель, сразу в образе. Раньше их было две —
	// дешёвая писала ответ, персона переписывала его в голосе персонажа, — но
	// для болталки это лишний круг: обе получали «отвечай в роли», ответ
	// выходил вдвое характернее нужного, и стоил двух запросов вместо одного.
	//
	// Vision — другое дело, и там пересказ остался: описание картинки делает
	// модель Nebius, персонажем она быть не умеет, так что её текст всё ещё
	// надо окрашивать отдельно.
	textLLM := orClient
	var visionPersona handlers.LLMClient
	if config.PersonaModel != "" {
		personaOR := models.NewOpenRouterClient(config.OpenRouterKey, models.NewStaticModel(config.PersonaModel), "")
		textLLM = personaOR
		visionPersona = personaOR
	}

	var loreRunner *lore.Runner
	if s != nil {
		loreLLM := orClient
		if config.LoreModel != "" {
			loreLLM = models.NewOpenRouterClient(config.OpenRouterKey, models.NewStaticModel(config.LoreModel), "")
		}
		loreRunner = lore.NewRunner(s, lore.NewExtractor(loreLLM), lore.NewCompactor(loreLLM), config.ContextSize)
		if config.LoreNotify && config.NotifyURL != "" {
			loreRunner = loreRunner.WithNotifier(lore.NewHTTPNotifier(config.NotifyURL))
		}
	}

	return &Module{
		order:         order,
		config:        config,
		store:         s,
		router:        router.New(orClient),
		textHandler:   handlers.NewTextHandler(textLLM),
		visionHandler: handlers.NewVisionHandler(nbClient, visionPersona),
		imageHandler:  handlers.NewImageGenHandler(imgClient),
		loreRunner:    loreRunner,
		cancelRefresh: cancel,
	}
}

// personaChangeText: смена личности — это смена ключа. Переписанный промпт при
// том же ключе почти всегда значит, что ключ поменять забыли, и новому
// персонажу вот-вот достанется чужая биография.
func personaChangeText(c store.PersonaChange) string {
	if c == store.PersonaCreated {
		return "created"
	}
	return "prompt overwritten — is this a new character that kept the old key?"
}

// systemPromptFor собирает системное сообщение под конкретный чат: канон
// персоны плюс её лор. Без стора работает как раньше — на промпте из конфига.
func (m *Module) systemPromptFor(chatID int64) (store.Persona, string) {
	if m.store == nil {
		return store.Persona{}, m.config.SystemPrompt
	}
	p, err := m.store.ResolvePersona(chatID, m.config.DefaultPersona)
	if err != nil {
		log.Printf("resolve persona: %v", err)
		return store.Persona{}, m.config.SystemPrompt
	}
	records, err := m.store.LoreForPrompt(chatID, p.ID, m.config.LoreWindow)
	if err != nil {
		log.Printf("lore for prompt: %v", err)
		return p, p.SystemPrompt
	}
	block := lore.BuildBlock(records)
	if block == "" {
		return p, p.SystemPrompt
	}
	return p, p.SystemPrompt + "\n\n" + block
}

func (m *Module) Register() botModules.Registration {
	return botModules.Registration{
		Order:       m.order,
		Label:       "AI-ответ",
		Description: "Отвечает через языковую модель",
	}
}

func (m *Module) IsCalled(msg *tgbotapi.Message) bool {
	if msg == nil {
		return false
	}
	if m.store != nil {
		if err := m.store.SaveMessage(msg); err != nil {
			log.Printf("store.SaveMessage: %v", err)
		}
	}
	// Лор растёт на каждом сообщении чата, а не только когда бот отвечает:
	// IsCalled видит весь поток, и никакого расписания для этого не нужно.
	if m.store != nil && m.loreRunner != nil && msg.Chat != nil {
		if p, err := m.store.ResolvePersona(msg.Chat.ID, m.config.DefaultPersona); err == nil {
			m.loreRunner.Maybe(msg.Chat.ID, p.ID, p.SystemPrompt)
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

// Answer отвечает и запоминает собственный ответ: без этого в контексте видны
// только реплики людей, и на реплай боту модель видит ответ, но не видит, на
// что он был.
func (m *Module) Answer(payload *botModules.Payload) (botModules.RichAnswer, error) {
	answer, err := m.answer(payload)
	if err != nil || answer.Text == "" || m.store == nil {
		return answer, err
	}
	if saveErr := m.store.SaveBotMessage(
		payload.Msg.Chat.ID, m.config.BotUsername, answer.Text, time.Now().Unix(),
	); saveErr != nil {
		// Не роняем ответ из-за незаписанной истории: сказать сейчас важнее,
		// чем помнить потом.
		log.Printf("store.SaveBotMessage: %v", saveErr)
	}
	return answer, err
}

func (m *Module) answer(payload *botModules.Payload) (botModules.RichAnswer, error) {
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

	photoURL, _ := payload.Extra["photo_url"].(string)
	_, system := m.systemPromptFor(msg.Chat.ID)

	if isDirectAddress(msg, m.config.BotUsername) {
		route, err := m.router.Route(ctx, msg)
		if err != nil {
			log.Printf("router.Route error: %v", err)
			route = router.RouteChat
		}
		return m.dispatch(ctx, route, system, msg, history, photoURL)
	}

	text, err := m.textHandler.Chat(ctx, system, msg, history)
	if err != nil {
		log.Printf("textHandler.Chat error: %v", err)
		return botModules.RichAnswer{}, nil
	}
	return botModules.RichAnswer{Text: text}, nil
}

func (m *Module) dispatch(ctx context.Context, route router.Route, system string, msg *tgbotapi.Message, history []store.ContextMessage, photoURL string) (botModules.RichAnswer, error) {
	switch route {
	case router.RouteImageGen:
		prompt := msg.Text
		if prompt == "" {
			prompt = msg.Caption
		}
		result, err := m.imageHandler.Generate(ctx, prompt)
		if err != nil {
			log.Printf("imagegen error: %v", err)
			return botModules.RichAnswer{Text: "Не удалось сгенерировать изображение"}, nil
		}
		return result, nil

	case router.RouteVision:
		text, err := m.visionHandler.Describe(ctx, system, msg, photoURL)
		if err != nil {
			log.Printf("vision error: %v", err)
			return botModules.RichAnswer{Text: "Не удалось обработать изображение"}, nil
		}
		return botModules.RichAnswer{Text: text}, nil

	case router.RouteTranslate:
		text, err := m.textHandler.Translate(ctx, system, msg, nil)
		if err != nil {
			log.Printf("translate error: %v", err)
			return botModules.RichAnswer{}, nil
		}
		return botModules.RichAnswer{Text: text}, nil

	case router.RouteQuestion:
		text, err := m.textHandler.Answer(ctx, system, msg, history)
		if err != nil {
			log.Printf("answer error: %v", err)
			return botModules.RichAnswer{}, nil
		}
		return botModules.RichAnswer{Text: text}, nil

	default: // RouteChat
		text, err := m.textHandler.Chat(ctx, system, msg, history)
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
