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
	// Should be the 3 newest: msg2, msg3, msg4
	if msgs[0].Text != "msg2" || msgs[1].Text != "msg3" || msgs[2].Text != "msg4" {
		t.Errorf("GetContext(limit=3) returned wrong messages: got %v, want msg2,msg3,msg4",
			[]string{msgs[0].Text, msgs[1].Text, msgs[2].Text})
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

// Свои реплики тоже должны попадать в контекст: иначе на реплай боту модель
// видит ответ человека и не видит, на что он был.
func TestSaveBotMessageJoinsTheContext(t *testing.T) {
	s := newTestStore(t)

	if err := s.SaveMessage(&tgbotapi.Message{
		Chat: &tgbotapi.Chat{ID: 7}, From: &tgbotapi.User{ID: 1, UserName: "vasya"},
		Text: "привет", Date: 100,
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.SaveBotMessage(7, "calarbot", "здорово", 101); err != nil {
		t.Fatal(err)
	}

	got, err := s.GetContext(7, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("в контексте %d сообщений, ожидалось два: %+v", len(got), got)
	}
	if got[1].Username != "calarbot" || got[1].Text != "здорово" {
		t.Errorf("ответ бота не в контексте: %+v", got)
	}
}

// В одну секунду может прийти несколько сообщений. Без вторичного ключа их
// порядок произволен, и разговор в промпте перемешивается.
func TestGetContextIsStableWithinASecond(t *testing.T) {
	s := newTestStore(t)
	for _, text := range []string{"раз", "два", "три"} {
		if err := s.SaveMessage(&tgbotapi.Message{
			Chat: &tgbotapi.Chat{ID: 9}, From: &tgbotapi.User{ID: 1, UserName: "vasya"},
			Text: text, Date: 500,
		}); err != nil {
			t.Fatal(err)
		}
	}

	got, err := s.GetContext(9, 10)
	if err != nil {
		t.Fatal(err)
	}
	var order []string
	for _, m := range got {
		order = append(order, m.Text)
	}
	if len(order) != 3 || order[0] != "раз" || order[1] != "два" || order[2] != "три" {
		t.Fatalf("порядок в одной секунде разъехался: %v", order)
	}
}
