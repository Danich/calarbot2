// Vibecoded it because I'm lazy AF

package main

import (
	"testing"

	"calarbot2/common"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func TestModuleOrder(t *testing.T) {
	module := Module{order: 42}
	if module.Order() != 42 {
		t.Errorf("Expected order to be 42, got %d", module.Order())
	}
}

func TestModuleIsCalledNilMessage(t *testing.T) {
	module := Module{aiConfig: AIConfig{AnswerLevel: 500}}
	if module.IsCalled(nil) {
		t.Errorf("Expected IsCalled(nil) to return false")
	}
}

func TestModuleIsCalledReplyToBot(t *testing.T) {
	// This test verifies that the ReplyWeight is added when replying to the bot
	// We can't test the random part, but we can check that the logic for adding weights works

	// Create a module with a very high AnswerLevel so it would normally not be called
	module := Module{
		aiConfig: AIConfig{
			AnswerLevel: DiceSize + 100, // Ensure it's higher than any possible roll
			ReplyWeight: DiceSize + 200, // Ensure it's high enough to exceed the threshold
			BotUsername: "testbot",
		},
	}

	// Create a message that is a reply to the bot
	msg := &tgbotapi.Message{
		Text: "Hello",
		ReplyToMessage: &tgbotapi.Message{
			From: &tgbotapi.User{
				UserName: "testbot",
			},
		},
	}

	// The method should return true because the ReplyWeight is high enough
	if !module.IsCalled(msg) {
		t.Errorf("Expected IsCalled to return true for a reply to the bot with high ReplyWeight")
	}
}

func TestModuleIsCalledMentionBot(t *testing.T) {
	// This test verifies that the CallWeight is added when mentioning the bot

	// Create a module with a very high AnswerLevel so it would normally not be called
	module := Module{
		aiConfig: AIConfig{
			AnswerLevel: DiceSize + 100, // Ensure it's higher than any possible roll
			CallWeight:  DiceSize + 200, // Ensure it's high enough to exceed the threshold
			BotUsername: "testbot",
		},
	}

	// Create a message that mentions the bot
	msg := &tgbotapi.Message{
		Text: "Hello @testbot",
		Entities: []tgbotapi.MessageEntity{
			{
				Type:   "mention",
				Offset: 6,
				Length: 8,
			},
		},
	}

	// The method should return true because the CallWeight is high enough
	if !module.IsCalled(msg) {
		t.Errorf("Expected IsCalled to return true for a message mentioning the bot with high CallWeight")
	}
}

func TestExtractMentions(t *testing.T) {
	tests := []struct {
		name     string
		msg      *tgbotapi.Message
		expected []string
	}{
		{
			name: "message with mention",
			msg: &tgbotapi.Message{
				Text: "Hello @testbot",
				Entities: []tgbotapi.MessageEntity{
					{
						Type:   "mention",
						Offset: 6,
						Length: 8,
					},
				},
			},
			expected: []string{"@testbot"},
		},
		{
			name: "message with multiple mentions",
			msg: &tgbotapi.Message{
				Text: "Hello @testbot and @otherbot",
				Entities: []tgbotapi.MessageEntity{
					{
						Type:   "mention",
						Offset: 6,
						Length: 8,
					},
					{
						Type:   "mention",
						Offset: 19,
						Length: 9,
					},
				},
			},
			expected: []string{"@testbot", "@otherbot"},
		},
		{
			name: "message with no mentions",
			msg: &tgbotapi.Message{
				Text:     "Hello world",
				Entities: []tgbotapi.MessageEntity{},
			},
			expected: []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mentions := common.ExtractMentions(tt.msg)
			if len(mentions) != len(tt.expected) {
				t.Errorf("ExtractMentions() returned %d mentions, want %d", len(mentions), len(tt.expected))
				return
			}
			for i, mention := range mentions {
				if mention != tt.expected[i] {
					t.Errorf("ExtractMentions()[%d] = %q, want %q", i, mention, tt.expected[i])
				}
			}
		})
	}
}
