package store_test

import (
	"fmt"
	"testing"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"calarbot2/modules/aiAnswer/store"
)

func newTestStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.New(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func msg(chatID int64, userID int64, username, text string) *tgbotapi.Message {
	return &tgbotapi.Message{
		Chat: &tgbotapi.Chat{ID: chatID},
		From: &tgbotapi.User{ID: userID, UserName: username},
		Text: text,
		Date: int(time.Now().Unix()),
	}
}

func TestSaveAndGetContext(t *testing.T) {
	s := newTestStore(t)
	if err := s.SaveMessage(msg(100, 1, "alice", "hello")); err != nil {
		t.Fatalf("SaveMessage: %v", err)
	}
	msgs, err := s.GetContext(100, 10)
	if err != nil {
		t.Fatalf("GetContext: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("len(msgs) = %d, want 1", len(msgs))
	}
	if msgs[0].Text != "hello" || msgs[0].Username != "alice" {
		t.Errorf("got %+v", msgs[0])
	}
}

func TestContextIsolatedByChatID(t *testing.T) {
	s := newTestStore(t)
	s.SaveMessage(msg(100, 1, "alice", "msg in chat 100"))
	s.SaveMessage(msg(200, 2, "bob", "msg in chat 200"))

	msgs, _ := s.GetContext(100, 10)
	if len(msgs) != 1 {
		t.Fatalf("chat 100: len=%d, want 1", len(msgs))
	}
	if msgs[0].Text != "msg in chat 100" {
		t.Errorf("chat 100 got wrong message: %q", msgs[0].Text)
	}
}

func TestGetContextChronological(t *testing.T) {
	s := newTestStore(t)
	for i := 0; i < 3; i++ {
		m := msg(100, 1, "alice", fmt.Sprintf("msg%d", i))
		m.Date = int(time.Now().Unix()) + i
		s.SaveMessage(m)
	}
	msgs, _ := s.GetContext(100, 10)
	if len(msgs) != 3 {
		t.Fatalf("len=%d, want 3", len(msgs))
	}
	for i, m := range msgs {
		want := fmt.Sprintf("msg%d", i)
		if m.Text != want {
			t.Errorf("msgs[%d].Text = %q, want %q", i, m.Text, want)
		}
	}
}

func TestGetContextLimit(t *testing.T) {
	s := newTestStore(t)
	for i := 0; i < 5; i++ {
		s.SaveMessage(msg(100, 1, "alice", fmt.Sprintf("msg%d", i)))
	}
	msgs, _ := s.GetContext(100, 3)
	if len(msgs) != 3 {
		t.Fatalf("len=%d, want 3", len(msgs))
	}
}

func TestMeta(t *testing.T) {
	s := newTestStore(t)

	_, ok, err := s.GetMeta("missing")
	if err != nil || ok {
		t.Fatalf("expected empty, got ok=%v err=%v", ok, err)
	}

	if err := s.SetMeta("key", "value1"); err != nil {
		t.Fatalf("SetMeta: %v", err)
	}
	val, ok, _ := s.GetMeta("key")
	if !ok || val != "value1" {
		t.Errorf("GetMeta = %q ok=%v", val, ok)
	}

	s.SetMeta("key", "value2")
	val, _, _ = s.GetMeta("key")
	if val != "value2" {
		t.Errorf("upsert: got %q, want value2", val)
	}
}
