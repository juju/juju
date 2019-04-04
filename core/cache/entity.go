package cache

import "sync"

type State uint8

const (
	Stale State = iota
	Active
)

// Entity represents a base entity within the model cache
type Entity struct {
	state State
	mu    sync.Mutex
}

func (e *Entity) mark() {
	e.mu.Lock()
	e.state = Stale
	e.mu.Unlock()
}

func (e *Entity) isStale() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.state == Stale
}
