# Web admin panel for calarbot2

**Date:** 2026-09-02

## Context

Every runtime knob of the bot lives in two YAML files that Ansible templates onto
the host: `calarbot.yaml` (module registry, per-module `enabled_on` chat lists)
and `aiConfig.yaml` (aiAnswer's weights, context size, persona, keys). Changing
which chats a module answers in means editing a role in another repository and
running a playbook.

This spec adds an admin panel: a new container in the same compose project,
reachable only over Tailscale, that manages the channel list and per-channel
module settings. The design reference is `design_guide/calarbot2` (Nocturne
design system, two screens: Channels and Direct messages).

The panel also changes how modules describe themselves, so that a bot can be
built without a stock module or with a third-party one, and the panel adapts
without knowing any module by name.

## Goals

- See every chat the bot is in; per chat, toggle each module and tune its settings.
- See every user who has a DM thread with the bot.
- Leave a channel from the panel.
- Settings apply without recreating containers.
- The panel hardcodes no module name. A module declares its own settings form.
- A newly discovered chat has **every module off** until switched on explicitly.

## Non-goals

- Editing secrets, provider URLs, or model slugs. Those stay in Ansible.
- A global settings screen. Only per-channel settings in this version.
- Creating or editing personas. The panel picks among existing ones.
- Reporting a chat as spam. The Bot API has no such method (`reportPeer` is
  MTProto only), so the button from the design is dropped.
- Pagination. The design leaves it open; lists are small enough for now.
- Backfilling the chat list from history. See "Known gaps".

## Ownership boundary

| Lives in | Owned by | Contents |
|---|---|---|
| `calarbot.yaml`, `aiConfig.yaml` | Ansible | secrets, provider URLs, model slugs, system prompt, module registry (name → url), per-module defaults |
| SQLite (`/data/calarbot.db`) | admin panel | chat list, per-chat module on/off, per-chat module settings |

YAML values are **defaults**. A row in the database is an explicit decision that
overrides its default. Absence of a row means "not touched" and keeps following
the YAML — so changing a default in Ansible still reaches every channel nobody
has overridden.

## Module protocol

`Order()` is replaced by `Register()`: one call at engine startup in which the
module introduces itself and declares its settings.

```go
type Option struct {
    Value string `json:"value"`
    Label string `json:"label"`
}

type Field struct {
    Key         string   `json:"key"`
    Label       string   `json:"label"`
    Description string   `json:"description,omitempty"`
    Type        string   `json:"type"`              // number | bool | select | text
    Min         *int     `json:"min,omitempty"`     // number only
    Max         *int     `json:"max,omitempty"`     // number only
    Options     []Option `json:"options,omitempty"` // select only
    Default     any      `json:"default,omitempty"`
}

type Registration struct {
    Order       int     `json:"order"`
    Label       string  `json:"label"`
    Description string  `json:"description,omitempty"`
    Fields      []Field `json:"fields,omitempty"`
}

type BotModule interface {
    Register() Registration
    IsCalled(payload *Payload) bool
    Answer(payload *Payload) (RichAnswer, error)
}
```

Wire changes in `botModules/`:

- `httpserver.go`: `GET /order` becomes `GET /register`, returning `Registration`.
- `httpserver.go`: `isCalledAction` passes the whole `*Payload` to the module
  instead of `payload.Msg`. Without this, injected settings never reach the
  module.
- `moduleClient.go`: `Order() int` becomes `Register() (Registration, error)`.
  On a transport or decode failure it returns the error and a `Registration`
  with `Order: 9999`, preserving today's "sink to the bottom" fallback.

`Fields` may be empty: a module with no settings is still a valid module, and
the panel renders a bare on/off toggle for it.

`Options` is computed by the module at call time, not fixed in a config — that is
what lets aiAnswer offer the persona list from its own `personas` table.

### Settings injection

The engine resolves each module's settings for the chat and puts them in
`Payload.Extra["settings"]` (a `map[string]any`) on every `/is_called` and
`/answer` call. `Extra` already carries `photo_url`, so nothing new is needed on
the wire.

The map is always complete: the engine overlays explicit database rows on the
`Default` values from the module's own `Registration`. A module therefore never
implements fallback logic, and a module in another language needs no storage and
no database access to have per-chat settings.

Values are coerced by the field's `Type`. A stored value that fails to parse is
logged and the default is used.

## Database

Three new tables, created by a new top-level `settings/` package against the
same file the aiAnswer store already uses.

```sql
CREATE TABLE IF NOT EXISTS chats (
    id         INTEGER PRIMARY KEY,  -- telegram chat id
    type       TEXT NOT NULL,        -- private | group | supergroup | channel
    title      TEXT NOT NULL DEFAULT '',
    username   TEXT NOT NULL DEFAULT '',
    first_seen INTEGER NOT NULL,
    last_seen  INTEGER NOT NULL,
    left_at    INTEGER               -- NULL while the bot is in the chat
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
```

`left_at` rather than `left`: `LEFT` is a reserved word in SQLite.

One generic settings table with a `module` column, rather than a table per
module created from module-supplied names: the effect is the same — a new module
simply produces rows with a new `module` value — without runtime DDL built from
strings that arrived over the network, and without `ALTER TABLE` every time a
module's field set changes.

### Concurrency

Three processes now write to one file: the engine (chat upserts), aiAnswer
(messages, lore), and the panel (settings). `settings.New` opens the database
with `journal_mode(WAL)` and a `busy_timeout`. The same `busy_timeout` is added
to `store.New` in aiAnswer — it is a per-connection setting, not a per-database
one, so the second writer needs it too or it gets `database is locked` instead of
waiting.

### Caching

`settings` keeps resolved values in memory with a 5 second TTL. The engine reads
them for every module on every message, so hitting SQLite each time is wasteful.
Invalidation is by TTL, not by notification: five seconds of lag after a click in
the panel is fine, and cross-process signalling is not worth its cost here. Cache
keys are (chat, module) pairs.

## Resolution and defaults

| Value | Explicit | Otherwise |
|---|---|---|
| module on/off in a chat | row in `chat_modules` | **off** |
| a module's setting | row in `chat_module_settings` | `Default` from that module's `Registration` |

There is no per-module "default enabled" and no `enabled_on` at runtime. A chat
the bot is added to tomorrow gets every module off until someone turns one on.
This is deliberate: the bot sits in chats nobody wants it talking in.

Turning a module off leaves its `chat_module_settings` rows in place. The design
requires that collapsing aiAnswer's settings block not lose entered values, and
the same holds for disabling the module and enabling it again later.

aiAnswer reports its `aiConfig.yaml` values (`answer_level`, `call_weight`,
`reply_weight`, `context_size`, `default_persona`) as the `Default` of its
fields. Today's tuning therefore becomes the default for new channels with no
extra mechanism.

## Seeding

Because every module now defaults to off, and because `skazka` and `sber`
currently have no `enabled_on` and thus run *everywhere*, the current behaviour
cannot be reconstructed automatically: Telegram will not list the chats a bot is
in, and `messages` only holds the three chats aiAnswer was enabled for. A second
problem compounds it — right after the deploy `chats` is empty, so there would be
nothing in the panel to switch on.

Both are solved by an explicit one-time seed, templated by Ansible into
`calarbot.yaml`:

```yaml
# Starting point, derived from today's enabled_on lists plus the fact that
# skazka and sber currently run everywhere. Titles and the final module lists
# are filled in by whoever reviews this before the first run.
seed_chats:
  - id: -386946235
    title: ""
    type: group
    modules: [simpleReply, skazka, sber, aiAnswer]
  - id: -1002463162692
    title: ""
    type: supergroup
    modules: [skazka, sber, aiAnswer]
  - id: -39192805
    title: ""
    type: group
    modules: [skazka, sber, aiAnswer]
```

`type` is required because `chats.type` is `NOT NULL`; an empty value seeds as
`group`. Both it and `title` are corrected on the chat's first real activity.

On startup the engine, if `settings_meta` has no `seeded` key, inserts a `chats`
row for each entry (titles are corrected on first real activity) and an
`enabled = 1` row in `chat_modules` for each listed module, then sets
`seeded = 1`. The list is reviewed by a human: it decides which chats the bot
keeps talking in, and guessing it is not the deploy's job. After the first run
the variable can be deleted from the role.

## The `admin` service

New top-level directory `admin/`, next to `notify/` — like notify it is not a
BotModule, the engine never polls it.

Go, server-rendered `html/template`, plain JavaScript for tab switching, card
expansion and the PATCH calls. CSS and JS are embedded with `embed.FS`; the
Nocturne tokens from the handoff are ported by hand into one stylesheet, icons
are inline SVG taken from the design file. No Node in the image or the pipeline.

### Routes

```
GET   /                                  both tabs, one document
PATCH /api/chats/{id}/modules/{module}   {"enabled": true}
PATCH /api/chats/{id}/settings/{module}  {"answer_level": 990, "persona": "mamkin"}
POST  /api/chats/{id}/leave
GET   /healthz
```

Tab switching is client-side, so both lists are rendered into one document, as
the design requires.

`PATCH .../settings/{module}` accepts a partial object. A field set to `null`
deletes its row and returns the setting to its default. Values are validated
against the module's `Registration` (`type`, `min`, `max`, `options`); anything
outside the declared range is a 400, never a silent clamp.

`POST .../leave` calls Telegram `leaveChat` and sets `left_at`. Lists exclude
rows with a non-NULL `left_at`. Any later update from that chat clears it again,
because the bot is evidently back in.

### Rendering

The module list comes from the `modules:` map in `calarbot.yaml`, mounted
read-only — the same registry the engine uses. For each module the panel calls
`GET /register` directly and live, so the persona list is never stale.

`Registration` is cached per module (it does not depend on the chat), and the
per-chat settings form is only built when a card is expanded — the design has
cards collapsed by default, so a first render costs one call per module, not one
per module per chat.

A module that does not answer `/register` is rendered with its config key as the
label and a message in place of the form. Its toggle still works: enabling and
disabling a module does not require asking the module.

The Direct messages tab lists `chats` with `type = 'private'`.

### Access

The container publishes no port. A `tailscale/tailscale` sidecar joins the
tailnet as `calarbot-admin`, and the admin container runs with
`network_mode: service:ts-admin`. The panel is reachable at
`calarbot-admin.<tailnet>.ts.net:8080` and from nowhere else. The sidecar sits on
the compose network, so `http://aiAnswer:8080` still resolves from inside the
shared namespace.

The tailnet is the security boundary; there is no application-level auth. This
matches `notify`, the only other service in the stack with an exposed port.

The auth key must be reusable and **not** ephemeral: with `TS_STATE_DIR` on a
volume the node survives the key's expiry, while an ephemeral node would vanish
on the first restart.

## Changes to the engine

1. `main.go` opens `settings` and passes it to `Bot`.
2. `InitModules` calls `Register()` instead of `Order()` and keeps each
   `Registration` (order for sorting, fields for resolving defaults). A failed
   registration is logged and the module is ordered last, as today.
3. `shouldIAnswer` replaces the `EnabledOn` check with
   `settings.ModuleEnabled(chatID, module)`.
4. Before each `/is_called`, the engine resolves that module's settings for the
   chat and writes them to `Payload.Extra["settings"]`.
5. Every update upserts `chats` (title, type, username, `last_seen`) and clears
   `left_at`.
6. `my_chat_member` updates are handled. Telegram sends them by default; the
   loop currently only looks at `update.Message`, so without this a channel
   appears only once somebody speaks, and the bot being added or kicked is
   invisible.
7. One-time seed as described above.
8. `CalarbotConfig`: `enabled_on` is removed. `seed_chats` is added.

## Changes to aiAnswer

1. `Register()` returns order, label "AI-ответ", description, and five fields:
   `persona` (select, options from `personas`, default `default_persona`),
   `answer_level`, `call_weight`, `reply_weight` (numbers, 0–1000),
   `context_size` (number, min 0). Defaults are the `aiConfig.yaml` values.
2. `IsCalled(payload)` reads the three weights from `Extra["settings"]` instead
   of `m.config`.
3. `answer()` reads `context_size` from the same place for `GetContext`.
4. Persona arrives as a key string in `Extra["settings"]["persona"]`.
   `ResolvePersona(chatID, defaultKey)` becomes `PersonaByKey(key)`; the
   `chat_persona` table is dropped from `migrate()` (nothing writes to it today,
   so nothing is lost). Lore still hangs on `persona_id`, which now comes from
   the key lookup.
5. `store.New` gains `busy_timeout`.

aiAnswer stores no per-chat settings itself. It has no dependency on the
`settings` package.

## Changes to the other modules

`simpleReply`, `skazka` and `sber` each implement `Register()` — order plus the
label and description from the design ("Простой ответ", "Сказка",
"Сберификатор") and no fields — and take `*Payload` in `IsCalled`. All three
ignore the argument, so the change is mechanical.

## Ansible (`hw.danich.ru`, role `calarbot2`)

No new role: the panel is part of the same compose project.

- `docker-compose.yml.j2` gains `admin` and the `ts-admin` sidecar
  (`NET_ADMIN`, `/dev/net/tun`, a volume for `TS_STATE_DIR`). `admin` mounts
  `calarbot.yaml` read-only, the token, and the data directory read-write, and
  publishes no port.
- New secret `secrets/calarbot2-tailscale-authkey`, wired in `site.yml` like the
  other calarbot2 secrets.
- `calarbot.yaml.j2`: `enabled_on` blocks are removed, a `seed_chats` block is
  added. `calarbot2_simple_reply_chats` and `calarbot2_ai_answer_chats` are
  replaced by `calarbot2_seed_chats` — keeping them would tell a reader of the
  role that they still control something.
- New handler "Recreate calarbot2 admin".
- The `aiConfig.yaml` task also notifies "Recreate calarbot2 engine". The engine
  caches each module's declared defaults from startup, so a changed default in
  `aiConfig.yaml` reaches it only on restart.

### Deploy order

The compose file on the host is written by Ansible, while CI runs
`docker-compose build admin`. Merging to `main` before running the playbook fails
the deploy with "no such service: admin". Run the playbook first, then merge —
the same sequence `notify` needed, as the role's handler comments record.

The `Register()` change is a breaking protocol change: an old module serving
`/order` against a new engine calling `/register` does not work. This is already
handled — `botModules/` is in `REBUILD_EVERYTHING` in
`detect_changed_services.py`, so every service is rebuilt together.

## CI

`detect_changed_services.py` gains two branches: `admin/` maps to the `admin`
service, and `settings/` maps to `{engine, admin}` — the two binaries it is
compiled into. It is deliberately not added to `REBUILD_EVERYTHING`, which would
rebuild all seven services on a 2 GB host for no reason.

## Testing

- `settings`: resolution and default fallback, on/off with no row, the seed
  running exactly once, value coercion per field type, a corrupt stored value
  falling back to its default. Against a temporary database file, as the existing
  `store` tests do.
- `botModules`: `/register` round trip, `Extra` surviving into `IsCalled`, a
  module without `Fields`, a `Registration` decode failure yielding order 9999.
- `admin`: handlers on `httptest` with a fake `settings` and a stub module
  server — validation rejects out-of-range values, `null` deletes a row, leave
  calls Telegram and sets `left_at`. The Telegram call sits behind an interface,
  like `Sender` in `notify`.
- `engine`: `shouldIAnswer` against `chat_modules`, chat upsert, `my_chat_member`
  handling, settings injection into the payload.
- `aiAnswer`: weights and context size read from `Extra`, persona resolved by
  key, defaults reported in `Register()`.

## Known gaps

The chat list fills in as chats become active or as membership events arrive; it
is not reconstructed from history. A backfill from `messages` would produce a
list of bare ids with no titles or types, which is worse than a list that fills
in honestly — and the seed already covers the chats that matter on day one.

The engine's copy of a module's declared defaults is refreshed only on engine
restart, while the panel reads `Register()` live. Between an `aiConfig.yaml`
change and the engine restart the panel can show a default the engine is not yet
injecting. The Ansible handler above closes this for the deploy path; a periodic
re-registration in the engine would close it in general and is not worth its cost
yet.
