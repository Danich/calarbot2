# Calarbot2 Admin Panel Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a Tailscale-only web admin panel that manages which modules answer in which chats, plus each module's per-chat settings, with modules declaring their own settings forms.

**Architecture:** A new `settings` package owns three tables in the existing SQLite file (chat list, module on/off, per-module key/value settings). The module protocol replaces `Order()` with `Register()`, in which a module introduces itself and declares its settings fields; the engine resolves stored values over the declared defaults and injects them into `Payload.Extra["settings"]` on every call. A new `admin` container renders the panel server-side and knows no module by name.

**Tech Stack:** Go 1.23, `modernc.org/sqlite`, `html/template`, `embed`, `go-telegram-bot-api/v5`, plain JavaScript (no Node in the build).

**Spec:** `docs/superpowers/specs/2026-09-02-admin-panel-design.md`

## Global Constraints

- Go 1.23.0, module path `calarbot2`. No new third-party dependencies: everything below uses the standard library plus the four modules already in `go.mod`.
- No Node, npm, or any JavaScript build step. CSS and JS ship as files embedded with `embed.FS`.
- Every module setting defaults to **off**: a chat with no `chat_modules` row has every module disabled. There is no "default enabled".
- Design tokens are fixed (Nocturne): bg `#161826`, surface `#232532`, text `#e9e9ed`, accent `#9184d9`, radii 4/8/14px, font Inter with a system fallback stack, spacing scale `--space-1` 2.8px … `--space-8` 22.4px.
- Module labels in the UI: `simpleReply` → "Простой ответ", `skazka` → "Сказка", `sber` → "Сберификатор", `aiAnswer` → "AI-ответ".
- Existing comments in this codebase explain *why*, in Russian, and are load-bearing. Match that: comment only what is non-obvious, in Russian, and do not reformat surrounding code.
- Run `go build ./... && go vet ./... && go test ./...` before every commit.
- Do not run `git push`, and do not open a PR. Commits only.

---

### Task 1: `settings` package — schema and connection

**Files:**
- Create: `settings/store.go`
- Test: `settings/store_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces: `settings.New(path string) (*Store, error)`, `(*Store).Close() error`, `(*Store).Meta(key string) (string, bool, error)`, `(*Store).SetMeta(key, value string) error`. Tables `chats`, `chat_modules`, `chat_module_settings`, `settings_meta`.

- [ ] **Step 1: Write the failing test**

```go
package settings

