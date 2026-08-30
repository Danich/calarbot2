# Persona Pipeline Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Wrap every aiAnswer text response through a persona model (OpenRouter, fixed model) that rewrites it in the bot's character voice.

**Architecture:** `PersonaClient` is a decorator implementing the same `Completer` interface as `OpenRouterClient`; it calls the inner client first, then passes the raw answer to a second OpenRouter client with a fixed model. `VisionHandler` gets an optional `persona LLMClient` field; imagegen is not wrapped. `ModelSelector` and the new `StaticModel` both satisfy a new `ModelGetter` interface so `OpenRouterClient` can accept either.

**Tech Stack:** Go, `github.com/openai/openai-go`, existing `openrouter_key` credential.

## Global Constraints

- No new API keys — persona model runs on existing `openrouter_key`
- `persona_model` config field is optional; if empty, no wrapping occurs (graceful degradation)
- Persona failure must never kill the primary answer — always fall back to raw text and log
- Do not push to `main`/`master`; all work goes to a PR
- All existing tests must continue to pass after each task

---

### Task 1: ModelGetter interface + StaticModel

**Files:**
- Modify: `modules/aiAnswer/models/openrouter.go`
- Modify: `modules/aiAnswer/models/openrouter_test.go`

**Interfaces:**
- Produces: `models.ModelGetter` interface `{ Get() string }`, `models.StaticModel` struct, `models.NewStaticModel(model string) StaticModel`
- `models.NewOpenRouterClient` signature changes from `(apiKey string, sel *ModelSelector, baseURL string)` to `(apiKey string, sel ModelGetter, baseURL string)` — callers passing `*ModelSelector` are unaffected (pointer receiver satisfies interface)

---

- [ ] **Step 1: Write failing test for StaticModel**

Add to `modules/aiAnswer/models/openrouter_test.go`:

```go
func TestStaticModelGet(t *testing.T) {
	m := models.NewStaticModel("openai/gpt-4o-mini")
	if m.Get() != "openai/gpt-4o-mini" {
		t.Errorf("Get() = %q, want %q", m.Get(), "openai/gpt-4o-mini")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

```bash
go test ./modules/aiAnswer/models/... -run TestStaticModelGet -v
```

Expected: FAIL — `models.NewStaticModel undefined`

- [ ] **Step 3: Add ModelGetter, StaticModel, update OpenRouterClient**

In `modules/aiAnswer/models/openrouter.go`, after the `const` block, add:

```go
// ModelGetter returns the model ID to use for a completion request.
type ModelGetter interface {
	Get() string
}

// StaticModel is a ModelGetter that always returns the same model ID.
type StaticModel struct{ model string }

func NewStaticModel(model string) StaticModel { return StaticModel{model: model} }

func (s StaticModel) Get() string { return s.model }
```

Change `OpenRouterClient` struct field and constructor:

```go
// Before:
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

// After:
type OpenRouterClient struct {
	apiKey  string
	sel     ModelGetter
	baseURL string
}

func NewOpenRouterClient(apiKey string, sel ModelGetter, baseURL string) *OpenRouterClient {
	if baseURL == "" {
		baseURL = openrouterBaseURL
	}
	return &OpenRouterClient{apiKey: apiKey, sel: sel, baseURL: baseURL}
}
```

No other changes — `*ModelSelector` already has `Get() string` so existing callers compile unchanged.

- [ ] **Step 4: Run all models tests**

```bash
go test ./modules/aiAnswer/models/... -v
```

Expected: all PASS including new `TestStaticModelGet`

- [ ] **Step 5: Run full test suite to confirm nothing broken**

```bash
go test ./... 2>&1 | tail -20
```

Expected: all PASS

- [ ] **Step 6: Commit**

```bash
git add modules/aiAnswer/models/openrouter.go modules/aiAnswer/models/openrouter_test.go
git commit -m "feat(models): add ModelGetter interface and StaticModel"
```

---

### Task 2: PersonaClient

**Files:**
- Create: `modules/aiAnswer/models/persona.go`
- Create: `modules/aiAnswer/models/persona_test.go`

**Interfaces:**
- Consumes: `models.ModelGetter` from Task 1
- Produces: `models.Completer` interface `{ Complete(ctx context.Context, system, user string) (string, error) }`, `models.PersonaClient`, `models.NewPersonaClient(inner, persona Completer, sysPrompt string) *PersonaClient`
- `*PersonaClient` satisfies both `models.Completer` and `handlers.LLMClient` (same method shape, Go structural typing)

---

- [ ] **Step 1: Write failing tests**

Create `modules/aiAnswer/models/persona_test.go`:

```go
package models_test

