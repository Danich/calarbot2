// Command admin — веб-панель управления ботом. Как и notify, это не BotModule:
// движок её не опрашивает, она сама ходит в базу и в модули.
package main

import (
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"calarbot2/common"
	"calarbot2/settings"
)

const configPath = "/calarbot.yaml"

type ModuleEntry struct {
	Url string `yaml:"url"`
}

type AdminConfig struct {
	Modules     map[string]ModuleEntry `yaml:"modules"`
	TgTokenFile string                 `yaml:"tgTokenFile"`
	SQLitePath  string                 `yaml:"sqlitePath"`
}

func moduleURLs(c AdminConfig) map[string]string {
	urls := make(map[string]string, len(c.Modules))
	for name, m := range c.Modules {
		urls[name] = m.Url
	}
	return urls
}

func main() {
	var config AdminConfig
	if err := common.ReadConfig(configPath, &config); err != nil {
		log.Fatalf("config error: %v", err)
	}

	store, err := settings.New(config.SQLitePath)
	if err != nil {
		log.Fatalf("settings: %v", err)
	}
	defer store.Close()

	token, err := os.ReadFile(config.TgTokenFile)
	if err != nil {
		log.Fatalf("token: %v", err)
	}
	botAPI, err := tgbotapi.NewBotAPI(strings.TrimSpace(string(token)))
	if err != nil {
		log.Fatalf("telegram: %v", err)
	}

	// TTL короткий: options у select'ов модуль считает на лету, и персона,
	// заведённая без перезапуска, должна появиться в выпадашке сама.
	registry := NewRegistry(moduleURLs(config), 30*time.Second)
	page := &Page{Store: store, Registry: registry}
	api := &API{
		Store:    store,
		Registry: registry,
		Leaver:   &BotLeaver{API: botAPI},
		Now:      func() int64 { return time.Now().Unix() },
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", page.Handler())
	mux.Handle("/static/", http.FileServer(http.FS(staticFS)))
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	api.Routes(mux)

	port := os.Getenv("ADMIN_PORT")
	if port == "" {
		port = "8080"
	}

	// Порта наружу у контейнера нет вовсе: в тайлнет его выводит sidecar, с
	// которым панель делит сетевое пространство имён.
	log.Printf("admin listening on :%s", port)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		log.Fatal(err)
	}
}
