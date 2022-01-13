// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package overlord

import "github.com/juju/juju/overlord/logstate"

// Overlord is the central manager of the system, keeping track
// of all available state managers and related helpers.
type Overlord struct {
	stateEng *StateEngine

	// Managers
	logMgr LogManager
}

// New creates a new Overlord with all its state managers.
// It can be provided with an optional restart.Handler.
func New(s State) (*Overlord, error) {
	o := &Overlord{}

	o.logMgr = logstate.NewManager(s)
	o.stateEng.AddManager(o.logMgr)

	o.stateEng = NewStateEngine(s)
	return o, nil
}

// State returns the system state managed by the overlord.
func (o *Overlord) State() State {
	return o.stateEng.State()
}

// StateEngine returns the state engine used by overlord.
func (o *Overlord) StateEngine() *StateEngine {
	return o.stateEng
}

// LogManager returns the log manager responsible for logging under the
// overlord.
func (o *Overlord) LogManager() LogManager {
	return o.logMgr
}
