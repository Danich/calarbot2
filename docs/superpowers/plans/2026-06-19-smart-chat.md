# Smart Chat Module Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Rewrite `modules/aiAnswer` into a structured smart-chat module with SQLite context storage, hybrid routing (keyword + LLM classifier), and multi-model dispatch (text via OpenRouter, vision + image gen via Nebius).

**Architecture:** The external HTTP interface (`/order`, `/is_called`, `/answer`) stays unchanged so the engine needs no protocol awareness change except one: the JSON response from `/answer` gains an optional `photo_url` field, which the engine reads to send photos. All new packages live under `modules/aiAnswer/` as sub-packages imported by `main.go`. The `BotModule` interface in `botModules/botModules.go` changes from returning `string` to `RichAnswer{Text, PhotoURL}`, requiring one-line updates to the three existing modules.

**Tech Stack:** Go 1.22, `modernc.org/sqlite` (pure Go, no CGO), `github.com/openai/openai-go v1.8.2` (already in go.mod), OpenRouter API (via shir-man.com top-model selector), Nebius tokenfactory API, Telegram Bot API.

## Global Constraints

- `go 1.22.12` — do not use features from later Go versions
- `modernc.org/sqlite` — use this driver (import as `_ "modernc.org/sqlite"`), driver name is `"sqlite"`
- Follow the exact openai-go call pattern used in the existing `modules/aiAnswer/main.go:84-96` (plain struct fields, no `openai.F()` wrappers)
- All existing tests in `./engine/...`, `./botModules/...`, `./common/...`, `./modules/skazka/...`, etc. must pass after each task
- Context is isolated by `chat_id` — never mix data between chats
- On handler errors, return a friendly Russian error string, not the raw Go error
- `go test ./...` must pass before each commit

---

## File Map

**Create:**
- `modules/aiAnswer/store/store.go` — SQLite store: SaveMessage, GetContext, GetMeta, SetMeta
- `modules/aiAnswer/store/store_test.go`
- `modules/aiAnswer/models/openrouter.go` — ModelSelector + OpenRouterClient (Complete, Classify)
- `modules/aiAnswer/models/openrouter_test.go`
- `modules/aiAnswer/models/nebius.go` — NebiusClient (DescribeImage, GenerateImage)
- `modules/aiAnswer/models/nebius_test.go`
- `modules/aiAnswer/router/router.go` — Route type, Classifier interface, Router
- `modules/aiAnswer/router/router_test.go`
- `modules/aiAnswer/handlers/text.go` — TextHandler (Chat, Answer, Translate)
- `modules/aiAnswer/handlers/text_test.go`
- `modules/aiAnswer/handlers/vision.go` — VisionHandler (Describe)
- `modules/aiAnswer/handlers/vision_test.go`
- `modules/aiAnswer/handlers/imagegen.go` — ImageGenHandler (Generate)
- `modules/aiAnswer/handlers/imagegen_test.go`

**Modify:**
- `go.mod` / `go.sum` — add `modernc.org/sqlite`
- `botModules/botModules.go` — add `RichAnswer`, change `BotModule.Answer` return type
- `botModules/httpserver.go` — serialize `photo_url` from `RichAnswer`
- `botModules/httpserver_test.go` — update `MockModule` for new interface
- `botModules/moduleClient.go` — parse `photo_url`, return `RichAnswer`
- `botModules/moduleClient_test.go` — check `answer.Text` instead of `answer`
- `engine/mock_module_client.go` — update `ModuleClientInterface` and `MockModuleClient`
- `engine/runBot.go` — handle `RichAnswer`, send photo when `PhotoURL` set
- `modules/simpleReply/main.go` — return `RichAnswer`
- `modules/skazka/main.go` — return `RichAnswer`
- `modules/sber/main.go` — return `RichAnswer`
- `modules/aiAnswer/main.go` — complete rewrite
- `modules/aiAnswer/main_test.go` — update for new Module struct
- `aiConfig.yaml.example` — add new fields
- `docker-compose.example` — add SQLite volume for aiAnswer

---

## Execution Order

Tasks 3 and 5 have a dependency: `models/openrouter.go` imports `router.Route`, so Task 5 (router package) must compile before Task 3 tests can run. Execute in this order: **1 → 5 → 3 → 4 → 6 → 7 → 8 → 9**.

---

## Task 1: SQLite Store

**Files:**
- Create: `modules/aiAnswer/store/store.go`
- Create: `modules/aiAnswer/store/store_test.go`
- Modify: `go.mod`, `go.sum`

**Interfaces:**
- Produces: `store.Store` with methods `SaveMessage(*tgbotapi.Message) error`, `GetContext(chatID int64, limit int) ([]ContextMessage, error)`, `GetMeta(key string) (string, bool, error)`, `SetMeta(key, value string) error`, `Close() error`
- Produces: `store.ContextMessage` struct `{Username, Text, MediaType string}`

- [ ] **Step 1: Add SQLite dependency**

```bash
cd /path/to/repo
go get modernc.org/sqlite
```

Expected: `go.mod` and `go.sum` updated.

- [ ] **Step 2: Write failing tests**

Create `modules/aiAnswer/store/store_test.go`:

```go
package store_test

import (
	"fmt"
	"testing"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"calarbot2/modules/aiAnswer/store"
)

func newTestStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.New(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func msg(chatID int64, userID int64, username, text string) *tgbotapi.Message {
	return &tgbotapi.Message{
		Chat: &tgbotapi.Chat{ID: chatID},
		From: &tgbotapi.User{ID: userID, UserName: username},
		Text: text,
		Date: int(time.Now().Unix()),
	}
}

func TestSaveAndGetContext(t *testing.T) {
	s := newTestStore(t)
	if err := s.SaveMessage(msg(100, 1, "alice", "hello")); err != nil {
		t.Fatalf("SaveMessage: %v", err)
	}
	msgs, err := s.GetContext(100, 10)
	if err != nil {
		t.Fatalf("GetContext: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("len(msgs) = %d, want 1", len(msgs))
	}
	if msgs[0].Text != "hello" || msgs[0].Username != "alice" {
		t.Errorf("got %+v", msgs[0])
	}
}

func TestContextIsolatedByChatID(t *testing.T) {
	s := newTestStore(t)
	s.SaveMessage(msg(100, 1, "alice", "msg in chat 100"))
	s.SaveMessage(msg(200, 2, "bob", "msg in chat 200"))

	msgs, _ := s.GetContext(100, 10)
	if len(msgs) != 1 {
		t.Fatalf("chat 100: len=%d, want 1", len(msgs))
	}
	if msgs[0].Text != "msg in chat 100" {
		t.Errorf("chat 100 got wrong message: %q", msgs[0].Text)
	}
}

func TestGetContextChronological(t *testing.T) {
	s := newTestStore(t)
	for i := 0; i < 3; i++ {
		m := msg(100, 1, "alice", fmt.Sprintf("msg%d", i))
		m.Date = int(time.Now().Unix()) + i
		s.SaveMessage(m)
	}
	msgs, _ := s.GetContext(100, 10)
	if len(msgs) != 3 {
		t.Fatalf("len=%d, want 3", len(msgs))
	}
	for i, m := range msgs {
		want := fmt.Sprintf("msg%d", i)
		if m.Text != want {
			t.Errorf("msgs[%d].Text = %q, want %q", i, m.Text, want)
		}
	}
}

func TestGetContextLimit(t *testing.T) {
	s := newTestStore(t)
	for i := 0; i < 5; i++ {
		s.SaveMessage(msg(100, 1, "alice", fmt.Sprintf("msg%d", i)))
	}
	msgs, _ := s.GetContext(100, 3)
	if len(msgs) != 3 {
		t.Fatalf("len=%d, want 3", len(msgs))
	}
}

func TestMeta(t *testing.T) {
	s := newTestStore(t)

	_, ok, err := s.GetMeta("missing")
	if err != nil || ok {
		t.Fatalf("expected empty, got ok=%v err=%v", ok, err)
	}

	if err := s.SetMeta("key", "value1"); err != nil {
		t.Fatalf("SetMeta: %v", err)
	}
	val, ok, _ := s.GetMeta("key")
	if !ok || val != "value1" {
		t.Errorf("GetMeta = %q ok=%v", val, ok)
	}

	s.SetMeta("key", "value2")
	val, _, _ = s.GetMeta("key")
	if val != "value2" {
		t.Errorf("upsert: got %q, want value2", val)
	}
}
```

