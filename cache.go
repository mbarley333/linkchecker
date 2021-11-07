package linkchecker

import "sync"

type ResultCache struct {
	Store  map[string]Result
	Locker sync.Mutex
}
