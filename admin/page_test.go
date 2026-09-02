package main

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"calarbot2/botModules"
	"calarbot2/settings"
)

func testPage(t *testing.T) (*Page, http.Handler) {
	t.Helper()
	s, err := settings.New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("settings.New: %v", err)
	}
	t.Cleanup(func() { s.Close() })

	calls := 0
	srv := regServer(t, numberReg(), &calls)
	t.Cleanup(srv.Close)

	p := &Page{Store: s, Registry: NewRegistry(map[string]string{"aiAnswer": srv.URL}, time.Minute)}
	return p, p.Handler()
}

func TestPageRendersChannelsAndDMs(t *testing.T) {
	p, h := testPage(t)
	if err := p.Store.UpsertChat(settings.Chat{ID: -1, Type: "supergroup", Title: "болталка", FirstSeen: 1, LastSeen: 1}); err != nil {
		t.Fatalf("UpsertChat: %v", err)
	}
	if err := p.Store.UpsertChat(settings.Chat{ID: 42, Type: "private", Title: "Даня", Username: "danich", FirstSeen: 1, LastSeen: 1}); err != nil {
		t.Fatalf("UpsertChat: %v", err)
	}

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{"болталка", "AI-ответ", "Даня", "danich", "Личные сообщения"} {
		if !strings.Contains(body, want) {
			t.Errorf("page does not contain %q", want)
		}
	}
	// Личка — не канал: тумблеров модулей у неё в списке каналов быть не должно.
	if strings.Count(body, `data-chat="42"`) != 1 {
		t.Errorf("private chat rendered %d times; want once, in the DM table", strings.Count(body, `data-chat="42"`))
	}
}

func TestPageShowsStoredValueNotDefault(t *testing.T) {
	p, h := testPage(t)
	if err := p.Store.UpsertChat(settings.Chat{ID: -1, Type: "group", Title: "чат", FirstSeen: 1, LastSeen: 1}); err != nil {
		t.Fatalf("UpsertChat: %v", err)
	}
	if err := p.Store.SetModuleEnabled(-1, "aiAnswer", true); err != nil {
		t.Fatalf("SetModuleEnabled: %v", err)
	}
	if err := p.Store.SetValue(-1, "aiAnswer", "answer_level", "700"); err != nil {
		t.Fatalf("SetValue: %v", err)
	}

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if !strings.Contains(rec.Body.String(), `value="700"`) {
		t.Error("page does not show the stored 700")
	}
}

// bool и text — не number: у них должны быть чекбокс и текстовое поле, а не
// два числовых инпута, которые validateValue потом отверг бы с 400.
func TestPageRendersBoolAndTextFields(t *testing.T) {
	s, err := settings.New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("settings.New: %v", err)
	}
	defer s.Close()
	if err := s.UpsertChat(settings.Chat{ID: -1, Type: "group", Title: "чат", FirstSeen: 1, LastSeen: 1}); err != nil {
		t.Fatalf("UpsertChat: %v", err)
	}

	calls := 0
	reg := botModules.Registration{
		Order: 100, Label: "Тест",
		Fields: []botModules.Field{
			{Key: "loud", Type: botModules.FieldBool, Default: true},
			{Key: "greeting", Type: botModules.FieldText, Default: "привет"},
		},
	}
	srv := regServer(t, reg, &calls)
	defer srv.Close()

	p := &Page{Store: s, Registry: NewRegistry(map[string]string{"test": srv.URL}, time.Minute)}

	rec := httptest.NewRecorder()
	p.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	body := rec.Body.String()
	if !strings.Contains(body, `data-key="loud" type="checkbox"`) {
		t.Errorf("bool field did not render as a checkbox:\n%s", body)
	}
	if !strings.Contains(body, `data-key="greeting" type="text"`) {
		t.Errorf("text field did not render as a text input:\n%s", body)
	}
	if strings.Contains(body, `data-key="loud" type="number"`) || strings.Contains(body, `data-key="greeting" type="number"`) {
		t.Error("bool/text fields rendered as number inputs")
	}
}

// Модуль лежит — страница обязана отрисоваться, а тумблер остаться рабочим.
func TestPageSurvivesUnreachableModule(t *testing.T) {
	s, err := settings.New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("settings.New: %v", err)
	}
	defer s.Close()
	if err := s.UpsertChat(settings.Chat{ID: -1, Type: "group", Title: "чат", FirstSeen: 1, LastSeen: 1}); err != nil {
		t.Fatalf("UpsertChat: %v", err)
	}

	p := &Page{Store: s, Registry: NewRegistry(map[string]string{"dead": "http://127.0.0.1:1"}, time.Minute)}

	rec := httptest.NewRecorder()
	p.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "dead") {
		t.Error("page does not name the unreachable module")
	}
	if !strings.Contains(body, `data-module="dead"`) {
		t.Error("page has no toggle for the unreachable module")
	}
}
