package store_test

import (
	"errors"
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

	if _, err := s.PersonaByKey("nobody"); !errors.Is(err, store.ErrNoPersona) {
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