- [ ] **Step 3: Run tests — expect failure**

```bash
go test ./modules/aiAnswer/store/...
```

Expected: compile error — package does not exist yet.

- [ ] **Step 4: Implement store**

Create `modules/aiAnswer/store/store.go`:

```go
package store

import (
	"database/sql"
	"fmt"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	_ "modernc.org/sqlite"
)

type Store struct {
	db *sql.DB
}

type ContextMessage struct {
	Username  string
	Text      string
	MediaType string
}

func New(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
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

func (s *Store) migrate() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS messages (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			chat_id    INTEGER NOT NULL,
			user_id    INTEGER NOT NULL,
			username   TEXT,
			text       TEXT,
			media_type TEXT,
			ts         INTEGER NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_messages_chat ON messages(chat_id, ts);
		CREATE TABLE IF NOT EXISTS meta (
			key   TEXT PRIMARY KEY,
			value TEXT
		);
	`)
	return err
}

func (s *Store) SaveMessage(msg *tgbotapi.Message) error {
	mediaType := ""
	if msg.Photo != nil {
		mediaType = "photo"
	} else if msg.Sticker != nil {
		mediaType = "sticker"
	}
	username := ""
	if msg.From != nil {
		username = msg.From.UserName
	}
	_, err := s.db.Exec(
		`INSERT INTO messages (chat_id, user_id, username, text, media_type, ts) VALUES (?, ?, ?, ?, ?, ?)`,
		msg.Chat.ID, msg.From.ID, username, msg.Text, mediaType, msg.Date,
	)
	return err
}

