package settings

import (
	"log"
	"strconv"

	"calarbot2/botModules"
)

// Resolve накладывает явно выставленные значения на дефолты, объявленные самим
// модулем, и всегда возвращает полную карту.
//
// Полную — это несущее: модуль благодаря этому не пишет ни строчки фолбэка, а
// модуль на другом языке вообще ничего не знает ни про базу, ни про конфиг.
func Resolve(fields []botModules.Field, stored map[string]string) map[string]any {
	out := make(map[string]any, len(fields))

	for _, f := range fields {
		raw, ok := stored[f.Key]
		if !ok {
			out[f.Key] = f.Default
			continue
		}

		v, err := coerce(f.Type, raw)
		if err != nil {
			// Дефолт вместо нуля: ноль у веса — это «отвечать всегда», и тихо
			// подставить его вместо испорченного значения было бы хуже всего.
			log.Printf("settings: bad value %q for %s (%s), using default: %v", raw, f.Key, f.Type, err)
			out[f.Key] = f.Default
			continue
		}
		out[f.Key] = v
	}

	return out
}

func coerce(fieldType, raw string) (any, error) {
	switch fieldType {
	case botModules.FieldNumber:
		return strconv.Atoi(raw)
	case botModules.FieldBool:
		return strconv.ParseBool(raw)
	default:
		return raw, nil
	}
}
