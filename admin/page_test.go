package main

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

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
