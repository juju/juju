// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"sync"

	"github.com/juju/errors"

	"github.com/juju/juju/state"
)

var ErrControllerClosed = errors.New("controller closed")

// Tracker describes a type which wraps and manages the lifetime
// of a *state.Controller.
type Tracker interface {
	// Use returns wrapped Controller, recording the use of
	// it. ErrControllerClosed is returned if the Controller is closed.
	Use() (*state.Controller, error)

	// Done records that there's one less user of the wrapped Controller,
	// closing it if there's no more users. ErrControllerClosed is returned
	// if the Controller has already been closed (indicating that Done has
	// called too many times).
	Done() error
}

// tracker wraps a *state.Controller, keeping a reference count and
// closing it when there are no longer any references. It implements
// Tracker.
//
// The reference count starts at 1. Done should be called exactly 1 +
// number of calls to Use.
type tracker struct {
	mu         sync.Mutex
	st         *state.Controller
	references int
}

func newTracker(st *state.Controller) Tracker {
	return &tracker{
		st:         st,
		references: 1,
	}
}

// Use implements Tracker.
func (c *tracker) Use() (*state.Controller, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.references == 0 {
		return nil, ErrControllerClosed
	}
	c.references++
	return c.st, nil
}

// Done implements Tracker.
func (c *tracker) Done() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.references == 0 {
		return ErrControllerClosed
	}
	c.references--
	if c.references == 0 {
		err := c.st.Close()
		if err != nil {
			logger.Errorf("error when closing controller: %v", err)
		}
	}
	return nil
}
