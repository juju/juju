// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"sync"

	"github.com/juju/errors"

	"github.com/juju/juju/state"
)

var ErrStateAlreadyClosed = errors.New("state already closed")

// StateTracker wraps a *state.State, closing it when there are no
// longer any references to it.
//
// The Use method will return the wrapped *state.State and increment
// the reference count. The Done method will decrement the reference
// count, closing the State when the reference count hits 0. The
// reference count starts at 1. Done should be called exactly 1 +
// number of calls to Use.
type StateTracker struct {
	mu         sync.Mutex
	st         *state.State
	references int
}

func newStateTracker(st *state.State) *StateTracker {
	return &StateTracker{
		st:         st,
		references: 1,
	}
}

// Use increments the reference count for the wrapped State and
// returns it. ErrStateAlreadyClosed is returned if the State has
// already been closed.
func (c *StateTracker) Use() (*state.State, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.references == 0 {
		return nil, ErrStateAlreadyClosed
	}
	c.references++
	return c.st, nil
}

// Done decrements the reference count for the wrapped State, closing
// it if the reference count becomes 0. ErrStateAlreadyClosed is
// returned if the State has already been closed.
func (c *StateTracker) Done() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.references == 0 {
		return ErrStateAlreadyClosed
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
