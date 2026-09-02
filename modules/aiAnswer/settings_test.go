package main

import (
	"testing"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"calarbot2/botModules"
)

func TestIntSettingReadsInjectedValue(t *testing.T) {
	s := map[string]any{"answer_level": 700}

	if got := intSetting(s, "answer_level", 990); got != 700 {
		t.Fatalf("intSetting = %d; want 700", got)
	}
}

// По проводу настройки едут json'ом, а там всякое число — float64. Без этой
// ветки модуль тихо свалился бы на дефолт для каждой настройки.
func TestIntSettingAcceptsJSONFloat(t *testing.T) {
	s := map[string]any{"answer_level": float64(700)}

	if got := intSetting(s, "answer_level", 990); got != 700 {
		t.Fatalf("intSetting = %d; want 700", got)
	}
}

func TestIntSettingFallsBackWhenAbsent(t *testing.T) {
	if got := intSetting(map[string]any{}, "answer_level", 990); got != 990 {
		t.Fatalf("intSetting = %d; want the fallback 990", got)
	}
}

func TestStringSettingFallsBackWhenAbsent(t *testing.T) {
	if got := stringSetting(map[string]any{}, "persona", "mamkin"); got != "mamkin" {
		t.Fatalf("stringSetting = %q; want mamkin", got)
	}
}

func TestSettingsOfHandlesMissingExtra(t *testing.T) {
	got := settingsOf(&botModules.Payload{})

	if len(got) != 0 {
		t.Fatalf("settingsOf = %v; want an empty map", got)
	}
}

// Порог 1001 при броске d1000 делает исход однозначным при любом броске:
// с весом из настроек (1001) реплай срабатывает всегда, с дефолтным из
// конфига (0) — не срабатывает никогда. Со значениями внутри 0..1000 тест
// проходил бы по случайности в одном броске из тысячи.
//
// Сообщение обычное, не реплай боту и не упоминание: у reply_weight и
// call_weight в IsCalled нет своего пути мимо isDirectAddress — тот же самый
// признак (реплай/упоминание) сперва ловит isDirectAddress и возвращает true
// раньше, чем roll вообще посчитается, так что этими двумя весами через
// IsCalled никогда ничего не движется, ни из конфига, ни из настроек. Это не
// баг задачи 15 — то же самое верно и до неё (см. main.go до этой задачи),
// так что тест проверяет доезжающую настройку через answer_level, единственный
// вес, который реально участвует в roll для обычного сообщения.
func TestIsCalledUsesInjectedAnswerLevel(t *testing.T) {
	m := &Module{config: AIConfig{BotUsername: "calarbot", AnswerLevel: 1001}}

	payload := &botModules.Payload{
		Msg: plainMessage(),
		Extra: map[string]interface{}{
			"settings": map[string]any{"answer_level": 0},
		},
	}

	if !m.IsCalled(payload) {
		t.Fatal("IsCalled = false; want true — вес из настроек не доехал")
	}
}

func TestIsCalledFallsBackToConfigWeights(t *testing.T) {
	m := &Module{config: AIConfig{BotUsername: "calarbot", AnswerLevel: 1001}}

	if m.IsCalled(&botModules.Payload{Msg: plainMessage()}) {
		t.Fatal("IsCalled = true without settings; want false")
	}
}

func plainMessage() *tgbotapi.Message {
	return &tgbotapi.Message{
		Chat: &tgbotapi.Chat{ID: -1},
		From: &tgbotapi.User{ID: 5, UserName: "человек"},
		Text: "ага",
	}
}
