package main

import (
	"path/filepath"
	"testing"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"calarbot2/settings"
)

func botWithSettings(t *testing.T) *Bot {
	t.Helper()
	s, err := settings.New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("settings.New: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return &Bot{SettingsStore: s}
}

func TestRecordChatStoresTitleAndType(t *testing.T) {
	b := botWithSettings(t)

	b.recordChat(&tgbotapi.Chat{ID: -1, Type: "supergroup", Title: "болталка"}, 1000)

	chats, err := b.SettingsStore.ListChats()
	if err != nil {
		t.Fatalf("ListChats: %v", err)
	}
	if len(chats) != 1 {
		t.Fatalf("ListChats returned %d; want 1", len(chats))
	}
	if chats[0].Title != "болталка" || chats[0].Type != "supergroup" {
		t.Errorf("chat = %+v; want болталка/supergroup", chats[0])
	}
}

// У лички нет title — там имя и @username. Иначе список личек в панели пуст.
func TestRecordChatNamesPrivateChatsByUser(t *testing.T) {
	b := botWithSettings(t)

	b.recordChat(&tgbotapi.Chat{ID: 42, Type: "private", FirstName: "Даня", UserName: "danich"}, 1000)

	chats, err := b.SettingsStore.ListChats()
	if err != nil {
		t.Fatalf("ListChats: %v", err)
	}
	if len(chats) != 1 {
		t.Fatalf("ListChats returned %d; want 1", len(chats))
	}
	if chats[0].Type != "private" {
		t.Fatalf("type = %q; want private", chats[0].Type)
	}
	if chats[0].Title != "Даня" || chats[0].Username != "danich" {
		t.Errorf("chat = %+v; want Даня/danich", chats[0])
	}
}

// Бота выгнали — канал должен уйти из панели, не дожидаясь чужих сообщений.
func TestRecordMembershipMarksKickedChatLeft(t *testing.T) {
	b := botWithSettings(t)
	b.recordChat(&tgbotapi.Chat{ID: -1, Type: "group", Title: "турки"}, 1000)

	b.recordMembership(&tgbotapi.ChatMemberUpdated{
		Chat:          tgbotapi.Chat{ID: -1, Type: "group", Title: "турки"},
		NewChatMember: tgbotapi.ChatMember{Status: "kicked"},
		Date:          2000,
	})

	chats, err := b.SettingsStore.ListChats()
	if err != nil {
		t.Fatalf("ListChats: %v", err)
	}
	if len(chats) != 0 {
		t.Fatalf("ListChats returned %+v; want none — бота там больше нет", chats)
	}
}

func TestRecordMembershipAddsNewChat(t *testing.T) {
	b := botWithSettings(t)

	b.recordMembership(&tgbotapi.ChatMemberUpdated{
		Chat:          tgbotapi.Chat{ID: -7, Type: "group", Title: "новый"},
		NewChatMember: tgbotapi.ChatMember{Status: "member"},
		Date:          2000,
	})

	chats, err := b.SettingsStore.ListChats()
	if err != nil {
		t.Fatalf("ListChats: %v", err)
	}
	if len(chats) != 1 || chats[0].ID != -7 {
		t.Fatalf("ListChats = %+v; want the new chat -7", chats)
	}
}
