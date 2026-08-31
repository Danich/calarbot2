package lore_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"calarbot2/modules/aiAnswer/lore"
)

func TestHTTPNotifierPostsApplicationAndPayload(t *testing.T) {
	var got map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &got)
	}))
	defer srv.Close()

	lore.NewHTTPNotifier(srv.URL).Notify("нашёл оливье")

	if got["application"] == "" {
		t.Error("application missing")
	}
	if got["payload"] != "нашёл оливье" {
		t.Errorf("payload = %q", got["payload"])
	}
}

// Уведомления — не критичный путь: недоступный notify не должен ронять лор.
func TestHTTPNotifierSurvivesADeadService(t *testing.T) {
	lore.NewHTTPNotifier("http://127.0.0.1:1/notify").Notify("что угодно")
}
