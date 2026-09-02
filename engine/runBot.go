package main

import (
	"fmt"
	"log"
	"os"
	"sort"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"calarbot2/botModules"
	"calarbot2/settings"
)

// Ensure mock_module_client.go is included in the build
var _ = NewMockModuleClient

type Bot struct {
	BotAPI         *tgbotapi.BotAPI
	Flags          map[string]bool
	Modules        map[string]*botModules.ModuleClient
	Registrations  map[string]botModules.Registration
	SettingsStore  *settings.Store
	Settings       *settings.Cache
	BotConfig      *CalarbotConfig
	orderedModules []string
}

type moduleOrder struct {
	name  string
	order int
}

func readToken(filename string) (string, error) {
	token, err := os.ReadFile(filename)

	return strings.Trim(string(token), "\n"), err
}

func (b *Bot) InitBot(config *CalarbotConfig) {
	b.BotConfig = config

	// Паникуем, а не работаем без базы: без неё каждый модуль выключен, и бот
	// молча онемел бы во всех чатах сразу.
	if b.BotConfig.SQLitePath == "" {
		log.Panic("sqlitePath is empty: the engine cannot resolve module settings without it")
	}
	settingsStore, err := settings.New(b.BotConfig.SQLitePath)
	if err != nil {
		log.Panic(err)
	}
	b.SettingsStore = settingsStore
	b.Settings = settings.NewCache(settingsStore, 5*time.Second)

	token, err := readToken(b.BotConfig.TgTokenFile)
	if err != nil {
		log.Panic(err)
	}

	b.BotAPI, err = tgbotapi.NewBotAPI(token)
	if err != nil {
		log.Panic(err)
	}

	b.BotAPI.Debug = true

	b.InitModules()

	log.Printf("Authorized on account %s", b.BotAPI.Self.UserName)
}

func (b *Bot) InitModules() {
	// Create a slice to hold module names and their order values
	moduleOrders := make([]moduleOrder, 0, len(b.Modules))

	if b.Modules == nil {
		b.Modules = make(map[string]*botModules.ModuleClient)
	}
	b.Registrations = make(map[string]botModules.Registration)
	for configName, moduleConfig := range b.BotConfig.Modules {
		b.Modules[configName] = &botModules.ModuleClient{BaseURL: moduleConfig.Url}
		reg, err := b.Modules[configName].Register()
		if err != nil {
			log.Printf("module %s did not register: %v", configName, err)
		}
		b.Registrations[configName] = reg
		moduleOrders = append(moduleOrders, moduleOrder{name: configName, order: reg.Order})
	}

	moduleOrders = sortModules(moduleOrders)

	// Populate orderedModules with sorted module names
	b.orderedModules = make([]string, len(moduleOrders))
	for i, mo := range moduleOrders {
		b.orderedModules[i] = mo.name
	}

	fmt.Println("Initialized modules:")
	for _, moduleName := range b.orderedModules {
		client := b.Modules[moduleName]
		fmt.Printf("\t%s: %s (%d)\n", moduleName, client.BaseURL, b.Registrations[moduleName].Order)
	}
}

func sortModules(moduleOrders []moduleOrder) []moduleOrder {
	// Sort modules by their order
	sort.Slice(moduleOrders, func(i, j int) bool {
		return moduleOrders[i].order < moduleOrders[j].order
	})
	return moduleOrders
}

// recordChat запоминает чат, из которого пришёл апдейт. Списка своих чатов
// телеграм боту не отдаёт, так что панели брать его больше неоткуда.
func (b *Bot) recordChat(chat *tgbotapi.Chat, ts int64) {
	if b.SettingsStore == nil || chat == nil {
		return
	}

	// У лички нет title: там имя человека и его @username.
	title := chat.Title
	if title == "" {
		title = strings.TrimSpace(chat.FirstName + " " + chat.LastName)
	}
	// Ни имени, ни фамилии — бывает у удалённых аккаунтов. Тогда хотя бы
	// @username, а на самый крайний случай — id, лишь бы строка в списке
	// личек в панели не оказалась пустой.
	if title == "" {
		if chat.UserName != "" {
			title = "@" + chat.UserName
		} else {
			title = fmt.Sprintf("%d", chat.ID)
		}
	}

	if err := b.SettingsStore.UpsertChat(settings.Chat{
		ID:        chat.ID,
		Type:      chat.Type,
		Title:     title,
		Username:  chat.UserName,
		FirstSeen: ts,
		LastSeen:  ts,
	}); err != nil {
		log.Printf("settings.UpsertChat: %v", err)
	}
}

