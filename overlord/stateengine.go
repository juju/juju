// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package overlord

import (
	"database/sql"
	"sync"
)

// StateManager is implemented by types responsible for observing
// the system and manipulating it to reflect the desired state.
type StateManager interface {
	// Ensure forces a complete evaluation of the current state.
	// See StateEngine.Ensure for more details.
	Ensure() error
}

// StateWaiter is optionally implemented by StateManagers that have running
// activities that can be waited.
type StateWaiter interface {
	// Wait asks manager to wait for all running activities to finish.
	Wait()
}

// StateStopper is optionally implemented by StateManagers that have
// running activities that can be terminated.
type StateStopper interface {
	// Stop asks the manager to terminate all activities running
	// concurrently.  It must not return before these activities
	// are finished.
	Stop()
}

type State interface {
	DB() *sql.DB
}

// StateEngine controls the dispatching of state changes to state managers.
//
// Most of the actual work performed by the state engine is in fact done
// by the individual managers registered. These managers must be able to
// cope with Ensure calls in any order, coordinating among themselves
// solely via the state.
type StateEngine struct {
	state   State
	stopped bool
	// managers in use
	mutex    sync.Mutex
	managers []StateManager
}

// NewStateEngine returns a new state engine.
func NewStateEngine(s State) *StateEngine {
	return &StateEngine{
		state: s,
	}
}

// AddManager adds the provided manager to take part in state operations.
func (se *StateEngine) AddManager(m StateManager) {
	se.mutex.Lock()
	defer se.mutex.Unlock()

	se.managers = append(se.managers, m)
}

// Wait waits for all managers current activities.
func (se *StateEngine) Wait() {
	se.mutex.Lock()
	defer se.mutex.Unlock()

	if se.stopped {
		return
	}
	for _, m := range se.managers {
		if waiter, ok := m.(StateWaiter); ok {
			waiter.Wait()
		}
	}
}

// Stop asks all managers to terminate activities running concurrently.
func (se *StateEngine) Stop() {
	se.mutex.Lock()
	defer se.mutex.Unlock()

	if se.stopped {
		return
	}
	for _, m := range se.managers {
		if stopper, ok := m.(StateStopper); ok {
			stopper.Stop()
		}
	}
	se.stopped = true
}

// State returns the current system state.
func (se *StateEngine) State() State {
	return se.state
}
