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
	res, err := s.db.Exec(
		`UPDATE personas SET name = ?, system_prompt = ? WHERE id = ? AND source = 'config'`,
		name, prompt, p.ID,
	)
	if err != nil {
		return Persona{}, PersonaUnchanged, fmt.Errorf("update persona: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return Persona{}, PersonaUnchanged, fmt.Errorf("rows affected: %w", err)
	}
	if affected == 0 {
		// Персона существует, но владеет её кто-то другой (source != 'config').
		// Возвращаем то, что на самом деле в базе.
		return p, PersonaUnchanged, nil
	}
	p.Name, p.SystemPrompt = name, prompt
	return p, PersonaPromptOverwritten, nil
}

// PersonaByKey достаёт персону по ключу.
//
// Пришёл на смену ResolvePersona: ключ теперь приезжает в настройках чата от
// движка, и таблица chat_persona под это больше не нужна.
func (s *Store) PersonaByKey(key string) (Persona, error) {
	var p Persona
	err := s.db.QueryRow(
		`SELECT id, key, name, system_prompt FROM personas WHERE key = ?`, key,
	).Scan(&p.ID, &p.Key, &p.Name, &p.SystemPrompt)
	if errors.Is(err, sql.ErrNoRows) {
		return Persona{}, ErrNoPersona
	}
	if err != nil {
		return Persona{}, fmt.Errorf("select persona: %w", err)
	}
	return p, nil
}

// ListPersonas отдаёт всё, из чего админка строит выпадашку.
func (s *Store) ListPersonas() ([]Persona, error) {
	rows, err := s.db.Query(`SELECT id, key, name, system_prompt FROM personas ORDER BY key`)
	if err != nil {
		return nil, fmt.Errorf("select personas: %w", err)
	}
	defer rows.Close()

	var out []Persona
	for rows.Next() {
		var p Persona
		if err := rows.Scan(&p.ID, &p.Key, &p.Name, &p.SystemPrompt); err != nil {
			return nil, fmt.Errorf("scan persona: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
