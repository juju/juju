// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"sync"

	"github.com/juju/errors"
	"gopkg.in/juju/names.v2"
)

// NewStatePool returns a new StatePool instance. It takes a State
// connected to the system (controller model).
func NewStatePool(systemState *State) *StatePool {
	return &StatePool{
		systemState: systemState,
		pool:        make(map[string]*PoolItem),
	}
}

// PoolItem holds a State and tracks how many requests are using it
// and whether it's been marked for removal.
type PoolItem struct {
	state      *State
	references uint
	remove     bool
}

// StatePool is a cache of State instances for multiple
// models. Clients should call Put when they have finished with any
// state.
type StatePool struct {
	systemState *State
	// mu protects pool
	mu   sync.Mutex
	pool map[string]*PoolItem
}

// Get returns a State for a given model from the pool, creating one
// if required. If the State has been marked for removal because there
// are outstanding uses, an error will be returned.
func (p *StatePool) Get(modelUUID string) (*State, error) {
	if modelUUID == p.systemState.ModelUUID() {
		return p.systemState, nil
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	item, ok := p.pool[modelUUID]
	if ok && item.remove {
		// We don't want to allow increasing the refcount of a model
		// that's been removed.
		return nil, errors.Errorf("model %v has been removed", modelUUID)
	}
	if ok {
		item.references++
		return item.state, nil
	}

	st, err := p.systemState.ForModel(names.NewModelTag(modelUUID))
	if err != nil {
		return nil, errors.Annotatef(err, "failed to create state for model %v", modelUUID)
	}
	p.pool[modelUUID] = &PoolItem{state: st, references: 1}
	return st, nil
}

// Put indicates that the client has finished using the State. If the
// state has been marked for removal, it will be closed and removed
// when the final Put is done.
func (p *StatePool) Put(modelUUID string) error {
	if modelUUID == p.systemState.ModelUUID() {
		// We don't maintain a refcount for the controller.
		return nil
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	item, ok := p.pool[modelUUID]
	if !ok {
		return errors.Errorf("unable to return unknown model %v to the pool", modelUUID)
	}
	if item.references == 0 {
		return errors.Errorf("state pool refcount for model %v is already 0", modelUUID)
	}
	item.references--
	return p.maybeRemoveItem(modelUUID, item)
}

// Remove takes the state out of the pool and closes it, or marks it
// for removal if it's currently being used (indicated by Gets without
// corresponding Puts).
func (p *StatePool) Remove(modelUUID string) error {
	if modelUUID == p.systemState.ModelUUID() {
		// We don't manage the controller state.
		return nil
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	item, ok := p.pool[modelUUID]
	if !ok {
		// Don't require the client to keep track of what we've seen -
		// ignore unknown model ids.
		return nil
	}
	item.remove = true
	return p.maybeRemoveItem(modelUUID, item)
}

func (p *StatePool) maybeRemoveItem(modelUUID string, item *PoolItem) error {
	if item.remove && item.references == 0 {
		delete(p.pool, modelUUID)
		return item.state.Close()
	}
	return nil
}

// SystemState returns the State passed in to NewStatePool.
func (p *StatePool) SystemState() *State {
	return p.systemState
}

// KillWorkers tells the internal worker for all cached State
// instances in the pool to die.
func (p *StatePool) KillWorkers() {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, item := range p.pool {
		item.state.KillWorkers()
	}
}

// Close closes all State instances in the pool.
func (p *StatePool) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	var lastErr error
	for _, item := range p.pool {
		if item.references != 0 || item.remove {
			logger.Warningf(
				"state for %v leaked from pool - references: %v, removed: %v",
				item.state.ModelUUID(),
				item.references,
				item.remove,
			)
		}
		err := item.state.Close()
		if err != nil {
			lastErr = err
		}
	}
	p.pool = make(map[string]*PoolItem)
	return errors.Annotate(lastErr, "at least one error closing a state")
}