import (
	"context"
	"errors"
	"testing"

	"calarbot2/modules/aiAnswer/models"
)

type mockCompleter struct {
	response   string
	err        error
	lastSystem string
	lastUser   string
}

func (m *mockCompleter) Complete(_ context.Context, system, user string) (string, error) {
	m.lastSystem = system
	m.lastUser = user
	return m.response, m.err
}

func TestPersonaClient_wrapsRawAnswer(t *testing.T) {
	inner := &mockCompleter{response: "raw answer"}
	persona := &mockCompleter{response: "styled answer"}

	c := models.NewPersonaClient(inner, persona, "You are a pirate.")

	got, err := c.Complete(context.Background(), "original system", "user input")
	if err != nil {
		t.Fatalf("Complete() error: %v", err)
	}
	if got != "styled answer" {
		t.Errorf("got %q, want %q", got, "styled answer")
	}
	// persona receives raw answer as user message
	if persona.lastUser != "raw answer" {
		t.Errorf("persona.lastUser = %q, want %q", persona.lastUser, "raw answer")
	}
	// persona receives sysPrompt (not original system) as system
	if persona.lastSystem != "You are a pirate." {
		t.Errorf("persona.lastSystem = %q, want %q", persona.lastSystem, "You are a pirate.")
	}
}

func TestPersonaClient_fallsBackOnPersonaError(t *testing.T) {
	inner := &mockCompleter{response: "raw answer"}
	persona := &mockCompleter{err: errors.New("persona unavailable")}

	c := models.NewPersonaClient(inner, persona, "sys")

	got, err := c.Complete(context.Background(), "sys", "input")
	if err != nil {
		t.Fatalf("Complete() should not return error on persona failure, got: %v", err)
	}
	if got != "raw answer" {
		t.Errorf("got %q, want raw fallback %q", got, "raw answer")
	}
}

