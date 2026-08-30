package models_test

import (
	"context"
	"errors"
	"strings"
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
	if !strings.HasPrefix(persona.lastSystem, "You are a pirate.") {
		t.Errorf("persona.lastSystem = %q, want it to start with the character prompt", persona.lastSystem)
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

// Без явной инструкции персона-модель видит обычную реплику собеседника и
// отвечает на неё, вместо того чтобы пересказать — бот отвечает сам себе.
func TestPersonaClient_tellsThePersonaToRewrite(t *testing.T) {
	inner := &mockCompleter{response: "raw answer"}
	persona := &mockCompleter{response: "styled answer"}

	c := models.NewPersonaClient(inner, persona, "You are a pirate.")
	if _, err := c.Complete(context.Background(), "original system", "user input"); err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(persona.lastSystem, "Перескажи") {
		t.Errorf("персона-промпт не содержит инструкции пересказать: %q", persona.lastSystem)
	}
	// Сырой ответ идёт как есть: инструкция живёт в system, а не примешивается
	// к тексту, который надо пересказать.
	if persona.lastUser != "raw answer" {
		t.Errorf("persona.lastUser = %q, want %q", persona.lastUser, "raw answer")
	}
}
