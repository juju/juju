// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"time"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/names/v4"

	"github.com/juju/juju/apiserver/params"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/cmd/plugins/juju-wait-for/api"
	"github.com/juju/juju/cmd/plugins/juju-wait-for/query"
	"github.com/juju/juju/core/life"
)

func newModelCommand() cmd.Command {
	cmd := &modelCommand{
		applications: make(map[string]*params.ApplicationInfo),
		machines:     make(map[string]*params.MachineInfo),
		units:        make(map[string]*params.UnitInfo),
	}
	cmd.newWatchAllAPIFunc = func() (api.WatchAllAPI, error) {
		client, err := cmd.NewAPIClient()
		if err != nil {
			return nil, errors.Trace(err)
		}
		return watchAllAPIShim{
			Client: client,
		}, nil
	}
	return modelcmd.Wrap(cmd)
}

const modelCommandDoc = `
Wait for a given model to reach a goal state.

arguments:
name
   model name identifier

options:
--query (= 'life=="alive" && status=="available"')
   query represents the goal state of a given model
`

// modelCommand defines a command for waiting for models.
type modelCommand struct {
	waitForCommandBase

	name    string
	query   string
	timeout time.Duration
	found   bool

	// TODO (stickupkid): Generalize this to become a local cache, similar to
	// the model cache but not with the hierarchy or complexity (for example,
	// we don't need the mark+sweep gc).
	applications map[string]*params.ApplicationInfo
	machines     map[string]*params.MachineInfo
	units        map[string]*params.UnitInfo
}

// Info implements Command.Info.
func (c *modelCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "model",
		Args:    "[<name>]",
		Purpose: "wait for an model to reach a goal state",
		Doc:     modelCommandDoc,
	})
}

// SetFlags implements Command.SetFlags.
func (c *modelCommand) SetFlags(f *gnuflag.FlagSet) {
	c.waitForCommandBase.SetFlags(f)
	f.StringVar(&c.query, "query", `life=="alive" && status=="available"`, "query the goal state")
	f.DurationVar(&c.timeout, "timeout", time.Minute*10, "how long to wait, before timing out")
}

// Init implements Command.Init.
func (c *modelCommand) Init(args []string) (err error) {
	if len(args) == 0 {
		return errors.New("model name must be supplied when waiting for an model")
	}
	if len(args) != 1 {
		return errors.New("only one model name can be supplied as an argument to this command")
	}
	if ok := names.IsValidModelName(args[0]); !ok {
		return errors.Errorf("%q is not valid model name", args[0])
	}
	c.name = args[0]

	return nil
}

func (c *modelCommand) Run(ctx *cmd.Context) error {
	strategy := &Strategy{
		ClientFn: c.newWatchAllAPIFunc,
		Timeout:  c.timeout,
	}
	strategy.Subscribe(func(event EventType) {
		switch event {
		case WatchAllStarted:
			// When a watch has started, we should prime all the local caches,
			// this means we should evict all items in the model and resync them
			// again.
		}
	})
	err := strategy.Run(c.name, c.query, c.waitFor)
	return errors.Trace(err)
}

func (c *modelCommand) waitFor(name string, deltas []params.Delta, q query.Query) (bool, error) {
	var modelUpdate *params.ModelUpdate
	for _, delta := range deltas {
		logger.Tracef("delta %T: %v", delta.Entity, delta.Entity)

		switch entityInfo := delta.Entity.(type) {
		case *params.ApplicationInfo:
			if delta.Removed {
				delete(c.applications, entityInfo.Name)
				break
			}
			c.applications[entityInfo.Name] = entityInfo

		case *params.MachineInfo:
			if delta.Removed {
				delete(c.machines, entityInfo.Id)
				break
			}
			c.machines[entityInfo.Id] = entityInfo

		case *params.UnitInfo:
			if delta.Removed {
				delete(c.units, entityInfo.Name)
				break
			}
			c.units[entityInfo.Name] = entityInfo

		case *params.ModelUpdate:
			if entityInfo.Name == name {
				if delta.Removed {
					return false, errors.Errorf("model %v removed", name)
				}
				modelUpdate = entityInfo
				c.found = entityInfo.Life != life.Dead
			}
		}

		if modelUpdate != nil {
			scope := MakeModelScope(c, modelUpdate)
			if done, err := runQuery(q, scope); err != nil {
				return false, errors.Trace(err)
			} else if done {
				return true, nil
			}
		}
	}

	if !c.found {
		logger.Infof("model %q not found, waiting...", name)
		return false, nil
	}

	logger.Infof("model %q found, waiting...", name)
	return false, nil
}

