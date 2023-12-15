// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"sync"

	"github.com/juju/errors"

	"github.com/juju/juju/state"
)

var ErrStateClosed = errors.New("state closed")

// StateTracker describes a type which wraps and manages the lifetime
// of a *state.State and associated *state.StatePool.
type StateTracker interface {
	// Use returns the wrapped StatePool, recording the use of
	// it, and the system state. ErrStateClosed is returned if the StatePool is closed.
	Use() (*state.StatePool, *state.State, error)

	// Done records that there's one less user of the wrapped StatePool,
	// closing it if there's no more users. ErrStateClosed is returned
	// if the StatePool has already been closed (indicating that Done has
	// called too many times).
	Done() error

	// Report is used to give details about what is going on with this state tracker.
	Report() map[string]interface{}
}

// stateTracker wraps a *state.State, keeping a reference count and
// closing the State and associated *state.StatePool when there are
// no longer any references. It implements StateTracker.
//
// The reference count starts at 1. Done should be called exactly 1 +
// number of calls to Use.
type stateTracker struct {
	mu         sync.Mutex
	pool       *state.StatePool
	references int
}

func newStateTracker(pool *state.StatePool) StateTracker {
	return &stateTracker{
		pool:       pool,
		references: 1,
	}
}

// Use implements StateTracker.
func (c *stateTracker) Use() (*state.StatePool, *state.State, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.references == 0 {
		return nil, nil, ErrStateClosed
	}
	c.references++
	systemState, err := c.pool.SystemState()
	return c.pool, systemState, err
}

// Done implements StateTracker.
func (c *stateTracker) Done() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.references == 0 {
		return ErrStateClosed
	}
	c.references--
	if c.references == 0 {
		if err := c.pool.Close(); err != nil {
			logger.Errorf("error when closing state pool: %v", err)
		}
	}
	return nil
}

func (c *stateTracker) Report() map[string]interface{} {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.pool == nil {
		return nil
	}
	return c.pool.Report()
}
