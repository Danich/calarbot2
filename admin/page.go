package main

import (
	"embed"
	"html/template"
	"log"
	"net/http"
	"time"

	"calarbot2/botModules"
	"calarbot2/settings"
)

//go:embed templates/index.html
var templateFS embed.FS

//go:embed static
var staticFS embed.FS

var indexTemplate = template.Must(template.ParseFS(templateFS, "templates/index.html"))

type Page struct {
	Store    *settings.Store
	Registry *Registry
}

type fieldView struct {
	botModules.Field
	Value any
}

type moduleView struct {
	Key         string
	Label       string
	Description string
	Enabled     bool
	Fields      []fieldView
	Err         string
}

type channelView struct {
	settings.Chat
	TypeLabel     string
	Modules       []moduleView
	EnabledLabels []string
}

type dmView struct {
	settings.Chat
	LastSeenText string
}

type pageView struct {
	Channels []channelView
	DMs      []dmView
}

var typeLabels = map[string]string{
	"group":      "группа",
	"supergroup": "супергруппа",
	"channel":    "канал",
}

func (p *Page) Handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		view, err := p.view()
		if err != nil {
			log.Printf("build page: %v", err)
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if err := indexTemplate.Execute(w, view); err != nil {
			log.Printf("render page: %v", err)
		}
	}
}

func (p *Page) view() (pageView, error) {
	var view pageView

	chats, err := p.Store.ListChats()
	if err != nil {
		return view, err
	}

	for _, chat := range chats {
		if chat.Type == "private" {
			view.DMs = append(view.DMs, dmView{
				Chat:         chat,
				LastSeenText: time.Unix(chat.LastSeen, 0).Format("02.01.2006 15:04"),
			})
			continue
		}

		cv := channelView{Chat: chat, TypeLabel: typeLabels[chat.Type]}
		if cv.TypeLabel == "" {
			cv.TypeLabel = chat.Type
		}

		for _, name := range p.Registry.Names() {
			mv, err := p.moduleView(chat.ID, name)
			if err != nil {
				return view, err
			}
			cv.Modules = append(cv.Modules, mv)
			if mv.Enabled {
				cv.EnabledLabels = append(cv.EnabledLabels, mv.Label)
			}
		}
		view.Channels = append(view.Channels, cv)
	}

	return view, nil
}

// moduleView собирает карточку модуля. Недоступный модуль не выкидывается из
// списка: имя показываем ключом, форму заменяем сообщением, а тумблер работает
// и без него — включённость живёт в базе, а не в модуле.
func (p *Page) moduleView(chatID int64, name string) (moduleView, error) {
	mv := moduleView{Key: name, Label: name}

	enabled, err := p.Store.ModuleEnabled(chatID, name)
	if err != nil {
		return mv, err
	}
	mv.Enabled = enabled

	reg, err := p.Registry.Get(name)
	if err != nil {
		mv.Err = "модуль не отвечает"
		return mv, nil
	}
	if reg.Label != "" {
		mv.Label = reg.Label
	}
	mv.Description = reg.Description

	stored, err := p.Store.Values(chatID, name)
	if err != nil {
		return mv, err
	}
	resolved := settings.Resolve(reg.Fields, stored)
	for _, f := range reg.Fields {
		mv.Fields = append(mv.Fields, fieldView{Field: f, Value: resolved[f.Key]})
	}
	return mv, nil
}
