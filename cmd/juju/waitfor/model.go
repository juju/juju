// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package waitfor

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/juju/clock"
	"github.com/juju/cmd/v3"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/names/v5"
	"github.com/juju/retry"
	"gopkg.in/yaml.v2"

	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/juju/waitfor/api"
	"github.com/juju/juju/cmd/juju/waitfor/query"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/rpc/params"
)

func newModelCommand() cmd.Command {
	cmd := &modelCommand{}
	cmd.newWatchAllAPIFunc = func() (api.WatchAllAPI, error) {
		client, err := cmd.NewAPIClient()
		if err != nil {
			return nil, errors.Trace(err)
		}
		return modelAllWatchShim{
			Client: client,
		}, nil
	}
	return modelcmd.Wrap(cmd,
		modelcmd.WrapSkipModelInit,
		modelcmd.WrapSkipModelFlags,
	)
}

const modelCommandDoc = `
The ` + "`wait-for model`" + ` command waits for the model to reach a goal state. The goal
state can be defined programmatically using the query DSL (domain specific
language). The default query for a model just waits for the model to be
created and available.

The ` + "`wait-for`" + ` command is an optimized alternative to the ` + "`status`" + ` command for
determining programmatically if a goal state has been reached. The ` + "`wait-for`" + `
command streams delta changes from the underlying database, unlike the ` + "`status`" + `
command which performs a full query of the database.

The model query DSL can be used to programmatically define the goal state
for applications, machines and units within the scope of the model. This can
be achieved by using lambda expressions to iterate over the applications,
machines and units within the model. Multiple expressions can be combined to
define a complex goal state.
`

const modelCommandExamples = `
Waits for all the model units to start with ` + "`ubuntu`" + `:

    juju wait-for model default --query='forEach(units, unit => startsWith(unit.name, "ubuntu"))'

Waits for all the model applications to be active:

    juju wait-for model default --query='forEach(applications, app => app.status == "active")'

Waits for the model to be created and available and for all the model
applications to be active:

    juju wait-for model default --query='life=="alive" && status=="available" && forEach(applications, app => app.status == "active")'
`

// modelCommand defines a command for waiting for models.
type modelCommand struct {
	waitForCommandBase

	name    string
	query   string
	timeout time.Duration
	summary bool
	found   bool

	model        *params.ModelUpdate
	applications map[string]*params.ApplicationInfo
	machines     map[string]*params.MachineInfo
	units        map[string]*params.UnitInfo
}

// Info implements Command.Info.
func (c *modelCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "model",
		Args:     "[<name>]",
		Purpose:  "Wait for a model to reach a specified state.",
		Doc:      modelCommandDoc,
		Examples: modelCommandExamples,
		SeeAlso: []string{
			"wait-for application",
			"wait-for machine",
			"wait-for unit",
		},
	})
}

// SetFlags implements Command.SetFlags.
func (c *modelCommand) SetFlags(f *gnuflag.FlagSet) {
	c.waitForCommandBase.SetFlags(f)
	f.StringVar(&c.query, "query", `life=="alive" && status=="available"`, "Query the goal state")
	f.DurationVar(&c.timeout, "timeout", time.Minute*10, "How long to wait, before timing out")
	f.BoolVar(&c.summary, "summary", true, "Output a summary of the application query on exit")
}

// Init implements Command.Init.
func (c *modelCommand) Init(args []string) (err error) {
	if len(args) == 0 {
		return errors.New("model name must be supplied when waiting for an model")
	}
	if len(args) != 1 {
		return errors.New("only one model name can be supplied as an argument to this command")
	}
	controller, model := modelcmd.SplitModelName(args[0])
	if controller != "" && !names.IsValidControllerName(controller) {
		return errors.Errorf("%q is not valid controller name", controller)
	}
	if !names.IsValidModelName(model) {
		return errors.Errorf("%q is not valid model name", model)
	}

	c.name = model
	return c.locateModel(args[0])
}