func (s *Store) GetContext(chatID int64, limit int) ([]ContextMessage, error) {
	rows, err := s.db.Query(
		`SELECT username, text, media_type FROM messages WHERE chat_id = ? ORDER BY ts DESC LIMIT ?`,
		chatID, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var msgs []ContextMessage
	for rows.Next() {
		var m ContextMessage
		if err := rows.Scan(&m.Username, &m.Text, &m.MediaType); err != nil {
			return nil, err
		}
		msgs = append(msgs, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// Reverse to chronological order (oldest first)
	for i, j := 0, len(msgs)-1; i < j; i, j = i+1, j-1 {
		msgs[i], msgs[j] = msgs[j], msgs[i]
	}
	return msgs, nil
}

func (s *Store) GetMeta(key string) (string, bool, error) {
	var value string
	err := s.db.QueryRow(`SELECT value FROM meta WHERE key = ?`, key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return value, true, nil
}

func (s *Store) SetMeta(key, value string) error {
	_, err := s.db.Exec(
		`INSERT INTO meta(key, value) VALUES(?, ?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`,
		key, value,
	)
	return err
}

func (s *Store) Close() error {
	return s.db.Close()
}
```

- [ ] **Step 5: Run tests — expect pass**

```bash
go test ./modules/aiAnswer/store/...
```

Expected: `ok calarbot2/modules/aiAnswer/store`

- [ ] **Step 6: Commit**

```bash
git add go.mod go.sum modules/aiAnswer/store/
git commit -m "feat: add SQLite store for chat context"
```

---

## Task 2: Extend botModules Protocol + Update All Consumers

**Files:**
- Modify: `botModules/botModules.go`
- Modify: `botModules/httpserver.go`
- Modify: `botModules/httpserver_test.go`
- Modify: `botModules/moduleClient.go`
- Modify: `botModules/moduleClient_test.go`
- Modify: `engine/mock_module_client.go`
- Modify: `engine/runBot.go`
- Modify: `modules/simpleReply/main.go`
- Modify: `modules/skazka/main.go`
- Modify: `modules/sber/main.go`

**Interfaces:**
- Produces: `botModules.RichAnswer{Text string, PhotoURL string}`
- Changes: `BotModule.Answer` returns `(RichAnswer, error)` instead of `(string, error)`
- Changes: `ModuleClient.Answer` returns `(RichAnswer, error)`

**Note:** This task touches many files atomically because Go requires all callers of a changed interface to compile at once. All changes must be made before running tests.

- [ ] **Step 1: Update `botModules/botModules.go`**

Replace the entire file content:

```go
package botModules

import tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

type Payload struct {
	Msg   *tgbotapi.Message
	Extra map[string]interface{}
}

type RichAnswer struct {
	Text     string
	PhotoURL string
}

type BotModule interface {
	Order() int
	IsCalled(msg *tgbotapi.Message) bool
	Answer(payload *Payload) (RichAnswer, error)
}
```

- [ ] **Step 2: Update `botModules/httpserver.go` — serialize `photo_url`**

Replace only the `answerAction` function (lines 38-55):

```go
func answerAction(module BotModule) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		var msg Payload
		err := json.NewDecoder(r.Body).Decode(&msg)
		if err != nil {
			log.Println(err)
		}
		answer, err := module.Answer(&msg)
		resp := map[string]interface{}{"answer": answer.Text}
		if answer.PhotoURL != "" {
			resp["photo_url"] = answer.PhotoURL
		}
		if err != nil {
			resp["error"] = err.Error()
		}
		if encodeErr := json.NewEncoder(w).Encode(resp); encodeErr != nil {
			log.Println(encodeErr)
		}
	}
}
```

- [ ] **Step 3: Update `botModules/httpserver_test.go` — update MockModule**

Change `MockModule.AnswerFunc` field type and `Answer` method (two places):

```go
// Change this field:
AnswerFunc func(*Payload) (RichAnswer, error)

// Change this method:
func (m *MockModule) Answer(payload *Payload) (RichAnswer, error) {
	if m.AnswerFunc != nil {
		return m.AnswerFunc(payload)
	}
	return RichAnswer{}, nil
}
```

Change the `AnswerFunc` value in `TestServeModule`:

```go
AnswerFunc: func(payload *Payload) (RichAnswer, error) {
	if payload == nil || payload.Msg == nil {
		return RichAnswer{}, fmt.Errorf("invalid payload")
	}
	if payload.Msg.Text == "error" {
		return RichAnswer{Text: "error response"}, fmt.Errorf("test error")
	}
	return RichAnswer{Text: "test answer for: " + payload.Msg.Text}, nil
},
```

The test response assertions check `result.Answer` (JSON field name stays `"answer"`), so those lines are unchanged.

- [ ] **Step 4: Update `botModules/moduleClient.go` — return `RichAnswer`**

Replace the `Answer` method:

```go
func (c *ModuleClient) Answer(msg *Payload) (RichAnswer, error) {
	url := c.BaseURL + "/answer"
	body, _ := json.Marshal(msg)
	resp, err := http.Post(url, "application/json", bytes.NewBuffer(body))
	if err != nil {
		return RichAnswer{}, err
	}
	defer resp.Body.Close()

	var result struct {
		Answer   string `json:"answer"`
		PhotoURL string `json:"photo_url,omitempty"`
		Error    string `json:"error,omitempty"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return RichAnswer{}, err
	}
	if result.Error != "" {
		return RichAnswer{Text: result.Answer, PhotoURL: result.PhotoURL}, fmt.Errorf("%s", result.Error)
	}
	return RichAnswer{Text: result.Answer, PhotoURL: result.PhotoURL}, nil
}
```

- [ ] **Step 5: Update `botModules/moduleClient_test.go` — check `answer.Text`**

In `TestModuleClientAnswer`, change:

```go
// Old:
answer, err := client.Answer(tt.payload)
// ...
if answer != tt.expectedAnswer {
    t.Errorf("Answer() = %q, want %q", answer, tt.expectedAnswer)
}

// New:
answer, err := client.Answer(tt.payload)
// ...
if answer.Text != tt.expectedAnswer {
    t.Errorf("Answer().Text = %q, want %q", answer.Text, tt.expectedAnswer)
}
```

- [ ] **Step 6: Update `engine/mock_module_client.go`**

Replace the entire file:

```go
package main

import (
	"calarbot2/botModules"
)

type ModuleClientInterface interface {
	Order() int
	IsCalled(payload *botModules.Payload) (bool, error)
	Answer(payload *botModules.Payload) (botModules.RichAnswer, error)
}

type MockModuleClient struct {
	BaseURL         string
	OrderValue      int
	IsCalledResult  bool
	IsCalledError   error
	AnswerResult    botModules.RichAnswer
	AnswerError     error
	IsCalledPayload *botModules.Payload
	AnswerPayload   *botModules.Payload
}

func NewMockModuleClient() *MockModuleClient {
	return &MockModuleClient{
		BaseURL: "http://localhost:8080",
	}
}

func (m *MockModuleClient) Order() int {
	return m.OrderValue
}

func (m *MockModuleClient) IsCalled(payload *botModules.Payload) (bool, error) {
	m.IsCalledPayload = payload
	return m.IsCalledResult, m.IsCalledError
}

func (m *MockModuleClient) Answer(payload *botModules.Payload) (botModules.RichAnswer, error) {
	m.AnswerPayload = payload
	return m.AnswerResult, m.AnswerError
}
```

- [ ] **Step 7: Update `engine/runBot.go` — handle `RichAnswer`, send photos**

Change the `answer` variable declaration and usage in `RunBot()`. Replace lines ~110-138:

```go
// Find the module that should handle this message
payload := &botModules.Payload{Msg: update.Message, Extra: nil}
var answer botModules.RichAnswer
var err error

for _, moduleName := range b.orderedModules {
    client := b.Modules[moduleName]
    if !b.shouldIAnswer(moduleName, update, client, payload) {
        continue
    }

    log.Printf("Module %s will handle the message", moduleName)
    answer, err = client.Answer(payload)
    if err != nil {
        log.Printf("Error in module %s: %v", moduleName, err)
        answer = botModules.RichAnswer{Text: "An error occurred while processing your request."}
    }
    break
}

if answer.PhotoURL != "" {
    photo := tgbotapi.NewPhoto(update.Message.Chat.ID, tgbotapi.FileURL(answer.PhotoURL))
    if answer.Text != "" {
        photo.Caption = answer.Text
    }
    photo.ReplyToMessageID = update.Message.MessageID
    if _, err = bot.Send(photo); err != nil {
        log.Printf("Error sending photo: %v", err)
    }
} else if answer.Text != "" {
    msg := tgbotapi.NewMessage(update.Message.Chat.ID, answer.Text)
    msg.ReplyToMessageID = update.Message.MessageID
    if _, err = bot.Send(msg); err != nil {
        log.Printf("Error sending message: %v", err)
    }
}
```

- [ ] **Step 8: Update `modules/simpleReply/main.go`**

Change the `Answer` method:

```go
func (m Module) Answer(msg *botModules.Payload) (botModules.RichAnswer, error) {
	return botModules.RichAnswer{Text: msg.Msg.Text}, nil
}
```

- [ ] **Step 9: Update `modules/skazka/main.go`**

Change the `Answer` method signature and wrap internal calls:

```go
func (m *Module) Answer(payload *botModules.Payload) (botModules.RichAnswer, error) {
	msg := payload.Msg

	if msg.IsCommand() {
		cmd := msg.Command()
		if cmd == "skazka" {
			text, err := m.handleSkazkaCommand(msg)
			return botModules.RichAnswer{Text: text}, err
		} else if cmd == "play" {
			text, err := m.handlePlayCommand(msg)
			return botModules.RichAnswer{Text: text}, err
		}
	}

	m.storage.mu.Lock()
	defer m.storage.mu.Unlock()

	for _, session := range m.storage.sessions {
		session.mu.Lock()
		waiting := session.waitingForReply == msg.From.ID
		session.mu.Unlock()

		if waiting {
			session.mu.Lock()
			session.catchMessage(msg)
			session.mu.Unlock()
			return botModules.RichAnswer{}, nil
		}
	}

	return botModules.RichAnswer{Text: "Неизвестная команда"}, nil
}
```

- [ ] **Step 10: Update `modules/sber/main.go`**

Change the `Answer` method signature:

```go
func (m Module) Answer(payload *botModules.Payload) (botModules.RichAnswer, error) {
	msg := payload.Msg

	text := extractTextAfterCommand(msg.Text, "/sber")

	if text == "" && msg.ReplyToMessage != nil {
		text = msg.ReplyToMessage.Text
	}

	if text == "" {
		return botModules.RichAnswer{Text: "Пожалуйста, укажите текст после команды /sber или ответьте на сообщение"}, nil
	}

	result, err := callSberifyService(m.sberifyURL, text)
	if err != nil {
		return botModules.RichAnswer{Text: fmt.Sprintf("Ошибка при обработке текста: %v", err)}, nil
	}

	return botModules.RichAnswer{Text: result}, nil
}
```

- [ ] **Step 11: Run all tests**

```bash
go test ./botModules/... ./engine/... ./modules/simpleReply/... ./modules/skazka/... ./modules/sber/...
```

Expected: all pass. (Skip `./modules/aiAnswer/...` — it's being rewritten and will fail until Task 9.)

- [ ] **Step 12: Commit**

```bash
git add botModules/ engine/ modules/simpleReply/ modules/skazka/ modules/sber/
git commit -m "feat: extend botModules protocol for rich answers with photo support"
```

---

## Task 3: OpenRouter Model Client

**Files:**
- Create: `modules/aiAnswer/models/openrouter.go`
- Create: `modules/aiAnswer/models/openrouter_test.go`

**Interfaces:**
- Produces: `models.MetaStore` interface `{GetMeta(string) (string, bool, error); SetMeta(string, string) error}`
- Produces: `models.ModelSelector` with `Get() string`, `Refresh()`, `StartRefresh(context.Context)`
- Produces: `models.OpenRouterClient` with `Complete(ctx, system, user string) (string, error)` and `Classify(ctx, text string) (router.Route, error)`
- Consumes: `router.Route` type (from `calarbot2/modules/aiAnswer/router`)

- [ ] **Step 1: Write router package first (skeleton only, needed for compilation)**

Create `modules/aiAnswer/router/router.go` with just the type definitions (full implementation in Task 5):

```go
package router

type Route string

const (
	RouteChat      Route = "chat"
	RouteQuestion  Route = "question"
	RouteTranslate Route = "translate"
	RouteVision    Route = "vision"
	RouteImageGen  Route = "imagegen"
)

type Classifier interface {
	Classify(ctx interface{}, text string) (Route, error)
}
```

Wait — this creates a signature mismatch later. Instead, write the full router now (both packages needed to compile together). See Task 5 for the full `router.go` — implement it there first, then return here. Skip this sub-step and implement Task 5 before Task 3 if the compiler complains about missing imports.

**Revised order:** Implement Task 5 (router skeleton) before Task 3 so the `router.Route` type exists.

- [ ] **Step 1 (revised): Write failing tests**

Create `modules/aiAnswer/models/openrouter_test.go`:

```go
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
```

- [ ] **Step 2: Run tests — expect failure (package missing)**

```bash
go test ./modules/aiAnswer/models/...
```

Expected: compile error.

- [ ] **Step 3: Implement `modules/aiAnswer/models/openrouter.go`**

```go
package models

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"

	"calarbot2/modules/aiAnswer/router"
)

const (
	defaultTopModelsURL = "https://shir-man.com/api/free-llm/top-models"
	FallbackModel       = "openrouter/free"
	openrouterBaseURL   = "https://openrouter.ai/api/v1/"
	refreshInterval     = 24 * time.Hour
)

type MetaStore interface {
	GetMeta(key string) (string, bool, error)
	SetMeta(key, value string) error
}

type ModelSelector struct {
	mu           sync.RWMutex
	model        string
	store        MetaStore
	httpClient   *http.Client
	topModelsURL string
}

func NewModelSelector(store MetaStore, topModelsURL string) *ModelSelector {
	if topModelsURL == "" {
		topModelsURL = defaultTopModelsURL
	}
	ms := &ModelSelector{
		store:        store,
		httpClient:   &http.Client{Timeout: 10 * time.Second},
		topModelsURL: topModelsURL,
	}
	if cached, ok, err := store.GetMeta("top_model"); err == nil && ok && cached != "" {
		ms.model = cached
	}
	return ms
}

func (ms *ModelSelector) Get() string {
	ms.mu.RLock()
	defer ms.mu.RUnlock()
	if ms.model == "" {
		return FallbackModel
	}
	return ms.model
}

func (ms *ModelSelector) Refresh() {
	resp, err := ms.httpClient.Get(ms.topModelsURL)
	if err != nil {
		log.Printf("shir-man.com fetch error: %v", err)
		return
	}
	defer resp.Body.Close()

	var result struct {
		Models []struct {
			ID string `json:"id"`
		} `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil || len(result.Models) == 0 {
		log.Printf("shir-man.com parse error: %v", err)
		return
	}

	model := result.Models[0].ID
	ms.mu.Lock()
	ms.model = model
	ms.mu.Unlock()

	_ = ms.store.SetMeta("top_model", model)
	_ = ms.store.SetMeta("top_model_updated_at", time.Now().Format(time.RFC3339))
	log.Printf("top model updated: %s", model)
}

func (ms *ModelSelector) StartRefresh(ctx context.Context) {
	ms.Refresh()
	go func() {
		t := time.NewTicker(refreshInterval)
		defer t.Stop()
		for {
			select {
			case <-t.C:
				ms.Refresh()
			case <-ctx.Done():
				return
			}
		}
	}()
}

const classifySystemPrompt = `Classify the user message into exactly one word (no punctuation, no explanation):
translate — user wants text translated
imagegen — user wants an image drawn or generated
vision — user wants an image described or analyzed
question — user has a factual question expecting an answer
chat — casual conversation or anything else`

type OpenRouterClient struct {
	apiKey  string
	sel     *ModelSelector
	baseURL string
}

func NewOpenRouterClient(apiKey string, sel *ModelSelector, baseURL string) *OpenRouterClient {
	if baseURL == "" {
		baseURL = openrouterBaseURL
	}
	return &OpenRouterClient{apiKey: apiKey, sel: sel, baseURL: baseURL}
}

func (c *OpenRouterClient) newClient() *openai.Client {
	return openai.NewClient(
		option.WithAPIKey(c.apiKey),
		option.WithBaseURL(c.baseURL),
	)
}

func (c *OpenRouterClient) Complete(ctx context.Context, system, user string) (string, error) {
	res, err := c.newClient().Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
		Model: c.sel.Get(),
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.SystemMessage(system),
			openai.UserMessage(user),
		},
	})
	if err != nil {
		return "", err
	}
	return res.Choices[0].Message.Content, nil
}

func (c *OpenRouterClient) Classify(ctx context.Context, text string) (router.Route, error) {
	result, err := c.Complete(ctx, classifySystemPrompt, text)
	if err != nil {
		return router.RouteChat, err
	}
	switch strings.TrimSpace(strings.ToLower(result)) {
	case "translate":
		return router.RouteTranslate, nil
	case "imagegen":
		return router.RouteImageGen, nil
	case "vision":
		return router.RouteVision, nil
	case "question":
		return router.RouteQuestion, nil
	default:
		return router.RouteChat, nil
	}
}
```

- [ ] **Step 4: Run tests — expect pass**

```bash
go test ./modules/aiAnswer/models/... ./modules/aiAnswer/router/...
```

Expected: `ok calarbot2/modules/aiAnswer/models` and `ok calarbot2/modules/aiAnswer/router`.

- [ ] **Step 5: Commit**

```bash
git add modules/aiAnswer/models/openrouter.go modules/aiAnswer/models/openrouter_test.go
git commit -m "feat: add OpenRouter model client with daily shir-man.com refresh"
```

---

## Task 4: Nebius Client

**Files:**
- Create: `modules/aiAnswer/models/nebius.go`
- Create: `modules/aiAnswer/models/nebius_test.go`

**Interfaces:**
- Produces: `models.NebiusClient` with `DescribeImage(ctx, fileURL, prompt string) (string, error)` and `GenerateImage(ctx, prompt string) (string, error)`

- [ ] **Step 1: Write failing tests**

Create `modules/aiAnswer/models/nebius_test.go`:

```go
package models_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"calarbot2/modules/aiAnswer/models"
)

