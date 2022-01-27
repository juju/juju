// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package overlord

import (
	"context"
	"sync"

	"github.com/juju/juju/overlord/logstate"
	"github.com/juju/juju/overlord/schema"
	"github.com/juju/juju/overlord/schema/updates"
	"gopkg.in/tomb.v2"
)

// Overlord is the central manager of the system, keeping track
// of all available state managers and related helpers.
type Overlord struct {
	stateEng *StateEngine
	tomb     *tomb.Tomb
	// managers
	mutex     sync.Mutex
	started   bool
	schemaMgr *schema.SchemaManager
}

func newOverlord(s State, sch *schema.Schema) *Overlord {
	o := &Overlord{
		tomb:     new(tomb.Tomb),
		stateEng: NewStateEngine(s),
	}

	// Ensure we register the new schema manager first.
	o.schemaMgr = schema.NewManager(s, sch)
	o.stateEng.AddManager(o.schemaMgr)

	return o
}

// StartUp proceeds to run any expensive Overlord or managers initialization.
// After this is done once it is a noop.
func (o *Overlord) StartUp(ctx context.Context) error {
	// Use the mutex to prevent multiple calls to startup causing the engine
	// to startup.
	o.mutex.Lock()
	defer o.mutex.Unlock()

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

// Namespace represents the database namespaces.
type Namespace = string

const (
	LogNamespace   Namespace = "logs"
	ModelNamespace Namespace = "models"
)

// LogOverlord is an overlord that handles the logs database. As the logs
// database is separate from the models database, we have a special logging
// overlord that correctly handles just that case.
type LogOverlord struct {
	*Overlord
	logMgr LogManager
}

// NewLogOverlord creates a new Overlord that manages logging with all the
// correct state managers.
func NewLogOverlord(s State) (*LogOverlord, error) {
	o := &LogOverlord{
		Overlord: newOverlord(s, updates.LogSchema()),
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
		Overlord: newOverlord(s, updates.ModelSchema()),
	}

	return o, nil
}
