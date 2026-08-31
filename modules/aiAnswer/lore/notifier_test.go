package lore_test

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
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

// Notify молча проглатывает ошибочный статус — это по контракту (единственный
// канал контроля за бесплатной моделью не должен ронять лор), но такой статус
// обязан попасть в лог, иначе битый URL или токен никто не заметит.
func TestHTTPNotifierLogsNonSuccessStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	var logBuf bytes.Buffer
	orig := log.Writer()
	log.SetOutput(&logBuf)
	defer log.SetOutput(orig)

	lore.NewHTTPNotifier(srv.URL).Notify("нашёл оливье")

	if !strings.Contains(logBuf.String(), "500") {
		t.Errorf("log = %q, want the failing status code logged", logBuf.String())
	}
}
