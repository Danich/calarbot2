package main

import (
	"fmt"
	"log"
	"os"
	"sort"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"calarbot2/botModules"
	"calarbot2/common"
)

// Ensure mock_module_client.go is included in the build
var _ = NewMockModuleClient

type Bot struct {
	BotAPI         *tgbotapi.BotAPI
	Flags          map[string]bool
	Modules        map[string]*botModules.ModuleClient
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
	for configName, moduleConfig := range b.BotConfig.Modules {
		b.Modules[configName] = &botModules.ModuleClient{BaseURL: moduleConfig.Url}
		moduleOrders = append(moduleOrders, moduleOrder{
			name:  configName,
			order: b.Modules[configName].Order(),
		})

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
		fmt.Printf("\t%s: %s (%d)\n", moduleName, client.BaseURL, client.Order())
	}
}

func sortModules(moduleOrders []moduleOrder) []moduleOrder {
	// Sort modules by their order
	sort.Slice(moduleOrders, func(i, j int) bool {
		return moduleOrders[i].order < moduleOrders[j].order
	})
	return moduleOrders
}

func (b *Bot) RunBot() {
	bot := b.BotAPI

	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := bot.GetUpdatesChan(u)

	for update := range updates {
		if update.Message != nil && !update.Message.From.IsBot { // If we got a message
			log.Printf("[%s] %s", update.Message.From.UserName, update.Message.Text)

			// Find the module that should handle this message
			payload := &botModules.Payload{Msg: update.Message, Extra: nil}
			var answer string
			var err error

			for _, moduleName := range b.orderedModules {
				client := b.Modules[moduleName]
				if !b.shouldIAnswer(moduleName, update, client, payload) {
					continue
				}

				log.Printf("Module %s will handle the message", moduleName)
				answer, err = client.Answer(payload)
				if err != nil {
					log.Printf("Error in module %s: %v", moduleName, err)
					answer = "An error occurred while processing your request."
				}
				break
			}

			// Only send a response if there's something to say
			if answer != "" {
				msg := tgbotapi.NewMessage(update.Message.Chat.ID, answer)
				msg.ReplyToMessageID = update.Message.MessageID

				_, err = bot.Send(msg)
				if err != nil {
					log.Printf("Error sending message: %v", err)
				}
			}
		}
	}
}

func (b *Bot) shouldIAnswer(
	moduleName string,
	update tgbotapi.Update,
	client interface{},
	payload *botModules.Payload,
) bool {
	if b.BotConfig.Modules[moduleName].EnabledOn != nil && !common.Contains(b.BotConfig.Modules[moduleName].EnabledOn, update.Message.Chat.ID) {
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