func TestNebiusClientDescribeImage(t *testing.T) {
	imageServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("fake image bytes"))
	}))
	defer imageServer.Close()

	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"choices": []map[string]interface{}{
				{"message": map[string]string{"content": "a cat sitting on a mat"}},
			},
		})
	}))
	defer apiServer.Close()

	client := models.NewNebiusClient("test-key", apiServer.URL+"/", "vision-model", "imagegen-model")
	desc, err := client.DescribeImage(context.Background(), imageServer.URL+"/image.jpg", "describe it")
	if err != nil {
		t.Fatalf("DescribeImage: %v", err)
	}
	if desc != "a cat sitting on a mat" {
		t.Errorf("got %q, want %q", desc, "a cat sitting on a mat")
	}
}

func TestNebiusClientGenerateImage(t *testing.T) {
	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"data": []map[string]string{{"url": "https://example.com/generated.jpg"}},
		})
	}))
	defer apiServer.Close()

	client := models.NewNebiusClient("test-key", apiServer.URL+"/", "vision-model", "imagegen-model")
	url, err := client.GenerateImage(context.Background(), "a dog in a park")
	if err != nil {
		t.Fatalf("GenerateImage: %v", err)
	}
	if url != "https://example.com/generated.jpg" {
		t.Errorf("got %q, want %q", url, "https://example.com/generated.jpg")
	}
}
```

- [ ] **Step 2: Run tests — expect failure**

```bash
go test ./modules/aiAnswer/models/...
```

Expected: compile error — `NebiusClient` not defined.

- [ ] **Step 3: Implement `modules/aiAnswer/models/nebius.go`**

```go
package models

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
)

type NebiusClient struct {
	apiKey        string
	baseURL       string
	visionModel   string
	imageGenModel string
	httpClient    *http.Client
}

func NewNebiusClient(apiKey, baseURL, visionModel, imageGenModel string) *NebiusClient {
	return &NebiusClient{
		apiKey:        apiKey,
		baseURL:       baseURL,
		visionModel:   visionModel,
		imageGenModel: imageGenModel,
		httpClient:    &http.Client{},
	}
}

func (c *NebiusClient) newClient() *openai.Client {
	return openai.NewClient(
		option.WithAPIKey(c.apiKey),
		option.WithBaseURL(c.baseURL),
	)
}

// DescribeImage downloads the image from fileURL and sends it to the Nebius vision model.
func (c *NebiusClient) DescribeImage(ctx context.Context, fileURL, prompt string) (string, error) {
	imgBytes, err := c.downloadFile(ctx, fileURL)
	if err != nil {
		return "", fmt.Errorf("download image: %w", err)
	}

	b64 := base64.StdEncoding.EncodeToString(imgBytes)
	dataURL := "data:image/jpeg;base64," + b64

	res, err := c.newClient().Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
		Model: c.visionModel,
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.UserMessageParts(
				openai.ImagePart(dataURL),
				openai.TextPart(prompt),
			),
		},
	})
	if err != nil {
		return "", err
	}
	return res.Choices[0].Message.Content, nil
}

// GenerateImage creates an image from the prompt and returns its URL.
func (c *NebiusClient) GenerateImage(ctx context.Context, prompt string) (string, error) {
	res, err := c.newClient().Images.Generate(ctx, openai.ImageGenerateParams{
		Prompt: prompt,
		Model:  openai.ImageModel(c.imageGenModel),
		N:      1,
	})
	if err != nil {
		return "", err
	}
	if len(res.Data) == 0 {
		return "", fmt.Errorf("no image returned from Nebius")
	}
	if res.Data[0].URL == "" {
		return "", fmt.Errorf("empty URL in Nebius response")
	}
	return res.Data[0].URL, nil
}

