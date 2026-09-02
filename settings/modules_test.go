package settings

import "testing"

// Молчание по умолчанию — несущее правило: бот сидит в чатах, где говорить
// не должен, и новый чат не повод начинать.
func TestModuleDisabledWithoutRow(t *testing.T) {
	s := newTestStore(t)

	enabled, err := s.ModuleEnabled(-1, "aiAnswer")
	if err != nil {
		t.Fatalf("ModuleEnabled: %v", err)
	}
	if enabled {
		t.Error("ModuleEnabled without a row = true; want false")
	}
}

func TestSetModuleEnabledRoundTrip(t *testing.T) {
	s := newTestStore(t)

	if err := s.SetModuleEnabled(-1, "aiAnswer", true); err != nil {
		t.Fatalf("SetModuleEnabled: %v", err)
	}
	if enabled, err := s.ModuleEnabled(-1, "aiAnswer"); err != nil || !enabled {
		t.Fatalf("ModuleEnabled = %v, %v; want true, nil", enabled, err)
	}

	if err := s.SetModuleEnabled(-1, "aiAnswer", false); err != nil {
		t.Fatalf("SetModuleEnabled: %v", err)
	}
	if enabled, err := s.ModuleEnabled(-1, "aiAnswer"); err != nil || enabled {
		t.Fatalf("ModuleEnabled after disabling = %v, %v; want false, nil", enabled, err)
	}
}

func TestModuleEnabledIsScopedToItsChat(t *testing.T) {
	s := newTestStore(t)

	if err := s.SetModuleEnabled(-1, "skazka", true); err != nil {
		t.Fatalf("SetModuleEnabled: %v", err)
	}

	if enabled, _ := s.ModuleEnabled(-2, "skazka"); enabled {
		t.Error("ModuleEnabled(-2, skazka) = true; включённость не должна течь между чатами")
	}
	if enabled, _ := s.ModuleEnabled(-1, "sber"); enabled {
		t.Error("ModuleEnabled(-1, sber) = true; и между модулями тоже")
	}
}
