// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package waitfor

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/expr-lang/expr"
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
	"github.com/juju/juju/core/constraints"
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
The wait-for application command waits for the application to reach a goal
state. The goal state can be defined programmatically using the query DSL
(domain specific language). The default query for an application just waits
for the application to be created and active.

The wait-for command is an optimized alternative to the status command for 
determining programmatically if a goal state has been reached. The wait-for
command streams delta changes from the underlying database, unlike the status
command which performs a full query of the database.

The application query DSL can be used to programmatically define the goal state
for machines and units within the scope of the application. This can
be achieved by using lambda expressions to iterate over the machines and units
associated with the application. Multiple expressions can be combined to define 
a complex goal state.
`
const applicationCommandExamples = `
Waits for 4 units to be present.

    juju wait-for application ubuntu --query='len(units) == 4'

Waits for all the application units to start with ubuntu and to be created 
and available.

    juju wait-for application ubuntu --query='forEach(units, unit => unit.life=="alive" && unit.status=="available" && startsWith(unit.name, "ubuntu"))'
`

// applicationCommand defines a command for waiting for applications.
type applicationCommand struct {
	waitForCommandBase

	name    string
	query   string
	timeout time.Duration
	summary bool
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
	f.StringVar(&c.query, "query", `life=="alive" && status=="active"`, "query the goal state")
	f.DurationVar(&c.timeout, "timeout", time.Minute*10, "how long to wait, before timing out")
	f.BoolVar(&c.summary, "summary", true, "output a summary of the application query on exit")
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

func (c *applicationCommand) Run(ctx *cmd.Context) error {
	env := ApplicationEnv{}
	program, err := expr.Compile(c.query, expr.Env(env))
	if err != nil {
		return errors.Trace(err)
	}

	api, err := c.newWatchAllAPIFunc()
	if err != nil {
		return errors.Trace(err)
	}

	watcher, err := api.WatchAll()
	if err != nil {
		return errors.Trace(err)
	}

	var app *params.ApplicationInfo
	units := make(map[string]*params.UnitInfo)
	machines := make(map[string]*params.MachineInfo)

	for {
		delta, err := watcher.Next()
		if err != nil {
			return errors.Trace(err)
		}

	LOOP:
		for _, entity := range delta {
			switch entityInfo := entity.Entity.(type) {
			case *params.ApplicationInfo:
				if entityInfo.Name != c.name {
					continue LOOP
				}

				app = entityInfo

			case *params.UnitInfo:
				if entityInfo.Application != c.name {
					continue LOOP
				}
				units[entityInfo.Name] = entityInfo

			case *params.MachineInfo:
				machines[entityInfo.Id] = entityInfo
			}
		}

		for _, machine := range machines {
			for _, unit := range units {
				if unit.MachineId == machine.Id {
					continue
				}
				delete(machines, machine.Id)
			}
		}

		if app == nil {
			continue
		}

		output, err := expr.Run(program, ApplicationEnv{
			ModelUUID:       app.ModelUUID,
			Name:            app.Name,
			Exposed:         app.Exposed,
			CharmURL:        app.CharmURL,
			OwnerTag:        app.OwnerTag,
			Life:            string(app.Life),
			MinUnits:        app.MinUnits,
			Constraints:     toConstraints(app.Constraints),
			Config:          app.Config,
			Subordinate:     app.Subordinate,
			Status:          deriveApplicationStatus(app.Status.Current, units).String(),
			WorkloadVersion: app.WorkloadVersion,
			Units:           toSlice(units),
			Machines:        toSlice(machines),
		})
		if err != nil {
			return errors.Trace(err)
		}

		fmt.Println(output)

		if b, ok := output.(bool); ok && b {
			return nil
		} else if s, ok := output.(string); ok {
			if len(strings.TrimSpace(s)) > 0 {
				return nil
			}
		} else if x, ok := output.(int); ok && x > 0 {
			return nil
		} else if x, ok := output.(int64); ok && x > 0 {
			return nil
		}
	}
}

func toSlice[T any](m map[string]T) []T {
	result := make([]T, 0, len(m))
	for _, v := range m {
		result = append(result, v)
	}
	return result
}

func toConstraints(cons constraints.Value) map[string]any {
	result := make(map[string]any)
	if cons.HasAllocatePublicIP() {
		result["AllocatePublicIP"] = *cons.AllocatePublicIP
	}
	if cons.HasArch() {
		result["Arch"] = *cons.Arch
	}
	if cons.HasContainer() {
		result["Container"] = *cons.Container
	}
	if cons.HasCpuCores() {
		result["CpuCores"] = *cons.CpuCores
	}
	if cons.HasCpuPower() {
		result["CpuPower"] = *cons.CpuPower
	}
	if cons.HasInstanceType() {
		result["InstanceType"] = *cons.InstanceType
	}
	if cons.HasMem() {
		result["Mem"] = *cons.Mem
	}
	if cons.HasRootDisk() {
		result["RootDisk"] = *cons.RootDisk
	}
	if cons.HasRootDiskSource() {
		result["RootDiskSource"] = *cons.RootDiskSource
	}
	if cons.HasSpaces() {
		result["Spaces"] = *cons.Spaces
	}
	if tags := cons.Tags; tags != nil {
		result["Tags"] = *cons.Tags
	}
	if cons.HasVirtType() {
		result["VirtType"] = *cons.VirtType
	}
	if cons.HasZones() {
		result["Zones"] = *cons.Zones
	}
	if cons.HasInstanceRole() {
		result["InstanceRole"] = *cons.InstanceRole
	}
	return result
}

type ApplicationEnv struct {
	ModelUUID       string
	Name            string
	Exposed         bool
	CharmURL        string
	OwnerTag        string
	Life            string
	MinUnits        int
	Constraints     map[string]any
	Config          map[string]any
	Subordinate     bool
	Status          string
	WorkloadVersion string
	Units           []*params.UnitInfo
	Machines        []*params.MachineInfo
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