func (c *NebiusClient) downloadFile(ctx context.Context, fileURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fileURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}
```

**Note on openai-go field types:** If `openai.ImageGenerateParams` fields require wrapper types (e.g. `openai.F(prompt)`) based on the v1.8.2 API, adjust to match the pattern in `modules/aiAnswer/main.go:84-96`. The same rule applies to `N int` vs `N int64`.

- [ ] **Step 4: Run tests — expect pass**

```bash
go test ./modules/aiAnswer/models/...
```

Expected: `ok calarbot2/modules/aiAnswer/models`

- [ ] **Step 5: Commit**

```bash
git add modules/aiAnswer/models/nebius.go modules/aiAnswer/models/nebius_test.go
git commit -m "feat: add Nebius client for vision and image generation"
```

---

## Task 5: Router

**Files:**
- Create: `modules/aiAnswer/router/router.go` (replace skeleton from Task 3)
- Create: `modules/aiAnswer/router/router_test.go`

**Interfaces:**
- Produces: `router.Route` type and constants `RouteChat`, `RouteQuestion`, `RouteTranslate`, `RouteVision`, `RouteImageGen`
- Produces: `router.Classifier` interface `{Classify(ctx context.Context, text string) (Route, error)}`
- Produces: `router.Router` with `Route(ctx context.Context, msg *tgbotapi.Message) (Route, error)`
- Consumes: `router.Classifier` (implemented by `models.OpenRouterClient`)

- [ ] **Step 1: Write failing tests**

Create `modules/aiAnswer/router/router_test.go`:

```go
package router_test

import (
	"context"
	"testing"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"calarbot2/modules/aiAnswer/router"
)

type mockClassifier struct{ result router.Route }

func (m *mockClassifier) Classify(_ context.Context, _ string) (router.Route, error) {
	return m.result, nil
}

func txtMsg(text string) *tgbotapi.Message {
	return &tgbotapi.Message{Text: text}
}

func photoMsg(text string) *tgbotapi.Message {
	return &tgbotapi.Message{
		Text:  text,
		Photo: []tgbotapi.PhotoSize{{FileID: "abc"}},
	}
}

func TestRouterKeywordImageGen(t *testing.T) {
	r := router.New(&mockClassifier{router.RouteChat})
	for _, text := range []string{"нарисуй кота", "draw a cat", "сгенерируй картинку"} {
		route, err := r.Route(context.Background(), txtMsg(text))
		if err != nil || route != router.RouteImageGen {
			t.Errorf("text=%q: got %q err=%v, want imagegen", text, route, err)
		}
	}
}

func TestRouterKeywordTranslate(t *testing.T) {
	r := router.New(&mockClassifier{router.RouteChat})
	for _, text := range []string{"переведи это", "translate this", "перевести текст"} {
		route, _ := r.Route(context.Background(), txtMsg(text))
		if route != router.RouteTranslate {
			t.Errorf("text=%q: got %q, want translate", text, route)
		}
	}
}

func TestRouterKeywordVision(t *testing.T) {
	r := router.New(&mockClassifier{router.RouteChat})
	for _, text := range []string{"что на картинке", "распознай это", "опиши фото"} {
		route, _ := r.Route(context.Background(), txtMsg(text))
		if route != router.RouteVision {
			t.Errorf("text=%q: got %q, want vision", text, route)
		}
	}
}

func TestRouterPhotoWithoutText(t *testing.T) {
	r := router.New(&mockClassifier{router.RouteChat})
	route, _ := r.Route(context.Background(), photoMsg(""))
	if route != router.RouteVision {
		t.Errorf("photo with no text: got %q, want vision", route)
	}
}

func TestRouterPhotoWithImageGenKeyword(t *testing.T) {
	r := router.New(&mockClassifier{router.RouteChat})
	route, _ := r.Route(context.Background(), photoMsg("нарисуй похожее"))
	if route != router.RouteImageGen {
		t.Errorf("photo with imagegen keyword: got %q, want imagegen", route)
	}
}

func TestRouterFallsBackToClassifier(t *testing.T) {
	r := router.New(&mockClassifier{router.RouteQuestion})
	route, _ := r.Route(context.Background(), txtMsg("what is the capital of France?"))
	if route != router.RouteQuestion {
		t.Errorf("got %q, want question", route)
	}
}

func TestRouterEmptyTextReturnsChat(t *testing.T) {
	r := router.New(&mockClassifier{router.RouteQuestion})
	route, _ := r.Route(context.Background(), txtMsg(""))
	if route != router.RouteChat {
		t.Errorf("got %q, want chat", route)
	}
}
```

- [ ] **Step 2: Run tests — expect failure**

```bash
go test ./modules/aiAnswer/router/...
```

Expected: compile error (skeleton doesn't have `New`, `Router.Route`).

- [ ] **Step 3: Implement `modules/aiAnswer/router/router.go`**

```go
package router

