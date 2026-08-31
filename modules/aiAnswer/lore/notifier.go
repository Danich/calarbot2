package lore

import (
	"bytes"
	"encoding/json"
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
	resp.Body.Close()
}
