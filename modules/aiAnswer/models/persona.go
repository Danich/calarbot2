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
