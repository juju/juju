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
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/rpc/params"
)

func newApplicationCommand() cmd.Command {
	cmd := &applicationCommand{}
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

const applicationCommandDoc = `
The ` + "`wait-for application`" + ` command waits for the application to reach a goal
state. The goal state can be defined programmatically using the query DSL
(domain specific language). The default query for an application just waits
for the application to be created and active.

The ` + "`wait-for`" + ` command is an optimized alternative to the ` + "`status`" + ` command for
determining programmatically if a goal state has been reached. The ` + "`wait-for`" + `
command streams delta changes from the underlying database, unlike the ` + "`status`" + `
command which performs a full query of the database.

The application query DSL can be used to programmatically define the goal state
for machines and units within the scope of the application. This can
be achieved by using lambda expressions to iterate over the machines and units
associated with the application. Multiple expressions can be combined to define
a complex goal state.
`
const applicationCommandExamples = `
Waits for 4 units to be present:

    juju wait-for application ubuntu --query='len(units) == 4'

Waits for all the application units to start with ` + "`ubuntu`" + ` and to be created
and available:

    juju wait-for application ubuntu --query='forEach(units, unit => unit.life=="alive" && unit.status=="available" && startsWith(unit.name, "ubuntu"))'
`

// applicationCommand defines a command for waiting for applications.
type applicationCommand struct {
	waitForCommandBase

	name    string
	query   string
	timeout time.Duration
	summary bool

	appInfo  *params.ApplicationInfo
	units    map[string]*params.UnitInfo
	machines map[string]*params.MachineInfo
}

// Info implements Command.Info.
func (c *applicationCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "application",
		Args:     "[<name>]",
		Purpose:  "Wait for an application to reach a specified state.",
		Doc:      applicationCommandDoc,
		Examples: applicationCommandExamples,
		SeeAlso: []string{
			"wait-for model",
			"wait-for machine",
			"wait-for unit",
		},
	})
}

// SetFlags implements Command.SetFlags.
func (c *applicationCommand) SetFlags(f *gnuflag.FlagSet) {
	c.waitForCommandBase.SetFlags(f)
	f.StringVar(&c.query, "query", `life=="alive" && status=="active"`, "Query the goal state")
	f.DurationVar(&c.timeout, "timeout", time.Minute*10, "How long to wait, before timing out")
	f.BoolVar(&c.summary, "summary", true, "Output a summary of the application query on exit")
}

// Init implements Command.Init.
func (c *applicationCommand) Init(args []string) (err error) {
	if len(args) == 0 {
		return errors.New("application name must be supplied when waiting for an application")
	}
	if len(args) != 1 {
		return errors.New("only one application name can be supplied as an argument to this command")
	}
	if ok := names.IsValidApplication(args[0]); !ok {
		return errors.Errorf("%q is not valid application name", args[0])
	}
	c.name = args[0]

	return nil
}