func (c *modelCommand) Run(ctx *cmd.Context) (err error) {
	scopedContext := MakeScopeContext()

	defer func() {
		if err != nil || c.model == nil || !c.summary {
			return
		}

		switch c.model.Life {
		case life.Dead:
			ctx.Infof("model %q has been removed", c.name)
		case life.Dying:
			ctx.Infof("model %q is being removed", c.name)
		default:
			ctx.Infof("model %q is running", c.name)
			outputModelSummary(ctx.Stdout, scopedContext, c.model, c.applications, c.units, c.machines)
		}
	}()

	strategy := &Strategy{
		ClientFn: c.newWatchAllAPIFunc,
		Timeout:  c.timeout,
	}
	strategy.Subscribe(func(event EventType) {
		switch event {
		case WatchAllStarted:
			c.primeCache()
		}
	})
	err = strategy.Run(ctx, c.name, c.query, c.waitFor(c.query, scopedContext, ctx), func(err error, attempt int) {
		if errors.Is(err, errors.NotFound) {
			ctx.Infof("model %q not found, waiting...", c.name)
		}
	})
	return errors.Trace(err)
}

func (c *modelCommand) locateModel(name string) error {
	timeoutContext, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()

	if err := retry.Call(retry.CallArgs{
		Clock:       clock.WallClock,
		Delay:       1 * time.Second,
		Attempts:    200,
		MaxDelay:    10 * time.Second,
		MaxDuration: c.timeout,
		BackoffFunc: retry.DoubleDelay,
		Stop:        timeoutContext.Done(),
		IsFatalError: func(err error) bool {
			return !errors.Is(err, errors.NotFound)
		},
		NotifyFunc: func(err error, attempt int) {
			fmt.Fprintf(os.Stderr, "model %q not found, waiting...\n", c.name)
		},
		Func: func() error {
			return c.SetModelIdentifier(name, true)
		},
	}); err != nil {
		return fmt.Errorf("unable to locate model %q%w", c.name, errors.Hide(err))
	}
	return nil
}

func (c *modelCommand) primeCache() {
	c.applications = make(map[string]*params.ApplicationInfo)
	c.machines = make(map[string]*params.MachineInfo)
	c.units = make(map[string]*params.UnitInfo)
}

func (c *modelCommand) waitFor(input string, ctx ScopeContext, logger Logger) func(string, []params.Delta, query.Query) (bool, error) {
	run := func(q query.Query) (bool, error) {
		scope := MakeModelScope(ctx, c.model, c.applications, c.units, c.machines)
		if done, err := runQuery(input, q, scope); err != nil {
			return false, errors.Trace(err)
		} else if done {
			return true, nil
		}
		return c.model.Life == life.Dead, nil
	}
	return func(name string, deltas []params.Delta, q query.Query) (bool, error) {
		for _, delta := range deltas {
			logger.Verbosef("delta %T: %v", delta.Entity, delta.Entity)

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
				if entityInfo.Name != name {
					break
				}

				if delta.Removed {
					return false, errors.Errorf("model %v removed", name)
				}
				c.model = entityInfo
				c.found = entityInfo.Life != life.Dead
			}
		}

		if c.model != nil {
			if done, err := run(q); err != nil {
				return false, errors.Trace(err)
			} else if done {
				return true, nil
			}
		} else {
			logger.Infof("model %q not found, waiting...", name)
			return false, nil
		}

		logger.Infof("model %q found, waiting...", name)
		return false, nil
	}
}

// ModelScope allows the query to introspect a model entity.
type ModelScope struct {
	ctx              ScopeContext
	ModelInfo        *params.ModelUpdate
	ApplicationInfos map[string]*params.ApplicationInfo
	UnitInfos        map[string]*params.UnitInfo
	MachineInfos     map[string]*params.MachineInfo
}

// MakeModelScope creates a ModelScope from a ModelUpdate
func MakeModelScope(ctx ScopeContext,
	modelInfo *params.ModelUpdate,
	applicationInfos map[string]*params.ApplicationInfo,
	unitInfos map[string]*params.UnitInfo,
	machineInfos map[string]*params.MachineInfo,
) ModelScope {
	return ModelScope{
		ctx:              ctx,
		ModelInfo:        modelInfo,
		ApplicationInfos: applicationInfos,
		UnitInfos:        unitInfos,
		MachineInfos:     machineInfos,
	}
}

// GetIdents returns the identifiers with in a given scope.
func (m ModelScope) GetIdents() []string {
	idents := set.NewStrings(getIdents(m.ModelInfo)...)
	return set.NewStrings("applications", "machines", "units").Union(idents).SortedValues()
}

