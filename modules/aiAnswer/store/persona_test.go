package store_test

import (
	"testing"

	"calarbot2/modules/aiAnswer/store"
)

func TestUpsertConfigPersonaReportsCreation(t *testing.T) {
	s := newTestStore(t)
	p, change, err := s.UpsertConfigPersona("mamkin", "mamkin", "you are Mamkin")
	if err != nil {
		t.Fatalf("UpsertConfigPersona: %v", err)
	}
	if change != store.PersonaCreated {
		t.Errorf("change = %v, want PersonaCreated", change)
	}
	if p.ID == 0 || p.Key != "mamkin" {
		t.Errorf("got %+v", p)
	}
}

func TestUpsertConfigPersonaIsQuietWhenNothingChanged(t *testing.T) {
	s := newTestStore(t)
	s.UpsertConfigPersona("mamkin", "mamkin", "you are Mamkin")
	_, change, err := s.UpsertConfigPersona("mamkin", "mamkin", "you are Mamkin")
	if err != nil {
		t.Fatalf("UpsertConfigPersona: %v", err)
	}
	if change != store.PersonaUnchanged {
		t.Errorf("change = %v, want PersonaUnchanged", change)
	}
}

// Переписанный промпт при том же ключе почти всегда значит, что личность
// сменили, а ключ поменять забыли — и лор старого персонажа прирастёт новому.
func TestUpsertConfigPersonaFlagsOverwrittenPrompt(t *testing.T) {
	s := newTestStore(t)
	s.UpsertConfigPersona("mamkin", "mamkin", "you are Mamkin")
	p, change, err := s.UpsertConfigPersona("mamkin", "mamkin", "you are a pigeon")
	if err != nil {
		t.Fatalf("UpsertConfigPersona: %v", err)
	}
	if change != store.PersonaPromptOverwritten {
		t.Errorf("change = %v, want PersonaPromptOverwritten", change)
	}
	if p.SystemPrompt != "you are a pigeon" {
		t.Errorf("prompt = %q, want the new text", p.SystemPrompt)
	}
}

func TestResolvePersonaFallsBackToDefault(t *testing.T) {
	s := newTestStore(t)
	seeded, _, _ := s.UpsertConfigPersona("mamkin", "mamkin", "you are Mamkin")
	got, err := s.ResolvePersona(100, "mamkin")
	if err != nil {
		t.Fatalf("ResolvePersona: %v", err)
	}
	if got.ID != seeded.ID {
		t.Errorf("id = %d, want %d", got.ID, seeded.ID)
	}
}

func TestChatPersonaOverridesDefault(t *testing.T) {
	s := newTestStore(t)
	s.UpsertConfigPersona("mamkin", "mamkin", "you are Mamkin")
	genadiy, _, _ := s.UpsertConfigPersona("genadiy", "genadiy", "you are a pigeon")
	if err := s.SetChatPersona(100, genadiy.ID); err != nil {
		t.Fatalf("SetChatPersona: %v", err)
	}
	got, err := s.ResolvePersona(100, "mamkin")
	if err != nil {
		t.Fatalf("ResolvePersona: %v", err)
	}
	if got.Key != "genadiy" {
		t.Errorf("key = %q, want genadiy", got.Key)
	}
}

func TestResolvePersonaWithoutSeed(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.ResolvePersona(100, "mamkin"); err != store.ErrNoPersona {
		t.Errorf("err = %v, want ErrNoPersona", err)
	}
}

// Граница владения (source) должна быть окончательной: деплой не может перезаписать
// админовскую персону. Если попытается, функция возвращает то, что в базе, и PersonaUnchanged.
func TestUpsertConfigPersonaRespectSourceBoundary(t *testing.T) {
	s := newTestStore(t)
	// Создаём персону через deплой (source = 'config')
	p, _, _ := s.UpsertConfigPersona("mamkin", "mamkin", "you are Mamkin")

	// Симулируем переход в админку: меняем source на 'admin'
	if err := s.SetPersonaSourceForTesting(p.ID, "admin"); err != nil {
		t.Fatalf("SetPersonaSourceForTesting: %v", err)
	}

	// Деплой пытается обновить персону с новым промптом
	got, change, err := s.UpsertConfigPersona("mamkin", "mamkin", "you are a pigeon")
	if err != nil {
		t.Fatalf("UpsertConfigPersona: %v", err)
	}

	// Граница должна быть соблюдена: деплой не переписал
	if change != store.PersonaUnchanged {
		t.Errorf("change = %v, want PersonaUnchanged", change)
	}

	// Функция должна вернуть то, что на самом деле в базе (админский промпт), не то, что деплой хотел
	if got.SystemPrompt != "you are Mamkin" {
		t.Errorf("prompt = %q, want %q (admin's value, not config's)", got.SystemPrompt, "you are Mamkin")
	}
}
