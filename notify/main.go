// Command notify принимает сообщения от соседних контейнеров и пересылает их
// владельцу бота в личку.
//
// Это не BotModule: движок его не опрашивает и в чат он не отвечает, он только
// слушает. Поэтому и лежит верхним уровнем, рядом с engine/, а не в modules/.
package main

import (
	"encoding/json"
	"errors"
	"html"
	"log"
	"net/http"
	"os"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"calarbot2/common"
)

const configPath = "/calarbot.yaml"

// Телеграм отвергает сообщение длиннее 4096 знаков. Заголовок и перевод строки
// тоже занимают место, поэтому режем нагрузку с запасом.
const maxMessageRunes = 4000

type Config struct {
	TgTokenFile string `yaml:"tgTokenFile"`
	AdminID     int64  `yaml:"adminId"`
}

// Sender — та часть tgbotapi.BotAPI, которой пользуется сервис. Вынесена
// интерфейсом, чтобы обработчик тестировался без телеграма.
type Sender interface {
	Send(c tgbotapi.Chattable) (tgbotapi.Message, error)
}

// Request — то, что постит соседний контейнер.
type Request struct {
	Application string `json:"application"`
	Payload     string `json:"payload"`
}

// Format собирает тело сообщения.
//
// В HTML телеграма нет <h1> — разрешены только <b>, <i>, <u>, <s>, <code>,
// <pre>, <a> и <blockquote>, — поэтому имя приложения идёт жирным.
func Format(req Request) string {
	return "<b>" + html.EscapeString(req.Application) + "</b>\n" +
		html.EscapeString(truncate(req.Payload, maxMessageRunes))
}

func truncate(s string, limit int) string {
	runes := []rune(s)
	if len(runes) <= limit {
		return s
	}
	return string(runes[:limit]) + "…"
}

func handleNotify(sender Sender, adminID int64) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req Request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "cannot decode body: "+err.Error(), http.StatusBadRequest)
			return
		}
		if strings.TrimSpace(req.Application) == "" || strings.TrimSpace(req.Payload) == "" {
			http.Error(w, "application and payload are both required", http.StatusBadRequest)
			return
		}

		msg := tgbotapi.NewMessage(adminID, Format(req))
		msg.ParseMode = tgbotapi.ModeHTML
		if _, err := sender.Send(msg); err != nil {
			log.Printf("notify: отправка %q: %v", req.Application, err)
			http.Error(w, "telegram rejected the message", http.StatusBadGateway)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func readToken(filename string) (string, error) {
	token, err := os.ReadFile(filename)
	return strings.Trim(string(token), "\n"), err
}

func main() {
	config := &Config{}
	if err := common.ReadConfig(configPath, config); err != nil {
		log.Panic(err)
	}
	// Без adminId сервис молча отправлял бы в никуда, а заметить это можно
	// было бы только по ненаступившему алерту.
	if config.AdminID == 0 {
		log.Panic(errors.New("adminId не задан в " + configPath))
	}

	token, err := readToken(config.TgTokenFile)
	if err != nil {
		log.Panic(err)
	}
	bot, err := tgbotapi.NewBotAPI(token)
	if err != nil {
		log.Panic(err)
	}

	port := os.Getenv("NOTIFY_PORT")
	if port == "" {
		port = "8080"
	}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /notify", handleNotify(bot, config.AdminID))

	addr := ":" + port
	log.Printf("notify: пересылаю в %d, слушаю %s", config.AdminID, addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}
