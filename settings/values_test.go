package settings

import "testing"

func TestValuesRoundTripScopedByModule(t *testing.T) {
	s := newTestStore(t)

	if err := s.SetValue(-1, "aiAnswer", "answer_level", "990"); err != nil {
		t.Fatalf("SetValue: %v", err)
	}
	if err := s.SetValue(-1, "other", "answer_level", "1"); err != nil {
		t.Fatalf("SetValue: %v", err)
	}

	got, err := s.Values(-1, "aiAnswer")
	if err != nil {
		t.Fatalf("Values: %v", err)
	}
	if len(got) != 1 || got["answer_level"] != "990" {
		t.Fatalf("Values = %v; want {answer_level: 990}", got)
	}
}

func TestSetValueOverwrites(t *testing.T) {
	s := newTestStore(t)

	if err := s.SetValue(-1, "aiAnswer", "context_size", "10"); err != nil {
		t.Fatalf("SetValue: %v", err)
	}
	if err := s.SetValue(-1, "aiAnswer", "context_size", "25"); err != nil {
		t.Fatalf("SetValue: %v", err)
	}

	got, err := s.Values(-1, "aiAnswer")
	if err != nil {
		t.Fatalf("Values: %v", err)
	}
	if got["context_size"] != "25" {
		t.Fatalf("context_size = %q; want \"25\"", got["context_size"])
	}
}

// Удаление строки — это возврат к дефолту, объявленному модулем, а не запись
// нуля: иначе смена дефолта в конфиге перестала бы доезжать до чата.
func TestDeleteValueReturnsToDefault(t *testing.T) {
	s := newTestStore(t)

	if err := s.SetValue(-1, "aiAnswer", "context_size", "25"); err != nil {
		t.Fatalf("SetValue: %v", err)
	}
	if err := s.DeleteValue(-1, "aiAnswer", "context_size"); err != nil {
		t.Fatalf("DeleteValue: %v", err)
	}

	got, err := s.Values(-1, "aiAnswer")
	if err != nil {
		t.Fatalf("Values: %v", err)
	}
	if _, ok := got["context_size"]; ok {
		t.Fatalf("Values = %v; want no context_size key", got)
	}
}
