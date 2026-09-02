package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"calarbot2/botModules"
)

func regServer(t *testing.T, reg botModules.Registration, calls *int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*calls++
		json.NewEncoder(w).Encode(reg)
	}))
}

func TestRegistryGetFetchesOnceWithinTTL(t *testing.T) {
	calls := 0
	srv := regServer(t, botModules.Registration{Order: 100, Label: "AI-ответ"}, &calls)
	defer srv.Close()

	r := NewRegistry(map[string]string{"aiAnswer": srv.URL}, time.Minute)

	for i := 0; i < 3; i++ {
		reg, err := r.Get("aiAnswer")
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		if reg.Label != "AI-ответ" {
			t.Fatalf("Label = %q; want AI-ответ", reg.Label)
		}
	}
	if calls != 1 {
		t.Errorf("module was asked %d times; want 1 — регистрация от чата не зависит", calls)
	}
}

func TestRegistryRefetchesAfterTTL(t *testing.T) {
	calls := 0
	srv := regServer(t, botModules.Registration{Order: 100}, &calls)
	defer srv.Close()

	now := time.Unix(1000, 0)
	r := NewRegistry(map[string]string{"aiAnswer": srv.URL}, 30*time.Second)
	r.now = func() time.Time { return now }

	if _, err := r.Get("aiAnswer"); err != nil {
		t.Fatalf("Get: %v", err)
	}
	now = now.Add(31 * time.Second)
	if _, err := r.Get("aiAnswer"); err != nil {
		t.Fatalf("Get: %v", err)
	}

	if calls != 2 {
		t.Errorf("module was asked %d times; want 2", calls)
	}
}

// Модуль лежит — панель обязана всё равно отрисоваться: тумблер работает и без
// него, включать и выключать модуль можно, не спрашивая модуль.
func TestRegistryReportsUnreachableModule(t *testing.T) {
	r := NewRegistry(map[string]string{"dead": "http://127.0.0.1:1"}, time.Minute)

	if _, err := r.Get("dead"); err == nil {
		t.Fatal("Get on an unreachable module returned nil error")
	}
	if names := r.Names(); len(names) != 1 || names[0] != "dead" {
		t.Fatalf("Names = %v; want [dead] — модуль из реестра не исчезает", names)
	}
}

func TestRegistryNamesOrderedByModuleOrder(t *testing.T) {
	calls := 0
	late := regServer(t, botModules.Registration{Order: 1000}, &calls)
	defer late.Close()
	early := regServer(t, botModules.Registration{Order: 100}, &calls)
	defer early.Close()

	r := NewRegistry(map[string]string{"simpleReply": late.URL, "aiAnswer": early.URL}, time.Minute)

	names := r.Names()
	if len(names) != 2 || names[0] != "aiAnswer" || names[1] != "simpleReply" {
		t.Fatalf("Names = %v; want [aiAnswer simpleReply]", names)
	}
}