// recordMembership ловит добавление и изгнание бота. Телеграм шлёт эти апдейты
// по умолчанию, и без них канал появлялся бы в панели только тогда, когда в нём
// кто-то напишет.
func (b *Bot) recordMembership(u *tgbotapi.ChatMemberUpdated) {
	if b.SettingsStore == nil || u == nil {
		return
	}

	b.recordChat(&u.Chat, int64(u.Date))

	// "restricted" сам по себе не значит "бота выгнали" — админ мог просто
	// урезать бота в правах, оставив его в чате. Различает это IsMember:
	// false — бот действительно снаружи (и leaveChat в панели на таком чате
	// иначе просто падал бы с ошибкой), true — бот на месте, только притих.
	gone := u.NewChatMember.Status == "left" ||
		u.NewChatMember.Status == "kicked" ||
		(u.NewChatMember.Status == "restricted" && !u.NewChatMember.IsMember)

	if gone {
		if err := b.SettingsStore.MarkLeft(u.Chat.ID, int64(u.Date)); err != nil {
			log.Printf("settings.MarkLeft: %v", err)
		}
	}
}

func (b *Bot) RunBot() {
	bot := b.BotAPI

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := bot.GetUpdatesChan(u)

	for update := range updates {
		if update.MyChatMember != nil {
			b.recordMembership(update.MyChatMember)
		}

		if update.Message != nil && !update.Message.From.IsBot { // If we got a message
			b.recordChat(update.Message.Chat, int64(update.Message.Date))
			log.Printf("[%s] %s", update.Message.From.UserName, update.Message.Text)

			// Find the module that should handle this message
			extra := map[string]interface{}{}
			if msg := update.Message; len(msg.Photo) > 0 {
				largest := msg.Photo[len(msg.Photo)-1]
				if photoURL, err := bot.GetFileDirectURL(largest.FileID); err == nil {
					extra["photo_url"] = photoURL
				} else {
					log.Printf("Failed to resolve photo URL: %v", err)
				}
			}
			payload := &botModules.Payload{Msg: update.Message, Extra: extra}
			var answer botModules.RichAnswer
			var err error

			for _, moduleName := range b.orderedModules {
				client := b.Modules[moduleName]
				// Настройки кладём до shouldIAnswer: модуль решает, отвечать ли,
				// уже с их учётом — веса живут именно там.
				payload.Extra["settings"] = b.settingsFor(update.Message.Chat.ID, moduleName)
				if !b.shouldIAnswer(moduleName, update, client, payload) {
					continue
				}

				log.Printf("Module %s will handle the message", moduleName)
				answer, err = client.Answer(payload)
				if err != nil {
					log.Printf("Error in module %s: %v", moduleName, err)
					answer = botModules.RichAnswer{Text: "An error occurred while processing your request."}
				}
				break
			}

			if len(answer.Photo) > 0 || answer.PhotoURL != "" {
				// Байты, когда они есть: генераторы картинок отдают base64, и
				// ссылки, за которой телеграм мог бы сходить, не существует.
				var file tgbotapi.RequestFileData = tgbotapi.FileURL(answer.PhotoURL)
				if len(answer.Photo) > 0 {
					file = tgbotapi.FileBytes{Name: "image.png", Bytes: answer.Photo}
				}
				photo := tgbotapi.NewPhoto(update.Message.Chat.ID, file)
				if answer.Text != "" {
					photo.Caption = answer.Text
				}
				photo.ReplyToMessageID = update.Message.MessageID
				if _, err = bot.Send(photo); err != nil {
					log.Printf("Error sending photo: %v", err)
				}
			} else if answer.Text != "" {
				msg := tgbotapi.NewMessage(update.Message.Chat.ID, answer.Text)
				msg.ReplyToMessageID = update.Message.MessageID
				if _, err = bot.Send(msg); err != nil {
					log.Printf("Error sending message: %v", err)
				}
			}
		}
	}
}

// settingsFor собирает настройки модуля для чата: явно выставленные значения
// поверх дефолтов, которые модуль объявил при регистрации.
func (b *Bot) settingsFor(chatID int64, moduleName string) map[string]any {
	return settings.Resolve(
		b.Registrations[moduleName].Fields,
		b.Settings.Values(chatID, moduleName),
	)
}

func (b *Bot) shouldIAnswer(
	moduleName string,
	update tgbotapi.Update,
	client interface{},
	payload *botModules.Payload,
) bool {
	if update.Message == nil || update.Message.Chat == nil {
		return false
	}
	if !b.Settings.ModuleEnabled(update.Message.Chat.ID, moduleName) {
		return false
	}

	var isCalled bool
	var err error

	// Check if client is a MockModuleClient (for testing)
	if mockClient, ok := client.(*MockModuleClient); ok {
		isCalled, err = mockClient.IsCalled(payload)
	} else if moduleClient, ok := client.(*botModules.ModuleClient); ok {
		// Regular ModuleClient
		isCalled, err = moduleClient.IsCalled(payload)
	} else {
		log.Printf("Unknown client type for module %s", moduleName)
		return false
	}

	if err != nil {
		log.Printf("Error checking if module %s is called: %v", moduleName, err)
		return false
	}
	return isCalled
}
