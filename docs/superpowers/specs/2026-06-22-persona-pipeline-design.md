# Persona Pipeline for aiAnswer

**Date:** 2026-06-22

## Context

Any request that isn't handled by earlier bot modules falls through to aiAnswer. Currently the pipeline is: route → dispatch → handler, and the character persona is baked into the system prompt passed directly to the LLM. This spec adds a fourth step: after getting a raw answer, pass it through a separate persona-model that rewrites it in character.

## Goals

- Every response with text goes through a persona wrapper (OpenRouter, configurable model)
- Responses without text (generated images) are not wrapped
- The system prompt from config is the single source of truth for the character, used in both the main handler and the persona wrapper
- No new API keys — persona model runs on the existing `openrouter_key`

## Architecture

### New: `ModelGetter` interface

```go
type ModelGetter interface {
    Get() string
}
```

`ModelSelector` already satisfies this. New `StaticModel` struct also satisfies it:

```go
type StaticModel struct{ model string }
func (s StaticModel) Get() string { return s.model }
```

`OpenRouterClient` changes its `sel` field from `*ModelSelector` to `ModelGetter`. No behaviour change for existing callers.

### New: `PersonaClient` (in `models`)

Decorator over `LLMClient`. Fields: `inner LLMClient`, `persona LLMClient`, `sysPrompt string`.

```
Complete(ctx, system, user):
  1. raw, err = inner.Complete(ctx, system, user)
  2. if err → return "", err
  3. styled, err = persona.Complete(ctx, sysPrompt, raw)
  4. if err → log, return raw, nil   ← fallback: persona failure doesn't kill the answer
  5. return styled, nil
```

The persona model receives the character's system prompt as `system` and the raw answer as `user`. Its job: deliver the same information but in character.

### Updated: `VisionHandler`

New optional field `persona LLMClient` + `sysPrompt string`. After `DescribeImage()` succeeds, if `persona != nil`, applies `persona.Complete(ctx, sysPrompt, raw)` with the same fallback behaviour.

### Unchanged

- `ImageGenHandler` — no text output, no wrapping
- `Router` — unchanged
- `TextHandler` — unchanged (receives `PersonaClient` instead of plain `OpenRouterClient`, knows nothing about the wrapping)
- `store`, `botModules`, `engine` — no changes

## Data Flow

```
RouteChat / RouteQuestion / RouteTranslate
  TextHandler → PersonaClient.Complete()
                  └─ inner (OpenRouter, dynamic model) → raw
                  └─ persona (OpenRouter, static persona_model) → styled
  ← RichAnswer{Text: styled}

RouteVision
  VisionHandler.Describe()
    └─ NebiusClient.DescribeImage() → raw
    └─ persona.Complete(ctx, sysPrompt, raw) → styled
  ← RichAnswer{Text: styled}

RouteImageGen
  ImageGenHandler.Generate()
    └─ NebiusClient.GenerateImage() → URL
  ← RichAnswer{PhotoURL: url}   ← no persona wrap
```

## Config Changes

One new field in `AIConfig` and `aiConfig.yaml.example`:

```yaml
persona_model: "openai/gpt-4o-mini"  # OpenRouter model ID for persona wrapping
```

`openrouter_key` is reused — no new credentials.

## Error Handling

| Failure point | Behaviour |
|---|---|
| `inner.Complete()` fails | Return error as before (handler logs, dispatch returns empty or hardcoded message) |
| `persona.Complete()` fails | Log warning, return raw answer as fallback |
| `persona.DescribeImage()` fails in VisionHandler | Same as before (return error up) |
| `persona` field is nil in VisionHandler | Skip wrapping, return raw |

## Testing

- `PersonaClient` unit test: mock inner + mock persona, verify persona receives raw output as user message and sysPrompt as system
- `PersonaClient` fallback test: persona returns error → raw answer returned, no error propagated
- `StaticModel` unit test: trivial
- `VisionHandler` updated tests: inject mock persona, verify styled output; nil persona → raw output
- Integration: existing `TextHandler` tests unaffected (they test against `LLMClient` interface)
