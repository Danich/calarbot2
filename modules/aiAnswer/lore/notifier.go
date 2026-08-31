package lore

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"time"
)

// Notifier показывает владельцу, что бот только что запомнил. Извлекает
// бесплатная автоподобранная модель, и первые недели важно видеть, не пишет ли
// она чушь в вечное хранилище.
type Notifier interface {
	Notify(text string)
}

type HTTPNotifier struct {
	url    string
	client *http.Client
}

func NewHTTPNotifier(url string) *HTTPNotifier {
	return &HTTPNotifier{url: url, client: &http.Client{Timeout: 5 * time.Second}}
}

func (n *HTTPNotifier) Notify(text string) {
	body, err := json.Marshal(map[string]string{"application": "calarbot lore", "payload": text})
	if err != nil {
		log.Printf("notify marshal: %v", err)
		return
	}
	resp, err := n.client.Post(n.url, "application/json", bytes.NewReader(body))
	if err != nil {
		// Не критичный путь: память важнее уведомления о ней.
		log.Printf("notify: %v", err)
		return
	}
	// Дочитать тело перед закрытием — иначе транспорт не переиспользует
	// соединение и на каждое уведомление открывается новое.
	defer func() {
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}()
	if resp.StatusCode >= 300 {
		// Единственный способ заметить дохлый notify: без этого лога
		// битый URL или токен молчаливо съедает единственный канал контроля
		// за тем, что бесплатная модель пишет в вечное хранилище.
		log.Printf("notify: %s responded %d", n.url, resp.StatusCode)
	}
}
