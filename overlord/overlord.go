// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package overlord

import (
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/overlord/logstate"
)

// Overlord is the central manager of the system, keeping track
// of all available state managers and related helpers.
type Overlord struct {
	stateEng *StateEngine
	tomb     *tomb.Tomb
	// Managers
	logMgr LogManager
}

// New creates a new Overlord with all its state managers.
// It can be provided with an optional restart.Handler.
func New(s State) (*Overlord, error) {
	o := &Overlord{
		tomb: new(tomb.Tomb),
	}

	o.stateEng = NewStateEngine(s)

	o.logMgr = logstate.NewManager(s)
	o.stateEng.AddManager(o.logMgr)

	return o, nil
}

// Stop stops the ensure loop and the managers under the StateEngine.
func (o *Overlord) Stop() error {
	o.tomb.Kill(nil)
	err := o.tomb.Wait()
	o.stateEng.Stop()
	return err
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
