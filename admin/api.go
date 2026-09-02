package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strconv"

	"calarbot2/botModules"
	"calarbot2/settings"
)

type API struct {
	Store    *settings.Store
	Registry *Registry
	Leaver   Leaver
	Now      func() int64
}

func (a *API) Routes(mux *http.ServeMux) {
	mux.HandleFunc("PATCH /api/chats/{id}/modules/{module}", a.patchModule)
	mux.HandleFunc("PATCH /api/chats/{id}/settings/{module}", a.patchSettings)
	mux.HandleFunc("POST /api/chats/{id}/leave", a.leave)
}

func chatIDFrom(r *http.Request) (int64, error) {
	return strconv.ParseInt(r.PathValue("id"), 10, 64)
}

func (a *API) patchModule(w http.ResponseWriter, r *http.Request) {
	chatID, err := chatIDFrom(r)
	if err != nil {
		http.Error(w, "bad chat id", http.StatusBadRequest)
		return
	}

	var body struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad body", http.StatusBadRequest)
		return
	}

	if err := a.Store.SetModuleEnabled(chatID, r.PathValue("module"), body.Enabled); err != nil {
		log.Printf("set module enabled: %v", err)
		http.Error(w, "storage error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// patchSettings принимает частичный объект: приезжает только то, что поменяли.
// null означает «вернуть к дефолту модуля», то есть удалить строку, а не
// записать ноль — иначе смена дефолта в конфиге перестала бы доезжать до чата.
func (a *API) patchSettings(w http.ResponseWriter, r *http.Request) {
	chatID, err := chatIDFrom(r)
	if err != nil {
		http.Error(w, "bad chat id", http.StatusBadRequest)
		return
	}
	module := r.PathValue("module")

	var body map[string]any
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "bad body", http.StatusBadRequest)
		return
	}

	reg, err := a.Registry.Get(module)
	if err != nil {
		http.Error(w, "module did not register: "+err.Error(), http.StatusBadGateway)
		return
	}
	fields := map[string]botModules.Field{}
	for _, f := range reg.Fields {
		fields[f.Key] = f
	}

	// Сначала проверяем всё, потом пишем: половина применённой формы хуже,
	// чем отвергнутая целиком.
	type write struct {
		key    string
		value  string
		delete bool
	}
	writes := make([]write, 0, len(body))
	for key, raw := range body {
		f, ok := fields[key]
		if !ok {
			http.Error(w, "unknown setting "+key, http.StatusBadRequest)
			return
		}
		if raw == nil {
			writes = append(writes, write{key: key, delete: true})
			continue
		}
		v, err := validateValue(f, raw)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		writes = append(writes, write{key: key, value: v})
	}

	for _, wr := range writes {
		if wr.delete {
			err = a.Store.DeleteValue(chatID, module, wr.key)
		} else {
			err = a.Store.SetValue(chatID, module, wr.key, wr.value)
		}
		if err != nil {
			log.Printf("write setting %s: %v", wr.key, err)
			http.Error(w, "storage error", http.StatusInternalServerError)
			return
		}
	}
	w.WriteHeader(http.StatusNoContent)
}

// validateValue проверяет значение по описанию, которое дал сам модуль, и
// возвращает то, что нужно положить в базу. Панель не знает ни одного имени
// настройки — только их типы и границы.
func validateValue(f botModules.Field, v any) (string, error) {
	switch f.Type {
	case botModules.FieldNumber:
		n, ok := v.(float64)
		if !ok || n != float64(int(n)) {
			return "", fmt.Errorf("%s must be a whole number", f.Key)
		}
		i := int(n)
		if f.Min != nil && i < *f.Min {
			return "", fmt.Errorf("%s must be at least %d", f.Key, *f.Min)
		}
		if f.Max != nil && i > *f.Max {
			return "", fmt.Errorf("%s must be at most %d", f.Key, *f.Max)
		}
		return strconv.Itoa(i), nil

	case botModules.FieldBool:
		b, ok := v.(bool)
		if !ok {
			return "", fmt.Errorf("%s must be true or false", f.Key)
		}
		return strconv.FormatBool(b), nil

	case botModules.FieldSelect:
		s, ok := v.(string)
		if !ok {
			return "", fmt.Errorf("%s must be a string", f.Key)
		}
		for _, opt := range f.Options {
			if opt.Value == s {
				return s, nil
			}
		}
		return "", fmt.Errorf("%s is not one of the offered options", f.Key)

	default:
		s, ok := v.(string)
		if !ok {
			return "", fmt.Errorf("%s must be a string", f.Key)
		}
		return s, nil
	}
}

// leave помечает чат покинутым только после того, как телеграм подтвердил
// выход: иначе панель потеряет из виду канал, в котором бот остался.
//
// Только у этой ручки проверяем Origin. PATCH-запросы у панели всегда с JSON-
// телом, значит всегда с preflight, и браузер их кросс-доменно без разрешения
// не отправит. POST /leave тела не несёт — для CORS это «простой» запрос, его
// браузер шлёт кросс-доменно без preflight, и любая страница, открытая в той же
// вкладке в тайлнете, могла бы тихо вынести бота из чата. Это не аутентификация
// (панель и так без неё), а защита именно от межсайтового POST из браузера —
// прямой заход без Origin (curl, открыть ссылку) остаётся разрешён.
func (a *API) leave(w http.ResponseWriter, r *http.Request) {
	if origin := r.Header.Get("Origin"); origin != "" {
		u, err := url.Parse(origin)
		if err != nil || u.Host != r.Host {
			http.Error(w, "cross-origin request refused", http.StatusForbidden)
			return
		}
	}

	chatID, err := chatIDFrom(r)
	if err != nil {
		http.Error(w, "bad chat id", http.StatusBadRequest)
		return
	}

	if err := a.Leaver.LeaveChat(chatID); err != nil {
		log.Printf("leave chat %d: %v", chatID, err)
		http.Error(w, "telegram refused: "+err.Error(), http.StatusBadGateway)
		return
	}
	if err := a.Store.MarkLeft(chatID, a.Now()); err != nil {
		log.Printf("mark left: %v", err)
		http.Error(w, "storage error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