func (c *applicationCommand) Run(ctx *cmd.Context) (err error) {
	scopedContext := MakeScopeContext()

	defer func() {
		if err != nil || !c.summary || c.appInfo == nil {
			return
		}

		switch c.appInfo.Life {
		case life.Dead:
			ctx.Infof("Application %q has been removed", c.name)
		case life.Dying:
			ctx.Infof("Application %q is being removed", c.name)
		default:
			ctx.Infof("Application %q is running", c.name)
			outputApplicationSummary(ctx.Stdout, scopedContext, c.appInfo, c.units, c.machines)
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

func (c *applicationCommand) primeCache() {
	c.units = make(map[string]*params.UnitInfo)
	c.machines = make(map[string]*params.MachineInfo)
}

func (c *applicationCommand) waitFor(input string, ctx ScopeContext, logger Logger) func(string, []params.Delta, query.Query) (bool, error) {
	run := func(q query.Query) (bool, error) {
		scope := MakeApplicationScope(ctx, c.appInfo, c.units, c.machines)
		if done, err := runQuery(input, q, scope); err != nil {
			return false, errors.Trace(err)
		} else if done {
			return true, nil
		}
		return c.appInfo.Life == life.Dead, nil
	}
	return func(name string, deltas []params.Delta, q query.Query) (bool, error) {
		for _, delta := range deltas {
			logger.Verbosef("delta %T: %v", delta.Entity, delta.Entity)

			switch entityInfo := delta.Entity.(type) {
			case *params.ApplicationInfo:
				if entityInfo.Name != name {
					break
				}
				if delta.Removed {
					return false, errors.Errorf("application %v removed", name)
				}

				c.appInfo = entityInfo

			case *params.UnitInfo:
				if delta.Removed {
					delete(c.units, entityInfo.Name)
					break
				}
				if entityInfo.Application == name {
					c.units[entityInfo.Name] = entityInfo
				}

			case *params.MachineInfo:
				if delta.Removed {
					delete(c.machines, entityInfo.Id)
					break
				}
				c.machines[entityInfo.Id] = entityInfo
			}
		}

		var currentStatus status.Status
		if c.appInfo != nil {
			// Store the current status so we can restore it after the query
			// has been run.
			currentStatus = c.appInfo.Status.Current

			// This is required because we derive the status from the units
			// and not the application itself, unless it has been explicitly
			// set.
			c.appInfo.Status.Current = deriveApplicationStatus(currentStatus, c.units)

			if found, err := run(q); err != nil {
				return false, errors.Trace(err)
			} else if found {
				return true, nil
			}

			// Restore the current status of the application.
			c.appInfo.Status.Current = currentStatus
		} else {
			logger.Infof("application %q not found, waiting...", name)
			return false, nil
		}

		logger.Infof("application %q found with %q, waiting...", name, deriveApplicationStatus(currentStatus, c.units))
		return false, nil
	}
}

// ApplicationScope allows the query to introspect a application entity.
type ApplicationScope struct {
	ctx             ScopeContext
	ApplicationInfo *params.ApplicationInfo
	UnitInfos       map[string]*params.UnitInfo
	MachineInfos    map[string]*params.MachineInfo
}

// MakeApplicationScope creates an ApplicationScope from an ApplicationInfo
func MakeApplicationScope(ctx ScopeContext,
	appInfo *params.ApplicationInfo,
	unitInfos map[string]*params.UnitInfo,
	machineInfos map[string]*params.MachineInfo,
) ApplicationScope {
	return ApplicationScope{
		ctx:             ctx,
		ApplicationInfo: appInfo,
		UnitInfos:       unitInfos,
		MachineInfos:    machineInfos,
	}
}

// GetIdents returns the identifiers with in a given scope.
func (m ApplicationScope) GetIdents() []string {
	idents := set.NewStrings(getIdents(m.ApplicationInfo)...)
	return set.NewStrings("units", "machines").Union(idents).SortedValues()
}

// GetIdentValue returns the value of the identifier in a given scope.
func (m ApplicationScope) GetIdentValue(name string) (query.Box, error) {
	m.ctx.RecordIdent(name)

	switch name {
	case "name":
		return query.NewString(m.ApplicationInfo.Name), nil
	case "life":
		return query.NewString(string(m.ApplicationInfo.Life)), nil
	case "exposed":
		return query.NewBool(m.ApplicationInfo.Exposed), nil
	case "charm-url":
		return query.NewString(m.ApplicationInfo.CharmURL), nil
	case "min-units":
		return query.NewInteger(int64(m.ApplicationInfo.MinUnits)), nil
	case "subordinate":
		return query.NewBool(m.ApplicationInfo.Subordinate), nil
	case "status":
		return query.NewString(string(m.ApplicationInfo.Status.Current)), nil
	case "workload-version":
		return query.NewString(m.ApplicationInfo.WorkloadVersion), nil
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
	case "machines":
		scopes := make(map[string]query.Scope)
		for k, machine := range m.MachineInfos {
			var found bool
			for _, unit := range m.UnitInfos {
				if unit.Application == m.ApplicationInfo.Name && unit.MachineId == machine.Id {
					found = true
					break
				}
			}
			if found {
				scopes[k] = MakeMachineScope(m.ctx.Child(name, machine.Id), machine)
			}
		}
	}
	return nil, errors.Annotatef(query.ErrInvalidIdentifier(name, m), "%q on ApplicationInfo", name)
}

func deriveApplicationStatus(appStatus status.Status, units map[string]*params.UnitInfo) status.Status {
	if appStatus != status.Unset {
		return appStatus
	}

	statuses := make([]status.StatusInfo, 0)
	for _, unit := range units {
		agentStatus := unit.WorkloadStatus
		statuses = append(statuses, status.StatusInfo{
			Status: agentStatus.Current,
		})
	}

	derived := status.DeriveStatus(statuses)
	return derived.Status
}

func outputApplicationSummary(writer io.Writer,
	scopedContext ScopeContext,
	appInfo *params.ApplicationInfo,
	units map[string]*params.UnitInfo,
	machines map[string]*params.MachineInfo,
) {
	result := struct {
		Properties map[string]any            `yaml:"properties"`
		Units      map[string]map[string]any `yaml:"units,omitempty"`
		Machines   map[string]map[string]any `yaml:"machines,omitempty"`
	}{
		Properties: make(map[string]any),
		Units:      make(map[string]map[string]any),
		Machines:   make(map[string]map[string]any),
	}

	idents := scopedContext.RecordedIdents()
	for _, ident := range idents {
		// We have to special case status here because of the issue that
		// unset propagates through and we have to read it via the unit
		// information.
		if ident == "status" {
			result.Properties[ident] = deriveApplicationStatus(appInfo.Status.Current, units).String()
			continue
		}

		scope := MakeApplicationScope(scopedContext, appInfo, units, machines)
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