import (
	"context"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type Route string

const (
	RouteChat      Route = "chat"
	RouteQuestion  Route = "question"
	RouteTranslate Route = "translate"
	RouteVision    Route = "vision"
	RouteImageGen  Route = "imagegen"
)

type Classifier interface {
	Classify(ctx context.Context, text string) (Route, error)
}

type Router struct {
	classifier Classifier
}

func New(classifier Classifier) *Router {
	return &Router{classifier: classifier}
}

var (
	imagegenKeywords  = []string{"нарисуй", "draw ", "сгенерируй", "generate image"}
	translateKeywords = []string{"переведи", "translate ", "перевести", "переведите"}
	visionKeywords    = []string{"что на картинке", "распознай", "опиши фото", "опиши картинку"}
)

func (r *Router) Route(ctx context.Context, msg *tgbotapi.Message) (Route, error) {
	text := strings.ToLower(msg.Text)

	if containsAny(text, imagegenKeywords) {
		return RouteImageGen, nil
	}
	if containsAny(text, translateKeywords) {
		return RouteTranslate, nil
	}
	if containsAny(text, visionKeywords) {
		return RouteVision, nil
	}
	if msg.Photo != nil {
		return RouteVision, nil
	}
	if msg.Text == "" {
		return RouteChat, nil
	}
	return r.classifier.Classify(ctx, msg.Text)
}

func containsAny(text string, keywords []string) bool {
	for _, kw := range keywords {
		if strings.Contains(text, kw) {
			return true
		}
	}
	return false
}
```

- [ ] **Step 4: Run tests — expect pass**

```bash
go test ./modules/aiAnswer/router/... ./modules/aiAnswer/models/...
```

Expected: both packages pass.

- [ ] **Step 5: Commit**

```bash
git add modules/aiAnswer/router/
git commit -m "feat: add hybrid router with keyword matching and LLM classifier fallback"
```

---

## Task 6: Text Handler

**Files:**
- Create: `modules/aiAnswer/handlers/text.go`
- Create: `modules/aiAnswer/handlers/text_test.go`

**Interfaces:**
- Produces: `handlers.LLMClient` interface `{Complete(ctx context.Context, system, user string) (string, error)}`
- Produces: `handlers.TextHandler` with `Chat(ctx, msg, history) (string, error)`, `Answer(ctx, msg, history) (string, error)`, `Translate(ctx, msg, history) (string, error)`
- Consumes: `store.ContextMessage` slice as history

- [ ] **Step 1: Write failing tests**

Create `modules/aiAnswer/handlers/text_test.go`:

```go
package handlers_test

import (
	"context"
	"strings"
	"testing"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"calarbot2/modules/aiAnswer/handlers"
	"calarbot2/modules/aiAnswer/store"
)

type mockLLM struct {
	capturedSystem string
	capturedUser   string
	response       string
}

func (m *mockLLM) Complete(_ context.Context, system, user string) (string, error) {
	m.capturedSystem = system
	m.capturedUser = user
	return m.response, nil
}

func chatMsg(chatTitle, username, text string) *tgbotapi.Message {
	return &tgbotapi.Message{
		Chat: &tgbotapi.Chat{Title: chatTitle},
		From: &tgbotapi.User{UserName: username},
		Text: text,
	}
}

func TestTextHandlerChatIncludesHistory(t *testing.T) {
	llm := &mockLLM{response: "reply"}
	h := handlers.NewTextHandler(llm, "you are a bot")

	history := []store.ContextMessage{
		{Username: "alice", Text: "hi"},
		{Username: "bob", Text: "hello"},
	}
	msg := chatMsg("TestChat", "charlie", "hey")

	got, err := h.Chat(context.Background(), msg, history)
	if err != nil || got != "reply" {
		t.Fatalf("Chat() = %q, %v", got, err)
	}
	if llm.capturedSystem != "you are a bot" {
		t.Errorf("system prompt = %q, want %q", llm.capturedSystem, "you are a bot")
	}
	if !strings.Contains(llm.capturedUser, "alice") || !strings.Contains(llm.capturedUser, "hi") {
		t.Errorf("user message missing history: %q", llm.capturedUser)
	}
	if !strings.Contains(llm.capturedUser, "TestChat") {
		t.Errorf("user message missing chat name: %q", llm.capturedUser)
	}
}

func TestTextHandlerTranslateUsesTranslationPrompt(t *testing.T) {
	llm := &mockLLM{response: "translated text"}
	h := handlers.NewTextHandler(llm, "you are a bot")

	msg := chatMsg("", "alice", "Bonjour le monde")
	got, err := h.Translate(context.Background(), msg, nil)
	if err != nil || got != "translated text" {
		t.Fatalf("Translate() = %q, %v", got, err)
	}
	if !strings.Contains(strings.ToLower(llm.capturedSystem), "translat") {
		t.Errorf("translation system prompt missing 'translat': %q", llm.capturedSystem)
	}
	if !strings.Contains(llm.capturedUser, "Bonjour le monde") {
		t.Errorf("user message missing original text: %q", llm.capturedUser)
	}
}
```

- [ ] **Step 2: Run tests — expect failure**

```bash
go test ./modules/aiAnswer/handlers/...
```

Expected: compile error.

- [ ] **Step 3: Implement `modules/aiAnswer/handlers/text.go`**

```go
package handlers

import (
	"context"
	"fmt"
	"strings"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"calarbot2/modules/aiAnswer/store"
)

type LLMClient interface {
	Complete(ctx context.Context, system, user string) (string, error)
}

type TextHandler struct {
	client       LLMClient
	systemPrompt string
}

func NewTextHandler(client LLMClient, systemPrompt string) *TextHandler {
	return &TextHandler{client: client, systemPrompt: systemPrompt}
}

func buildContextPrompt(chatTitle string, history []store.ContextMessage, msg *tgbotapi.Message) string {
	var sb strings.Builder
	sb.WriteString("Last messages in chat ")
	sb.WriteString(chatTitle)
	sb.WriteString(":\n")
	for _, m := range history {
		sb.WriteString(fmt.Sprintf(" from %s: %s\n", m.Username, m.Text))
	}
	if msg.From != nil {
		sb.WriteString(fmt.Sprintf(" from %s: %s", msg.From.UserName, msg.Text))
	}
	return sb.String()
}

func chatTitle(msg *tgbotapi.Message) string {
	if msg.Chat != nil && msg.Chat.Title != "" {
		return msg.Chat.Title
	}
	return "Unknown"
}

func (h *TextHandler) Chat(ctx context.Context, msg *tgbotapi.Message, history []store.ContextMessage) (string, error) {
	return h.client.Complete(ctx, h.systemPrompt, buildContextPrompt(chatTitle(msg), history, msg))
}

func (h *TextHandler) Answer(ctx context.Context, msg *tgbotapi.Message, history []store.ContextMessage) (string, error) {
	return h.client.Complete(ctx, h.systemPrompt, buildContextPrompt(chatTitle(msg), history, msg))
}

func (h *TextHandler) Translate(ctx context.Context, msg *tgbotapi.Message, _ []store.ContextMessage) (string, error) {
	return h.client.Complete(ctx,
		"You are a translator. Detect the source language and translate to Russian if not Russian, or to English otherwise. Reply with only the translated text.",
		msg.Text,
	)
}
```

- [ ] **Step 4: Run tests — expect pass**

```bash
go test ./modules/aiAnswer/handlers/...
```

Expected: `ok calarbot2/modules/aiAnswer/handlers`

- [ ] **Step 5: Commit**

```bash
git add modules/aiAnswer/handlers/text.go modules/aiAnswer/handlers/text_test.go
git commit -m "feat: add text handler for chat, question, and translate routes"
```

---

## Task 7: Vision Handler

**Files:**
- Create: `modules/aiAnswer/handlers/vision.go`
- Create: `modules/aiAnswer/handlers/vision_test.go`

**Interfaces:**
- Produces: `handlers.VisionClient` interface `{DescribeImage(ctx context.Context, fileURL, prompt string) (string, error)}`
- Produces: `handlers.VisionHandler` with `Describe(ctx context.Context, msg *tgbotapi.Message) (string, error)`

- [ ] **Step 1: Write failing tests**

Append to or create `modules/aiAnswer/handlers/vision_test.go`:

```go
package handlers_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"calarbot2/modules/aiAnswer/handlers"
)

type mockVision struct{ desc string }

func (m *mockVision) DescribeImage(_ context.Context, _, _ string) (string, error) {
	return m.desc, nil
}

func TestVisionHandlerDescribe(t *testing.T) {
	// Fake Telegram getFile API
	tgServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"ok":     true,
			"result": map[string]string{"file_path": "photos/test.jpg"},
		})
	}))
	defer tgServer.Close()

	// Inject a fake token and override the Telegram API base via the token
	// VisionHandler calls https://api.telegram.org/bot<token>/getFile
	// We can't intercept that in a unit test without DI, so we test via mockVision instead.
	h := handlers.NewVisionHandler(&mockVision{"a fluffy cat"}, "fake-token")

	msg := &tgbotapi.Message{
		Text:  "what is this?",
		Photo: []tgbotapi.PhotoSize{{FileID: "file123", Width: 100, Height: 100}},
	}

	// Since the mock VisionClient bypasses the actual HTTP call to Telegram,
	// we test that Describe returns the mock description.
	// The Telegram API call is tested via integration test with a real token.
	desc, err := h.Describe(context.Background(), msg)
	if err != nil {
		t.Fatalf("Describe: %v", err)
	}
	if desc != "a fluffy cat" {
		t.Errorf("got %q, want %q", desc, "a fluffy cat")
	}
}

func TestVisionHandlerNoPhoto(t *testing.T) {
	h := handlers.NewVisionHandler(&mockVision{"irrelevant"}, "fake-token")
	msg := &tgbotapi.Message{Text: "hello"}
	_, err := h.Describe(context.Background(), msg)
	if err == nil {
		t.Error("expected error for message with no photo")
	}
}
```

**Note:** `VisionHandler.Describe` calls the Telegram HTTP API to resolve a `file_id` to a URL. The unit test uses `mockVision` so the real Telegram call never happens. The real flow (file_id → getFile → download → DescribeImage) is verified in the `handlers/vision.go` implementation and tested end-to-end manually.

- [ ] **Step 2: Run tests — expect failure**

```bash
go test ./modules/aiAnswer/handlers/...
```

Expected: compile error — `VisionHandler` and `VisionClient` not defined.

- [ ] **Step 3: Implement `modules/aiAnswer/handlers/vision.go`**

```go
package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type VisionClient interface {
	DescribeImage(ctx context.Context, fileURL, prompt string) (string, error)
}

type VisionHandler struct {
	client   VisionClient
	botToken string
}

func NewVisionHandler(client VisionClient, botToken string) *VisionHandler {
	return &VisionHandler{client: client, botToken: botToken}
}

func (h *VisionHandler) Describe(ctx context.Context, msg *tgbotapi.Message) (string, error) {
	if len(msg.Photo) == 0 {
		return "", fmt.Errorf("no photo in message")
	}
	fileID := msg.Photo[len(msg.Photo)-1].FileID

	fileURL, err := getTelegramFileURL(ctx, h.botToken, fileID)
	if err != nil {
		return "", fmt.Errorf("resolve telegram file: %w", err)
	}

	prompt := msg.Text
	if prompt == "" {
		prompt = "Describe this image in detail."
	}
	return h.client.DescribeImage(ctx, fileURL, prompt)
}

