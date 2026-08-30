package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

func TestFormatPutsTheApplicationInBold(t *testing.T) {
	got := Format(Request{Application: "hw-ru", Payload: "disk / 94%"})
	want := "<b>hw-ru</b>\ndisk / 94%"
	if got != want {
		t.Fatalf("Format() = %q, want %q", got, want)
	}
}

// Полезная нагрузка приходит из чужих сообщений об ошибках, где < и & —
// обычное дело. Без эскейпинга телеграм отвергает сообщение целиком.
func TestFormatEscapesBothFields(t *testing.T) {
	got := Format(Request{Application: "a<b", Payload: "x & y > z"})
	want := "<b>a&lt;b</b>\nx &amp; y &gt; z"
	if got != want {
		t.Fatalf("Format() = %q, want %q", got, want)
	}
}

// Усечённый алерт полезнее непришедшего.
func TestFormatTruncatesAnOverlongPayload(t *testing.T) {
	got := Format(Request{Application: "a", Payload: strings.Repeat("я", 5000)})
	if !strings.HasSuffix(got, "…") {
		t.Fatal("длинная нагрузка не усечена")
	}
	if n := len([]rune(got)); n > maxMessageRunes+20 {
		t.Fatalf("после усечения %d рун, ожидалось около %d", n, maxMessageRunes)
	}
}

type stubSender struct {
	sent []tgbotapi.MessageConfig
	err  error
}

func (s *stubSender) Send(c tgbotapi.Chattable) (tgbotapi.Message, error) {
	if msg, ok := c.(tgbotapi.MessageConfig); ok {
		s.sent = append(s.sent, msg)
	}
	return tgbotapi.Message{}, s.err
}

func post(t *testing.T, h http.Handler, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/notify", strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestNotifySendsToTheAdmin(t *testing.T) {
	sender := &stubSender{}
	rec := post(t, handleNotify(sender, 42), `{"application":"hw-ru","payload":"диск кончается"}`)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("код %d, ожидался 204: %s", rec.Code, rec.Body)
	}
	if len(sender.sent) != 1 {
		t.Fatalf("отправлено %d сообщений, ожидалось одно", len(sender.sent))
	}
	msg := sender.sent[0]
	if msg.ChatID != 42 {
		t.Errorf("ChatID = %d, ожидался adminId 42", msg.ChatID)
	}
	if msg.ParseMode != tgbotapi.ModeHTML {
		t.Errorf("ParseMode = %q, без HTML заголовок приедет тегами", msg.ParseMode)
	}
	if want := "<b>hw-ru</b>\nдиск кончается"; msg.Text != want {
		t.Errorf("Text = %q, want %q", msg.Text, want)
	}
}

func TestNotifyRejectsBadRequests(t *testing.T) {
	cases := map[string]string{
		"не json":           `{`,
		"пустое приложение": `{"application":"","payload":"x"}`,
		"пустая нагрузка":   `{"application":"x","payload":"   "}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			sender := &stubSender{}
			rec := post(t, handleNotify(sender, 42), body)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("код %d, ожидался 400", rec.Code)
			}
			if len(sender.sent) != 0 {
				t.Fatalf("отправлено %d сообщений, ожидалось ноль", len(sender.sent))
			}
		})
	}
}

// Отправитель должен узнать, что сообщение не доехало, а не считать что всё
// хорошо: на той стороне алертер по ошибке повторит попытку.
func TestNotifyReportsATelegramFailure(t *testing.T) {
	sender := &stubSender{err: errors.New("forbidden")}
	rec := post(t, handleNotify(sender, 42), `{"application":"x","payload":"y"}`)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("код %d, ожидался 502", rec.Code)
	}
}

func TestRequestJSONShape(t *testing.T) {
	var req Request
	if err := json.Unmarshal([]byte(`{"application":"a","payload":"p"}`), &req); err != nil {
		t.Fatal(err)
	}
	if req.Application != "a" || req.Payload != "p" {
		t.Fatalf("разобралось как %+v", req)
	}
}
