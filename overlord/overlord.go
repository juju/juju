// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package overlord

import (
	"context"

	"github.com/juju/juju/overlord/logstate"
	"gopkg.in/tomb.v2"
)

// Overlord is the central manager of the system, keeping track
// of all available state managers and related helpers.
type Overlord struct {
	stateEng *StateEngine
	tomb     *tomb.Tomb
	started  bool
}

func newOverlord(s State) *Overlord {
	return &Overlord{
		tomb:     new(tomb.Tomb),
		stateEng: NewStateEngine(s),
	}
}

// StartUp proceeds to run any expensive Overlord or managers initialization.
// After this is done once it is a noop.
func (o *Overlord) StartUp(ctx context.Context) error {
	if o.started {
		return nil
	}
	o.started = true

	return o.stateEng.StartUp(ctx)
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

func (o *Overlord) LogManager() LogManager {
	return nil
}

// LogOverlord is an overlord that handles the logs database. As the logs
// database is separete from the models database, we have a special logging
// overlord that correctly handles just that case.
type LogOverlord struct {
	*Overlord
	logMgr LogManager
}

// NewLogOverlord creates a new Overlord that manages logging with all the
// correct state managers.
func NewLogOverlord(s State) (*LogOverlord, error) {
	o := &LogOverlord{
		Overlord: newOverlord(s),
	}

	o.logMgr = logstate.NewManager(s)
	o.stateEng.AddManager(o.logMgr)

	return o, nil
}

func (o *LogOverlord) LogManager() LogManager {
	return o.logMgr
}

type ModelOverlord struct {
	*Overlord
}

// NewModelOverlord creates a new Overlord that manages models with all the
// correct state managers.
func NewModelOverlord(s State) (*ModelOverlord, error) {
	o := &ModelOverlord{
		Overlord: newOverlord(s),
	}

	return o, nil
}