import (
	"path/filepath"
	"testing"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestMetaRoundTrip(t *testing.T) {
	s := newTestStore(t)

	if _, ok, err := s.Meta("seeded"); err != nil || ok {
		t.Fatalf("Meta on empty store = ok %v, err %v; want false, nil", ok, err)
	}

	if err := s.SetMeta("seeded", "1"); err != nil {
		t.Fatalf("SetMeta: %v", err)
	}

	v, ok, err := s.Meta("seeded")
	if err != nil || !ok || v != "1" {
		t.Fatalf("Meta = %q, %v, %v; want \"1\", true, nil", v, ok, err)
	}
}

// WAL нужен потому, что в один файл теперь пишут три процесса: движок,
// aiAnswer и админка. Без него они встают в очередь на весь файл.
func TestNewEnablesWAL(t *testing.T) {
	s := newTestStore(t)

	var mode string
	if err := s.db.QueryRow(`PRAGMA journal_mode`).Scan(&mode); err != nil {
		t.Fatalf("PRAGMA journal_mode: %v", err)
	}
	if mode != "wal" {
		t.Fatalf("journal_mode = %q; want \"wal\"", mode)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./settings/ -run 'TestMetaRoundTrip|TestNewEnablesWAL' -v`
Expected: FAIL — the `settings` package does not exist yet (`no Go files in .../settings`).

- [ ] **Step 3: Write minimal implementation**

```go
// Package settings хранит то, чем управляет админка: список чатов, включённость
// модулей по чатам и их настройки. Секреты и дефолты по-прежнему живут в yaml —
// строка здесь означает явно принятое решение, а её отсутствие «не трогали».
package settings

import (
	"database/sql"
	"errors"
	"fmt"

	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
}

// New открывает базу в режиме, пригодном для нескольких процессов сразу.
//
// busy_timeout задаётся на соединении, а не на базе, поэтому его мало включить
// один раз где-то ещё: без него второй писатель получит SQLITE_BUSY вместо
// того, чтобы подождать.
func New(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)")
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) migrate() error {
	// left_at, а не left: LEFT — зарезервированное слово в SQLite.
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS chats (
			id         INTEGER PRIMARY KEY,
			type       TEXT NOT NULL,
			title      TEXT NOT NULL DEFAULT '',
			username   TEXT NOT NULL DEFAULT '',
			first_seen INTEGER NOT NULL,
			last_seen  INTEGER NOT NULL,
			left_at    INTEGER
		);
		CREATE TABLE IF NOT EXISTS chat_modules (
			chat_id INTEGER NOT NULL,
			module  TEXT    NOT NULL,
			enabled INTEGER NOT NULL,
			PRIMARY KEY (chat_id, module)
		);
		CREATE TABLE IF NOT EXISTS chat_module_settings (
			chat_id INTEGER NOT NULL,
			module  TEXT    NOT NULL,
			key     TEXT    NOT NULL,
			value   TEXT    NOT NULL,
			PRIMARY KEY (chat_id, module, key)
		);
		CREATE TABLE IF NOT EXISTS settings_meta (
			key   TEXT PRIMARY KEY,
			value TEXT NOT NULL
		);
	`)
	return err
}

func (s *Store) Meta(key string) (string, bool, error) {
	var v string
	err := s.db.QueryRow(`SELECT value FROM settings_meta WHERE key = ?`, key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("select meta: %w", err)
	}
	return v, true, nil
}

func (s *Store) SetMeta(key, value string) error {
	_, err := s.db.Exec(
		`INSERT INTO settings_meta (key, value) VALUES (?, ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
		key, value,
	)
	return err
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./settings/ -v`
Expected: PASS, both tests.

- [ ] **Step 5: Commit**

```bash
git add settings/store.go settings/store_test.go
git commit -m "feat(settings): add the store the admin panel writes to"
```

---

### Task 2: `settings` — the chat list

**Files:**
- Create: `settings/chats.go`
- Test: `settings/chats_test.go`

**Interfaces:**
- Consumes: `settings.New`, `(*Store).db` from Task 1.
- Produces: `settings.Chat{ID int64; Type, Title, Username string; FirstSeen, LastSeen int64; LeftAt *int64}`, `(*Store).UpsertChat(c Chat) error`, `(*Store).ListChats() ([]Chat, error)`, `(*Store).MarkLeft(chatID, ts int64) error`.

- [ ] **Step 1: Write the failing test**

```go
package settings

import "testing"

func TestUpsertChatKeepsFirstSeenAndUpdatesTitle(t *testing.T) {
	s := newTestStore(t)

	if err := s.UpsertChat(Chat{ID: -1, Type: "group", Title: "старое", FirstSeen: 100, LastSeen: 100}); err != nil {
		t.Fatalf("UpsertChat: %v", err)
	}
	if err := s.UpsertChat(Chat{ID: -1, Type: "supergroup", Title: "новое", FirstSeen: 200, LastSeen: 200}); err != nil {
		t.Fatalf("UpsertChat: %v", err)
	}

	got, err := s.ListChats()
	if err != nil {
		t.Fatalf("ListChats: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("ListChats returned %d chats; want 1", len(got))
	}
	if got[0].Title != "новое" || got[0].Type != "supergroup" {
		t.Errorf("chat = %q/%q; want \"новое\"/\"supergroup\"", got[0].Title, got[0].Type)
	}
	if got[0].FirstSeen != 100 {
		t.Errorf("FirstSeen = %d; want 100 — апсерт не должен его двигать", got[0].FirstSeen)
	}
	if got[0].LastSeen != 200 {
		t.Errorf("LastSeen = %d; want 200", got[0].LastSeen)
	}
}

func TestListChatsHidesLeftChats(t *testing.T) {
	s := newTestStore(t)

	for _, c := range []Chat{
		{ID: -1, Type: "group", Title: "группа", FirstSeen: 1, LastSeen: 1},
		{ID: 42, Type: "private", Title: "человек", FirstSeen: 1, LastSeen: 1},
		{ID: -2, Type: "group", Title: "турки", FirstSeen: 1, LastSeen: 1},
	} {
		if err := s.UpsertChat(c); err != nil {
			t.Fatalf("UpsertChat: %v", err)
		}
	}
	if err := s.MarkLeft(-2, 500); err != nil {
		t.Fatalf("MarkLeft: %v", err)
	}

	all, err := s.ListChats()
	if err != nil {
		t.Fatalf("ListChats: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("ListChats returned %d chats; want 2 — покинутый не показываем", len(all))
	}
	for _, c := range all {
		if c.ID == -2 {
			t.Error("покинутый чат остался в списке")
		}
	}
}

// Бота выкинули и добавили обратно: он снова там, значит пометка снимается.
func TestUpsertChatClearsLeftAt(t *testing.T) {
	s := newTestStore(t)

	if err := s.UpsertChat(Chat{ID: -1, Type: "group", FirstSeen: 1, LastSeen: 1}); err != nil {
		t.Fatalf("UpsertChat: %v", err)
	}
	if err := s.MarkLeft(-1, 500); err != nil {
		t.Fatalf("MarkLeft: %v", err)
	}
	if err := s.UpsertChat(Chat{ID: -1, Type: "group", FirstSeen: 600, LastSeen: 600}); err != nil {
		t.Fatalf("UpsertChat: %v", err)
	}

	all, err := s.ListChats()
	if err != nil {
		t.Fatalf("ListChats: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("ListChats returned %d chats; want 1", len(all))
	}
	if all[0].LeftAt != nil {
		t.Errorf("LeftAt = %v; want nil", *all[0].LeftAt)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./settings/ -run TestUpsertChat -v`
Expected: FAIL — `undefined: Chat`.

- [ ] **Step 3: Write minimal implementation**

```go
package settings

import "fmt"

type Chat struct {
	ID        int64
	Type      string
	Title     string
	Username  string
	FirstSeen int64
	LastSeen  int64
	LeftAt    *int64
}

// UpsertChat записывает чат, увиденный движком.
//
// first_seen не двигается, а left_at сбрасывается: апдейт из чата означает, что
// бот там снова есть, независимо от того, выходил ли он оттуда через админку.
func (s *Store) UpsertChat(c Chat) error {
	_, err := s.db.Exec(
		`INSERT INTO chats (id, type, title, username, first_seen, last_seen, left_at)
		 VALUES (?, ?, ?, ?, ?, ?, NULL)
		 ON CONFLICT(id) DO UPDATE SET
		     type      = excluded.type,
		     title     = excluded.title,
		     username  = excluded.username,
		     last_seen = excluded.last_seen,
		     left_at   = NULL`,
		c.ID, c.Type, c.Title, c.Username, c.FirstSeen, c.LastSeen,
	)
	return err
}

// ListChats отдаёт чаты, где бот сейчас есть, в одном запросе. Личку от групп
// отделяет вью: разделение по типу нужно ровно в одном месте, и делать ради
// него второй проход по базе незачем.
func (s *Store) ListChats() ([]Chat, error) {
	rows, err := s.db.Query(
		`SELECT id, type, title, username, first_seen, last_seen, left_at
		 FROM chats WHERE left_at IS NULL ORDER BY last_seen DESC`)
	if err != nil {
		return nil, fmt.Errorf("select chats: %w", err)
	}
	defer rows.Close()

	var out []Chat
	for rows.Next() {
		var c Chat
		if err := rows.Scan(&c.ID, &c.Type, &c.Title, &c.Username, &c.FirstSeen, &c.LastSeen, &c.LeftAt); err != nil {
			return nil, fmt.Errorf("scan chat: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) MarkLeft(chatID, ts int64) error {
	_, err := s.db.Exec(`UPDATE chats SET left_at = ? WHERE id = ?`, ts, chatID)
	return err
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./settings/ -v`
Expected: PASS, all tests.

- [ ] **Step 5: Commit**

```bash
git add settings/chats.go settings/chats_test.go
git commit -m "feat(settings): track the chats the bot is in"
```

---

### Task 3: `settings` — module on/off

**Files:**
- Create: `settings/modules.go`
- Test: `settings/modules_test.go`

**Interfaces:**
- Consumes: `settings.New` from Task 1.
- Produces: `(*Store).ModuleEnabled(chatID int64, module string) (bool, error)`, `(*Store).SetModuleEnabled(chatID int64, module string, enabled bool) error`.

- [ ] **Step 1: Write the failing test**

```go
package settings

import "testing"

// Молчание по умолчанию — несущее правило: бот сидит в чатах, где говорить
// не должен, и новый чат не повод начинать.
func TestModuleDisabledWithoutRow(t *testing.T) {
	s := newTestStore(t)

	enabled, err := s.ModuleEnabled(-1, "aiAnswer")
	if err != nil {
		t.Fatalf("ModuleEnabled: %v", err)
	}
	if enabled {
		t.Error("ModuleEnabled without a row = true; want false")
	}
}

func TestSetModuleEnabledRoundTrip(t *testing.T) {
	s := newTestStore(t)

	if err := s.SetModuleEnabled(-1, "aiAnswer", true); err != nil {
		t.Fatalf("SetModuleEnabled: %v", err)
	}
	if enabled, err := s.ModuleEnabled(-1, "aiAnswer"); err != nil || !enabled {
		t.Fatalf("ModuleEnabled = %v, %v; want true, nil", enabled, err)
	}

	if err := s.SetModuleEnabled(-1, "aiAnswer", false); err != nil {
		t.Fatalf("SetModuleEnabled: %v", err)
	}
	if enabled, err := s.ModuleEnabled(-1, "aiAnswer"); err != nil || enabled {
		t.Fatalf("ModuleEnabled after disabling = %v, %v; want false, nil", enabled, err)
	}
}

func TestModuleEnabledIsScopedToItsChat(t *testing.T) {
	s := newTestStore(t)

	if err := s.SetModuleEnabled(-1, "skazka", true); err != nil {
		t.Fatalf("SetModuleEnabled: %v", err)
	}

	if enabled, _ := s.ModuleEnabled(-2, "skazka"); enabled {
		t.Error("ModuleEnabled(-2, skazka) = true; включённость не должна течь между чатами")
	}
	if enabled, _ := s.ModuleEnabled(-1, "sber"); enabled {
		t.Error("ModuleEnabled(-1, sber) = true; и между модулями тоже")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./settings/ -run 'TestModuleDisabled|TestSetModuleEnabled|TestModuleEnabledIsScoped' -v`
Expected: FAIL — `s.ModuleEnabled undefined`.

- [ ] **Step 3: Write minimal implementation**

```go
package settings

import (
	"database/sql"
	"errors"
	"fmt"
)

// ModuleEnabled: нет строки — выключен. Дефолта «включён» не существует ни у
// одного модуля, включение всегда явное.
func (s *Store) ModuleEnabled(chatID int64, module string) (bool, error) {
	var enabled int
	err := s.db.QueryRow(
		`SELECT enabled FROM chat_modules WHERE chat_id = ? AND module = ?`,
		chatID, module,
	).Scan(&enabled)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("select chat module: %w", err)
	}
	return enabled != 0, nil
}

func (s *Store) SetModuleEnabled(chatID int64, module string, enabled bool) error {
	v := 0
	if enabled {
		v = 1
	}
	_, err := s.db.Exec(
		`INSERT INTO chat_modules (chat_id, module, enabled) VALUES (?, ?, ?)
		 ON CONFLICT(chat_id, module) DO UPDATE SET enabled = excluded.enabled`,
		chatID, module, v,
	)
	return err
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./settings/ -v`
Expected: PASS, all tests.

- [ ] **Step 5: Commit**

```bash
git add settings/modules.go settings/modules_test.go
git commit -m "feat(settings): switch modules on and off per chat"
```

---

### Task 4: `settings` — per-module settings storage

**Files:**
- Create: `settings/values.go`
- Test: `settings/values_test.go`

**Interfaces:**
- Consumes: `settings.New` from Task 1.
- Produces: `(*Store).Values(chatID int64, module string) (map[string]string, error)`, `(*Store).SetValue(chatID int64, module, key, value string) error`, `(*Store).DeleteValue(chatID int64, module, key string) error`.

- [ ] **Step 1: Write the failing test**

```go
package settings

import "testing"

func TestValuesRoundTripScopedByModule(t *testing.T) {
	s := newTestStore(t)

	if err := s.SetValue(-1, "aiAnswer", "answer_level", "990"); err != nil {
		t.Fatalf("SetValue: %v", err)
	}
	if err := s.SetValue(-1, "other", "answer_level", "1"); err != nil {
		t.Fatalf("SetValue: %v", err)
	}

	got, err := s.Values(-1, "aiAnswer")
	if err != nil {
		t.Fatalf("Values: %v", err)
	}
	if len(got) != 1 || got["answer_level"] != "990" {
		t.Fatalf("Values = %v; want {answer_level: 990}", got)
	}
}

func TestSetValueOverwrites(t *testing.T) {
	s := newTestStore(t)

	if err := s.SetValue(-1, "aiAnswer", "context_size", "10"); err != nil {
		t.Fatalf("SetValue: %v", err)
	}
	if err := s.SetValue(-1, "aiAnswer", "context_size", "25"); err != nil {
		t.Fatalf("SetValue: %v", err)
	}

	got, err := s.Values(-1, "aiAnswer")
	if err != nil {
		t.Fatalf("Values: %v", err)
	}
	if got["context_size"] != "25" {
		t.Fatalf("context_size = %q; want \"25\"", got["context_size"])
	}
}

// Удаление строки — это возврат к дефолту, объявленному модулем, а не запись
// нуля: иначе смена дефолта в конфиге перестала бы доезжать до чата.
func TestDeleteValueReturnsToDefault(t *testing.T) {
	s := newTestStore(t)

	if err := s.SetValue(-1, "aiAnswer", "context_size", "25"); err != nil {
		t.Fatalf("SetValue: %v", err)
	}
	if err := s.DeleteValue(-1, "aiAnswer", "context_size"); err != nil {
		t.Fatalf("DeleteValue: %v", err)
	}

	got, err := s.Values(-1, "aiAnswer")
	if err != nil {
		t.Fatalf("Values: %v", err)
	}
	if _, ok := got["context_size"]; ok {
		t.Fatalf("Values = %v; want no context_size key", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./settings/ -run 'TestValues|TestSetValue|TestDeleteValue' -v`
Expected: FAIL — `s.SetValue undefined`.

- [ ] **Step 3: Write minimal implementation**

```go
package settings

import "fmt"

// Values отдаёт только явно выставленные значения. Наложить их на дефолты
// модуля — дело Resolve.
func (s *Store) Values(chatID int64, module string) (map[string]string, error) {
	rows, err := s.db.Query(
		`SELECT key, value FROM chat_module_settings WHERE chat_id = ? AND module = ?`,
		chatID, module,
	)
	if err != nil {
		return nil, fmt.Errorf("select module settings: %w", err)
	}
	defer rows.Close()

	out := map[string]string{}
	for rows.Next() {
		var k, v string
		if err := rows.Scan(&k, &v); err != nil {
			return nil, fmt.Errorf("scan module setting: %w", err)
		}
		out[k] = v
	}
	return out, rows.Err()
}

func (s *Store) SetValue(chatID int64, module, key, value string) error {
	_, err := s.db.Exec(
		`INSERT INTO chat_module_settings (chat_id, module, key, value) VALUES (?, ?, ?, ?)
		 ON CONFLICT(chat_id, module, key) DO UPDATE SET value = excluded.value`,
		chatID, module, key, value,
	)
	return err
}

func (s *Store) DeleteValue(chatID int64, module, key string) error {
	_, err := s.db.Exec(
		`DELETE FROM chat_module_settings WHERE chat_id = ? AND module = ? AND key = ?`,
		chatID, module, key,
	)
	return err
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./settings/ -v`
Expected: PASS, all tests.

- [ ] **Step 5: Commit**

```bash
git add settings/values.go settings/values_test.go
git commit -m "feat(settings): store per-chat module settings"
```

---

### Task 5: Module protocol — `Register()` replaces `Order()`

**Files:**
- Modify: `botModules/botModules.go`
- Modify: `botModules/httpserver.go:76-84` (`orderAction`) and its route registration at `botModules/httpserver.go:21`
- Modify: `botModules/moduleClient.go:14-30` (`Order`)
- Modify: `modules/simpleReply/main.go:16`, `modules/sber/main.go:24`, `modules/skazka/main.go:78`, `modules/aiAnswer/main.go:192`
- Modify: `engine/runBot.go:62-86` (`InitModules`), `engine/mock_module_client.go:8-31`
- Test: `botModules/moduleClient_test.go:15` (`TestModuleClientOrder`), `botModules/httpserver_test.go`, `engine/runBot_test.go:26`

**Interfaces:**
- Consumes: nothing from earlier tasks.
- Produces: `botModules.Option{Value, Label string}`, `botModules.Field{Key, Label, Description, Type string; Min, Max *int; Options []Option; Default any}`, `botModules.Registration{Order int; Label, Description string; Fields []Field}`, constants `botModules.FieldNumber/FieldBool/FieldSelect/FieldText`, interface method `Register() Registration`, `(*ModuleClient).Register() (Registration, error)`, HTTP route `GET /register`.

- [ ] **Step 1: Write the failing test**

Add to `botModules/moduleClient_test.go` (and delete `TestModuleClientOrder`, which tests the removed method):

```go
func TestModuleClientRegister(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/register" {
			t.Errorf("path = %q; want /register", r.URL.Path)
		}
		json.NewEncoder(w).Encode(Registration{
			Order: 100,
			Label: "AI-ответ",
			Fields: []Field{{
				Key: "answer_level", Label: "Вес", Type: FieldNumber, Default: 990,
			}},
		})
	}))
	defer srv.Close()

	c := &ModuleClient{BaseURL: srv.URL}
	reg, err := c.Register()
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if reg.Order != 100 || reg.Label != "AI-ответ" {
		t.Errorf("Registration = %+v; want order 100, label AI-ответ", reg)
	}
	if len(reg.Fields) != 1 || reg.Fields[0].Key != "answer_level" {
		t.Errorf("Fields = %+v; want one answer_level field", reg.Fields)
	}
}

// Модуль лежит — он всё равно должен оказаться последним в очереди, а не
// первым. Это поведение было у Order() и его нельзя потерять.
func TestModuleClientRegisterSinksUnreachableModule(t *testing.T) {
	c := &ModuleClient{BaseURL: "http://127.0.0.1:1"}

	reg, err := c.Register()
	if err == nil {
		t.Error("Register on an unreachable module returned nil error")
	}
	if reg.Order != 9999 {
		t.Errorf("Order = %d; want 9999", reg.Order)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./botModules/ -run TestModuleClientRegister -v`
Expected: FAIL — `undefined: Registration`.

- [ ] **Step 3: Write minimal implementation**

In `botModules/botModules.go`, replace the `BotModule` interface and add the new types:

```go
// Типы полей, которые модуль может объявить в Registration.
const (
	FieldNumber = "number"
	FieldBool   = "bool"
	FieldSelect = "select"
	FieldText   = "text"
)

type Option struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// Field — одно поле формы настроек, как его описывает сам модуль.
//
// Options модуль считает в момент вызова, а не берёт из конфига: список персон
// у aiAnswer живёт в его же базе и меняется без перезапуска.
type Field struct {
	Key         string   `json:"key"`
	Label       string   `json:"label"`
	Description string   `json:"description,omitempty"`
	Type        string   `json:"type"`
	Min         *int     `json:"min,omitempty"`
	Max         *int     `json:"max,omitempty"`
	Options     []Option `json:"options,omitempty"`
	Default     any      `json:"default,omitempty"`
}

// Registration — то, чем модуль представляется движку на старте: порядок в
// очереди, человеческое имя для админки и описание своих настроек.
type Registration struct {
	Order       int     `json:"order"`
	Label       string  `json:"label"`
	Description string  `json:"description,omitempty"`
	Fields      []Field `json:"fields,omitempty"`
}

type BotModule interface {
	Register() Registration
	IsCalled(msg *tgbotapi.Message) bool
	Answer(payload *Payload) (RichAnswer, error)
}
```

In `botModules/httpserver.go`, replace `orderAction` and its route:

```go
	mux.HandleFunc("/register", registerAction(module))
```

```go
func registerAction(module BotModule) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewEncoder(w).Encode(module.Register()); err != nil {
			fmt.Printf("error encoding registration: %v", err)
		}
	}
}
```

In `botModules/moduleClient.go`, replace `Order`:

```go
// Register спрашивает модуль, кто он и какие у него настройки.
//
// Недоступный модуль получает Order 9999 — то же «в конец очереди», что было у
// Order(), — но теперь вместе с ошибкой: движку есть что записать в лог.
func (c *ModuleClient) Register() (Registration, error) {
	fallback := Registration{Order: 9999}

	resp, err := http.Get(c.BaseURL + "/register")
	if err != nil {
		return fallback, fmt.Errorf("register %s: %w", c.BaseURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fallback, fmt.Errorf("register %s: status %d", c.BaseURL, resp.StatusCode)
	}

	var reg Registration
	if err := json.NewDecoder(resp.Body).Decode(&reg); err != nil {
		return fallback, fmt.Errorf("decode registration from %s: %w", c.BaseURL, err)
	}
	return reg, nil
}
```

In `engine/mock_module_client.go`, change the interface and the mock:

```go
type ModuleClientInterface interface {
	Register() (botModules.Registration, error)
	IsCalled(payload *botModules.Payload) (bool, error)
	Answer(payload *botModules.Payload) (botModules.RichAnswer, error)
}
```

```go
	RegistrationValue botModules.Registration
	RegistrationError error
```

```go
func (m *MockModuleClient) Register() (botModules.Registration, error) {
	return m.RegistrationValue, m.RegistrationError
}
```

Delete the `OrderValue` field and the `Order()` method; update `engine/runBot_test.go` to set `RegistrationValue: botModules.Registration{Order: N}` wherever it set `OrderValue: N`.

In `engine/runBot.go`, add a `Registrations` field to `Bot` and populate it in `InitModules`:

```go
type Bot struct {
	BotAPI         *tgbotapi.BotAPI
	Flags          map[string]bool
	Modules        map[string]*botModules.ModuleClient
	Registrations  map[string]botModules.Registration
	BotConfig      *CalarbotConfig
	orderedModules []string
}
```

```go
	b.Registrations = make(map[string]botModules.Registration)
	for configName, moduleConfig := range b.BotConfig.Modules {
		b.Modules[configName] = &botModules.ModuleClient{BaseURL: moduleConfig.Url}
		reg, err := b.Modules[configName].Register()
		if err != nil {
			log.Printf("module %s did not register: %v", configName, err)
		}
		b.Registrations[configName] = reg
		moduleOrders = append(moduleOrders, moduleOrder{name: configName, order: reg.Order})
	}
```

In each of the four modules, replace the `Order()` method. `modules/simpleReply/main.go`:

```go
func (m Module) Register() botModules.Registration {
	return botModules.Registration{
		Order:       m.order,
		Label:       "Простой ответ",
		Description: "Отвечает «тыква», когда не сработал никто другой",
	}
}
```

`modules/sber/main.go`:

```go
func (m Module) Register() botModules.Registration {
	return botModules.Registration{
		Order:       m.order,
		Label:       "Сберификатор",
		Description: "Приделывает «сбер» к существительным во фразе",
	}
}
```

`modules/skazka/main.go`:

```go
func (m *Module) Register() botModules.Registration {
	return botModules.Registration{
		Order:       m.order,
		Label:       "Сказка",
		Description: "Играет с чатом в сочинение сказки",
	}
}
```

`modules/aiAnswer/main.go` — for now the same shape, fields come in Task 9:

```go
func (m *Module) Register() botModules.Registration {
	return botModules.Registration{
		Order:       m.order,
		Label:       "AI-ответ",
		Description: "Отвечает через языковую модель",
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go build ./... && go test ./... -v`
Expected: PASS. `TestServeModule` in `botModules/httpserver_test.go` and `TestInitModules` in `engine/runBot_test.go` need their `/order` and `OrderValue` references updated to `/register` and `RegistrationValue`; do that as part of this step.

- [ ] **Step 5: Commit**

```bash
git add botModules/ modules/ engine/
git commit -m "refactor(botModules): let a module introduce itself with Register()"
```

---

### Task 6: Module protocol — the whole payload reaches `IsCalled`

**Files:**
- Modify: `botModules/botModules.go` (`BotModule` interface)
- Modify: `botModules/httpserver.go:61-74` (`isCalledAction`)
- Modify: `modules/simpleReply/main.go`, `modules/sber/main.go`, `modules/skazka/main.go`, `modules/aiAnswer/main.go`
- Test: `botModules/httpserver_test.go`

**Interfaces:**
- Consumes: `botModules.Registration` from Task 5.
- Produces: interface method `IsCalled(payload *Payload) bool`. `Payload.Extra` now survives into every module's `IsCalled`.

- [ ] **Step 1: Write the failing test**

Add to `botModules/httpserver_test.go`:

```go
type extraSpy struct {
	seen map[string]interface{}
}

func (s *extraSpy) Register() Registration { return Registration{Order: 1} }
func (s *extraSpy) IsCalled(payload *Payload) bool {
	s.seen = payload.Extra
	return true
}
func (s *extraSpy) Answer(payload *Payload) (RichAnswer, error) {
	return RichAnswer{}, nil
}

// Настройки едут к модулю в Extra, и /is_called обязан их донести: без этого
// модуль узнаёт свои настройки только в Answer, а решает-то он раньше.
func TestIsCalledCarriesExtra(t *testing.T) {
	spy := &extraSpy{}
	srv, _ := ServeModule(spy, "127.0.0.1:0")
	defer srv.Close()

	handler := srv.Handler
	body, _ := json.Marshal(Payload{
		Msg:   &tgbotapi.Message{Text: "привет"},
		Extra: map[string]interface{}{"settings": map[string]interface{}{"answer_level": 990.0}},
	})
	req := httptest.NewRequest(http.MethodPost, "/is_called", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	settings, ok := spy.seen["settings"].(map[string]interface{})
	if !ok {
		t.Fatalf("IsCalled saw Extra = %v; want a settings map", spy.seen)
	}
	if settings["answer_level"] != 990.0 {
		t.Errorf("answer_level = %v; want 990", settings["answer_level"])
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./botModules/ -run TestIsCalledCarriesExtra -v`
Expected: FAIL — `*extraSpy does not implement BotModule (wrong type for method IsCalled)`.

- [ ] **Step 3: Write minimal implementation**

In `botModules/botModules.go`:

```go
type BotModule interface {
	Register() Registration
	IsCalled(payload *Payload) bool
	Answer(payload *Payload) (RichAnswer, error)
}
```

In `botModules/httpserver.go`, `isCalledAction` passes the payload through:

```go
		result := module.IsCalled(&payload)
```

Change each module's signature. `modules/simpleReply/main.go`:

```go
func (m Module) IsCalled(_ *botModules.Payload) bool { return true }
```

`modules/sber/main.go`:

```go
func (m Module) IsCalled(payload *botModules.Payload) bool {
	msg := payload.Msg
	if msg == nil || msg.Text == "" {
		return false
	}

	// Check if the message starts with /sber
	return strings.HasPrefix(msg.Text, "/sber")
}
```

`modules/skazka/main.go`:

```go
func (m *Module) IsCalled(payload *botModules.Payload) bool {
	msg := payload.Msg
	if msg == nil {
		return false
	}
	if msg.IsCommand() {
```

(the rest of the body is unchanged, and `msg` is already the name it uses)

`modules/aiAnswer/main.go`:

```go
func (m *Module) IsCalled(payload *botModules.Payload) bool {
	msg := payload.Msg
	if msg == nil {
		return false
	}
```

(the rest of the body is unchanged for now; reading settings comes in Task 10)

Remove the now-unused `tgbotapi` import from `modules/simpleReply/main.go` if `go build` reports it.

- [ ] **Step 4: Run test to verify it passes**

Run: `go build ./... && go vet ./... && go test ./... -v`
Expected: PASS. Update `modules/*/main_test.go` call sites that pass a bare `*tgbotapi.Message` to `IsCalled` — wrap them in `&botModules.Payload{Msg: msg}`.

- [ ] **Step 5: Commit**

```bash
git add botModules/ modules/
git commit -m "refactor(botModules): hand IsCalled the whole payload"
```

---

### Task 7: `settings` — resolving stored values over declared defaults

**Files:**
- Create: `settings/resolve.go`
- Test: `settings/resolve_test.go`

**Interfaces:**
- Consumes: `botModules.Field`, `botModules.FieldNumber/FieldBool/FieldSelect/FieldText` from Task 5.
- Produces: `settings.Resolve(fields []botModules.Field, stored map[string]string) map[string]any`.

- [ ] **Step 1: Write the failing test**

```go
package settings

import (
	"testing"

	"calarbot2/botModules"
)

func numberField(key string, def int) botModules.Field {
	return botModules.Field{Key: key, Type: botModules.FieldNumber, Default: def}
}

func TestResolveFallsBackToDeclaredDefault(t *testing.T) {
	fields := []botModules.Field{numberField("answer_level", 990)}

	got := Resolve(fields, map[string]string{})

	if got["answer_level"] != 990 {
		t.Fatalf("answer_level = %v; want 990", got["answer_level"])
	}
}

func TestResolvePrefersStoredValue(t *testing.T) {
	fields := []botModules.Field{numberField("answer_level", 990)}

	got := Resolve(fields, map[string]string{"answer_level": "700"})

	if got["answer_level"] != 700 {
		t.Fatalf("answer_level = %v; want 700", got["answer_level"])
	}
}

// Мусор в базе не должен ронять бота и не должен молча превращаться в ноль:
// ноль у answer_level значит «отвечать на всё подряд».
func TestResolveFallsBackOnUnparsableValue(t *testing.T) {
	fields := []botModules.Field{numberField("answer_level", 990)}

	got := Resolve(fields, map[string]string{"answer_level": "не число"})

	if got["answer_level"] != 990 {
		t.Fatalf("answer_level = %v; want the default 990", got["answer_level"])
	}
}

func TestResolveHandlesBoolAndText(t *testing.T) {
	fields := []botModules.Field{
		{Key: "loud", Type: botModules.FieldBool, Default: false},
		{Key: "persona", Type: botModules.FieldSelect, Default: "mamkin"},
	}

	got := Resolve(fields, map[string]string{"loud": "true", "persona": "genadiy"})

	if got["loud"] != true {
		t.Errorf("loud = %v; want true", got["loud"])
	}
	if got["persona"] != "genadiy" {
		t.Errorf("persona = %v; want genadiy", got["persona"])
	}
}

// Значение без объявленного поля — мусор от удалённой настройки. Модуль его
// не ждёт, отдавать не за чем.
func TestResolveIgnoresUndeclaredKeys(t *testing.T) {
	fields := []botModules.Field{numberField("answer_level", 990)}

	got := Resolve(fields, map[string]string{"gone": "1"})

	if _, ok := got["gone"]; ok {
		t.Fatalf("Resolve = %v; want no \"gone\" key", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./settings/ -run TestResolve -v`
Expected: FAIL — `undefined: Resolve`.

- [ ] **Step 3: Write minimal implementation**

```go
package settings

import (
	"log"
	"strconv"

	"calarbot2/botModules"
)

// Resolve накладывает явно выставленные значения на дефолты, объявленные самим
// модулем, и всегда возвращает полную карту.
//
// Полную — это несущее: модуль благодаря этому не пишет ни строчки фолбэка, а
// модуль на другом языке вообще ничего не знает ни про базу, ни про конфиг.
func Resolve(fields []botModules.Field, stored map[string]string) map[string]any {
	out := make(map[string]any, len(fields))

	for _, f := range fields {
		raw, ok := stored[f.Key]
		if !ok {
			out[f.Key] = f.Default
			continue
		}

		v, err := coerce(f.Type, raw)
		if err != nil {
			// Дефолт вместо нуля: ноль у веса — это «отвечать всегда», и тихо
			// подставить его вместо испорченного значения было бы хуже всего.
			log.Printf("settings: bad value %q for %s (%s), using default: %v", raw, f.Key, f.Type, err)
			out[f.Key] = f.Default
			continue
		}
		out[f.Key] = v
	}

	return out
}

func coerce(fieldType, raw string) (any, error) {
	switch fieldType {
	case botModules.FieldNumber:
		return strconv.Atoi(raw)
	case botModules.FieldBool:
		return strconv.ParseBool(raw)
	default:
		return raw, nil
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./settings/ -v`
Expected: PASS, all tests.

- [ ] **Step 5: Commit**

```bash
git add settings/resolve.go settings/resolve_test.go
git commit -m "feat(settings): resolve stored values over module defaults"
```

---

### Task 8: `settings` — the read cache

**Files:**
- Create: `settings/cache.go`
- Test: `settings/cache_test.go`

**Interfaces:**
- Consumes: `(*Store).ModuleEnabled`, `(*Store).Values` from Tasks 3 and 4.
- Produces: `settings.NewCache(s *Store, ttl time.Duration) *Cache`, `(*Cache).ModuleEnabled(chatID int64, module string) bool`, `(*Cache).Values(chatID int64, module string) map[string]string`, and the injectable `Cache.now func() time.Time` used by tests.

- [ ] **Step 1: Write the failing test**

```go
package settings

import (
	"testing"
	"time"
)

func TestCacheServesRepeatReadsWithoutTouchingTheStore(t *testing.T) {
	s := newTestStore(t)
	if err := s.SetModuleEnabled(-1, "aiAnswer", true); err != nil {
		t.Fatalf("SetModuleEnabled: %v", err)
	}

	c := NewCache(s, time.Minute)
	if !c.ModuleEnabled(-1, "aiAnswer") {
		t.Fatal("ModuleEnabled = false; want true")
	}

	// Правка мимо кэша: пока TTL не истёк, кэш обязан отдавать старое.
	if err := s.SetModuleEnabled(-1, "aiAnswer", false); err != nil {
		t.Fatalf("SetModuleEnabled: %v", err)
	}
	if !c.ModuleEnabled(-1, "aiAnswer") {
		t.Error("ModuleEnabled = false before the TTL expired; want the cached true")
	}
}

func TestCacheRefreshesAfterTTL(t *testing.T) {
	s := newTestStore(t)
	if err := s.SetModuleEnabled(-1, "aiAnswer", true); err != nil {
		t.Fatalf("SetModuleEnabled: %v", err)
	}

	now := time.Unix(1000, 0)
	c := NewCache(s, 5*time.Second)
	c.now = func() time.Time { return now }

	if !c.ModuleEnabled(-1, "aiAnswer") {
		t.Fatal("ModuleEnabled = false; want true")
	}

	if err := s.SetModuleEnabled(-1, "aiAnswer", false); err != nil {
		t.Fatalf("SetModuleEnabled: %v", err)
	}
	now = now.Add(6 * time.Second)

	if c.ModuleEnabled(-1, "aiAnswer") {
		t.Error("ModuleEnabled = true after the TTL expired; want the fresh false")
	}
}

func TestCacheKeysByChatAndModule(t *testing.T) {
	s := newTestStore(t)
	if err := s.SetModuleEnabled(-1, "aiAnswer", true); err != nil {
		t.Fatalf("SetModuleEnabled: %v", err)
	}

	c := NewCache(s, time.Minute)

	if !c.ModuleEnabled(-1, "aiAnswer") {
		t.Error("ModuleEnabled(-1, aiAnswer) = false; want true")
	}
	if c.ModuleEnabled(-1, "skazka") {
		t.Error("ModuleEnabled(-1, skazka) = true; want false")
	}
	if c.ModuleEnabled(-2, "aiAnswer") {
		t.Error("ModuleEnabled(-2, aiAnswer) = true; want false")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./settings/ -run TestCache -v`
Expected: FAIL — `undefined: NewCache`.

- [ ] **Step 3: Write minimal implementation**

```go
package settings

import (
	"log"
	"sync"
	"time"
)

// Cache снимает с базы нагрузку от движка: тот спрашивает включённость и
// настройки для каждого модуля на каждое сообщение.
//
// Инвалидация по TTL, а не по сигналу от админки: пять секунд задержки после
// клика — приемлемо, межпроцессная нотификация ради этого — нет.
type Cache struct {
	store *Store
	ttl   time.Duration
	now   func() time.Time

	mu      sync.Mutex
	enabled map[cacheKey]cachedBool
	values  map[cacheKey]cachedValues
}

type cacheKey struct {
	chatID int64
	module string
}

type cachedBool struct {
	v  bool
	at time.Time
}

type cachedValues struct {
	v  map[string]string
	at time.Time
}

func NewCache(s *Store, ttl time.Duration) *Cache {
	return &Cache{
		store:   s,
		ttl:     ttl,
		now:     time.Now,
		enabled: map[cacheKey]cachedBool{},
		values:  map[cacheKey]cachedValues{},
	}
}

// ModuleEnabled при ошибке базы отвечает «выключен»: молчание — безопасный
// отказ, а заговорить в чате, где не должен, бот не имеет права.
func (c *Cache) ModuleEnabled(chatID int64, module string) bool {
	k := cacheKey{chatID, module}

	c.mu.Lock()
	defer c.mu.Unlock()

	if hit, ok := c.enabled[k]; ok && c.now().Sub(hit.at) < c.ttl {
		return hit.v
	}

	v, err := c.store.ModuleEnabled(chatID, module)
	if err != nil {
		log.Printf("settings: module enabled for chat %d, module %s: %v", chatID, module, err)
		return false
	}
	c.enabled[k] = cachedBool{v: v, at: c.now()}
	return v
}

func (c *Cache) Values(chatID int64, module string) map[string]string {
	k := cacheKey{chatID, module}

	c.mu.Lock()
	defer c.mu.Unlock()

	if hit, ok := c.values[k]; ok && c.now().Sub(hit.at) < c.ttl {
		return hit.v
	}

	v, err := c.store.Values(chatID, module)
	if err != nil {
		log.Printf("settings: values for chat %d, module %s: %v", chatID, module, err)
		return map[string]string{}
	}
	c.values[k] = cachedValues{v: v, at: c.now()}
	return v
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./settings/ -v`
Expected: PASS, all tests.

- [ ] **Step 5: Commit**

```bash
git add settings/cache.go settings/cache_test.go
git commit -m "feat(settings): cache reads the engine makes on every message"
```

---

### Task 9: Engine — config learns about the database and the seed

**Files:**
- Modify: `engine/config.go`
- Modify: `engine/main.go`
- Modify: `engine/runBot.go` (`Bot` struct, `InitBot`)
- Test: `engine/config_test.go:13`

**Interfaces:**
- Consumes: `settings.New` from Task 1.
- Produces: `CalarbotConfig{Modules map[string]ModulesConfig; TgTokenFile string; SQLitePath string; SeedChats []SeedChat}`, `ModulesConfig{Url string}` (no `EnabledOn`), `SeedChat{ID int64; Title, Type string; Modules []string}`, `Bot.Settings *settings.Cache`, `Bot.SettingsStore *settings.Store`.

- [ ] **Step 1: Write the failing test**

Replace `TestCalarbotConfig` in `engine/config_test.go`:

```go
func TestCalarbotConfig(t *testing.T) {
	const raw = `
tgTokenFile: /.tgtoken
sqlitePath: /data/calarbot.db
modules:
  aiAnswer:
    url: "http://aiAnswer:8080"
seed_chats:
  - id: -386946235
    title: "тестовый"
    type: group
    modules: [skazka, aiAnswer]
`
	var config CalarbotConfig
	if err := yaml.Unmarshal([]byte(raw), &config); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	if config.SQLitePath != "/data/calarbot.db" {
		t.Errorf("SQLitePath = %q; want /data/calarbot.db", config.SQLitePath)
	}
	if config.Modules["aiAnswer"].Url != "http://aiAnswer:8080" {
		t.Errorf("aiAnswer url = %q", config.Modules["aiAnswer"].Url)
	}
	if len(config.SeedChats) != 1 {
		t.Fatalf("SeedChats = %+v; want one entry", config.SeedChats)
	}
	seed := config.SeedChats[0]
	if seed.ID != -386946235 || seed.Type != "group" {
		t.Errorf("seed = %+v; want id -386946235, type group", seed)
	}
	if len(seed.Modules) != 2 || seed.Modules[0] != "skazka" {
		t.Errorf("seed modules = %v; want [skazka aiAnswer]", seed.Modules)
	}
}
```

Keep whatever imports the file already has, adding `gopkg.in/yaml.v3` if it is not there.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./engine/ -run TestCalarbotConfig -v`
Expected: FAIL — `config.SQLitePath undefined`.

- [ ] **Step 3: Write minimal implementation**

`engine/config.go`:

```go
package main

type CalarbotConfig struct {
	Modules     map[string]ModulesConfig `yaml:"modules"`
	TgTokenFile string                   `yaml:"tgTokenFile"`
	SQLitePath  string                   `yaml:"sqlitePath"`
	// SeedChats — разовый посев: чем включённость модулей была до появления
	// админки. Отрабатывает один раз, дальше правда живёт в базе.
	SeedChats []SeedChat `yaml:"seed_chats"`
}

type ModulesConfig struct {
	Url string `yaml:"url"`
}

type SeedChat struct {
	ID      int64    `yaml:"id"`
	Title   string   `yaml:"title"`
	Type    string   `yaml:"type"`
	Modules []string `yaml:"modules"`
}
```

`engine/runBot.go` — add the fields to `Bot` and open the store in `InitBot`, right after reading the config:

```go
type Bot struct {
	BotAPI         *tgbotapi.BotAPI
	Flags          map[string]bool
	Modules        map[string]*botModules.ModuleClient
	Registrations  map[string]botModules.Registration
	SettingsStore  *settings.Store
	Settings       *settings.Cache
	BotConfig      *CalarbotConfig
	orderedModules []string
}
```

In `InitBot`, before `readToken`:

```go
	// Паникуем, а не работаем без базы: без неё каждый модуль выключен, и бот
	// молча онемел бы во всех чатах сразу.
	if b.BotConfig.SQLitePath == "" {
		log.Panic("sqlitePath is empty: the engine cannot resolve module settings without it")
	}
	settingsStore, err := settings.New(b.BotConfig.SQLitePath)
	if err != nil {
		log.Panic(err)
	}
	b.SettingsStore = settingsStore
	b.Settings = settings.NewCache(settingsStore, 5*time.Second)
```

Add `"time"` and `"calarbot2/settings"` to the imports. The existing
`token, err := readToken(...)` below still compiles: `token` is a new name, so
`:=` stays legal.

- [ ] **Step 4: Run test to verify it passes**

Run: `go build ./... && go test ./engine/ -v`
Expected: PASS. `TestShouldIAnswer` still references `EnabledOn`; delete those subtests for now — Task 11 replaces them.

- [ ] **Step 5: Commit**

```bash
git add engine/
git commit -m "feat(engine): open the settings database and read the seed list"
```

---

### Task 10: Engine — record every chat the bot sees

**Files:**
- Modify: `engine/runBot.go:97-160` (`RunBot`)
- Test: `engine/chats_test.go` (create)

**Interfaces:**
- Consumes: `settings.Chat`, `(*Store).UpsertChat`, `(*Store).MarkLeft` from Task 2; `Bot.SettingsStore` from Task 9.
- Produces: `(*Bot).recordChat(chat *tgbotapi.Chat, ts int64)`, `(*Bot).recordMembership(u *tgbotapi.ChatMemberUpdated)`.

- [ ] **Step 1: Write the failing test**

```go
package main

import (
	"path/filepath"
	"testing"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"calarbot2/settings"
)

func botWithSettings(t *testing.T) *Bot {
	t.Helper()
	s, err := settings.New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("settings.New: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return &Bot{SettingsStore: s}
}

func TestRecordChatStoresTitleAndType(t *testing.T) {
	b := botWithSettings(t)

	b.recordChat(&tgbotapi.Chat{ID: -1, Type: "supergroup", Title: "болталка"}, 1000)

	chats, err := b.SettingsStore.ListChats()
	if err != nil {
		t.Fatalf("ListChats: %v", err)
	}
	if len(chats) != 1 {
		t.Fatalf("ListChats returned %d; want 1", len(chats))
	}
	if chats[0].Title != "болталка" || chats[0].Type != "supergroup" {
		t.Errorf("chat = %+v; want болталка/supergroup", chats[0])
	}
}

// У лички нет title — там имя и @username. Иначе список личек в панели пуст.
func TestRecordChatNamesPrivateChatsByUser(t *testing.T) {
	b := botWithSettings(t)

	b.recordChat(&tgbotapi.Chat{ID: 42, Type: "private", FirstName: "Даня", UserName: "danich"}, 1000)

	chats, err := b.SettingsStore.ListChats()
	if err != nil {
		t.Fatalf("ListChats: %v", err)
	}
	if len(chats) != 1 {
		t.Fatalf("ListChats returned %d; want 1", len(chats))
	}
	if chats[0].Type != "private" {
		t.Fatalf("type = %q; want private", chats[0].Type)
	}
	if chats[0].Title != "Даня" || chats[0].Username != "danich" {
		t.Errorf("chat = %+v; want Даня/danich", chats[0])
	}
}

// Бота выгнали — канал должен уйти из панели, не дожидаясь чужих сообщений.
func TestRecordMembershipMarksKickedChatLeft(t *testing.T) {
	b := botWithSettings(t)
	b.recordChat(&tgbotapi.Chat{ID: -1, Type: "group", Title: "турки"}, 1000)

	b.recordMembership(&tgbotapi.ChatMemberUpdated{
		Chat: tgbotapi.Chat{ID: -1, Type: "group", Title: "турки"},
		NewChatMember: tgbotapi.ChatMember{Status: "kicked"},
		Date:          2000,
	})

	chats, err := b.SettingsStore.ListChats()
	if err != nil {
		t.Fatalf("ListChats: %v", err)
	}
	if len(chats) != 0 {
		t.Fatalf("ListChats returned %+v; want none — бота там больше нет", chats)
	}
}

func TestRecordMembershipAddsNewChat(t *testing.T) {
	b := botWithSettings(t)

	b.recordMembership(&tgbotapi.ChatMemberUpdated{
		Chat:          tgbotapi.Chat{ID: -7, Type: "group", Title: "новый"},
		NewChatMember: tgbotapi.ChatMember{Status: "member"},
		Date:          2000,
	})

	chats, err := b.SettingsStore.ListChats()
	if err != nil {
		t.Fatalf("ListChats: %v", err)
	}
	if len(chats) != 1 || chats[0].ID != -7 {
		t.Fatalf("ListChats = %+v; want the new chat -7", chats)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./engine/ -run 'TestRecordChat|TestRecordMembership' -v`
Expected: FAIL — `b.recordChat undefined`.

- [ ] **Step 3: Write minimal implementation**

Add to `engine/runBot.go`:

```go
// recordChat запоминает чат, из которого пришёл апдейт. Списка своих чатов
// телеграм боту не отдаёт, так что панели брать его больше неоткуда.
func (b *Bot) recordChat(chat *tgbotapi.Chat, ts int64) {
	if b.SettingsStore == nil || chat == nil {
		return
	}

	// У лички нет title: там имя человека и его @username.
	title := chat.Title
	if title == "" {
		title = strings.TrimSpace(chat.FirstName + " " + chat.LastName)
	}

	if err := b.SettingsStore.UpsertChat(settings.Chat{
		ID:        chat.ID,
		Type:      chat.Type,
		Title:     title,
		Username:  chat.UserName,
		FirstSeen: ts,
		LastSeen:  ts,
	}); err != nil {
		log.Printf("settings.UpsertChat: %v", err)
	}
}

// recordMembership ловит добавление и изгнание бота. Телеграм шлёт эти апдейты
// по умолчанию, и без них канал появлялся бы в панели только тогда, когда в нём
// кто-то напишет.
func (b *Bot) recordMembership(u *tgbotapi.ChatMemberUpdated) {
	if b.SettingsStore == nil || u == nil {
		return
	}

	b.recordChat(&u.Chat, int64(u.Date))

	switch u.NewChatMember.Status {
	case "left", "kicked":
		if err := b.SettingsStore.MarkLeft(u.Chat.ID, int64(u.Date)); err != nil {
			log.Printf("settings.MarkLeft: %v", err)
		}
	}
}
```

In `RunBot`, handle both update kinds. Replace the loop header:

```go
	for update := range updates {
		if update.MyChatMember != nil {
			b.recordMembership(update.MyChatMember)
		}

		if update.Message != nil && !update.Message.From.IsBot { // If we got a message
			b.recordChat(update.Message.Chat, int64(update.Message.Date))
			log.Printf("[%s] %s", update.Message.From.UserName, update.Message.Text)
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go build ./... && go test ./engine/ -v`
Expected: PASS, all four new tests.

- [ ] **Step 5: Commit**

```bash
git add engine/runBot.go engine/chats_test.go
git commit -m "feat(engine): record the chats the bot is in"
```

---

### Task 11: Engine — module enablement and settings injection

**Files:**
- Modify: `engine/runBot.go` (`RunBot` module loop, `shouldIAnswer`)
- Test: `engine/runBot_test.go:70` (`TestShouldIAnswer`)

**Interfaces:**
- Consumes: `(*Cache).ModuleEnabled`, `(*Cache).Values` from Task 8; `settings.Resolve` from Task 7; `Bot.Registrations` from Task 5.
- Produces: `(*Bot).settingsFor(chatID int64, moduleName string) map[string]any`. `Payload.Extra["settings"]` is set before every `IsCalled` and `Answer`.

- [ ] **Step 1: Write the failing test**

Replace `TestShouldIAnswer` in `engine/runBot_test.go`:

```go
func TestShouldIAnswerSkipsDisabledModule(t *testing.T) {
	b := botWithSettings(t)
	b.Settings = settings.NewCache(b.SettingsStore, time.Minute)
	b.BotConfig = &CalarbotConfig{Modules: map[string]ModulesConfig{"aiAnswer": {Url: "x"}}}

	client := NewMockModuleClient()
	client.IsCalledResult = true
	update := tgbotapi.Update{Message: &tgbotapi.Message{Chat: &tgbotapi.Chat{ID: -1}}}
	payload := &botModules.Payload{Msg: update.Message, Extra: map[string]interface{}{}}

	if b.shouldIAnswer("aiAnswer", update, client, payload) {
		t.Error("shouldIAnswer = true for a module with no row; want false")
	}
}

func TestShouldIAnswerAsksEnabledModule(t *testing.T) {
	b := botWithSettings(t)
	b.Settings = settings.NewCache(b.SettingsStore, time.Minute)
	b.BotConfig = &CalarbotConfig{Modules: map[string]ModulesConfig{"aiAnswer": {Url: "x"}}}
	if err := b.SettingsStore.SetModuleEnabled(-1, "aiAnswer", true); err != nil {
		t.Fatalf("SetModuleEnabled: %v", err)
	}

	client := NewMockModuleClient()
	client.IsCalledResult = true
	update := tgbotapi.Update{Message: &tgbotapi.Message{Chat: &tgbotapi.Chat{ID: -1}}}
	payload := &botModules.Payload{Msg: update.Message, Extra: map[string]interface{}{}}

	if !b.shouldIAnswer("aiAnswer", update, client, payload) {
		t.Error("shouldIAnswer = false for an enabled module; want true")
	}
}

func TestSettingsForOverlaysStoredValuesOnDefaults(t *testing.T) {
	b := botWithSettings(t)
	b.Settings = settings.NewCache(b.SettingsStore, time.Minute)
	b.Registrations = map[string]botModules.Registration{
		"aiAnswer": {Fields: []botModules.Field{
			{Key: "answer_level", Type: botModules.FieldNumber, Default: 990},
			{Key: "context_size", Type: botModules.FieldNumber, Default: 10},
		}},
	}
	if err := b.SettingsStore.SetValue(-1, "aiAnswer", "context_size", "25"); err != nil {
		t.Fatalf("SetValue: %v", err)
	}

	got := b.settingsFor(-1, "aiAnswer")

	if got["answer_level"] != 990 {
		t.Errorf("answer_level = %v; want the default 990", got["answer_level"])
	}
	if got["context_size"] != 25 {
		t.Errorf("context_size = %v; want the stored 25", got["context_size"])
	}
}
```

Add `"time"`, `"calarbot2/botModules"` and `"calarbot2/settings"` to the test file's imports.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./engine/ -run 'TestShouldIAnswer|TestSettingsFor' -v`
Expected: FAIL — `b.settingsFor undefined`.

- [ ] **Step 3: Write minimal implementation**

In `engine/runBot.go`, replace the `EnabledOn` check at the top of `shouldIAnswer`:

```go
	if update.Message == nil || update.Message.Chat == nil {
		return false
	}
	if !b.Settings.ModuleEnabled(update.Message.Chat.ID, moduleName) {
		return false
	}
```

Add:

```go
// settingsFor собирает настройки модуля для чата: явно выставленные значения
// поверх дефолтов, которые модуль объявил при регистрации.
func (b *Bot) settingsFor(chatID int64, moduleName string) map[string]any {
	return settings.Resolve(
		b.Registrations[moduleName].Fields,
		b.Settings.Values(chatID, moduleName),
	)
}
```

In `RunBot`, inside the module loop, set the settings before asking the module anything:

```go
			for _, moduleName := range b.orderedModules {
				client := b.Modules[moduleName]
				// Настройки кладём до shouldIAnswer: модуль решает, отвечать ли,
				// уже с их учётом — веса живут именно там.
				payload.Extra["settings"] = b.settingsFor(update.Message.Chat.ID, moduleName)
				if !b.shouldIAnswer(moduleName, update, client, payload) {
					continue
				}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go build ./... && go test ./engine/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add engine/
git commit -m "feat(engine): resolve module enablement and settings per chat"
```

---

### Task 12: Engine — the one-time seed

**Files:**
- Create: `engine/seed.go`
- Test: `engine/seed_test.go`

**Interfaces:**
- Consumes: `SeedChat` from Task 9; `(*Store).UpsertChat`, `(*Store).SetModuleEnabled`, `(*Store).Meta`, `(*Store).SetMeta` from Tasks 1–3.
- Produces: `(*Bot).seedSettings(now int64) error`, called from `InitBot` after the store is open.

- [ ] **Step 1: Write the failing test**

```go
package main

import (
	"testing"
)

func TestSeedEnablesListedModules(t *testing.T) {
	b := botWithSettings(t)
	b.BotConfig = &CalarbotConfig{SeedChats: []SeedChat{
		{ID: -1, Title: "болталка", Type: "supergroup", Modules: []string{"skazka", "aiAnswer"}},
	}}

	if err := b.seedSettings(1000); err != nil {
		t.Fatalf("seedSettings: %v", err)
	}

	for _, m := range []string{"skazka", "aiAnswer"} {
		if enabled, err := b.SettingsStore.ModuleEnabled(-1, m); err != nil || !enabled {
			t.Errorf("ModuleEnabled(-1, %s) = %v, %v; want true", m, enabled, err)
		}
	}
	if enabled, _ := b.SettingsStore.ModuleEnabled(-1, "sber"); enabled {
		t.Error("ModuleEnabled(-1, sber) = true; посев не должен включать неперечисленное")
	}

	chats, err := b.SettingsStore.ListChats()
	if err != nil {
		t.Fatalf("ListChats: %v", err)
	}
	if len(chats) != 1 || chats[0].Title != "болталка" {
		t.Fatalf("ListChats = %+v; want the seeded chat", chats)
	}
}

// Посев обязан быть разовым: иначе он воскресит модуль, выключенный руками.
func TestSeedRunsOnlyOnce(t *testing.T) {
	b := botWithSettings(t)
	b.BotConfig = &CalarbotConfig{SeedChats: []SeedChat{
		{ID: -1, Type: "group", Modules: []string{"aiAnswer"}},
	}}

	if err := b.seedSettings(1000); err != nil {
		t.Fatalf("seedSettings: %v", err)
	}
	if err := b.SettingsStore.SetModuleEnabled(-1, "aiAnswer", false); err != nil {
		t.Fatalf("SetModuleEnabled: %v", err)
	}
	if err := b.seedSettings(2000); err != nil {
		t.Fatalf("seedSettings: %v", err)
	}

	if enabled, _ := b.SettingsStore.ModuleEnabled(-1, "aiAnswer"); enabled {
		t.Error("ModuleEnabled = true after a second seed; посев должен был не сработать")
	}
}

func TestSeedDefaultsMissingTypeToGroup(t *testing.T) {
	b := botWithSettings(t)
	b.BotConfig = &CalarbotConfig{SeedChats: []SeedChat{{ID: -1, Modules: []string{"aiAnswer"}}}}

	if err := b.seedSettings(1000); err != nil {
		t.Fatalf("seedSettings: %v", err)
	}

	chats, err := b.SettingsStore.ListChats()
	if err != nil {
		t.Fatalf("ListChats: %v", err)
	}
	if len(chats) != 1 || chats[0].Type != "group" {
		t.Fatalf("chat type = %+v; want group", chats)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./engine/ -run TestSeed -v`
Expected: FAIL — `b.seedSettings undefined`.

- [ ] **Step 3: Write minimal implementation**

```go
package main

import (
	"log"

	"calarbot2/settings"
)

const seededMetaKey = "seeded"

// seedSettings переносит в базу то, чем включённость модулей была до появления
// админки: enabled_on в конфиге не восстановим автоматически — телеграм не
// отдаёт список чатов бота, а skazka с sber вообще работали везде.
//
// Заодно создаёт строки в chats, иначе сразу после выката панель была бы пуста
// и включать в ней было бы нечего.
//
// Разовый: флаг в settings_meta. Повторный прогон воскресил бы модуль,
// выключенный руками, — то есть ровно то, ради чего админку и делали.
func (b *Bot) seedSettings(now int64) error {
	if _, seeded, err := b.SettingsStore.Meta(seededMetaKey); err != nil {
		return err
	} else if seeded {
		return nil
	}

	for _, seed := range b.BotConfig.SeedChats {
		chatType := seed.Type
		if chatType == "" {
			chatType = "group"
		}
		if err := b.SettingsStore.UpsertChat(settings.Chat{
			ID:        seed.ID,
			Type:      chatType,
			Title:     seed.Title,
			FirstSeen: now,
			LastSeen:  now,
		}); err != nil {
			return err
		}
		for _, module := range seed.Modules {
			if err := b.SettingsStore.SetModuleEnabled(seed.ID, module, true); err != nil {
				return err
			}
		}
		log.Printf("seeded chat %d with modules %v", seed.ID, seed.Modules)
	}

	return b.SettingsStore.SetMeta(seededMetaKey, "1")
}
```

In `engine/runBot.go`, call it in `InitBot` right after the store is opened:

```go
	if err := b.seedSettings(time.Now().Unix()); err != nil {
		log.Panic(err)
	}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go build ./... && go test ./engine/ -v`
Expected: PASS, all three seed tests.

- [ ] **Step 5: Commit**

```bash
git add engine/seed.go engine/seed_test.go engine/runBot.go
git commit -m "feat(engine): seed today's module enablement once"
```

---

### Task 13: aiAnswer store — personas by key, and no more `chat_persona`

**Files:**
- Modify: `modules/aiAnswer/store/store.go:23-31` (`New`), `:56-62` (the `chat_persona` DDL)
- Modify: `modules/aiAnswer/store/persona.go:84-115` (`SetChatPersona`, `ResolvePersona`)
- Test: `modules/aiAnswer/store/persona_test.go:64` (`TestChatPersonaOverridesDefault`)

**Interfaces:**
- Consumes: nothing from earlier tasks.
- Produces: `(*Store).PersonaByKey(key string) (Persona, error)` returning `ErrNoPersona` when absent, `(*Store).ListPersonas() ([]Persona, error)`. `SetChatPersona` and `ResolvePersona` are removed.

- [ ] **Step 1: Write the failing test**

Replace `TestChatPersonaOverridesDefault` in `modules/aiAnswer/store/persona_test.go`:

```go
func TestPersonaByKey(t *testing.T) {
	s := newTestStore(t)
	mamkin, _, err := s.UpsertConfigPersona("mamkin", "Мамкин", "ты финансист")
	if err != nil {
		t.Fatalf("UpsertConfigPersona: %v", err)
	}

	got, err := s.PersonaByKey("mamkin")
	if err != nil {
		t.Fatalf("PersonaByKey: %v", err)
	}
	if got.ID != mamkin.ID || got.SystemPrompt != "ты финансист" {
		t.Errorf("PersonaByKey = %+v; want %+v", got, mamkin)
	}
}

func TestPersonaByKeyReportsMissing(t *testing.T) {
	s := newTestStore(t)

	if _, err := s.PersonaByKey("nobody"); !errors.Is(err, ErrNoPersona) {
		t.Fatalf("PersonaByKey error = %v; want ErrNoPersona", err)
	}
}

// Список для выпадашки в админке. Он же причина, по которой options считает
// модуль, а не конфиг: персоны заводятся в базе.
func TestListPersonas(t *testing.T) {
	s := newTestStore(t)
	if _, _, err := s.UpsertConfigPersona("mamkin", "Мамкин", "a"); err != nil {
		t.Fatalf("UpsertConfigPersona: %v", err)
	}
	if _, _, err := s.UpsertConfigPersona("genadiy", "Геннадий", "b"); err != nil {
		t.Fatalf("UpsertConfigPersona: %v", err)
	}

	got, err := s.ListPersonas()
	if err != nil {
		t.Fatalf("ListPersonas: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("ListPersonas returned %d; want 2", len(got))
	}
	if got[0].Key != "genadiy" || got[1].Key != "mamkin" {
		t.Errorf("ListPersonas = %+v; want them ordered by key", got)
	}
}
```

Add `"errors"` to the test file's imports if it is not already there.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./modules/aiAnswer/store/ -run 'TestPersonaByKey|TestListPersonas' -v`
Expected: FAIL — `s.PersonaByKey undefined`.

- [ ] **Step 3: Write minimal implementation**

In `modules/aiAnswer/store/persona.go`, delete `SetChatPersona` and `ResolvePersona` and add:

```go
// PersonaByKey достаёт персону по ключу.
//
// Пришёл на смену ResolvePersona: ключ теперь приезжает в настройках чата от
// движка, и таблица chat_persona под это больше не нужна.
func (s *Store) PersonaByKey(key string) (Persona, error) {
	var p Persona
	err := s.db.QueryRow(
		`SELECT id, key, name, system_prompt FROM personas WHERE key = ?`, key,
	).Scan(&p.ID, &p.Key, &p.Name, &p.SystemPrompt)
	if errors.Is(err, sql.ErrNoRows) {
		return Persona{}, ErrNoPersona
	}
	if err != nil {
		return Persona{}, fmt.Errorf("select persona: %w", err)
	}
	return p, nil
}

// ListPersonas отдаёт всё, из чего админка строит выпадашку.
func (s *Store) ListPersonas() ([]Persona, error) {
	rows, err := s.db.Query(`SELECT id, key, name, system_prompt FROM personas ORDER BY key`)
	if err != nil {
		return nil, fmt.Errorf("select personas: %w", err)
	}
	defer rows.Close()

	var out []Persona
	for rows.Next() {
		var p Persona
		if err := rows.Scan(&p.ID, &p.Key, &p.Name, &p.SystemPrompt); err != nil {
			return nil, fmt.Errorf("scan persona: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
```

In `modules/aiAnswer/store/store.go`, add the busy timeout to `New`:

```go
	// busy_timeout живёт на соединении, а не на базе: в этот же файл теперь
	// пишут движок и админка, и без ожидания второй писатель получит
	// «database is locked».
	db, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(5000)")
```

and in `migrate` replace the `chat_persona` DDL with a drop — ничего не теряется, писателя у неё никогда не было:

```go
		DROP TABLE IF EXISTS chat_persona;
```

Removing `ResolvePersona` breaks the two call sites in
`modules/aiAnswer/main.go` — in `IsCalled` and in `systemPromptFor`. Task 15
threads the real per-chat key through; for now make them compile on the config
default:

```go
	if p, err := m.store.PersonaByKey(m.config.DefaultPersona); err == nil {
```

```go
	p, err := m.store.PersonaByKey(m.config.DefaultPersona)
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go build ./... && go test ./modules/aiAnswer/... -v`
Expected: PASS, all three persona tests.

- [ ] **Step 5: Commit**

```bash
git add modules/aiAnswer/store/ modules/aiAnswer/main.go
git commit -m "refactor(aiAnswer): resolve personas by key instead of a chat table"
```

---

### Task 14: aiAnswer — declare its settings in `Register()`

**Files:**
- Modify: `modules/aiAnswer/main.go` (`Register`)
- Test: `modules/aiAnswer/main_test.go`

**Interfaces:**
- Consumes: `botModules.Registration`, `botModules.Field`, `botModules.Option` from Task 5; `(*Store).ListPersonas` from Task 13.
- Produces: `(*Module).Register()` returning five fields keyed `persona`, `answer_level`, `call_weight`, `reply_weight`, `context_size`. These key names are what Task 15 and the panel read.

- [ ] **Step 1: Write the failing test**

```go
func TestRegisterDeclaresConfigValuesAsDefaults(t *testing.T) {
	m := &Module{order: 100, config: AIConfig{
		AnswerLevel: 990, CallWeight: 700, ReplyWeight: 400,
		ContextSize: 10, DefaultPersona: "mamkin",
	}}

	reg := m.Register()

	if reg.Order != 100 || reg.Label != "AI-ответ" {
		t.Errorf("Registration = %+v; want order 100, label AI-ответ", reg)
	}

	byKey := map[string]botModules.Field{}
	for _, f := range reg.Fields {
		byKey[f.Key] = f
	}

	// Сегодняшняя настройка обязана стать дефолтом нового канала — это и есть
	// весь механизм «прикопать нынешние значения», отдельного нет.
	for key, want := range map[string]any{
		"answer_level": 990, "call_weight": 700, "reply_weight": 400, "context_size": 10,
	} {
		f, ok := byKey[key]
		if !ok {
			t.Fatalf("Register did not declare %s", key)
		}
		if f.Default != want {
			t.Errorf("%s default = %v; want %v", key, f.Default, want)
		}
		if f.Type != botModules.FieldNumber {
			t.Errorf("%s type = %q; want number", key, f.Type)
		}
	}

	if byKey["persona"].Default != "mamkin" {
		t.Errorf("persona default = %v; want mamkin", byKey["persona"].Default)
	}
	if byKey["persona"].Type != botModules.FieldSelect {
		t.Errorf("persona type = %q; want select", byKey["persona"].Type)
	}
}

// Веса — это бросок d1000, значения вне диапазона осмысленного смысла не имеют,
// и админка обязана узнать границы от модуля, а не угадать их.
func TestRegisterBoundsTheWeights(t *testing.T) {
	m := &Module{config: AIConfig{}}

	for _, f := range m.Register().Fields {
		switch f.Key {
		case "answer_level", "call_weight", "reply_weight":
			if f.Min == nil || *f.Min != 0 || f.Max == nil || *f.Max != 1000 {
				t.Errorf("%s bounds = %v..%v; want 0..1000", f.Key, f.Min, f.Max)
			}
		case "context_size":
			if f.Min == nil || *f.Min != 0 {
				t.Errorf("context_size min = %v; want 0", f.Min)
			}
		}
	}
}
```

Add `"calarbot2/botModules"` to the test file's imports if it is not already there.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./modules/aiAnswer/ -run TestRegister -v`
Expected: FAIL — `Register did not declare answer_level`.

- [ ] **Step 3: Write minimal implementation**

Replace the placeholder `Register` from Task 5 in `modules/aiAnswer/main.go`:

```go
func intPtr(v int) *int { return &v }

// Register описывает форму настроек для админки и отдаёт свои конфигурационные
// значения как дефолты: канал, которого никто не трогал, ведёт себя ровно так,
// как настроен бот в целом.
func (m *Module) Register() botModules.Registration {
	return botModules.Registration{
		Order:       m.order,
		Label:       "AI-ответ",
		Description: "Отвечает через языковую модель",
		Fields: []botModules.Field{
			{
				Key: "persona", Label: "Персона", Type: botModules.FieldSelect,
				Options: m.personaOptions(), Default: m.config.DefaultPersona,
			},
			{
				Key: "answer_level", Label: "Вес: обычный триггер", Type: botModules.FieldNumber,
				Min: intPtr(0), Max: intPtr(1000), Default: m.config.AnswerLevel,
			},
			{
				Key: "call_weight", Label: "Вес: по обращению", Type: botModules.FieldNumber,
				Min: intPtr(0), Max: intPtr(1000), Default: m.config.CallWeight,
			},
			{
				Key: "reply_weight", Label: "Вес: по реплаю", Type: botModules.FieldNumber,
				Min: intPtr(0), Max: intPtr(1000), Default: m.config.ReplyWeight,
			},
			{
				Key: "context_size", Label: "Окно контекста (сообщений)", Type: botModules.FieldNumber,
				Min: intPtr(0), Default: m.config.ContextSize,
			},
		},
	}
}

// personaOptions считается на каждый вызов: персоны живут в базе и заводятся
// без перезапуска, поэтому зашить их список было бы неверно.
func (m *Module) personaOptions() []botModules.Option {
	if m.store == nil {
		return nil
	}
	personas, err := m.store.ListPersonas()
	if err != nil {
		log.Printf("list personas: %v", err)
		return nil
	}
	opts := make([]botModules.Option, 0, len(personas))
	for _, p := range personas {
		opts = append(opts, botModules.Option{Value: p.Key, Label: p.Name})
	}
	return opts
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go build ./... && go test ./modules/aiAnswer/ -v`
Expected: PASS, both tests.

- [ ] **Step 5: Commit**

```bash
git add modules/aiAnswer/main.go modules/aiAnswer/main_test.go
git commit -m "feat(aiAnswer): declare its per-chat settings form"
```

---

### Task 15: aiAnswer — read per-chat settings from the payload

**Files:**
- Create: `modules/aiAnswer/settings.go`
- Modify: `modules/aiAnswer/main.go` (`IsCalled`, `answer`, `systemPromptFor`)
- Test: `modules/aiAnswer/settings_test.go`

**Interfaces:**
- Consumes: `Payload.Extra["settings"]` populated by Task 11; field keys from Task 14; `(*Store).PersonaByKey` from Task 13.
- Produces: `settingsOf(payload *botModules.Payload) map[string]any`, `intSetting(s map[string]any, key string, fallback int) int`, `stringSetting(s map[string]any, key, fallback string) string`. `(*Module).systemPromptFor(chatID int64, personaKey string)`.

- [ ] **Step 1: Write the failing test**

```go
package main

import (
	"testing"

	"calarbot2/botModules"
)

func TestIntSettingReadsInjectedValue(t *testing.T) {
	s := map[string]any{"answer_level": 700}

	if got := intSetting(s, "answer_level", 990); got != 700 {
		t.Fatalf("intSetting = %d; want 700", got)
	}
}

// По проводу настройки едут json'ом, а там всякое число — float64. Без этой
// ветки модуль тихо свалился бы на дефолт для каждой настройки.
func TestIntSettingAcceptsJSONFloat(t *testing.T) {
	s := map[string]any{"answer_level": float64(700)}

	if got := intSetting(s, "answer_level", 990); got != 700 {
		t.Fatalf("intSetting = %d; want 700", got)
	}
}

func TestIntSettingFallsBackWhenAbsent(t *testing.T) {
	if got := intSetting(map[string]any{}, "answer_level", 990); got != 990 {
		t.Fatalf("intSetting = %d; want the fallback 990", got)
	}
}

func TestStringSettingFallsBackWhenAbsent(t *testing.T) {
	if got := stringSetting(map[string]any{}, "persona", "mamkin"); got != "mamkin" {
		t.Fatalf("stringSetting = %q; want mamkin", got)
	}
}

func TestSettingsOfHandlesMissingExtra(t *testing.T) {
	got := settingsOf(&botModules.Payload{})

	if len(got) != 0 {
		t.Fatalf("settingsOf = %v; want an empty map", got)
	}
}

// Порог 1001 при броске d1000 делает исход однозначным при любом броске:
// с весом из настроек (1001) реплай срабатывает всегда, с дефолтным из
// конфига (0) — не срабатывает никогда. Со значениями внутри 0..1000 тест
// проходил бы по случайности в одном броске из тысячи.
func TestIsCalledUsesInjectedReplyWeight(t *testing.T) {
	m := &Module{config: AIConfig{BotUsername: "calarbot", AnswerLevel: 1001, ReplyWeight: 0}}

	payload := &botModules.Payload{
		Msg: replyToBotMessage("calarbot"),
		Extra: map[string]interface{}{
			"settings": map[string]any{"answer_level": 1001, "reply_weight": 1001},
		},
	}

	if !m.IsCalled(payload) {
		t.Fatal("IsCalled = false; want true — вес из настроек не доехал")
	}
}

func TestIsCalledFallsBackToConfigWeights(t *testing.T) {
	m := &Module{config: AIConfig{BotUsername: "calarbot", AnswerLevel: 1001, ReplyWeight: 0}}

	if m.IsCalled(&botModules.Payload{Msg: replyToBotMessage("calarbot")}) {
		t.Fatal("IsCalled = true without settings; want false")
	}
}
```

Add this helper to the same file:

```go
func replyToBotMessage(botUsername string) *tgbotapi.Message {
	return &tgbotapi.Message{
		Chat: &tgbotapi.Chat{ID: -1},
		From: &tgbotapi.User{ID: 5, UserName: "человек"},
		Text: "ага",
		ReplyToMessage: &tgbotapi.Message{
			From: &tgbotapi.User{UserName: botUsername},
		},
	}
}
```

with `tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"` imported.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./modules/aiAnswer/ -run 'TestIntSetting|TestStringSetting|TestSettingsOf|TestIsCalledUses' -v`
Expected: FAIL — `undefined: intSetting`.

- [ ] **Step 3: Write minimal implementation**

Create `modules/aiAnswer/settings.go`:

```go
package main

import "calarbot2/botModules"

// Настройки чата приезжают от движка в Extra — модуль их не хранит и в базу за
// ними не ходит. Полную карту собирает движок, так что фолбэки здесь нужны
// только на случай вызова в обход него: тесты, локальный запуск, старый движок.
func settingsOf(payload *botModules.Payload) map[string]any {
	if payload == nil {
		return map[string]any{}
	}
	s, _ := payload.Extra["settings"].(map[string]any)
	if s == nil {
		return map[string]any{}
	}
	return s
}

// intSetting понимает и int, и float64: по проводу настройки едут json'ом, а
// там всякое число — float64.
func intSetting(s map[string]any, key string, fallback int) int {
	switch v := s[key].(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	default:
		return fallback
	}
}

func stringSetting(s map[string]any, key, fallback string) string {
	if v, ok := s[key].(string); ok && v != "" {
		return v
	}
	return fallback
}
```

In `modules/aiAnswer/main.go`, `IsCalled` reads the weights and the persona key from the payload:

```go
func (m *Module) IsCalled(payload *botModules.Payload) bool {
	msg := payload.Msg
	if msg == nil {
		return false
	}
	s := settingsOf(payload)
	personaKey := stringSetting(s, "persona", m.config.DefaultPersona)

	if m.store != nil {
		if err := m.store.SaveMessage(msg); err != nil {
			log.Printf("store.SaveMessage: %v", err)
		}
	}
	// Лор растёт на каждом сообщении чата, а не только когда бот отвечает:
	// IsCalled видит весь поток, и никакого расписания для этого не нужно.
	if m.store != nil && m.loreRunner != nil && msg.Chat != nil {
		if p, err := m.store.PersonaByKey(personaKey); err == nil {
			m.loreRunner.Maybe(msg.Chat.ID, p.ID, p.SystemPrompt)
		}
	}
	if isDirectAddress(msg, m.config.BotUsername) {
		return true
	}
	roll := rand.Intn(DiceSize + 1)
	if msg.ReplyToMessage != nil && msg.ReplyToMessage.From != nil &&
		msg.ReplyToMessage.From.UserName == m.config.BotUsername {
		roll += intSetting(s, "reply_weight", m.config.ReplyWeight)
	}
	if common.Contains(common.ExtractMentions(msg), "@"+m.config.BotUsername) {
		roll += intSetting(s, "call_weight", m.config.CallWeight)
	}
	return roll >= intSetting(s, "answer_level", m.config.AnswerLevel)
}
```

`systemPromptFor` takes the key instead of resolving it:

```go
func (m *Module) systemPromptFor(chatID int64, personaKey string) (store.Persona, string) {
	if m.store == nil {
		return store.Persona{}, m.config.SystemPrompt
	}
	p, err := m.store.PersonaByKey(personaKey)
	if err != nil {
		log.Printf("persona %q: %v", personaKey, err)
		return store.Persona{}, m.config.SystemPrompt
	}
```

(the rest of the function body is unchanged)

And `answer` reads the context size and the persona from the same place:

```go
	s := settingsOf(payload)

	var history []store.ContextMessage
	if m.store != nil {
		var err error
		history, err = m.store.GetContext(msg.Chat.ID, intSetting(s, "context_size", m.config.ContextSize))
		if err != nil {
			log.Printf("store.GetContext: %v", err)
		}
	}

	photoURL, _ := payload.Extra["photo_url"].(string)
	_, system := m.systemPromptFor(msg.Chat.ID, stringSetting(s, "persona", m.config.DefaultPersona))
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go build ./... && go vet ./... && go test ./... -v`
Expected: PASS across every package.

- [ ] **Step 5: Commit**

```bash
git add modules/aiAnswer/
git commit -m "feat(aiAnswer): take its per-chat settings from the payload"
```

---

### Task 16: Admin — the module registry client

**Files:**
- Create: `admin/registry.go`
- Test: `admin/registry_test.go`

**Interfaces:**
- Consumes: `botModules.Registration`, `(*ModuleClient).Register` from Task 5.
- Produces: `NewRegistry(modules map[string]string, ttl time.Duration) *Registry`, `(*Registry).Get(name string) (botModules.Registration, error)`, `(*Registry).Names() []string` (ordered by `Order`, then by name), and the injectable `Registry.now func() time.Time`.

- [ ] **Step 1: Write the failing test**

```go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./admin/ -run TestRegistry -v`
Expected: FAIL — the `admin` package does not exist yet.

- [ ] **Step 3: Write minimal implementation**

```go
package main

import (
	"sort"
	"sync"
	"time"

	"calarbot2/botModules"
)

// Registry спрашивает у каждого модуля, кто он и какие у него настройки.
//
// Кэш на модуль, а не на чат: регистрация от чата не зависит, поэтому отрисовка
// страницы с полусотней карточек стоит одного вызова на модуль. TTL всё же
// короткий — options у select'ов модуль считает на лету, и список персон,
// заведённых без перезапуска, не должен висеть в панели устаревшим.
type Registry struct {
	modules map[string]string
	ttl     time.Duration
	now     func() time.Time

	mu    sync.Mutex
	cache map[string]cachedReg
}

type cachedReg struct {
	reg botModules.Registration
	at  time.Time
}

func NewRegistry(modules map[string]string, ttl time.Duration) *Registry {
	return &Registry{
		modules: modules,
		ttl:     ttl,
		now:     time.Now,
		cache:   map[string]cachedReg{},
	}
}

func (r *Registry) Get(name string) (botModules.Registration, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if hit, ok := r.cache[name]; ok && r.now().Sub(hit.at) < r.ttl {
		return hit.reg, nil
	}

	client := &botModules.ModuleClient{BaseURL: r.modules[name]}
	reg, err := client.Register()
	if err != nil {
		return reg, err
	}

	r.cache[name] = cachedReg{reg: reg, at: r.now()}
	return reg, nil
}

// Names отдаёт все модули из реестра, включая лежачие: их тумблеры работают.
func (r *Registry) Names() []string {
	names := make([]string, 0, len(r.modules))
	for name := range r.modules {
		names = append(names, name)
	}

	order := map[string]int{}
	for _, name := range names {
		reg, _ := r.Get(name)
		order[name] = reg.Order
	}

	sort.Slice(names, func(i, j int) bool {
		if order[names[i]] != order[names[j]] {
			return order[names[i]] < order[names[j]]
		}
		return names[i] < names[j]
	})
	return names
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./admin/ -v`
Expected: PASS, all four tests.

- [ ] **Step 5: Commit**

```bash
git add admin/registry.go admin/registry_test.go
git commit -m "feat(admin): ask each module to introduce itself"
```

---

### Task 17: Admin — the mutation API

**Files:**
- Create: `admin/api.go`, `admin/telegram.go`
- Test: `admin/api_test.go`

**Interfaces:**
- Consumes: `settings.Store` methods from Tasks 1–4; `(*Registry).Get` from Task 16; `botModules.Field` from Task 5.
- Produces: `Leaver` interface with `LeaveChat(chatID int64) error`, `API{Store *settings.Store; Registry *Registry; Leaver Leaver; Now func() int64}`, `(*API).Routes(mux *http.ServeMux)`, `validateValue(f botModules.Field, v any) (string, error)`.
- Routes: `PATCH /api/chats/{id}/modules/{module}`, `PATCH /api/chats/{id}/settings/{module}`, `POST /api/chats/{id}/leave`.

- [ ] **Step 1: Write the failing test**

```go
package main

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"calarbot2/botModules"
	"calarbot2/settings"
)

type fakeLeaver struct {
	left []int64
	err  error
}

func (f *fakeLeaver) LeaveChat(chatID int64) error {
	f.left = append(f.left, chatID)
	return f.err
}

func testAPI(t *testing.T, reg botModules.Registration) (*API, *fakeLeaver, http.Handler) {
	t.Helper()
	s, err := settings.New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("settings.New: %v", err)
	}
	t.Cleanup(func() { s.Close() })

	calls := 0
	srv := regServer(t, reg, &calls)
	t.Cleanup(srv.Close)

	leaver := &fakeLeaver{}
	api := &API{
		Store:    s,
		Registry: NewRegistry(map[string]string{"aiAnswer": srv.URL}, time.Minute),
		Leaver:   leaver,
		Now:      func() int64 { return 1000 },
	}
	mux := http.NewServeMux()
	api.Routes(mux)
	return api, leaver, mux
}

func numberReg() botModules.Registration {
	min, max := 0, 1000
	return botModules.Registration{
		Order: 100, Label: "AI-ответ",
		Fields: []botModules.Field{
			{Key: "answer_level", Type: botModules.FieldNumber, Min: &min, Max: &max, Default: 990},
			{Key: "persona", Type: botModules.FieldSelect, Default: "mamkin",
				Options: []botModules.Option{{Value: "mamkin"}, {Value: "genadiy"}}},
		},
	}
}

func do(t *testing.T, h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestPatchModuleEnables(t *testing.T) {
	api, _, h := testAPI(t, numberReg())

	if rec := do(t, h, http.MethodPatch, "/api/chats/-1/modules/aiAnswer", `{"enabled":true}`); rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, body %s; want 204", rec.Code, rec.Body)
	}

	if enabled, err := api.Store.ModuleEnabled(-1, "aiAnswer"); err != nil || !enabled {
		t.Fatalf("ModuleEnabled = %v, %v; want true", enabled, err)
	}
}

func TestPatchSettingsStoresValue(t *testing.T) {
	api, _, h := testAPI(t, numberReg())

	if rec := do(t, h, http.MethodPatch, "/api/chats/-1/settings/aiAnswer", `{"answer_level":700}`); rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, body %s; want 204", rec.Code, rec.Body)
	}

	values, err := api.Store.Values(-1, "aiAnswer")
	if err != nil {
		t.Fatalf("Values: %v", err)
	}
	if values["answer_level"] != "700" {
		t.Fatalf("answer_level = %q; want \"700\"", values["answer_level"])
	}
}

// Молча обрезать вылезшее за границу — худший вариант: настройка выглядит
// принятой, а бот ведёт себя иначе.
func TestPatchSettingsRejectsOutOfRange(t *testing.T) {
	api, _, h := testAPI(t, numberReg())

	rec := do(t, h, http.MethodPatch, "/api/chats/-1/settings/aiAnswer", `{"answer_level":5000}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d; want 400", rec.Code)
	}

	values, _ := api.Store.Values(-1, "aiAnswer")
	if _, ok := values["answer_level"]; ok {
		t.Error("отвергнутое значение всё-таки записалось")
	}
}

func TestPatchSettingsRejectsUnknownSelectOption(t *testing.T) {
	_, _, h := testAPI(t, numberReg())

	if rec := do(t, h, http.MethodPatch, "/api/chats/-1/settings/aiAnswer", `{"persona":"нет такой"}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d; want 400", rec.Code)
	}
}

func TestPatchSettingsRejectsUndeclaredKey(t *testing.T) {
	_, _, h := testAPI(t, numberReg())

	if rec := do(t, h, http.MethodPatch, "/api/chats/-1/settings/aiAnswer", `{"whatever":1}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d; want 400", rec.Code)
	}
}

// null — это «вернуть к дефолту», а не «записать ноль».
func TestPatchSettingsNullDeletesTheRow(t *testing.T) {
	api, _, h := testAPI(t, numberReg())
	if err := api.Store.SetValue(-1, "aiAnswer", "answer_level", "700"); err != nil {
		t.Fatalf("SetValue: %v", err)
	}

	if rec := do(t, h, http.MethodPatch, "/api/chats/-1/settings/aiAnswer", `{"answer_level":null}`); rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, body %s; want 204", rec.Code, rec.Body)
	}

	values, _ := api.Store.Values(-1, "aiAnswer")
	if _, ok := values["answer_level"]; ok {
		t.Fatalf("Values = %v; want the row gone", values)
	}
}

func TestLeaveCallsTelegramAndMarksTheChat(t *testing.T) {
	api, leaver, h := testAPI(t, numberReg())
	if err := api.Store.UpsertChat(settings.Chat{ID: -1, Type: "group", FirstSeen: 1, LastSeen: 1}); err != nil {
		t.Fatalf("UpsertChat: %v", err)
	}

	if rec := do(t, h, http.MethodPost, "/api/chats/-1/leave", ""); rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, body %s; want 204", rec.Code, rec.Body)
	}

	if len(leaver.left) != 1 || leaver.left[0] != -1 {
		t.Errorf("LeaveChat calls = %v; want [-1]", leaver.left)
	}
	chats, _ := api.Store.ListChats()
	if len(chats) != 0 {
		t.Errorf("ListChats = %+v; want the chat gone from the panel", chats)
	}
}

// Телеграм отказал — чат остаётся в панели: помечать уходом то, из чего не
// вышли, значит потерять канал из виду.
func TestLeaveKeepsChatWhenTelegramFails(t *testing.T) {
	api, leaver, h := testAPI(t, numberReg())
	leaver.err = errors.New("telegram is unhappy")
	if err := api.Store.UpsertChat(settings.Chat{ID: -1, Type: "group", FirstSeen: 1, LastSeen: 1}); err != nil {
		t.Fatalf("UpsertChat: %v", err)
	}

	if rec := do(t, h, http.MethodPost, "/api/chats/-1/leave", ""); rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d; want 502", rec.Code)
	}

	chats, _ := api.Store.ListChats()
	if len(chats) != 1 {
		t.Fatalf("ListChats = %+v; want the chat still listed", chats)
	}
}
```

Add `"errors"` to the imports.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./admin/ -run 'TestPatch|TestLeave' -v`
Expected: FAIL — `undefined: API`.

- [ ] **Step 3: Write minimal implementation**

`admin/telegram.go`:

```go
package main

import (
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// Leaver — та часть телеграма, которой пользуется панель. Интерфейсом, чтобы
// хендлеры тестировались без сети, как Sender в notify.
type Leaver interface {
	LeaveChat(chatID int64) error
}

type BotLeaver struct {
	API *tgbotapi.BotAPI
}

func (b *BotLeaver) LeaveChat(chatID int64) error {
	_, err := b.API.Request(tgbotapi.LeaveChatConfig{ChatID: chatID})
	return err
}
```

`admin/api.go`:

```go
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
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
func (a *API) leave(w http.ResponseWriter, r *http.Request) {
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
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./admin/ -v`
Expected: PASS, all tests.

- [ ] **Step 5: Commit**

```bash
git add admin/api.go admin/telegram.go admin/api_test.go
git commit -m "feat(admin): toggle modules, write settings, leave a chat"
```

---

### Task 18: Admin — the page

**Files:**
- Create: `admin/page.go`, `admin/templates/index.html`, `admin/static/style.css`, `admin/static/app.js`
- Test: `admin/page_test.go`

**Interfaces:**
- Consumes: `(*Store).ListChats`, `(*Store).ModuleEnabled`, `(*Store).Values`, `(*Registry).Get`, `(*Registry).Names`, `settings.Resolve`.
- Produces: `Page{Store *settings.Store; Registry *Registry}`, `(*Page).Handler() http.HandlerFunc` serving `GET /`, `(*Page).view() (pageView, error)`.

Everything is rendered server-side, including each card's settings form. The
form is in the HTML from the start and hidden by CSS until the card is expanded:
`Registry` caches per module, so a page with fifty channels still costs one call
per module, and the alternative — building the form in JavaScript from the
schema — would be far more script for no gain.

- [ ] **Step 1: Write the failing test**

```go
package main

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"calarbot2/settings"
)

func testPage(t *testing.T) (*Page, http.Handler) {
	t.Helper()
	s, err := settings.New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("settings.New: %v", err)
	}
	t.Cleanup(func() { s.Close() })

	calls := 0
	srv := regServer(t, numberReg(), &calls)
	t.Cleanup(srv.Close)

	p := &Page{Store: s, Registry: NewRegistry(map[string]string{"aiAnswer": srv.URL}, time.Minute)}
	return p, p.Handler()
}

func TestPageRendersChannelsAndDMs(t *testing.T) {
	p, h := testPage(t)
	if err := p.Store.UpsertChat(settings.Chat{ID: -1, Type: "supergroup", Title: "болталка", FirstSeen: 1, LastSeen: 1}); err != nil {
		t.Fatalf("UpsertChat: %v", err)
	}
	if err := p.Store.UpsertChat(settings.Chat{ID: 42, Type: "private", Title: "Даня", Username: "danich", FirstSeen: 1, LastSeen: 1}); err != nil {
		t.Fatalf("UpsertChat: %v", err)
	}

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{"болталка", "AI-ответ", "Даня", "danich", "Личные сообщения"} {
		if !strings.Contains(body, want) {
			t.Errorf("page does not contain %q", want)
		}
	}
	// Личка — не канал: тумблеров модулей у неё в списке каналов быть не должно.
	if strings.Count(body, `data-chat="42"`) != 1 {
		t.Errorf("private chat rendered %d times; want once, in the DM table", strings.Count(body, `data-chat="42"`))
	}
}

func TestPageShowsStoredValueNotDefault(t *testing.T) {
	p, h := testPage(t)
	if err := p.Store.UpsertChat(settings.Chat{ID: -1, Type: "group", Title: "чат", FirstSeen: 1, LastSeen: 1}); err != nil {
		t.Fatalf("UpsertChat: %v", err)
	}
	if err := p.Store.SetModuleEnabled(-1, "aiAnswer", true); err != nil {
		t.Fatalf("SetModuleEnabled: %v", err)
	}
	if err := p.Store.SetValue(-1, "aiAnswer", "answer_level", "700"); err != nil {
		t.Fatalf("SetValue: %v", err)
	}

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if !strings.Contains(rec.Body.String(), `value="700"`) {
		t.Error("page does not show the stored 700")
	}
}

// Модуль лежит — страница обязана отрисоваться, а тумблер остаться рабочим.
func TestPageSurvivesUnreachableModule(t *testing.T) {
	s, err := settings.New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("settings.New: %v", err)
	}
	defer s.Close()
	if err := s.UpsertChat(settings.Chat{ID: -1, Type: "group", Title: "чат", FirstSeen: 1, LastSeen: 1}); err != nil {
		t.Fatalf("UpsertChat: %v", err)
	}

	p := &Page{Store: s, Registry: NewRegistry(map[string]string{"dead": "http://127.0.0.1:1"}, time.Minute)}

	rec := httptest.NewRecorder()
	p.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "dead") {
		t.Error("page does not name the unreachable module")
	}
	if !strings.Contains(body, `data-module="dead"`) {
		t.Error("page has no toggle for the unreachable module")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./admin/ -run TestPage -v`
Expected: FAIL — `undefined: Page`.

- [ ] **Step 3: Write minimal implementation**

`admin/page.go`:

```go
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
```

`admin/templates/index.html` — one document, both tabs:

```html
<!doctype html>
<html lang="ru">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>calarbot2</title>
<link rel="stylesheet" href="/static/style.css">
</head>
<body>
<nav class="nav">
  <span class="brand">calarbot2</span>
  <button class="tab is-active" data-tab="channels">Каналы</button>
  <button class="tab" data-tab="dms">Личные сообщения</button>
</nav>

<main class="column">
  <section id="channels">
    <header class="head"><h1>Каналы</h1><span class="count">{{len .Channels}}</span></header>
    <div class="list">
      {{range .Channels}}
      <article class="card" data-chat="{{.ID}}">
        <div class="card-head">
          <div>
            <div class="row">
              <span class="name">{{if .Title}}{{.Title}}{{else}}без названия{{end}}</span>
              <span class="id">{{.ID}}</span>
              <span class="tag tag-neutral">{{.TypeLabel}}</span>
            </div>
            <div class="row tags">
              {{if .EnabledLabels}}
                {{range .EnabledLabels}}<span class="tag tag-outline">{{.}}</span>{{end}}
              {{else}}
                <span class="muted">Все модули выключены</span>
              {{end}}
            </div>
          </div>
          <div class="actions">
            <button class="btn-icon btn-secondary js-leave" title="Выйти из канала">⎋</button>
            <button class="btn-icon btn-secondary js-expand" title="Развернуть">⌄</button>
          </div>
        </div>

        <div class="panel" hidden>
          {{range .Modules}}
          <div class="module" data-module="{{.Key}}">
            <div class="module-head">
              <div>
                <div class="module-label">{{.Label}}</div>
                <div class="muted">{{.Description}}</div>
              </div>
              <div class="seg">
                <label class="seg-opt"><input type="radio" class="js-toggle" value="1" {{if .Enabled}}checked{{end}}> Включён</label>
                <label class="seg-opt"><input type="radio" class="js-toggle" value="0" {{if not .Enabled}}checked{{end}}> Выключен</label>
              </div>
            </div>
            {{if .Err}}<div class="muted err">{{.Err}}</div>{{end}}
            {{if .Fields}}
            <div class="settings" {{if not .Enabled}}hidden{{end}}>
              {{range .Fields}}
              <label class="field">
                <span>{{.Label}}</span>
                {{if eq .Type "select"}}
                <select class="input js-field" data-key="{{.Key}}">
                  {{$v := .Value}}
                  {{range .Options}}<option value="{{.Value}}" {{if eq .Value $v}}selected{{end}}>{{if .Label}}{{.Label}}{{else}}{{.Value}}{{end}}</option>{{end}}
                </select>
                {{else}}
                <input class="input js-field" data-key="{{.Key}}" type="number"
                       {{if .Min}}min="{{.Min}}"{{end}} {{if .Max}}max="{{.Max}}"{{end}}
                       value="{{.Value}}">
                {{end}}
              </label>
              {{end}}
            </div>
            {{end}}
          </div>
          {{end}}
        </div>
      </article>
      {{end}}
    </div>
  </section>

  <section id="dms" hidden>
    <header class="head"><h1>Личные сообщения</h1><span class="count">{{len .DMs}}</span></header>
    <table class="table">
      <thead><tr><th>Имя</th><th>User ID</th><th>Последняя активность</th></tr></thead>
      <tbody>
        {{range .DMs}}
        <tr data-chat="{{.ID}}">
          <td>{{.Title}}<div class="muted">@{{.Username}}</div></td>
          <td class="id">{{.ID}}</td>
          <td class="muted">{{.LastSeenText}}</td>
        </tr>
        {{end}}
      </tbody>
    </table>
  </section>
</main>

<dialog class="dialog" id="confirm">
  <p id="confirm-text"></p>
  <div class="dialog-actions">
    <button value="cancel" class="btn-secondary" id="confirm-no">Отмена</button>
    <button class="btn-primary" id="confirm-yes">Выйти</button>
  </div>
</dialog>

<script src="/static/app.js"></script>
</body>
</html>
```

Радиокнопки внутри `.seg` группируются в `app.js`: он раздаёт каждой паре
уникальный `name` на загрузке. В шаблоне `name` не проставить — вью не знает
номера карточки, а без группировки обе кнопки нажимались бы одновременно.

`admin/static/style.css` — Nocturne tokens and the components the template uses:

```css
:root {
  --color-bg: #161826;
  --color-surface: #232532;
  --color-text: #e9e9ed;
  --color-accent: #9184d9;
  --color-divider: color-mix(in srgb, var(--color-text) 16%, transparent);
  --color-muted: color-mix(in srgb, var(--color-text) 55%, transparent);
  --space-1: 2.8px; --space-2: 5.6px; --space-3: 8.4px; --space-4: 11.2px;
  --space-5: 14px; --space-6: 16.8px; --space-8: 22.4px;
  --radius-sm: 4px; --radius-md: 8px; --radius-lg: 14px;
  --font: Inter, -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif;
}
* { box-sizing: border-box; }
body {
  margin: 0; background: var(--color-bg); color: var(--color-text);
  font-family: var(--font); font-size: 14px; font-weight: 400;
}
.nav {
  display: flex; align-items: center; gap: var(--space-5);
  padding: var(--space-4) var(--space-6);
  border-bottom: 1px solid var(--color-divider);
}
.brand { font-weight: 500; }
.tab {
  background: none; border: none; color: var(--color-muted);
  font: inherit; cursor: pointer; padding: var(--space-2) 0;
}
.tab.is-active { color: var(--color-text); box-shadow: inset 0 -2px 0 var(--color-accent); }
.column { max-width: 760px; margin: 0 auto; padding: var(--space-8) var(--space-5); }
.head { display: flex; justify-content: space-between; align-items: baseline; }
h1 { font-size: 20px; font-weight: 500; margin: 0 0 var(--space-5); }
.count { color: var(--color-muted); }
.list { display: flex; flex-direction: column; gap: 12px; }
.card {
  border: 1px solid var(--color-divider); border-radius: var(--radius-lg);
  padding: var(--space-4);
}
.card-head { display: flex; justify-content: space-between; gap: var(--space-4); }
.row { display: flex; align-items: baseline; gap: var(--space-3); flex-wrap: wrap; }
.tags { margin-top: var(--space-2); }
.name { font-size: 16px; font-weight: 500; }
.id { font-family: ui-monospace, SFMono-Regular, Menlo, monospace; font-size: 12.5px; color: var(--color-muted); }
.muted { color: var(--color-muted); font-size: 12.5px; }
.err { margin-top: var(--space-2); }
.tag { border-radius: var(--radius-sm); padding: 1px var(--space-2); font-size: 12px; }
.tag-neutral { background: var(--color-surface); color: var(--color-muted); }
.tag-outline { border: 1px solid var(--color-divider); }
.actions { display: flex; gap: var(--space-2); align-items: flex-start; }
.btn-icon {
  width: 26px; height: 26px; border-radius: var(--radius-sm);
  background: none; color: var(--color-text); cursor: pointer; font-size: 15px;
  border: 1px solid var(--color-divider);
}
.btn-primary { border: 1px solid var(--color-accent); color: var(--color-accent); background: none; }
.btn-secondary, .btn-primary {
  border-radius: var(--radius-sm); padding: var(--space-2) var(--space-4);
  font: inherit; cursor: pointer;
}
.btn-secondary { border: 1px solid var(--color-divider); background: none; color: var(--color-text); }
.panel {
  margin-top: var(--space-4); padding-top: var(--space-4);
  border-top: 1px solid var(--color-divider);
  display: grid; gap: 8.4px;
}
.module-head { display: flex; justify-content: space-between; align-items: center; gap: var(--space-4); }
.module-label { font-size: 14px; font-weight: 500; }
.seg { display: flex; gap: var(--space-1); }
.seg-opt { font-size: 12.5px; color: var(--color-muted); cursor: pointer; }
.settings {
  margin-top: var(--space-3); background: var(--color-surface);
  border-radius: var(--radius-md); padding: var(--space-4);
  display: grid; grid-template-columns: repeat(3, 1fr); gap: var(--space-3);
}
.field { display: flex; flex-direction: column; gap: var(--space-1); font-size: 12.5px; }
.field:first-child, .field:last-child { grid-column: 1 / -1; }
.input {
  background: var(--color-bg); color: var(--color-text);
  border: 1px solid var(--color-divider); border-radius: var(--radius-sm);
  padding: var(--space-2); font: inherit;
}
.input:focus { outline: 2px solid var(--color-accent); outline-offset: 1px; }
.table { width: 100%; border-collapse: collapse; }
.table th { text-align: left; font-weight: 500; color: var(--color-muted); font-size: 12.5px; }
.table th, .table td { padding: var(--space-3); border-bottom: 1px solid var(--color-divider); }
.dialog {
  background: var(--color-surface); color: var(--color-text);
  border: 1px solid var(--color-divider); border-radius: var(--radius-lg);
  padding: var(--space-6);
}
.dialog-actions { display: flex; gap: var(--space-3); justify-content: flex-end; }
[hidden] { display: none !important; }
```

`admin/static/app.js`:

```js
// Панель только мутирует состояние: разметку целиком отдаёт сервер, поэтому
// здесь нет ни шаблонов, ни сборки форм.
const api = (method, url, body) =>
  fetch(url, {
    method,
    headers: body ? { "Content-Type": "application/json" } : {},
    body: body ? JSON.stringify(body) : null,
  }).then((r) => {
    if (!r.ok) return r.text().then((t) => Promise.reject(new Error(t || r.status)));
  });

const fail = (e) => alert("Не сохранилось: " + e.message);

document.querySelectorAll(".seg").forEach((seg, i) => {
  seg.querySelectorAll("input[type=radio]").forEach((r) => (r.name = "seg-" + i));
});

document.querySelectorAll(".tab").forEach((tab) => {
  tab.onclick = () => {
    document.querySelectorAll(".tab").forEach((t) => t.classList.toggle("is-active", t === tab));
    document.getElementById("channels").hidden = tab.dataset.tab !== "channels";
    document.getElementById("dms").hidden = tab.dataset.tab !== "dms";
  };
});

document.querySelectorAll(".js-expand").forEach((btn) => {
  btn.onclick = () => {
    const panel = btn.closest(".card").querySelector(".panel");
    panel.hidden = !panel.hidden;
    btn.style.transform = panel.hidden ? "" : "rotate(180deg)";
  };
});

document.querySelectorAll(".js-toggle").forEach((radio) => {
  radio.onchange = () => {
    const row = radio.closest(".module");
    const chat = radio.closest(".card").dataset.chat;
    const enabled = radio.value === "1";
    const settings = row.querySelector(".settings");
    // Настройки прячем сразу, но не стираем: значения живут в базе и вернутся
    // такими же, когда модуль включат обратно.
    if (settings) settings.hidden = !enabled;
    api("PATCH", `/api/chats/${chat}/modules/${row.dataset.module}`, { enabled }).catch(fail);
  };
});

document.querySelectorAll(".js-field").forEach((input) => {
  input.onchange = () => {
    const row = input.closest(".module");
    const chat = input.closest(".card").dataset.chat;
    const raw = input.value;
    const value = input.tagName === "SELECT" ? raw : raw === "" ? null : Number(raw);
    api("PATCH", `/api/chats/${chat}/settings/${row.dataset.module}`, {
      [input.dataset.key]: value,
    }).catch(fail);
  };
});

const dialog = document.getElementById("confirm");
let pending = null;

document.querySelectorAll(".js-leave").forEach((btn) => {
  btn.onclick = () => {
    const card = btn.closest(".card");
    pending = card;
    document.getElementById("confirm-text").textContent =
      "Выйти из «" + card.querySelector(".name").textContent + "»?";
    dialog.showModal();
  };
});

document.getElementById("confirm-no").onclick = () => dialog.close();
document.getElementById("confirm-yes").onclick = () => {
  const card = pending;
  dialog.close();
  api("POST", `/api/chats/${card.dataset.chat}/leave`)
    .then(() => card.remove())
    .catch(fail);
};
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./admin/ -v`
Expected: PASS, all three page tests.

- [ ] **Step 5: Commit**

```bash
git add admin/page.go admin/templates/ admin/static/ admin/page_test.go
git commit -m "feat(admin): render the channels and DM screens"
```

---

### Task 19: Admin — wiring and the image

**Files:**
- Create: `admin/main.go`, `admin/Dockerfile`
- Test: `admin/main_test.go`

**Interfaces:**
- Consumes: everything from Tasks 16–18.
- Produces: `AdminConfig{Modules map[string]ModuleEntry; TgTokenFile string; SQLitePath string}`, `moduleURLs(c AdminConfig) map[string]string`, and a `main` that serves `/`, `/static/`, `/healthz` and the API on `ADMIN_PORT` (default `8080`).

- [ ] **Step 1: Write the failing test**

```go
package main

import (
	"testing"

	"gopkg.in/yaml.v3"
)

// Реестр модулей у панели тот же, что у движка: тот же файл, тот же ключ.
// Собрали бота без sber — строки нет, и в панели он не появится.
func TestModuleURLsComeFromTheEngineConfig(t *testing.T) {
	const raw = `
tgTokenFile: /.tgtoken
sqlitePath: /data/calarbot.db
modules:
  aiAnswer:
    url: "http://aiAnswer:8080"
  skazka:
    url: "http://skazka:8080"
`
	var c AdminConfig
	if err := yaml.Unmarshal([]byte(raw), &c); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	urls := moduleURLs(c)

	if len(urls) != 2 {
		t.Fatalf("moduleURLs = %v; want two entries", urls)
	}
	if urls["aiAnswer"] != "http://aiAnswer:8080" {
		t.Errorf("aiAnswer url = %q", urls["aiAnswer"])
	}
	if c.SQLitePath != "/data/calarbot.db" || c.TgTokenFile != "/.tgtoken" {
		t.Errorf("config = %+v; want the db path and token file read", c)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./admin/ -run TestModuleURLs -v`
Expected: FAIL — `undefined: AdminConfig`.

- [ ] **Step 3: Write minimal implementation**

`admin/main.go`:

```go
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
```

`admin/Dockerfile` — the same two-stage build the other services use:

```dockerfile
FROM golang:1.23-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
COPY common/ ./common/
COPY botModules/ ./botModules/
COPY settings/ ./settings/
COPY admin/ ./admin/
RUN go build -o /admin ./admin

FROM alpine:3.19
WORKDIR /app
COPY --from=builder /admin ./admin
CMD ["./admin"]
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go build ./... && go vet ./... && go test ./... -v`
Expected: PASS across every package.

Then check the image builds: `docker build -f admin/Dockerfile -t calarbot2-admin:test .`
Expected: a successful build.

- [ ] **Step 5: Commit**

```bash
git add admin/main.go admin/Dockerfile admin/main_test.go
git commit -m "feat(admin): wire the panel up and give it an image"
```

---

### Task 20: CI — teach the deploy about the new services

**Files:**
- Modify: `detect_changed_services.py`
- Test: `test_detect_changed_services.py`

**Interfaces:**
- Consumes: nothing.
- Produces: `detect_services` maps `admin/**` to `{"admin"}` and `settings/**` to `{"engine", "admin"}`.

- [ ] **Step 1: Write the failing test**

Add to `test_detect_changed_services.py`:

```python
def test_admin_is_its_own_service():
    assert detect_services(["admin/page.go"]) == {"admin"}


def test_settings_rebuilds_only_what_links_it():
    # settings компилируется в движок и в панель. В REBUILD_EVERYTHING он
    # намеренно не попал: пересобирать из-за него все семь сервисов на хосте
    # с двумя гигабайтами памяти незачем.
    assert detect_services(["settings/store.go"]) == {"engine", "admin"}


def test_settings_does_not_rebuild_everything():
    assert "all" not in detect_services(["settings/store.go"])
```

- [ ] **Step 2: Run test to verify it fails**

Run: `python3 -m pytest test_detect_changed_services.py -v`
Expected: FAIL — `detect_services(["admin/page.go"])` returns `set()`.

- [ ] **Step 3: Write minimal implementation**

In `detect_changed_services.py`, add the two branches next to the `notify/` one:

```python
        # admin лежит верхним уровнем по той же причине, что и notify: это не
        # BotModule, движок его не опрашивает.
        elif file.startswith("admin/"):
            services.add("admin")
        # settings линкуется в движок и в панель. В REBUILD_EVERYTHING он
        # намеренно не попал: пересобирать из-за него все семь сервисов на
        # хосте с двумя гигабайтами памяти незачем.
        elif file.startswith("settings/"):
            services.update(("engine", "admin"))
```

- [ ] **Step 4: Run test to verify it passes**

Run: `python3 -m pytest test_detect_changed_services.py -v`
Expected: PASS, all tests.

- [ ] **Step 5: Commit**

```bash
git add detect_changed_services.py test_detect_changed_services.py
git commit -m "ci: deploy the admin panel and what links the settings package"
```

---

### Task 21: Documentation

**Files:**
- Modify: `README.md`
- Modify: `calarbot.yaml.example`
- Modify: `docker-compose.example`

**Interfaces:**
- Consumes: everything above.
- Produces: no code.

- [ ] **Step 1: Update `calarbot.yaml.example`**

```yaml
tgTokenFile: /.tgtoken
adminId: 0            # telegram user id, куда notify шлёт сообщения
sqlitePath: /data/calarbot.db
modules:
  simpleReply:
    url: "http://simpleReply:8080"
  skazka:
    url: "http://skazka:8080"
  "sber":
    url: "http://sber:8080"
  "aiAnswer":
    url: "http://aiAnswer:8080"

# Разовый посев: с чего начинается включённость модулей по чатам. Отрабатывает
# один раз, дальше правда живёт в базе и правится в админке. enabled_on больше
# нет — модуль в новом чате всегда выключен, пока его не включат явно.
seed_chats:
  - id: -100500
    title: "болталка"
    type: supergroup
    modules: [skazka, aiAnswer]
```

- [ ] **Step 2: Add the admin service to `docker-compose.example`**

```yaml
  admin:
    build:
      context: .
      dockerfile: admin/Dockerfile
    image: calarbot2-admin:latest
    restart: unless-stopped
    command: ["./admin"]
    environment:
      - ADMIN_PORT=8080
    volumes:
      - /opt/calarbot/calarbot.yaml:/calarbot.yaml:ro
      - /opt/calarbot/tokens/.tgtoken:/.tgtoken:ro
      - /opt/calarbot/data:/data
    # Порт наружу не публикуется: в тайлнет панель выводит sidecar. Как это
    # поднято на боевом хосте — см. роль calarbot2 в ансибле.
    ports:
      - "127.0.0.1:8092:8080"
```

- [ ] **Step 3: Document the panel and the module protocol in `README.md`**

Add a "Web admin" section after "Skazka Module" describing the two screens and
that the panel is reachable only over Tailscale, and replace the "Adding a new
module" list with the current protocol:

```markdown
## Web admin

A separate `admin` service serves a web panel: every chat the bot is in, which
modules are on in each, and each module's per-chat settings. It is published on
the tailnet only. Modules default to **off** in a chat until switched on.

### Adding a new module

1. Create a new directory in the `modules` directory
2. Implement the `BotModule` interface: `Register()`, `IsCalled(*Payload)`, `Answer(*Payload)`
3. Return your settings form from `Register()` — the panel renders it and stores
   the values, and the engine hands them back in `Payload.Extra["settings"]` on
   every call. A module stores nothing itself.
4. Add a build stage to the Dockerfile
5. Add a service to docker-compose.yml
6. Add your module's url to the `modules` map in `calarbot.yaml`

A module with no settings simply returns no `Fields` and gets a bare on/off
toggle in the panel.
```

- [ ] **Step 4: Verify**

Run: `go build ./... && go test ./...`
Expected: PASS (documentation only, but confirm nothing was broken).

- [ ] **Step 5: Commit**

```bash
git add README.md calarbot.yaml.example docker-compose.example
git commit -m "docs: describe the admin panel and the module protocol"
```

---

## After this plan

The Ansible side lives in the `hw.danich.ru` repository and is a separate plan:
role `calarbot2` gains the `admin` and `ts-admin` services, the Tailscale auth
key secret, the `seed_chats` variable in place of the `enabled_on` lists, a
`sqlitePath` in the engine config template, the data directory mounted into the
engine and the panel, and a "Recreate calarbot2 admin" handler.

**The playbook must run before this branch merges to `main`.** CI builds only
the services named in the compose file, and that file is written by Ansible; a
merge that lands first fails the deploy with "no such service: admin".
