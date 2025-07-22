// Vibecoded it because I'm lazy AF

package main

import (
	"testing"

	"calarbot2/botModules"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func TestModuleOrder(t *testing.T) {
	module := Module{order: 42}
	if module.Order() != 42 {
		t.Errorf("Expected order to be 42, got %d", module.Order())
	}
}

func TestModuleIsCalled(t *testing.T) {
	module := Module{}

	// Test with nil message
	if !module.IsCalled(nil) {
		t.Errorf("Expected IsCalled(nil) to return true")
	}

	// Test with a message
	msg := &tgbotapi.Message{
		Text: "Hello",
	}
	if !module.IsCalled(msg) {
		t.Errorf("Expected IsCalled(msg) to return true")
	}
}

func TestModuleAnswer(t *testing.T) {
	module := Module{}

	tests := []struct {
		name     string
		message  string
		expected string
	}{
		{
			name:     "empty message",
			message:  "",
			expected: "",
		},
		{
			name:     "simple message",
			message:  "Hello",
			expected: "Hello",
		},
		{
			name:     "message with special characters",
			message:  "Hello, world! 123 @#$%^&*()",
			expected: "Hello, world! 123 @#$%^&*()",
		},
		{
			name:     "message with emoji",
			message:  "Hello 😀",
			expected: "Hello 😀",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a payload with the test message
			payload := &botModules.Payload{
				Msg: &tgbotapi.Message{
					Text: tt.message,
				},
			}

			// Call the Answer method
			answer, err := module.Answer(payload)

			// Check for errors
			if err != nil {
				t.Errorf("Answer() returned an error: %v", err)
			}

			// Check the answer
			if answer != tt.expected {
				t.Errorf("Answer() = %q, want %q", answer, tt.expected)
			}
		})
	}
}