// ModelScope allows the query to introspect a model entity.
type ModelScope struct {
	ModelInfo *params.ModelUpdate
	Model     *modelCommand
}

// MakeModelScope creates an ModelScope from an ModelUpdate
func MakeModelScope(model *modelCommand, info *params.ModelUpdate) ModelScope {
	return ModelScope{
		ModelInfo: info,
		Model:     model,
	}
}

// GetIdents returns the identifiers with in a given scope.
func (m ModelScope) GetIdents() []string {
	return getIdents(m.ModelInfo)
}

// GetIdentValue returns the value of the identifier in a given scope.
func (m ModelScope) GetIdentValue(name string) (query.Box, error) {
	switch name {
	case "name":
		return query.NewString(m.ModelInfo.Name), nil
	case "life":
		return query.NewString(string(m.ModelInfo.Life)), nil
	case "is-controller":
		return query.NewBool(m.ModelInfo.IsController), nil
	case "status":
		return query.NewString(string(m.ModelInfo.Status.Current)), nil
	case "config":
		return query.NewMapStringInterface(m.ModelInfo.Config), nil
	case "applications":
		scopes := make(map[string]query.Scope)
		for k, app := range m.Model.applications {
			units := make(map[string]*params.UnitInfo)
			for n, unit := range m.Model.units {
				if unit.Application == app.Name {
					units[n] = unit
				}
			}

			currentStatus := app.Status.Current
			newStatus := deriveApplicationStatus(currentStatus, units)

			appInfo := app
			appInfo.Status.Current = newStatus

			scopes[k] = MakeApplicationScope(appInfo)
		}
		return NewScopedBox(scopes), nil
	case "machines":
		scopes := make(map[string]query.Scope)
		for k, machine := range m.Model.machines {
			scopes[k] = MakeMachineScope(machine)
		}
		return NewScopedBox(scopes), nil
	case "units":
		scopes := make(map[string]query.Scope)
		for k, unit := range m.Model.units {
			scopes[k] = MakeUnitScope(unit)
		}
		return NewScopedBox(scopes), nil
	}
	return nil, errors.Annotatef(query.ErrInvalidIdentifier(name), "Runtime Error: identifier %q not found on ModelInfo", name)
}

// ScopedBox defines a scoped box of scopes.
// Lifts any scope into a box to be used later on.
type ScopedBox struct {
	scopes map[string]query.Scope
}

// NewScopedBox creates a new Box value
func NewScopedBox(scopes map[string]query.Scope) *ScopedBox {
	return &ScopedBox{
		scopes: scopes,
	}
}

// Less checks if a ScopedBox is less than another ScopedBox.
func (o *ScopedBox) Less(other query.Ord) bool {
	return false
}

// Equal checks if an ScopedBox is equal to another ScopedBox.
func (o *ScopedBox) Equal(other query.Ord) bool {
	return false
}

// IsZero returns if the underlying value is zero.
func (o *ScopedBox) IsZero() bool {
	return len(o.scopes) == 0
}

// Value defines the shadow type value of the Box.
func (o *ScopedBox) Value() interface{} {
	return o
}

// ForEach iterates over each value in the box.
func (o *ScopedBox) ForEach(fn func(interface{}) bool) {
	for _, v := range o.scopes {
		if !fn(v) {
			return
		}
	}
}
