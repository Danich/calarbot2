package main

import (
	"sort"
	"sync"
	"time"

	"calarbot2/botModules"
)

// Registry спрашивает у каждого модуля, кто он и какие у него настройки.
//
// Кэш на модуль, а не на чат: регистрация от чата не зависит, поэтому отрисовка
// страницы с полусотней карточек стоит одного вызова на модуль. TTL всё же
// короткий — options у select'ов модуль считает на лету, и список персон,
// заведённых без перезапуска, не должен висеть в панели устаревшим.
//
// Провалившуюся регистрацию тоже кэшируем, но на отдельное, более короткое
// окно failTTL: у Register() есть таймаут (см. moduleClient.go), но без кэша
// лежачий модуль всё равно стоил бы по одному зависанию на каждую карточку
// чата при отрисовке страницы, а не по одному на всю страницу.
type Registry struct {
	modules map[string]string
	ttl     time.Duration
	now     func() time.Time

	mu    sync.Mutex
	cache map[string]cachedReg
}

type cachedReg struct {
	reg botModules.Registration
	err error
	at  time.Time
}

// failTTL короче обычного ttl нарочно: модуль, который сейчас не отвечает,
// может отжить в течение секунд, и панель не должна залипать в «не отвечает»
// дольше, чем нужно, чтобы не долбить его при каждой карточке.
const failTTL = 5 * time.Second

func NewRegistry(modules map[string]string, ttl time.Duration) *Registry {
	return &Registry{
		modules: modules,
		ttl:     ttl,
		now:     time.Now,
		cache:   map[string]cachedReg{},
	}
}

func (r *Registry) Get(name string) (botModules.Registration, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if hit, ok := r.cache[name]; ok {
		window := r.ttl
		if hit.err != nil {
			window = failTTL
		}
		if r.now().Sub(hit.at) < window {
			return hit.reg, hit.err
		}
	}

	client := &botModules.ModuleClient{BaseURL: r.modules[name]}
	reg, err := client.Register()
	r.cache[name] = cachedReg{reg: reg, err: err, at: r.now()}
	return reg, err
}

// Names отдаёт все модули из реестра, включая лежачие: их тумблеры работают.
func (r *Registry) Names() []string {
	names := make([]string, 0, len(r.modules))
	for name := range r.modules {
		names = append(names, name)
	}

	order := map[string]int{}
	for _, name := range names {
		reg, _ := r.Get(name)
		order[name] = reg.Order
	}

	sort.Slice(names, func(i, j int) bool {
		if order[names[i]] != order[names[j]] {
			return order[names[i]] < order[names[j]]
		}
		return names[i] < names[j]
	})
	return names
}