func TestPersonaClient_propagatesInnerError(t *testing.T) {
	inner := &mockCompleter{err: errors.New("inner down")}
	persona := &mockCompleter{response: "styled"}

	c := models.NewPersonaClient(inner, persona, "sys")

	_, err := c.Complete(context.Background(), "sys", "input")
	if err == nil {
		t.Error("expected error when inner client fails")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./modules/aiAnswer/models/... -run TestPersonaClient -v
```

Expected: FAIL — `models.NewPersonaClient undefined`

- [ ] **Step 3: Implement PersonaClient**

Create `modules/aiAnswer/models/persona.go`:

```go
package models

import (
	"context"
	"log"
)

// Completer is satisfied by any LLM client that can complete a prompt.
type Completer interface {
	Complete(ctx context.Context, system, user string) (string, error)
}

// PersonaClient is a Completer decorator: it calls inner to get a raw answer,
// then calls persona to rewrite it in character. If persona fails, raw is returned.
type PersonaClient struct {
	inner     Completer
	persona   Completer
	sysPrompt string
}

func NewPersonaClient(inner, persona Completer, sysPrompt string) *PersonaClient {
	return &PersonaClient{inner: inner, persona: persona, sysPrompt: sysPrompt}
}

func (c *PersonaClient) Complete(ctx context.Context, system, user string) (string, error) {
	raw, err := c.inner.Complete(ctx, system, user)
	if err != nil {
		return "", err
	}
	styled, err := c.persona.Complete(ctx, c.sysPrompt, raw)
	if err != nil {
		log.Printf("persona wrap error: %v", err)
		return raw, nil
	}
	return styled, nil
}
```

- [ ] **Step 4: Run PersonaClient tests**

```bash
go test ./modules/aiAnswer/models/... -run TestPersonaClient -v
```

Expected: all 3 PASS

- [ ] **Step 5: Run full test suite**

```bash
go test ./... 2>&1 | tail -20
```

Expected: all PASS

- [ ] **Step 6: Commit**

```bash
git add modules/aiAnswer/models/persona.go modules/aiAnswer/models/persona_test.go
git commit -m "feat(models): add PersonaClient decorator"
```

---

### Task 3: VisionHandler persona + config + main.go wiring

**Files:**
- Modify: `modules/aiAnswer/handlers/vision.go`
- Modify: `modules/aiAnswer/handlers/vision_test.go`
- Modify: `modules/aiAnswer/main.go`
- Modify: `aiConfig.yaml.example`

**Interfaces:**
- Consumes: `models.NewPersonaClient`, `models.NewStaticModel`, `models.NewOpenRouterClient` from Tasks 1–2
- `handlers.LLMClient` is `interface { Complete(ctx context.Context, system, user string) (string, error) }` — defined in `handlers/text.go`; `*models.PersonaClient` and `*models.OpenRouterClient` both satisfy it automatically

---

- [ ] **Step 1: Write failing VisionHandler tests**

Replace contents of `modules/aiAnswer/handlers/vision_test.go`:

```go
package handlers_test

import (
	"context"
	"errors"
	"testing"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"calarbot2/modules/aiAnswer/handlers"
)

type mockVision struct {
	desc string
	err  error
}

func (m *mockVision) DescribeImage(_ context.Context, _, _ string) (string, error) {
	return m.desc, m.err
}

type mockLLM struct {
	response string
	err      error
	lastUser string
}

func (m *mockLLM) Complete(_ context.Context, _, user string) (string, error) {
	m.lastUser = user
	return m.response, m.err
}

func TestVisionHandler_Describe_noPersona(t *testing.T) {
	h := handlers.NewVisionHandler(&mockVision{desc: "a fluffy cat"}, nil, "")
	msg := &tgbotapi.Message{Caption: "что это?"}

	got, err := h.Describe(context.Background(), msg, "https://cdn.telegram.org/file/photos/test.jpg")
	if err != nil {
		t.Fatalf("Describe: %v", err)
	}
	if got != "a fluffy cat" {
		t.Errorf("got %q, want %q", got, "a fluffy cat")
	}
}

func TestVisionHandler_Describe_withPersona(t *testing.T) {
	persona := &mockLLM{response: "arrr, a fluffy cat it be!"}
	h := handlers.NewVisionHandler(&mockVision{desc: "a fluffy cat"}, persona, "You are a pirate.")

	msg := &tgbotapi.Message{Caption: "что это?"}
	got, err := h.Describe(context.Background(), msg, "https://cdn.telegram.org/file/photos/test.jpg")
	if err != nil {
		t.Fatalf("Describe: %v", err)
	}
	if got != "arrr, a fluffy cat it be!" {
		t.Errorf("got %q, want persona-styled answer", got)
	}
	if persona.lastUser != "a fluffy cat" {
		t.Errorf("persona received user=%q, want raw description", persona.lastUser)
	}
}

func TestVisionHandler_Describe_personaErrorFallback(t *testing.T) {
	persona := &mockLLM{err: errors.New("persona down")}
	h := handlers.NewVisionHandler(&mockVision{desc: "a fluffy cat"}, persona, "sys")

	msg := &tgbotapi.Message{}
	got, err := h.Describe(context.Background(), msg, "https://cdn.telegram.org/file/photos/test.jpg")
	if err != nil {
		t.Fatalf("Describe: %v", err)
	}
	if got != "a fluffy cat" {
		t.Errorf("got %q, want raw fallback on persona error", got)
	}
}

func TestVisionHandler_Describe_noPhotoURL(t *testing.T) {
	h := handlers.NewVisionHandler(&mockVision{desc: "irrelevant"}, nil, "")
	msg := &tgbotapi.Message{Text: "hello"}
	_, err := h.Describe(context.Background(), msg, "")
	if err == nil {
		t.Error("expected error when no photo URL provided")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
go test ./modules/aiAnswer/handlers/... -run TestVisionHandler -v
```

Expected: FAIL — `handlers.NewVisionHandler` wrong number of args

- [ ] **Step 3: Update VisionHandler**

Replace `modules/aiAnswer/handlers/vision.go`:

```go
package handlers

import (
	"context"
	"fmt"
	"log"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

type VisionClient interface {
	DescribeImage(ctx context.Context, fileURL, prompt string) (string, error)
}

type VisionHandler struct {
	client    VisionClient
	persona   LLMClient
	sysPrompt string
}

func NewVisionHandler(client VisionClient, persona LLMClient, sysPrompt string) *VisionHandler {
	return &VisionHandler{client: client, persona: persona, sysPrompt: sysPrompt}
}

func (h *VisionHandler) Describe(ctx context.Context, msg *tgbotapi.Message, photoURL string) (string, error) {
	if photoURL == "" {
		return "", fmt.Errorf("no photo URL provided")
	}
	prompt := msg.Text
	if prompt == "" {
		prompt = msg.Caption
	}
	if prompt == "" {
		prompt = "Describe this image in detail."
	}
	raw, err := h.client.DescribeImage(ctx, photoURL, prompt)
	if err != nil {
		return "", err
	}
	if h.persona == nil {
		return raw, nil
	}
	styled, err := h.persona.Complete(ctx, h.sysPrompt, raw)
	if err != nil {
		log.Printf("vision persona wrap error: %v", err)
		return raw, nil
	}
	return styled, nil
}
```

- [ ] **Step 4: Run VisionHandler tests**

```bash
go test ./modules/aiAnswer/handlers/... -run TestVisionHandler -v
```

Expected: all 4 PASS

- [ ] **Step 5: Update AIConfig and main.go**

In `modules/aiAnswer/main.go`:

Add `PersonaModel` to `AIConfig`:

```go
type AIConfig struct {
	BotUsername  string `yaml:"bot_username"`
	AnswerLevel  int    `yaml:"answer_level"`
	ReplyWeight  int    `yaml:"reply_weight"`
	CallWeight   int    `yaml:"call_weight"`
	SystemPrompt string `yaml:"system_prompt"`
	ContextSize  int    `yaml:"context_size"`

	OpenRouterKey       string `yaml:"openrouter_key"`
	NebiusKey           string `yaml:"nebius_key"`
	NebiusURL           string `yaml:"nebius_url"`
	NebiusVisionModel   string `yaml:"nebius_vision_model"`
	NebiusImageGenModel string `yaml:"nebius_imagegen_model"`
	PersonaModel        string `yaml:"persona_model"`
	SQLitePath          string `yaml:"sqlite_path"`
}
```

Update `NewModule` to wire persona. Replace the section that creates `orClient`, `nbClient`, and the handlers (lines 82–94) with:

```go
	sel := models.NewModelSelector(metaBackend(s), "")
	ctx, cancel := context.WithCancel(context.Background())
	sel.StartRefresh(ctx)

	orClient := models.NewOpenRouterClient(config.OpenRouterKey, sel, "")
	nbClient := models.NewNebiusClient(config.NebiusKey, config.NebiusURL, config.NebiusVisionModel, config.NebiusImageGenModel)

	// textLLM and visionPersona are the OpenRouter client by default;
	// if persona_model is set, wrap text responses in a character persona.
	var textLLM models.Completer = orClient
	var visionPersona handlers.LLMClient
	if config.PersonaModel != "" {
		personaOR := models.NewOpenRouterClient(config.OpenRouterKey, models.NewStaticModel(config.PersonaModel), "")
		textLLM = models.NewPersonaClient(orClient, personaOR, config.SystemPrompt)
		visionPersona = personaOR
	}

	return &Module{
		order:         order,
		config:        config,
		store:         s,
		router:        router.New(orClient),
		textHandler:   handlers.NewTextHandler(textLLM, config.SystemPrompt),
		visionHandler: handlers.NewVisionHandler(nbClient, visionPersona, config.SystemPrompt),
		imageHandler:  handlers.NewImageGenHandler(nbClient),
		cancelRefresh: cancel,
	}
```

Note: `models.Completer` is the interface defined in `persona.go`; `*models.OpenRouterClient` satisfies it. `handlers.NewTextHandler` accepts `handlers.LLMClient` which has the same `Complete` signature — Go structural typing means `*models.PersonaClient` satisfies it without any cast.

- [ ] **Step 6: Update aiConfig.yaml.example**

Add after `nebius_imagegen_model`:

```yaml
persona_model: "openai/gpt-4o-mini"  # OpenRouter model for persona wrapping; leave empty to disable
```

- [ ] **Step 7: Run full test suite**

```bash
go test ./... 2>&1 | tail -30
```

Expected: all PASS

- [ ] **Step 8: Commit**

```bash
git add modules/aiAnswer/handlers/vision.go modules/aiAnswer/handlers/vision_test.go \
        modules/aiAnswer/main.go aiConfig.yaml.example
git commit -m "feat(aiAnswer): persona pipeline — wrap text responses in character voice"
```

- [ ] **Step 9: Open PR**

```bash
gh pr create \
  --title "feat(aiAnswer): persona pipeline for text responses" \
  --body "$(cat <<'EOF'
## Summary

- Adds \`ModelGetter\` interface + \`StaticModel\` to allow OpenRouter client with a fixed model
- Adds \`PersonaClient\` decorator: calls inner LLM, then rewrites answer through a persona model
- Updates \`VisionHandler\` to optionally wrap descriptions through persona
- \`ImageGenHandler\` unchanged (no text output)
- New config field \`persona_model\` (optional; persona disabled when empty)

## Test plan

- [ ] All existing tests pass
- [ ] \`TestPersonaClient_wrapsRawAnswer\` — persona receives raw answer as user message
- [ ] \`TestPersonaClient_fallsBackOnPersonaError\` — raw answer returned on persona failure
- [ ] \`TestVisionHandler_Describe_withPersona\` — vision response styled by persona
- [ ] \`TestVisionHandler_Describe_personaErrorFallback\` — raw description returned on persona error
- [ ] Manual: set \`persona_model: openai/gpt-4o-mini\` in config, send a message, verify character voice in reply

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```
