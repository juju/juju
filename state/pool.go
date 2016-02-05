// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"sync"

	"github.com/juju/errors"
	"github.com/juju/names"
)

// NewStatePool returns a new StatePool instance. It takes a State
// connected to the system (controller model).
func NewStatePool(systemState *State) *StatePool {
	return &StatePool{
		systemState: systemState,
		pool:        make(map[string]*State),
	}
}

// StatePool is a simple cache of State instances for multiple models.
type StatePool struct {
	systemState *State
	// mu protects pool
	mu   sync.Mutex
	pool map[string]*State
}

// Get returns a State for a given model from the pool, creating
// one if required.
func (p *StatePool) Get(modelUUID string) (*State, error) {
	if modelUUID == p.systemState.ModelUUID() {
		return p.systemState, nil
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	st, ok := p.pool[modelUUID]
	if ok {
		return st, nil
	}

	st, err := p.systemState.ForModel(names.NewModelTag(modelUUID))
	if err != nil {
		return nil, errors.Annotatef(err, "failed to create state for model %v", modelUUID)
	}
	p.pool[modelUUID] = st
	return st, nil
}

// SystemState returns the State passed in to NewStatePool.
func (p *StatePool) SystemState() *State {
	return p.systemState
}

// Close closes all State instances in the pool.
func (p *StatePool) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	var lastErr error
	for _, st := range p.pool {
		err := st.Close()
		if err != nil {
			lastErr = err
		}
	}
	p.pool = make(map[string]*State)
	return errors.Annotate(lastErr, "at least one error closing a state")
}
