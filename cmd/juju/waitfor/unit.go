// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package waitfor

import (
	"io"
	"time"

	"github.com/juju/cmd/v3"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/names/v5"
	"gopkg.in/yaml.v2"

	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/juju/waitfor/api"
	"github.com/juju/juju/cmd/juju/waitfor/query"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/rpc/params"
)

func newUnitCommand() cmd.Command {
	cmd := &unitCommand{}
	cmd.newWatchAllAPIFunc = func() (api.WatchAllAPI, error) {
		client, err := cmd.NewAPIClient()
		if err != nil {
			return nil, errors.Trace(err)
		}
		return modelAllWatchShim{
			Client: client,
		}, nil
	}
	return modelcmd.Wrap(cmd)
}

const unitCommandDoc = `
The ` + "`wait-for unit`" + ` command waits for the unit to reach a goal state. The goal
state can be defined programmatically using the query DSL (domain specific
language). The default query for a unit just waits for the unit to be created
and active.

The ` + "`wait-for`" + ` command is an optimized alternative to the ` + "`status`" + ` command for
determining programmatically if a goal state has been reached. The ` + "`wait-for`" + `
command streams delta changes from the underlying database, unlike the ` + "`status`" + `
command which performs a full query of the database.

The unit query DSL can be used to programmatically define the goal state
for machine within the scope of the unit. This can be achieved by using lambda
expressions to iterate over the machines associated with the unit. Multiple
expressions can be combined to define a complex goal state.
`

const unitCommandExamples = `
Waits for a units to be machines to be length of 1.

    juju wait-for unit ubuntu/0 --query='len(machines) == 1'

Waits for the unit to be created and active.

    juju wait-for unit ubuntu/0 --query='life=="alive" && workload-status=="active"'
`

// unitCommand defines a command for waiting for units.
type unitCommand struct {
	waitForCommandBase

	name    string
	query   string
	timeout time.Duration
	summary bool

	unitInfo *params.UnitInfo
	machines map[string]*params.MachineInfo
}

// Info implements Command.Info.
func (c *unitCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "unit",
		Args:     "[<name>]",
		Purpose:  "Wait for a unit to reach a specified state.",
		Doc:      unitCommandDoc,
		Examples: unitCommandExamples,
		SeeAlso: []string{
			"wait-for model",
			"wait-for application",
			"wait-for machine",
		},
	})
}

// SetFlags implements Command.SetFlags.
func (c *unitCommand) SetFlags(f *gnuflag.FlagSet) {
	c.waitForCommandBase.SetFlags(f)
	f.StringVar(&c.query, "query", `life=="alive" && workload-status=="active"`, "Query the goal state")
	f.DurationVar(&c.timeout, "timeout", time.Minute*10, "How long to wait, before timing out")
	f.BoolVar(&c.summary, "summary", true, "Output a summary of the application query on exit")
}

// Init implements Command.Init.
func (c *unitCommand) Init(args []string) (err error) {
	if len(args) == 0 {
		return errors.New("unit name must be supplied when waiting for an unit")
	}
	if len(args) != 1 {
		return errors.New("only one unit name can be supplied as an argument to this command")
	}
	if ok := names.IsValidUnit(args[0]); !ok {
		return errors.Errorf("%q is not valid unit name", args[0])
	}
	c.name = args[0]

	return nil
}

