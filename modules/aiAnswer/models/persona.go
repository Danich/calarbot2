package models

import (
	"context"
	"log"
)

// Completer is satisfied by any LLM client that can complete a prompt.
type Completer interface {
	Complete(ctx context.Context, system, user string) (string, error)
}

// RewriteInstruction is appended to the character prompt for the persona pass.
//
// Without it the persona model sees an ordinary chat turn — a character prompt
// plus someone's message — and replies to the raw answer instead of restyling
// it, so the bot answers its own answer. The deployed character prompt makes
// that worse: it says in so many words to reply to the last message.
const RewriteInstruction = `

Сейчас тебе дают готовый ответ. Перескажи его своим голосом, сохранив все факты
и смысл. Не отвечай на этот текст и не добавляй ничего от себя — только
перескажи его в образе. Верни один лишь пересказ, без пояснений.`

// PersonaClient is a Completer decorator: it calls inner to get a raw answer,
// then calls persona to rewrite it in character. If persona fails, raw is returned.
//
// Сейчас никем не используется, и это осознанно. Болталке он оказался не нужен:
// одна модель отвечает сразу в образе, а два круга давали ответ вдвое
// характернее нужного и стоили двух запросов. Но он остаётся под маршруты, где
// точность и голос — разные задачи: перевод разумно доверить модели, которая
// переводит хорошо, а окрасить отдельно. Vision уже устроен ровно так, только
// вручную, без этого декоратора.
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
	styled, err := c.persona.Complete(ctx, c.sysPrompt+RewriteInstruction, raw)
	if err != nil {
		log.Printf("persona wrap error: %v", err)
		return raw, nil
	}
	return styled, nil
}
