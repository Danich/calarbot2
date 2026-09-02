package settings

import (
	"testing"

	"calarbot2/botModules"
)

func numberField(key string, def int) botModules.Field {
	return botModules.Field{Key: key, Type: botModules.FieldNumber, Default: def}
}

func TestResolveFallsBackToDeclaredDefault(t *testing.T) {
	fields := []botModules.Field{numberField("answer_level", 990)}

	got := Resolve(fields, map[string]string{})

	if got["answer_level"] != 990 {
		t.Fatalf("answer_level = %v; want 990", got["answer_level"])
	}
}

func TestResolvePrefersStoredValue(t *testing.T) {
	fields := []botModules.Field{numberField("answer_level", 990)}

	got := Resolve(fields, map[string]string{"answer_level": "700"})

	if got["answer_level"] != 700 {
		t.Fatalf("answer_level = %v; want 700", got["answer_level"])
	}
}

// Мусор в базе не должен ронять бота и не должен молча превращаться в ноль:
// ноль у answer_level значит «отвечать на всё подряд».
func TestResolveFallsBackOnUnparsableValue(t *testing.T) {
	fields := []botModules.Field{numberField("answer_level", 990)}

	got := Resolve(fields, map[string]string{"answer_level": "не число"})

	if got["answer_level"] != 990 {
		t.Fatalf("answer_level = %v; want the default 990", got["answer_level"])
	}
}

func TestResolveHandlesBoolAndText(t *testing.T) {
	fields := []botModules.Field{
		{Key: "loud", Type: botModules.FieldBool, Default: false},
		{Key: "persona", Type: botModules.FieldSelect, Default: "mamkin"},
	}

	got := Resolve(fields, map[string]string{"loud": "true", "persona": "genadiy"})

	if got["loud"] != true {
		t.Errorf("loud = %v; want true", got["loud"])
	}
	if got["persona"] != "genadiy" {
		t.Errorf("persona = %v; want genadiy", got["persona"])
	}
}

// Значение без объявленного поля — мусор от удалённой настройки. Модуль его
// не ждёт, отдавать не за чем.
func TestResolveIgnoresUndeclaredKeys(t *testing.T) {
	fields := []botModules.Field{numberField("answer_level", 990)}

	got := Resolve(fields, map[string]string{"gone": "1"})

	if _, ok := got["gone"]; ok {
		t.Fatalf("Resolve = %v; want no \"gone\" key", got)
	}
}
