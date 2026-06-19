package main

import (
	"testing"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"calarbot2/common"
)

func TestModuleOrder(t *testing.T) {
	m := NewModule(42, AIConfig{BotUsername: "testbot", AnswerLevel: 500})
	if m.Order() != 42 {
		t.Errorf("Order() = %d, want 42", m.Order())
	}
}

func TestModuleIsCalledNilMessage(t *testing.T) {
	m := NewModule(0, AIConfig{BotUsername: "testbot", AnswerLevel: 500})
	if m.IsCalled(nil) {
		t.Error("IsCalled(nil) should return false")
	}
}

func TestModuleIsCalledReplyToBot(t *testing.T) {
	m := NewModule(0, AIConfig{
		BotUsername: "testbot",
		AnswerLevel: DiceSize + 100,
		ReplyWeight: DiceSize + 200,
	})
	msg := &tgbotapi.Message{
		Text: "reply",
		Chat: &tgbotapi.Chat{ID: 1},
		From: &tgbotapi.User{ID: 1},
		ReplyToMessage: &tgbotapi.Message{
			From: &tgbotapi.User{UserName: "testbot"},
		},
	}
	if !m.IsCalled(msg) {
		t.Error("IsCalled with reply to bot should return true")
	}
}

func TestModuleIsCalledMentionBot(t *testing.T) {
	m := NewModule(0, AIConfig{
		BotUsername: "testbot",
		AnswerLevel: DiceSize + 100,
		CallWeight:  DiceSize + 200,
	})
	msg := &tgbotapi.Message{
		Text: "Hello @testbot",
		Chat: &tgbotapi.Chat{ID: 1},
		From: &tgbotapi.User{ID: 1},
		Entities: []tgbotapi.MessageEntity{
			{Type: "mention", Offset: 6, Length: 8},
		},
	}
	if !m.IsCalled(msg) {
		t.Error("IsCalled with mention should return true")
	}
}

func TestModuleIsCalledDirectReplyAlwaysTrue(t *testing.T) {
	// Direct reply to bot is always true regardless of AnswerLevel
	m := NewModule(0, AIConfig{
		BotUsername: "testbot",
		AnswerLevel: DiceSize + 999,
		ReplyWeight: 0,
	})
	msg := &tgbotapi.Message{
		Text: "reply",
		Chat: &tgbotapi.Chat{ID: 1},
		From: &tgbotapi.User{ID: 1},
		ReplyToMessage: &tgbotapi.Message{
			From: &tgbotapi.User{UserName: "testbot"},
		},
	}
	if !m.IsCalled(msg) {
		t.Error("IsCalled with direct reply to bot should always return true")
	}
}

func TestModuleIsCalledDirectMentionAlwaysTrue(t *testing.T) {
	// Direct @mention is always true regardless of AnswerLevel
	m := NewModule(0, AIConfig{
		BotUsername: "testbot",
		AnswerLevel: DiceSize + 999,
		CallWeight:  0,
	})
	msg := &tgbotapi.Message{
		Text: "Hello @testbot",
		Chat: &tgbotapi.Chat{ID: 1},
		From: &tgbotapi.User{ID: 1},
		Entities: []tgbotapi.MessageEntity{
			{Type: "mention", Offset: 6, Length: 8},
		},
	}
	if !m.IsCalled(msg) {
		t.Error("IsCalled with @mention should always return true (direct address)")
	}
}

func TestExtractMentions(t *testing.T) {
	tests := []struct {
		name     string
		msg      *tgbotapi.Message
		expected []string
	}{
		{
			name: "single mention",
			msg: &tgbotapi.Message{
				Text:     "Hello @testbot",
				Entities: []tgbotapi.MessageEntity{{Type: "mention", Offset: 6, Length: 8}},
			},
			expected: []string{"@testbot"},
		},
		{
			name:     "no mentions",
			msg:      &tgbotapi.Message{Text: "Hello world", Entities: []tgbotapi.MessageEntity{}},
			expected: []string{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mentions := common.ExtractMentions(tt.msg)
			if len(mentions) != len(tt.expected) {
				t.Errorf("got %d mentions, want %d", len(mentions), len(tt.expected))
				return
			}
			for i, m := range mentions {
				if m != tt.expected[i] {
					t.Errorf("mentions[%d] = %q, want %q", i, m, tt.expected[i])
				}
			}
		})
	}
}