// GetIdentValue returns the value of the identifier in a given scope.
func (m ModelScope) GetIdentValue(name string) (query.Box, error) {
	switch name {
	case "name":
		m.ctx.RecordIdent(name)
		return query.NewString(m.ModelInfo.Name), nil
	case "life":
		m.ctx.RecordIdent(name)
		return query.NewString(string(m.ModelInfo.Life)), nil
	case "is-controller":
		m.ctx.RecordIdent(name)
		return query.NewBool(m.ModelInfo.IsController), nil
	case "status":
		m.ctx.RecordIdent(name)
		return query.NewString(string(m.ModelInfo.Status.Current)), nil
	case "config":
		m.ctx.RecordIdent(name)
		return query.NewMapStringInterface(m.ModelInfo.Config), nil
	case "applications":
		scopes := make(map[string]query.Scope)
		for k, app := range m.ApplicationInfos {
			units := make(map[string]*params.UnitInfo)
			machines := make(map[string]*params.MachineInfo)
			for n, unit := range m.UnitInfos {
				if unit.Application == app.Name {
					for m, machine := range m.MachineInfos {
						if machine.Id == unit.MachineId {
							machines[m] = machine
						}
					}

					units[n] = unit
				}
			}

			appInfo := app
			appInfo.Status.Current = deriveApplicationStatus(appInfo.Status.Current, units)

			scopes[k] = MakeApplicationScope(m.ctx.Child(name, app.Name), appInfo, units, machines)
		}
		return NewScopedBox(scopes), nil
	case "machines":
		scopes := make(map[string]query.Scope)
		for k, machine := range m.MachineInfos {
			scopes[k] = MakeMachineScope(m.ctx.Child(name, machine.Id), machine)
		}
		return NewScopedBox(scopes), nil
	case "units":
		scopes := make(map[string]query.Scope)
		for k, unit := range m.UnitInfos {
			machines := make(map[string]*params.MachineInfo)
			for n, machine := range m.MachineInfos {
				if machine.Id == unit.MachineId {
					machines[n] = machine
				}
			}

			scopes[k] = MakeUnitScope(m.ctx.Child(name, unit.Name), unit, machines)
		}
		return NewScopedBox(scopes), nil
	}
	return nil, errors.Annotatef(query.ErrInvalidIdentifier(name, m), "%q on ModelInfo", name)
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
func (o *ScopedBox) Value() any {
	return o
}

// ForEach iterates over each value in the box.
func (o *ScopedBox) ForEach(fn func(any) bool) {
	for _, v := range o.scopes {
		if !fn(v) {
			return
		}
	}
}

func outputModelSummary(writer io.Writer,
	scopedContext ScopeContext,
	modelInfo *params.ModelUpdate,
	applications map[string]*params.ApplicationInfo,
	units map[string]*params.UnitInfo,
	machines map[string]*params.MachineInfo,
) {
	result := struct {
		Properties   map[string]any            `yaml:"properties"`
		Applications map[string]map[string]any `yaml:"applications,omitempty"`
		Units        map[string]map[string]any `yaml:"units,omitempty"`
		Machines     map[string]map[string]any `yaml:"machines,omitempty"`
	}{
		Properties:   make(map[string]any),
		Applications: make(map[string]map[string]any),
		Units:        make(map[string]map[string]any),
		Machines:     make(map[string]map[string]any),
	}

	idents := scopedContext.RecordedIdents()
	for _, ident := range idents {
		scope := MakeModelScope(scopedContext, modelInfo, applications, units, machines)
		box, err := scope.GetIdentValue(ident)
		if err != nil {
			continue
		}
		result.Properties[ident] = box.Value()
	}
	for entity, scopes := range scopedContext.children {
		for name, sctx := range scopes {
			idents := sctx.RecordedIdents()

			switch entity {
			case "applications":
				appInfo := applications[name]
				scope := MakeApplicationScope(scopedContext, appInfo, units, machines)

				result.Applications[name] = make(map[string]any)

				for _, ident := range idents {
					box, err := scope.GetIdentValue(ident)
					if err != nil {
						continue
					}
					result.Applications[name][ident] = box.Value()
				}

			case "units":
				unitInfo := units[name]
				scope := MakeUnitScope(scopedContext, unitInfo, machines)

				result.Units[name] = make(map[string]any)
				for _, ident := range idents {
					box, err := scope.GetIdentValue(ident)
					if err != nil {
						continue
					}
					result.Units[name][ident] = box.Value()
				}

			case "machines":
				machineInfo := machines[name]
				scope := MakeMachineScope(scopedContext, machineInfo)

				result.Machines[name] = make(map[string]any)

				for _, ident := range idents {
					box, err := scope.GetIdentValue(ident)
					if err != nil {
						continue
					}
					result.Machines[name][ident] = box.Value()
				}
			}
		}
	}

	_ = yaml.NewEncoder(writer).Encode(result)
}
