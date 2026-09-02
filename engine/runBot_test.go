// Vibecoded it because I'm lazy AF

package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"calarbot2/botModules"
)

// MockBotAPI is a mock implementation of the Telegram Bot API for testing
type MockBotAPI struct {
	SentMessages []tgbotapi.MessageConfig
}

func (m *MockBotAPI) Send(c tgbotapi.Chattable) (tgbotapi.Message, error) {
	msg, ok := c.(tgbotapi.MessageConfig)
	if ok {
		m.SentMessages = append(m.SentMessages, msg)
	}
	return tgbotapi.Message{}, nil
}

func TestInitModules(t *testing.T) {
	// Регистрируем модули по-настоящему, с разными Order: если оба модуля
	// недоступны, Register() отдаёт им один и тот же fallback Order 9999, и
	// sort.Slice (не стабильная сортировка) вперемешку с рандомизированным
	// порядком обхода карты модулей решает их порядок произвольно —
	// проверка ниже была бы недетерминированной.
	server1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(botModules.Registration{Order: 10, Label: "module1"})
	}))
	defer server1.Close()
	server2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(botModules.Registration{Order: 20, Label: "module2"})
	}))
	defer server2.Close()

	// Create real ModuleClient instances for the bot
	moduleClient1 := &botModules.ModuleClient{BaseURL: server1.URL}
	moduleClient2 := &botModules.ModuleClient{BaseURL: server2.URL}

	// Create a bot with a mock configuration
	bot := &Bot{
		BotConfig: &CalarbotConfig{
			Modules: map[string]ModulesConfig{
				"module1": {
					Url: server1.URL,
				},
				"module2": {
					Url: server2.URL,
				},
			},
		},
		Modules: map[string]*botModules.ModuleClient{
			"module1": moduleClient1,
			"module2": moduleClient2,
		},
	}

	// Call InitModules
	bot.InitModules()

	// Verify that the modules were initialized correctly
	if len(bot.orderedModules) != 2 {
		t.Errorf("Expected 2 ordered modules, got %d", len(bot.orderedModules))
	}

	// Verify that the modules are in the correct order
	if len(bot.orderedModules) >= 2 {
		if bot.orderedModules[0] != "module1" {
			t.Errorf("Expected first module to be 'module1', got '%s'", bot.orderedModules[0])
		}
		if bot.orderedModules[1] != "module2" {
			t.Errorf("Expected second module to be 'module2', got '%s'", bot.orderedModules[1])
		}
	}
}

func TestShouldIAnswer(t *testing.T) {
	tests := []struct {
		name           string
		moduleName     string
		chatID         int64
		isCalledResult bool
		isCalledError  error
		expected       bool
	}{
		{
			name:           "module called",
			moduleName:     "module1",
			chatID:         123456789,
			isCalledResult: true,
			isCalledError:  nil,
			expected:       true,
		},
		{
			name:           "module not called",
			moduleName:     "module1",
			chatID:         123456789,
			isCalledResult: false,
			isCalledError:  nil,
			expected:       false,
		},
		{
			name:           "error checking if called",
			moduleName:     "module1",
			chatID:         123456789,
			isCalledResult: false,
			isCalledError:  &tgbotapi.Error{},
			expected:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a bot with a mock configuration
			bot := &Bot{
				BotConfig: &CalarbotConfig{
					Modules: map[string]ModulesConfig{
						tt.moduleName: {
							Url: "http://localhost:8080",
						},
					},
				},
			}

			// Create a mock module client
			mockClient := NewMockModuleClient()
			mockClient.IsCalledResult = tt.isCalledResult
			mockClient.IsCalledError = tt.isCalledError

			// Create a test update
			update := tgbotapi.Update{
				Message: &tgbotapi.Message{
					Chat: &tgbotapi.Chat{
						ID: tt.chatID,
					},
				},
			}

			// Create a test payload
			payload := &botModules.Payload{
				Msg: update.Message,
			}

			// Call shouldIAnswer with our mock client
			result := bot.shouldIAnswer(tt.moduleName, update, mockClient, payload)

			// Verify the result
			if result != tt.expected {
				t.Errorf("shouldIAnswer() = %v, want %v", result, tt.expected)
			}

			// Verify that IsCalled was called with the correct payload
			if mockClient.IsCalledPayload == nil {
				t.Errorf("IsCalled was not called")
			} else if mockClient.IsCalledPayload.Msg != update.Message {
				t.Errorf("IsCalled was called with wrong message")
			}
		})
	}
}

func TestSortModules(t *testing.T) {
	tests := []struct {
		name     string
		input    []moduleOrder
		expected []moduleOrder
	}{
		{
			name: "already sorted",
			input: []moduleOrder{
				{name: "module1", order: 10},
				{name: "module2", order: 20},
				{name: "module3", order: 30},
			},
			expected: []moduleOrder{
				{name: "module1", order: 10},
				{name: "module2", order: 20},
				{name: "module3", order: 30},
			},
		},
		{
			name: "reverse sorted",
			input: []moduleOrder{
				{name: "module3", order: 30},
				{name: "module2", order: 20},
				{name: "module1", order: 10},
			},
			expected: []moduleOrder{
				{name: "module1", order: 10},
				{name: "module2", order: 20},
				{name: "module3", order: 30},
			},
		},
		{
			name: "random order",
			input: []moduleOrder{
				{name: "module2", order: 20},
				{name: "module1", order: 10},
				{name: "module3", order: 30},
			},
			expected: []moduleOrder{
				{name: "module1", order: 10},
				{name: "module2", order: 20},
				{name: "module3", order: 30},
			},
		},
		{
			name:     "empty slice",
			input:    []moduleOrder{},
			expected: []moduleOrder{},
		},
		{
			name: "duplicate order values",
			input: []moduleOrder{
				{name: "module2", order: 20},
				{name: "module1", order: 10},
				{name: "module3", order: 20}, // Same order as module2
				{name: "module4", order: 10}, // Same order as module1
			},
			expected: []moduleOrder{
				{name: "module1", order: 10},
				{name: "module4", order: 10},
				{name: "module2", order: 20},
				{name: "module3", order: 20},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sortModules(tt.input)

			// Check if the result has the same length as expected
			if len(result) != len(tt.expected) {
				t.Errorf("sortModules() returned slice of length %d, want %d", len(result), len(tt.expected))
				return
			}

			// Check if each element matches the expected value
			for i, v := range result {
				if v.name != tt.expected[i].name || v.order != tt.expected[i].order {
					t.Errorf("sortModules()[%d] = {name: %s, order: %d}, want {name: %s, order: %d}",
						i, v.name, v.order, tt.expected[i].name, tt.expected[i].order)
				}
			}
		})
	}
}
