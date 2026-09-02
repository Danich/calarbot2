package main

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"calarbot2/botModules"
	"calarbot2/settings"
)

type fakeLeaver struct {
	left []int64
	err  error
}

func (f *fakeLeaver) LeaveChat(chatID int64) error {
	f.left = append(f.left, chatID)
	return f.err
}

func testAPI(t *testing.T, reg botModules.Registration) (*API, *fakeLeaver, http.Handler) {
	t.Helper()
	s, err := settings.New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("settings.New: %v", err)
	}
	t.Cleanup(func() { s.Close() })

	calls := 0
	srv := regServer(t, reg, &calls)
	t.Cleanup(srv.Close)

	leaver := &fakeLeaver{}
	api := &API{
		Store:    s,
		Registry: NewRegistry(map[string]string{"aiAnswer": srv.URL}, time.Minute),
		Leaver:   leaver,
		Now:      func() int64 { return 1000 },
	}
	mux := http.NewServeMux()
	api.Routes(mux)
	return api, leaver, mux
}

func numberReg() botModules.Registration {
	min, max := 0, 1000
	return botModules.Registration{
		Order: 100, Label: "AI-ответ",
		Fields: []botModules.Field{
			{Key: "answer_level", Type: botModules.FieldNumber, Min: &min, Max: &max, Default: 990},
			{Key: "persona", Type: botModules.FieldSelect, Default: "mamkin",
				Options: []botModules.Option{{Value: "mamkin"}, {Value: "genadiy"}}},
		},
	}
}

func do(t *testing.T, h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestPatchModuleEnables(t *testing.T) {
	api, _, h := testAPI(t, numberReg())

	if rec := do(t, h, http.MethodPatch, "/api/chats/-1/modules/aiAnswer", `{"enabled":true}`); rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, body %s; want 204", rec.Code, rec.Body)
	}

	if enabled, err := api.Store.ModuleEnabled(-1, "aiAnswer"); err != nil || !enabled {
		t.Fatalf("ModuleEnabled = %v, %v; want true", enabled, err)
	}
}

func TestPatchSettingsStoresValue(t *testing.T) {
	api, _, h := testAPI(t, numberReg())

	if rec := do(t, h, http.MethodPatch, "/api/chats/-1/settings/aiAnswer", `{"answer_level":700}`); rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, body %s; want 204", rec.Code, rec.Body)
	}

	values, err := api.Store.Values(-1, "aiAnswer")
	if err != nil {
		t.Fatalf("Values: %v", err)
	}
	if values["answer_level"] != "700" {
		t.Fatalf("answer_level = %q; want \"700\"", values["answer_level"])
	}
}

// Молча обрезать вылезшее за границу — худший вариант: настройка выглядит
// принятой, а бот ведёт себя иначе.
func TestPatchSettingsRejectsOutOfRange(t *testing.T) {
	api, _, h := testAPI(t, numberReg())

	rec := do(t, h, http.MethodPatch, "/api/chats/-1/settings/aiAnswer", `{"answer_level":5000}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d; want 400", rec.Code)
	}

	values, _ := api.Store.Values(-1, "aiAnswer")
	if _, ok := values["answer_level"]; ok {
		t.Error("отвергнутое значение всё-таки записалось")
	}
}

func TestPatchSettingsRejectsUnknownSelectOption(t *testing.T) {
	_, _, h := testAPI(t, numberReg())

	if rec := do(t, h, http.MethodPatch, "/api/chats/-1/settings/aiAnswer", `{"persona":"нет такой"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d; want 400", rec.Code)
	}
}

func TestPatchSettingsRejectsUndeclaredKey(t *testing.T) {
	_, _, h := testAPI(t, numberReg())

	if rec := do(t, h, http.MethodPatch, "/api/chats/-1/settings/aiAnswer", `{"whatever":1}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d; want 400", rec.Code)
	}
}

// null — это «вернуть к дефолту», а не «записать ноль».
func TestPatchSettingsNullDeletesTheRow(t *testing.T) {
	api, _, h := testAPI(t, numberReg())
	if err := api.Store.SetValue(-1, "aiAnswer", "answer_level", "700"); err != nil {
		t.Fatalf("SetValue: %v", err)
	}

	if rec := do(t, h, http.MethodPatch, "/api/chats/-1/settings/aiAnswer", `{"answer_level":null}`); rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, body %s; want 204", rec.Code, rec.Body)
	}

	values, _ := api.Store.Values(-1, "aiAnswer")
	if _, ok := values["answer_level"]; ok {
		t.Fatalf("Values = %v; want the row gone", values)
	}
}

func TestLeaveCallsTelegramAndMarksTheChat(t *testing.T) {
	api, leaver, h := testAPI(t, numberReg())
	if err := api.Store.UpsertChat(settings.Chat{ID: -1, Type: "group", FirstSeen: 1, LastSeen: 1}); err != nil {
		t.Fatalf("UpsertChat: %v", err)
	}

	if rec := do(t, h, http.MethodPost, "/api/chats/-1/leave", ""); rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, body %s; want 204", rec.Code, rec.Body)
	}

	if len(leaver.left) != 1 || leaver.left[0] != -1 {
		t.Errorf("LeaveChat calls = %v; want [-1]", leaver.left)
	}
	chats, _ := api.Store.ListChats()
	if len(chats) != 0 {
		t.Errorf("ListChats = %+v; want the chat gone from the panel", chats)
	}
}

// Телеграм отказал — чат остаётся в панели: помечать уходом то, из чего не
// вышли, значит потерять канал из виду.
func TestLeaveKeepsChatWhenTelegramFails(t *testing.T) {
	api, leaver, h := testAPI(t, numberReg())
	leaver.err = errors.New("telegram is unhappy")
	if err := api.Store.UpsertChat(settings.Chat{ID: -1, Type: "group", FirstSeen: 1, LastSeen: 1}); err != nil {
		t.Fatalf("UpsertChat: %v", err)
	}

	if rec := do(t, h, http.MethodPost, "/api/chats/-1/leave", ""); rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d; want 502", rec.Code)
	}

	chats, _ := api.Store.ListChats()
	if len(chats) != 1 {
		t.Fatalf("ListChats = %+v; want the chat still listed", chats)
	}
}
