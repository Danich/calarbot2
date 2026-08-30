package store

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

var ErrNoPersona = errors.New("persona not found")

type Persona struct {
	ID           int64
	Key          string
	Name         string
	SystemPrompt string
}

// PersonaChange — что случилось с персоной при засеве из конфига.
type PersonaChange int

const (
	PersonaUnchanged PersonaChange = iota
	PersonaCreated
	PersonaPromptOverwritten
)

// UpsertConfigPersona засевает персону, описанную в конфиге, и рассказывает,
// что изменилось.
//
// Лор висит на persona_id, а не на тексте промпта. Поэтому переписанный промпт
// при том же ключе — повод сказать об этом вслух: чаще всего это значит, что
// личность сменили, а ключ поменять забыли, и новый персонаж вот-вот получит
// чужую биографию.
func (s *Store) UpsertConfigPersona(key, name, prompt string) (Persona, PersonaChange, error) {
	var p Persona
	err := s.db.QueryRow(
		`SELECT id, key, name, system_prompt FROM personas WHERE key = ?`, key,
	).Scan(&p.ID, &p.Key, &p.Name, &p.SystemPrompt)

	if errors.Is(err, sql.ErrNoRows) {
		res, insErr := s.db.Exec(
			`INSERT INTO personas (key, name, system_prompt, source, created_at)
			 VALUES (?, ?, ?, 'config', ?)`,
			key, name, prompt, time.Now().Unix(),
		)
		if insErr != nil {
			return Persona{}, PersonaUnchanged, fmt.Errorf("insert persona: %w", insErr)
		}
		id, idErr := res.LastInsertId()
		if idErr != nil {
			return Persona{}, PersonaUnchanged, fmt.Errorf("persona id: %w", idErr)
		}
		return Persona{ID: id, Key: key, Name: name, SystemPrompt: prompt}, PersonaCreated, nil
	}
	if err != nil {
		return Persona{}, PersonaUnchanged, fmt.Errorf("select persona: %w", err)
	}
	if p.SystemPrompt == prompt && p.Name == name {
		return p, PersonaUnchanged, nil
	}
	// source в условии — граница владения: деплой правит только свои строки,
	// админка потом будет править только свои.
	if _, err := s.db.Exec(
		`UPDATE personas SET name = ?, system_prompt = ? WHERE id = ? AND source = 'config'`,
		name, prompt, p.ID,
	); err != nil {
		return Persona{}, PersonaUnchanged, fmt.Errorf("update persona: %w", err)
	}
	p.Name, p.SystemPrompt = name, prompt
	return p, PersonaPromptOverwritten, nil
}

func (s *Store) SetChatPersona(chatID, personaID int64) error {
	_, err := s.db.Exec(
		`INSERT INTO chat_persona (chat_id, persona_id, set_at) VALUES (?, ?, ?)
		 ON CONFLICT(chat_id) DO UPDATE SET persona_id = excluded.persona_id, set_at = excluded.set_at`,
		chatID, personaID, time.Now().Unix(),
	)
	return err
}

// ResolvePersona: явная привязка чата важнее дефолта из конфига. Сегодня
// chat_persona никто не заполняет, но правило уже действует — админка появится
// как писатель в эту таблицу и модуль менять не придётся.
func (s *Store) ResolvePersona(chatID int64, defaultKey string) (Persona, error) {
	var p Persona
	err := s.db.QueryRow(`
		SELECT p.id, p.key, p.name, p.system_prompt
		FROM chat_persona cp JOIN personas p ON p.id = cp.persona_id
		WHERE cp.chat_id = ?`, chatID,
	).Scan(&p.ID, &p.Key, &p.Name, &p.SystemPrompt)
	if err == nil {
		return p, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return Persona{}, fmt.Errorf("select chat persona: %w", err)
	}

	err = s.db.QueryRow(
		`SELECT id, key, name, system_prompt FROM personas WHERE key = ?`, defaultKey,
	).Scan(&p.ID, &p.Key, &p.Name, &p.SystemPrompt)
	if errors.Is(err, sql.ErrNoRows) {
		return Persona{}, ErrNoPersona
	}
	if err != nil {
		return Persona{}, fmt.Errorf("select default persona: %w", err)
	}
	return p, nil
}
