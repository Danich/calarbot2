// Vibecoded it because I'm lazy AF

package main

import (
	"testing"

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

	tests := []struct {
		name     string
		message  *tgbotapi.Message
		expected bool
	}{
		{
			name:     "nil message",
			message:  nil,
			expected: false,
		},
		{
			name:     "empty message",
			message:  &tgbotapi.Message{Text: ""},
			expected: false,
		},
		{
			name:     "message with /sber command",
			message:  &tgbotapi.Message{Text: "/sber hello"},
			expected: true,
		},
		{
			name:     "message with just /sber",
			message:  &tgbotapi.Message{Text: "/sber"},
			expected: true,
		},
		{
			name:     "message without /sber",
			message:  &tgbotapi.Message{Text: "hello"},
			expected: false,
		},
		{
			name:     "message with /sber in the middle",
			message:  &tgbotapi.Message{Text: "hello /sber world"},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := module.IsCalled(tt.message)
			if result != tt.expected {
				t.Errorf("IsCalled() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestExtractTextAfterCommand(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		command  string
		expected string
	}{
		{
			name:     "command only",
			text:     "/sber",
			command:  "/sber",
			expected: "",
		},
		{
			name:     "command with text",
			text:     "/sber hello world",
			command:  "/sber",
			expected: "hello world",
		},
		{
			name:     "command with leading space",
			text:     "/sber  hello",
			command:  "/sber",
			expected: "hello",
		},
		{
			name:     "different command",
			text:     "/other hello",
			command:  "/sber",
			expected: "/other hello",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractTextAfterCommand(tt.text, tt.command)
			if result != tt.expected {
				t.Errorf("extractTextAfterCommand() = %q, want %q", result, tt.expected)
			}
		})
	}
}