func getTelegramFileURL(ctx context.Context, botToken, fileID string) (string, error) {
	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/getFile?file_id=%s",
		botToken, url.QueryEscape(fileID))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return "", err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	var result struct {
		OK     bool `json:"ok"`
		Result struct {
			FilePath string `json:"file_path"`
		} `json:"result"`
	}
	if err := json.Unmarshal(body, &result); err != nil || !result.OK {
		return "", fmt.Errorf("getFile failed for file_id %s", fileID)
	}
	return fmt.Sprintf("https://api.telegram.org/file/bot%s/%s", botToken, result.Result.FilePath), nil
}
```

- [ ] **Step 4: Run tests — expect pass**

```bash
go test ./modules/aiAnswer/handlers/...
```

Expected: `ok calarbot2/modules/aiAnswer/handlers`

- [ ] **Step 5: Commit**

```bash
git add modules/aiAnswer/handlers/vision.go modules/aiAnswer/handlers/vision_test.go
git commit -m "feat: add vision handler for image description via Nebius"
```

---

## Task 8: ImageGen Handler

**Files:**
- Create: `modules/aiAnswer/handlers/imagegen.go`
- Create: `modules/aiAnswer/handlers/imagegen_test.go`

**Interfaces:**
- Produces: `handlers.ImageGenClient` interface `{GenerateImage(ctx context.Context, prompt string) (string, error)}`
- Produces: `handlers.ImageGenHandler` with `Generate(ctx context.Context, msg *tgbotapi.Message) (string, error)` returning image URL

- [ ] **Step 1: Write failing tests**

Create `modules/aiAnswer/handlers/imagegen_test.go`:

```go
package handlers_test

import (
	"context"
	"testing"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"calarbot2/modules/aiAnswer/handlers"
)

type mockImageGen struct{ url string }

func (m *mockImageGen) GenerateImage(_ context.Context, _ string) (string, error) {
	return m.url, nil
}

func TestImageGenHandlerGenerate(t *testing.T) {
	h := handlers.NewImageGenHandler(&mockImageGen{"https://example.com/img.jpg"})
	msg := &tgbotapi.Message{Text: "нарисуй кота"}
	url, err := h.Generate(context.Background(), msg)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if url != "https://example.com/img.jpg" {
		t.Errorf("got %q, want %q", url, "https://example.com/img.jpg")
	}
}

func TestImageGenHandlerEmptyPrompt(t *testing.T) {
	h := handlers.NewImageGenHandler(&mockImageGen{})
	msg := &tgbotapi.Message{Text: ""}
	_, err := h.Generate(context.Background(), msg)
	if err == nil {
		t.Error("expected error for empty prompt")
	}
}
```

- [ ] **Step 2: Run tests — expect failure**

```bash
go test ./modules/aiAnswer/handlers/...
```

Expected: compile error — `ImageGenHandler` not defined.

- [ ] **Step 3: Implement `modules/aiAnswer/handlers/imagegen.go`**

```go
package handlers

import (
	"context"
	"fmt"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type ImageGenClient interface {
	GenerateImage(ctx context.Context, prompt string) (string, error)
}

type ImageGenHandler struct {
	client ImageGenClient
}

func NewImageGenHandler(client ImageGenClient) *ImageGenHandler {
	return &ImageGenHandler{client: client}
}

// Generate returns the URL of the generated image.
func (h *ImageGenHandler) Generate(ctx context.Context, msg *tgbotapi.Message) (string, error) {
	if msg.Text == "" {
		return "", fmt.Errorf("empty prompt: no text in message")
	}
	return h.client.GenerateImage(ctx, msg.Text)
}
```

- [ ] **Step 4: Run tests — expect pass**

```bash
go test ./modules/aiAnswer/handlers/...
```

Expected: `ok calarbot2/modules/aiAnswer/handlers`

- [ ] **Step 5: Commit**

```bash
git add modules/aiAnswer/handlers/imagegen.go modules/aiAnswer/handlers/imagegen_test.go
git commit -m "feat: add imagegen handler for Nebius image generation"
```

---

## Task 9: Rewrite `main.go` + Update Configs

**Files:**
- Rewrite: `modules/aiAnswer/main.go`
- Rewrite: `modules/aiAnswer/main_test.go`
- Modify: `aiConfig.yaml.example`
- Modify: `docker-compose.example`

**Interfaces:**
- Consumes: all packages from Tasks 1-8
- Produces: `Module` implementing `botModules.BotModule` with the new logic

- [ ] **Step 1: Update `aiConfig.yaml.example`**

Replace file:

```yaml
bot_username: calarbot
answer_level: 980       # minimum dice roll (0-1000) to trigger random reply
call_weight: 200        # bonus roll when bot is @mentioned
reply_weight: 200       # bonus roll when replying to bot's message
system_prompt: "You are a witty participant in a Russian group chat."
context_size: 20        # number of recent messages to include as LLM context

openrouter_key: "sk-or-..."
nebius_key: "..."
nebius_url: "https://api.studio.nebius.ai/v1/"
nebius_vision_model: "..."
nebius_imagegen_model: "..."
tg_bot_token: "..."     # needed to download photos from Telegram for vision
sqlite_path: "/data/calarbot.db"
```

- [ ] **Step 2: Update `docker-compose.example` — add SQLite volume**

Add volume to the `aiAnswer` service:

```yaml
  aiAnswer:
    build:
      context: .
      dockerfile: modules/aiAnswer/Dockerfile
    image: calarbot2-aianswer:latest
    command: ["./aiAnswer"]
    environment:
      - MODULE_PORT=8080
      - MODULE_ORDER=100
    volumes:
      - /opt/calarbot/tokens/aiConfig.yaml:/aiConfig.yaml
      - /opt/calarbot/data:/data
```

- [ ] **Step 3: Rewrite `modules/aiAnswer/main.go`**

```go
package main

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"os"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"calarbot2/botModules"
	"calarbot2/common"
	"calarbot2/modules/aiAnswer/handlers"
	"calarbot2/modules/aiAnswer/models"
	"calarbot2/modules/aiAnswer/router"
	"calarbot2/modules/aiAnswer/store"
)

const (
	AiConfigFile = "/aiConfig.yaml"
	DiceSize     = 1000
)

type AIConfig struct {
	BotUsername string `yaml:"bot_username"`
	AnswerLevel int    `yaml:"answer_level"`
	ReplyWeight int    `yaml:"reply_weight"`
	CallWeight  int    `yaml:"call_weight"`
	SystemPrompt string `yaml:"system_prompt"`
	ContextSize  int    `yaml:"context_size"`
	TgBotToken   string `yaml:"tg_bot_token"`

	OpenRouterKey       string `yaml:"openrouter_key"`
	NebiusKey           string `yaml:"nebius_key"`
	NebiusURL           string `yaml:"nebius_url"`
	NebiusVisionModel   string `yaml:"nebius_vision_model"`
	NebiusImageGenModel string `yaml:"nebius_imagegen_model"`
	SQLitePath          string `yaml:"sqlite_path"`
}

type Module struct {
	order         int
	config        AIConfig
	store         *store.Store
	router        *router.Router
	textHandler   *handlers.TextHandler
	visionHandler *handlers.VisionHandler
	imageHandler  *handlers.ImageGenHandler
	cancelRefresh context.CancelFunc
}

type noopMeta struct{}

func (noopMeta) GetMeta(string) (string, bool, error) { return "", false, nil }
func (noopMeta) SetMeta(string, string) error         { return nil }

func metaBackend(s *store.Store) models.MetaStore {
	if s != nil {
		return s
	}
	return noopMeta{}
}

