# Smart Chat Module Design

**Date:** 2026-06-19  
**Status:** Approved  
**Scope:** Rewrite `modules/aiAnswer` into a structured smart-chat module

---

## Problem

The current `aiAnswer` module listens to group chat messages and occasionally rolls a dice to respond using the last 5 messages as context. It has no persistent memory, no ability to detect user intent, and no way to perform specific tasks (translation, image recognition, image generation).

---

## Goal

Replace `aiAnswer` with a structured module that:
- Retains the "random participant" behavior for idle chat
- Detects when a user is explicitly requesting something and always responds
- Routes requests to the appropriate model/handler
- Persists chat context in SQLite across restarts

---

## Module Structure

```
modules/aiAnswer/
  main.go              — entry point, init, HTTP server
  context/
    store.go           — SQLite: read/write message history
  router/
    router.go          — hybrid routing: keywords → LLM classifier
  handlers/
    text.go            — text tasks: chat, question, translate
    vision.go          — image recognition (Nebius vision model)
    imagegen.go        — image generation (Nebius imagegen model)
  models/
    openrouter.go      — OpenRouter client (shir-man top-model)
    nebius.go          — Nebius client (vision + imagegen)
```

External interface is unchanged: `/order`, `/is_called`, `/answer`. The engine and other modules are unaffected.

---

## Trigger Logic

### Random messages (not directed at the bot)
- Every message is saved to SQLite context store
- `IsCalled` rolls dice as before (configurable `answer_level`, `reply_weight`, `call_weight`)
- If rolled: route as `chat`, respond with idle chatter using accumulated context
- If not rolled: stay silent

### Direct messages (mention or reply to bot)
- Always passed to the router
- Router determines intent → appropriate handler responds

---

## Context Storage (SQLite)

### Schema

```sql
CREATE TABLE IF NOT EXISTS messages (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    chat_id    INTEGER NOT NULL,
    user_id    INTEGER NOT NULL,
    username   TEXT,
    text       TEXT,
    media_type TEXT,    -- 'photo', 'sticker', NULL
    ts         INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS meta (
    key   TEXT PRIMARY KEY,
    value TEXT
);
```

- Context is strictly isolated by `chat_id` — no data leaks between chats
- Every incoming message is written to `messages` before routing decisions
- `meta` stores `top_model` and `top_model_updated_at` for model caching

### Fallback
If SQLite is unavailable at startup, the module continues in-memory only (same behavior as current `aiAnswer`). No crash, no silent data loss — just a log warning.

---

## Router

Two levels, evaluated in order:

### Level 1 — Keyword matching (zero tokens)

| Keywords | Route |
|---|---|
| `нарисуй`, `draw`, `сгенерируй` | `imagegen` |
| `переведи`, `translate`, `перевести` | `text:translate` |
| `что на картинке`, `распознай`, `опиши фото` | `vision` |
| No keyword match | → Level 2 |

### Level 2 — LLM classifier

Short prompt to a fast model:
> "Пользователь написал: `<text>`. Классифицируй одним словом: translate / imagegen / vision / question / chat"

Returns one of five labels: `translate`, `imagegen`, `vision`, `question`, `chat`.

Random (non-directed) messages skip both levels and are always routed as `chat`.

---

## Model Selection

### OpenRouter (text tasks: `chat`, `question`, `translate`, LLM classifier)

- Uses [shir-man.com/api/free-llm/top-models](https://shir-man.com/api/free-llm/top-models) to find current top free model
- Model is refreshed once per day via a background goroutine
- **Caching logic:**
  1. On startup: load last saved model from `meta` table (instant, no network)
  2. Background goroutine fetches from shir-man.com; on success, updates memory + `meta`
  3. If shir-man.com is unreachable and a cached model exists: keep cached model
  4. If shir-man.com is unreachable and no cache exists (first run): fall back to `openrouter/free`
- API is OpenAI-compatible; uses existing `openai-go` SDK with custom `BaseURL`

### Nebius (tokenfactory.nebius.ai)

- `vision`: multimodal model — receives photo from Telegram + user text
- `imagegen`: text-to-image — returns URL or base64; bot sends as photo
- Also OpenAI-compatible; same SDK, different `BaseURL` and token

---

## Configuration (`aiConfig.yaml`)

```yaml
bot_username: calarbot
answer_level: 980       # minimum dice roll (0-1000) to trigger random reply
call_weight: 200        # bonus added to roll when bot is @mentioned
reply_weight: 200       # bonus added to roll when replying to bot's message
system_prompt: "..."    # personality prompt sent to LLM on every request
context_size: 20        # how many recent messages to pull from SQLite as LLM context

openrouter_key: "..."
nebius_key: "..."
nebius_url: "https://api.studio.nebius.ai/v1/"
nebius_vision_model: "..."
nebius_imagegen_model: "..."
sqlite_path: "/data/calarbot.db"
```

SQLite file is mounted into the container via `docker-compose` volume.

---

## Error Handling

| Failure | Behavior |
|---|---|
| LLM returns error | Log, bot stays silent (no garbage to user) |
| Nebius vision fails | Reply: "Не удалось обработать изображение" |
| Nebius imagegen fails | Reply: "Не удалось сгенерировать изображение" |
| shir-man.com unreachable | Keep cached model; fall back to `openrouter/free` only on first run |
| SQLite unavailable | In-memory only, log warning, no crash |

---

## Testing

- `context/store_test.go` — SQLite CRUD, chat_id isolation, meta read/write
- `router/router_test.go` — keyword matching, LLM classifier (mocked LLM client)
- `main_test.go` — integration: `IsCalled` / `Answer` end-to-end (extends existing test)
