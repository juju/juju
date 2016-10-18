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
// of a *state.State.
type StateTracker interface {
	// Use returns wrapped State, recording the use of
	// it. ErrStateClosed is returned if the State is closed.
	Use() (*state.State, error)

	// Done records that there's one less user of the wrapped State,
	// closing it if there's no more users. ErrStateClosed is returned
	// if the State has already been closed (indicating that Done has
	// called too many times).
	Done() error
}

// stateTracker wraps a *state.State, keeping a reference count and
// closing the State when there are no longer any references to it. It
// implements StateTracker.
//
// The reference count starts at 1. Done should be called exactly 1 +
// number of calls to Use.
type stateTracker struct {
	mu         sync.Mutex
	st         *state.State
	references int
}

func newStateTracker(st *state.State) StateTracker {
	return &stateTracker{
		st:         st,
		references: 1,
	}
}

// Use implements StateTracker.
func (c *stateTracker) Use() (*state.State, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.references == 0 {
		return nil, ErrStateClosed
	}
	c.references++
	return c.st, nil
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
		err := c.st.Close()
		if err != nil {
			logger.Errorf("error when closing state: %v", err)
		}
	}
	return nil
}