func (c *unitCommand) Run(ctx *cmd.Context) (err error) {
	scopedContext := MakeScopeContext()

	defer func() {
		if err != nil || !c.summary || c.unitInfo == nil {
			return
		}

		switch c.unitInfo.Life {
		case life.Dead:
			ctx.Infof("unit %q has been removed", c.name)
		case life.Dying:
			ctx.Infof("unit %q is being removed", c.name)
		default:
			ctx.Infof("unit %q is running", c.name)
			outputUnitSummary(ctx.Stdout, scopedContext, c.unitInfo, c.machines)
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
	err = strategy.Run(ctx, c.name, c.query, c.waitFor(c.query, scopedContext, ctx), emptyNotify)
	return errors.Trace(err)
}

func (c *unitCommand) primeCache() {
	c.machines = make(map[string]*params.MachineInfo)
}

func (c *unitCommand) waitFor(input string, ctx ScopeContext, logger Logger) func(string, []params.Delta, query.Query) (bool, error) {
	run := func(q query.Query) (bool, error) {
		scope := MakeUnitScope(ctx, c.unitInfo, c.machines)
		if done, err := runQuery(input, q, scope); err != nil {
			return false, errors.Trace(err)
		} else if done {
			return true, nil
		}
		return c.unitInfo.Life == life.Dead, nil
	}
	return func(name string, deltas []params.Delta, q query.Query) (bool, error) {
		for _, delta := range deltas {
			logger.Verbosef("delta %T: %v", delta.Entity, delta.Entity)

			switch entityInfo := delta.Entity.(type) {
			case *params.UnitInfo:
				if entityInfo.Name != name {
					break
				}

				if delta.Removed {
					return false, errors.Errorf("unit %v removed", name)
				}

				c.unitInfo = entityInfo

			case *params.MachineInfo:
				if delta.Removed {
					delete(c.machines, entityInfo.Id)
					break
				}
				c.machines[entityInfo.Id] = entityInfo
			}
		}

		if c.unitInfo != nil {
			if found, err := run(q); err != nil {
				return false, errors.Trace(err)
			} else if found {
				return true, nil
			}
		} else {
			logger.Infof("unit %q not found, waiting...", name)
			return false, nil
		}

		logger.Infof("unit %q found, waiting...", name)
		return false, nil
	}
}

// UnitScope allows the query to introspect a unit entity.
type UnitScope struct {
	ctx          ScopeContext
	UnitInfo     *params.UnitInfo
	MachineInfos map[string]*params.MachineInfo
}

// MakeUnitScope creates an UnitScope from an UnitInfo
func MakeUnitScope(ctx ScopeContext, info *params.UnitInfo, machineInfos map[string]*params.MachineInfo) UnitScope {
	return UnitScope{
		ctx:          ctx,
		UnitInfo:     info,
		MachineInfos: machineInfos,
	}
}

// GetIdents returns the identifiers with in a given scope.
func (m UnitScope) GetIdents() []string {
	idents := set.NewStrings(getIdents(m.UnitInfo)...)
	return set.NewStrings("machines").Union(idents).SortedValues()
}

// GetIdentValue returns the value of the identifier in a given scope.
func (m UnitScope) GetIdentValue(name string) (query.Box, error) {
	m.ctx.RecordIdent(name)

	// UnitInfo could be nil - handle it here to avoid nil pointer dereference
	if m.UnitInfo == nil {
		return nil, errors.New("internal error: UnitInfo is missing")
	}

	switch name {
	case "name":
		return query.NewString(m.UnitInfo.Name), nil
	case "application":
		return query.NewString(m.UnitInfo.Application), nil
	case "base":
		return query.NewString(m.UnitInfo.Base), nil
	case "charm-url":
		return query.NewString(m.UnitInfo.CharmURL), nil
	case "life":
		return query.NewString(string(m.UnitInfo.Life)), nil
	case "public-address":
		return query.NewString(m.UnitInfo.PublicAddress), nil
	case "private-address":
		return query.NewString(m.UnitInfo.PrivateAddress), nil
	case "machine-id":
		return query.NewString(m.UnitInfo.MachineId), nil
	case "principal":
		return query.NewString(m.UnitInfo.Principal), nil
	case "subordinate":
		return query.NewBool(m.UnitInfo.Subordinate), nil
	case "workload-status":
		return query.NewString(string(m.UnitInfo.WorkloadStatus.Current)), nil
	case "workload-message":
		return query.NewString(m.UnitInfo.WorkloadStatus.Message), nil
	case "agent-status":
		return query.NewString(string(m.UnitInfo.AgentStatus.Current)), nil
	case "machines":
		scopes := make(map[string]query.Scope)
		for k, machine := range m.MachineInfos {
			if machine.Id == m.UnitInfo.MachineId {
				scopes[k] = MakeMachineScope(m.ctx.Child(name, machine.Id), machine)
			}
		}
		return NewScopedBox(scopes), nil
	}
	return nil, errors.Annotatef(query.ErrInvalidIdentifier(name, m), "%q on UnitInfo", name)
}

func outputUnitSummary(writer io.Writer, scopedContext ScopeContext, units *params.UnitInfo, machines map[string]*params.MachineInfo) {
	result := struct {
		Properties map[string]any            `yaml:"properties"`
		Machines   map[string]map[string]any `yaml:"machines,omitempty"`
	}{
		Properties: make(map[string]any),
		Machines:   make(map[string]map[string]any),
	}

	idents := scopedContext.RecordedIdents()
	for _, ident := range idents {
		scope := MakeUnitScope(scopedContext, units, machines)
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
