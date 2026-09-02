package settings

import (
	"log"
	"sync"
	"time"
)

// Cache снимает с базы нагрузку от движка: тот спрашивает включённость и
// настройки для каждого модуля на каждое сообщение.
//
// Инвалидация по TTL, а не по сигналу от админки: пять секунд задержки после
// клика — приемлемо, межпроцессная нотификация ради этого — нет.
type Cache struct {
	store *Store
	ttl   time.Duration
	now   func() time.Time

	mu      sync.Mutex
	enabled map[cacheKey]cachedBool
	values  map[cacheKey]cachedValues
}

type cacheKey struct {
	chatID int64
	module string
}

type cachedBool struct {
	v  bool
	at time.Time
}

type cachedValues struct {
	v  map[string]string
	at time.Time
}

func NewCache(s *Store, ttl time.Duration) *Cache {
	return &Cache{
		store:   s,
		ttl:     ttl,
		now:     time.Now,
		enabled: map[cacheKey]cachedBool{},
		values:  map[cacheKey]cachedValues{},
	}
}

// ModuleEnabled при ошибке базы отвечает «выключен»: молчание — безопасный
// отказ, а заговорить в чате, где не должен, бот не имеет права.
func (c *Cache) ModuleEnabled(chatID int64, module string) bool {
	k := cacheKey{chatID, module}

	c.mu.Lock()
	defer c.mu.Unlock()

	if hit, ok := c.enabled[k]; ok && c.now().Sub(hit.at) < c.ttl {
		return hit.v
	}

	v, err := c.store.ModuleEnabled(chatID, module)
	if err != nil {
		log.Printf("settings: module enabled for chat %d, module %s: %v", chatID, module, err)
		return false
	}
	c.enabled[k] = cachedBool{v: v, at: c.now()}
	return v
}

func (c *Cache) Values(chatID int64, module string) map[string]string {
	k := cacheKey{chatID, module}

	c.mu.Lock()
	defer c.mu.Unlock()

	if hit, ok := c.values[k]; ok && c.now().Sub(hit.at) < c.ttl {
		return hit.v
	}

	v, err := c.store.Values(chatID, module)
	if err != nil {
		log.Printf("settings: values for chat %d, module %s: %v", chatID, module, err)
		return map[string]string{}
	}
	c.values[k] = cachedValues{v: v, at: c.now()}
	return v
}
