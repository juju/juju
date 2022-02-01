// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"io"
	"time"

	"github.com/juju/cmd/v3"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/names/v4"
	"gopkg.in/yaml.v2"

	"github.com/juju/juju/apiserver/params"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/cmd/plugins/juju-wait-for/api"
	"github.com/juju/juju/cmd/plugins/juju-wait-for/query"
	"github.com/juju/juju/core/life"
)

func newUnitCommand() cmd.Command {
	cmd := &unitCommand{}
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

const unitCommandDoc = `
Wait for a given unit to reach a goal state.

arguments:
name
   unit name identifier

options:
--query (= 'life=="alive" && status=="active"')
   query represents the goal state of a given unit
`

// unitCommand defines a command for waiting for units.
type unitCommand struct {
	waitForCommandBase

	name    string
	query   string
	timeout time.Duration
	found   bool
	summary bool

	unitInfo params.UnitInfo
}

// Info implements Command.Info.
func (c *unitCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "unit",
		Args:    "[<name>]",
		Purpose: "wait for an unit to reach a goal state",
		Doc:     unitCommandDoc,
	})
}

// SetFlags implements Command.SetFlags.
func (c *unitCommand) SetFlags(f *gnuflag.FlagSet) {
	c.waitForCommandBase.SetFlags(f)
	f.StringVar(&c.query, "query", `life=="alive" && workload-status=="active"`, "query the goal state")
	f.DurationVar(&c.timeout, "timeout", time.Minute*10, "how long to wait, before timing out")
	f.BoolVar(&c.summary, "summary", true, "output a summary of the application query on exit")
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

func (c *unitCommand) Run(ctx *cmd.Context) error {
	scopedContext := MakeScopeContext()

	defer func() {
		if !c.summary {
			return
		}

		switch c.unitInfo.Life {
		case life.Dead:
			ctx.Infof("Unit %q has been removed", c.name)
		case life.Dying:
			ctx.Infof("Unit %q is being removed", c.name)
		default:
			ctx.Infof("Unit %q is running", c.name)
			outputUnitSummary(ctx.Stdout, scopedContext, &c.unitInfo)
		}
	}()

	strategy := &Strategy{
		ClientFn: c.newWatchAllAPIFunc,
		Timeout:  c.timeout,
	}
	err := strategy.Run(c.name, c.query, c.waitFor(scopedContext))
	return errors.Trace(err)
}

func (c *unitCommand) waitFor(ctx ScopeContext) func(string, []params.Delta, query.Query) (bool, error) {
	return func(name string, deltas []params.Delta, q query.Query) (bool, error) {
		for _, delta := range deltas {
			logger.Tracef("delta %T: %v", delta.Entity, delta.Entity)

			switch entityInfo := delta.Entity.(type) {
			case *params.UnitInfo:
				if entityInfo.Name != name {
					break
				}

				if delta.Removed {
					return false, errors.Errorf("unit %v removed", name)
				}

				c.unitInfo = *entityInfo

				scope := MakeUnitScope(ctx, entityInfo)
				if done, err := runQuery(q, scope); err != nil {
					return false, errors.Trace(err)
				} else if done {
					return true, nil
				}

				c.found = entityInfo.Life != life.Dead
			}
		}

		if !c.found {
			logger.Infof("unit %q not found, waiting...", name)
			return false, nil
		}

		logger.Infof("unit %q found, waiting...", name)
		return false, nil
	}
}

// UnitScope allows the query to introspect a unit entity.
type UnitScope struct {
	ctx      ScopeContext
	UnitInfo *params.UnitInfo
}

// MakeUnitScope creates an UnitScope from an UnitInfo
func MakeUnitScope(ctx ScopeContext, info *params.UnitInfo) UnitScope {
	return UnitScope{
		ctx:      ctx,
		UnitInfo: info,
	}
}

// GetIdents returns the identifiers with in a given scope.
func (m UnitScope) GetIdents() []string {
	return getIdents(m.UnitInfo)
}

// GetIdentValue returns the value of the identifier in a given scope.
func (m UnitScope) GetIdentValue(name string) (query.Box, error) {
	m.ctx.RecordIdent(name)

	switch name {
	case "name":
		return query.NewString(m.UnitInfo.Name), nil
	case "application":
		return query.NewString(m.UnitInfo.Application), nil
	case "series":
		return query.NewString(m.UnitInfo.Series), nil
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
	}
	return nil, errors.Annotatef(query.ErrInvalidIdentifier(name), "Runtime Error: identifier %q not found on UnitInfo", name)
}

func outputUnitSummary(writer io.Writer, scopedContext ScopeContext, unitInfo *params.UnitInfo) {
	result := struct {
		Elements map[string]interface{} `yaml:"properties"`
	}{
		Elements: make(map[string]interface{}),
	}

	idents := scopedContext.RecordedIdents()
	for _, ident := range idents {
		scope := MakeUnitScope(scopedContext, unitInfo)
		box, err := scope.GetIdentValue(ident)
		if err != nil {
			continue
		}
		result.Elements[ident] = box.Value()
	}

	_ = yaml.NewEncoder(writer).Encode(result)
}
