package models_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"calarbot2/modules/aiAnswer/models"
	"calarbot2/modules/aiAnswer/router"
)

type mockMeta struct{ data map[string]string }

func newMockMeta(kvs ...string) *mockMeta {
	m := &mockMeta{data: make(map[string]string)}
	for i := 0; i+1 < len(kvs); i += 2 {
		m.data[kvs[i]] = kvs[i+1]
	}
	return m
}

func (m *mockMeta) GetMeta(key string) (string, bool, error) {
	v, ok := m.data[key]
	return v, ok, nil
}

func (m *mockMeta) SetMeta(key, value string) error {
	m.data[key] = value
	return nil
}

func TestModelSelectorLoadsCachedModel(t *testing.T) {
	meta := newMockMeta("top_model", "cached-model-id")
	sel := models.NewModelSelector(meta, "")
	if sel.Get() != "cached-model-id" {
		t.Errorf("Get() = %q, want %q", sel.Get(), "cached-model-id")
	}
}

func TestModelSelectorFallbackWhenNoCache(t *testing.T) {
	sel := models.NewModelSelector(newMockMeta(), "")
	if sel.Get() != models.FallbackModel {
		t.Errorf("Get() = %q, want %q", sel.Get(), models.FallbackModel)
	}
}

func TestModelSelectorRefreshUpdatesModel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"models": []map[string]string{{"id": "new-top-model"}},
		})
	}))
	defer server.Close()

	meta := newMockMeta()
	sel := models.NewModelSelector(meta, server.URL)
	sel.Refresh()

	if sel.Get() != "new-top-model" {
		t.Errorf("Get() = %q after Refresh(), want %q", sel.Get(), "new-top-model")
	}
	if v, ok, _ := meta.GetMeta("top_model"); !ok || v != "new-top-model" {
		t.Errorf("meta top_model = %q ok=%v, want new-top-model", v, ok)
	}
}

func TestModelSelectorKeepsCachedOnRefreshFailure(t *testing.T) {
	meta := newMockMeta("top_model", "prev-model")
	sel := models.NewModelSelector(meta, "http://127.0.0.1:1") // unreachable
	sel.Refresh()                                               // should not panic or clear model

	if sel.Get() != "prev-model" {
		t.Errorf("Get() = %q after failed Refresh(), want prev-model", sel.Get())
	}
}

func TestStaticModelGet(t *testing.T) {
	m := models.NewStaticModel("openai/gpt-4o-mini")
	if m.Get() != "openai/gpt-4o-mini" {
		t.Errorf("Get() = %q, want %q", m.Get(), "openai/gpt-4o-mini")
	}
}

func TestOpenRouterClientClassify(t *testing.T) {
	tests := []struct {
		response string
		want     router.Route
	}{
		{"translate", router.RouteTranslate},
		{"imagegen", router.RouteImageGen},
		{"vision", router.RouteVision},
		{"question", router.RouteQuestion},
		{"chat", router.RouteChat},
		{"something unexpected", router.RouteChat},
	}

	for _, tt := range tests {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"choices": []map[string]interface{}{
					{"message": map[string]string{"content": tt.response}},
				},
			})
		}))

		meta := newMockMeta("top_model", "test-model")
		sel := models.NewModelSelector(meta, "")
		client := models.NewOpenRouterClient("test-key", sel, server.URL)

		got, err := client.Classify(context.Background(), "some text")
		server.Close()

		if err != nil {
			t.Errorf("Classify(%q) error: %v", tt.response, err)
			continue
		}
		if got != tt.want {
			t.Errorf("Classify(%q) = %q, want %q", tt.response, got, tt.want)
		}
	}
}