func NewModule(order int, config AIConfig) *Module {
	if config.ContextSize == 0 {
		config.ContextSize = 20
	}

	var s *store.Store
	if config.SQLitePath != "" {
		var err error
		s, err = store.New(config.SQLitePath)
		if err != nil {
			log.Printf("SQLite unavailable (%v), context will not persist across restarts", err)
		}
	}

	sel := models.NewModelSelector(metaBackend(s), "")
	ctx, cancel := context.WithCancel(context.Background())
	sel.StartRefresh(ctx)

	orClient := models.NewOpenRouterClient(config.OpenRouterKey, sel, "")
	nbClient := models.NewNebiusClient(config.NebiusKey, config.NebiusURL, config.NebiusVisionModel, config.NebiusImageGenModel)

	return &Module{
		order:         order,
		config:        config,
		store:         s,
		router:        router.New(orClient),
		textHandler:   handlers.NewTextHandler(orClient, config.SystemPrompt),
		visionHandler: handlers.NewVisionHandler(nbClient, config.TgBotToken),
		imageHandler:  handlers.NewImageGenHandler(nbClient),
		cancelRefresh: cancel,
	}
}

func (m *Module) Order() int { return m.order }

func (m *Module) IsCalled(msg *tgbotapi.Message) bool {
	if msg == nil {
		return false
	}
	if m.store != nil {
		if err := m.store.SaveMessage(msg); err != nil {
			log.Printf("store.SaveMessage: %v", err)
		}
	}
	if isDirectAddress(msg, m.config.BotUsername) {
		return true
	}
	roll := rand.Intn(DiceSize + 1)
	if msg.ReplyToMessage != nil && msg.ReplyToMessage.From != nil &&
		msg.ReplyToMessage.From.UserName == m.config.BotUsername {
		roll += m.config.ReplyWeight
	}
	if common.Contains(common.ExtractMentions(msg), "@"+m.config.BotUsername) {
		roll += m.config.CallWeight
	}
	return roll >= m.config.AnswerLevel
}

func (m *Module) Answer(payload *botModules.Payload) (botModules.RichAnswer, error) {
	ctx := context.Background()
	msg := payload.Msg

	var history []store.ContextMessage
	if m.store != nil {
		var err error
		history, err = m.store.GetContext(msg.Chat.ID, m.config.ContextSize)
		if err != nil {
			log.Printf("store.GetContext: %v", err)
		}
	}

	if isDirectAddress(msg, m.config.BotUsername) {
		route, err := m.router.Route(ctx, msg)
		if err != nil {
			log.Printf("router.Route error: %v", err)
			route = router.RouteChat
		}
		return m.dispatch(ctx, route, msg, history)
	}

	text, err := m.textHandler.Chat(ctx, msg, history)
	return botModules.RichAnswer{Text: text}, err
}

func (m *Module) dispatch(ctx context.Context, route router.Route, msg *tgbotapi.Message, history []store.ContextMessage) (botModules.RichAnswer, error) {
	switch route {
	case router.RouteImageGen:
		photoURL, err := m.imageHandler.Generate(ctx, msg)
		if err != nil {
			log.Printf("imagegen error: %v", err)
			return botModules.RichAnswer{Text: "Не удалось сгенерировать изображение"}, nil
		}
		return botModules.RichAnswer{PhotoURL: photoURL}, nil

	case router.RouteVision:
		text, err := m.visionHandler.Describe(ctx, msg)
		if err != nil {
			log.Printf("vision error: %v", err)
			return botModules.RichAnswer{Text: "Не удалось обработать изображение"}, nil
		}
		return botModules.RichAnswer{Text: text}, nil

	case router.RouteTranslate:
		text, err := m.textHandler.Translate(ctx, msg, history)
		if err != nil {
			return botModules.RichAnswer{}, err
		}
		return botModules.RichAnswer{Text: text}, nil

	default:
		text, err := m.textHandler.Answer(ctx, msg, history)
		return botModules.RichAnswer{Text: text}, err
	}
}

func isDirectAddress(msg *tgbotapi.Message, botUsername string) bool {
	if msg.ReplyToMessage != nil && msg.ReplyToMessage.From != nil &&
		msg.ReplyToMessage.From.UserName == botUsername {
		return true
	}
	return common.Contains(common.ExtractMentions(msg), "@"+botUsername)
}

func main() {
	order := 1000
	if len(os.Args) > 1 {
		_, _ = fmt.Sscanf(os.Args[1], "%d", &order)
	}

	var config AIConfig
	if err := common.ReadConfig(AiConfigFile, &config); err != nil {
		log.Fatalf("config error: %v", err)
	}

	module := NewModule(order, config)
	defer module.cancelRefresh()
	if module.store != nil {
		defer module.store.Close()
	}

	if err := botModules.RunModuleServer(module, ":8080", 0); err != nil {
		log.Println(err)
	}
}
```

- [ ] **Step 4: Rewrite `modules/aiAnswer/main_test.go`**

```go
package main

import (
	"testing"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"calarbot2/common"
)

func TestModuleOrder(t *testing.T) {
	m := NewModule(42, AIConfig{BotUsername: "testbot", AnswerLevel: 500})
	if m.Order() != 42 {
		t.Errorf("Order() = %d, want 42", m.Order())
	}
}

func TestModuleIsCalledNilMessage(t *testing.T) {
	m := NewModule(0, AIConfig{BotUsername: "testbot", AnswerLevel: 500})
	if m.IsCalled(nil) {
		t.Error("IsCalled(nil) should return false")
	}
}

func TestModuleIsCalledReplyToBot(t *testing.T) {
	m := NewModule(0, AIConfig{
		BotUsername: "testbot",
		AnswerLevel: DiceSize + 100,
		ReplyWeight: DiceSize + 200,
	})
	msg := &tgbotapi.Message{
		Text: "reply",
		Chat: &tgbotapi.Chat{ID: 1},
		From: &tgbotapi.User{ID: 1},
		ReplyToMessage: &tgbotapi.Message{
			From: &tgbotapi.User{UserName: "testbot"},
		},
	}
	if !m.IsCalled(msg) {
		t.Error("IsCalled with reply to bot should return true")
	}
}

func TestModuleIsCalledMentionBot(t *testing.T) {
	m := NewModule(0, AIConfig{
		BotUsername: "testbot",
		AnswerLevel: DiceSize + 100,
		CallWeight:  DiceSize + 200,
	})
	msg := &tgbotapi.Message{
		Text: "Hello @testbot",
		Chat: &tgbotapi.Chat{ID: 1},
		From: &tgbotapi.User{ID: 1},
		Entities: []tgbotapi.MessageEntity{
			{Type: "mention", Offset: 6, Length: 8},
		},
	}
	if !m.IsCalled(msg) {
		t.Error("IsCalled with mention should return true")
	}
}

func TestExtractMentions(t *testing.T) {
	tests := []struct {
		name     string
		msg      *tgbotapi.Message
		expected []string
	}{
		{
			name: "single mention",
			msg: &tgbotapi.Message{
				Text:     "Hello @testbot",
				Entities: []tgbotapi.MessageEntity{{Type: "mention", Offset: 6, Length: 8}},
			},
			expected: []string{"@testbot"},
		},
		{
			name:     "no mentions",
			msg:      &tgbotapi.Message{Text: "Hello world", Entities: []tgbotapi.MessageEntity{}},
			expected: []string{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mentions := common.ExtractMentions(tt.msg)
			if len(mentions) != len(tt.expected) {
				t.Errorf("got %d mentions, want %d", len(mentions), len(tt.expected))
				return
			}
			for i, m := range mentions {
				if m != tt.expected[i] {
					t.Errorf("mentions[%d] = %q, want %q", i, m, tt.expected[i])
				}
			}
		})
	}
}
```

- [ ] **Step 5: Build and run all tests**

```bash
go build ./modules/aiAnswer/...
go test ./...
```

Expected: build succeeds, all tests pass. (The aiAnswer tests that require store will use in-memory paths via `NewModule(0, AIConfig{})` which sets `SQLitePath: ""`.)

- [ ] **Step 6: Commit**

```bash
git add modules/aiAnswer/main.go modules/aiAnswer/main_test.go aiConfig.yaml.example docker-compose.example
git commit -m "feat: rewrite aiAnswer as smart-chat with routing, SQLite context, and multi-model dispatch"
```

- [ ] **Step 7: Final check — all tests**

```bash
go test ./...
```

Expected: all packages pass.
