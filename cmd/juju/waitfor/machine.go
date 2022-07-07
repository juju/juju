// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package waitfor

import (
	"io"
	"time"

	"github.com/juju/cmd/v3"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/names/v4"
	"gopkg.in/yaml.v2"

	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/juju/waitfor/api"
	"github.com/juju/juju/cmd/juju/waitfor/query"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/life"
	"github.com/juju/juju/rpc/params"
)

func newMachineCommand() cmd.Command {
	cmd := &machineCommand{}
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

const machineCommandDoc = `
Waits for a machine to reach a specified state.

arguments:
name
   machine name identifier

options:
--query (= 'life=="alive" && status=="started")
   query represents the sought state of the specified machine
`

// machineCommand defines a command for waiting for models.
type machineCommand struct {
	waitForCommandBase

	id      string
	query   string
	timeout time.Duration
	found   bool
	summary bool

	machineInfo params.MachineInfo
}

// Info implements Command.Info.
func (c *machineCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "machine",
		Args:    "[<id>]",
		Purpose: "Wait for a machine to reach a specified state.",
		Doc:     machineCommandDoc,
	})
}

// SetFlags implements Command.SetFlags.
func (c *machineCommand) SetFlags(f *gnuflag.FlagSet) {
	c.waitForCommandBase.SetFlags(f)
	f.StringVar(&c.query, "query", `life=="alive" && status=="started"`, "query the goal state")
	f.DurationVar(&c.timeout, "timeout", time.Minute*10, "how long to wait, before timing out")
	f.BoolVar(&c.summary, "summary", true, "output a summary of the application query on exit")
}

// Init implements Command.Init.
func (c *machineCommand) Init(args []string) (err error) {
	if len(args) == 0 {
		return errors.New("machine id must be supplied when waiting for an machine")
	}
	if len(args) != 1 {
		return errors.New("only one machine id can be supplied as an argument to this command")
	}
	if ok := names.IsValidMachine(args[0]); !ok {
		return errors.Errorf("%q is not valid machine id", args[0])
	}
	c.id = args[0]

	return nil
}

func (c *machineCommand) Run(ctx *cmd.Context) error {
	scopedContext := MakeScopeContext()

	defer func() {
		if !c.summary {
			return
		}

		switch c.machineInfo.Life {
		case life.Dead:
			ctx.Infof("Machine %q has been removed", c.id)
		case life.Dying:
			ctx.Infof("Machine %q is being removed", c.id)
		default:
			ctx.Infof("Machine %q is running", c.id)
			outputMachineSummary(ctx.Stdout, scopedContext, &c.machineInfo)
		}
	}()

	strategy := &Strategy{
		ClientFn: c.newWatchAllAPIFunc,
		Timeout:  c.timeout,
	}
	err := strategy.Run(c.id, c.query, c.waitFor(scopedContext))
	return errors.Trace(err)
}

func (c *machineCommand) waitFor(ctx ScopeContext) func(string, []params.Delta, query.Query) (bool, error) {
	return func(id string, deltas []params.Delta, q query.Query) (bool, error) {
		for _, delta := range deltas {
			logger.Tracef("delta %T: %v", delta.Entity, delta.Entity)

			switch entityInfo := delta.Entity.(type) {
			case *params.MachineInfo:
				if entityInfo.Id != id {
					break
				}

				if delta.Removed {
					return false, errors.Errorf("machine %v removed", id)
				}

				c.machineInfo = *entityInfo

				scope := MakeMachineScope(ctx, entityInfo)
				if done, err := runQuery(q, scope); err != nil {
					return false, errors.Trace(err)
				} else if done {
					return true, nil
				}
				c.found = entityInfo.Life != life.Dead
			}
		}

		if !c.found {
			logger.Infof("machine %q not found, waiting...", id)
			return false, nil
		}

		logger.Infof("machine %q found, waiting...", id)
		return false, nil
	}
}

// MachineScope allows the query to introspect a model entity.
type MachineScope struct {
	ctx         ScopeContext
	MachineInfo *params.MachineInfo
}

// MakeMachineScope creates an MachineScope from an MachineInfo
func MakeMachineScope(ctx ScopeContext, info *params.MachineInfo) MachineScope {
	return MachineScope{
		ctx:         ctx,
		MachineInfo: info,
	}
}

// GetIdents returns the identifiers with in a given scope.
func (m MachineScope) GetIdents() []string {
	return getIdents(m.MachineInfo)
}

// GetIdentValue returns the value of the identifier in a given scope.
func (m MachineScope) GetIdentValue(name string) (query.Box, error) {
	m.ctx.RecordIdent(name)

	switch name {
	case "id":
		return query.NewString(m.MachineInfo.Id), nil
	case "life":
		return query.NewString(string(m.MachineInfo.Life)), nil
	case "status", "agent-status":
		return query.NewString(string(m.MachineInfo.AgentStatus.Current)), nil
	case "instance-status":
		return query.NewString(string(m.MachineInfo.InstanceStatus.Current)), nil
	case "series":
		return query.NewString(m.MachineInfo.Series), nil
	case "container-type":
		return query.NewString(m.MachineInfo.ContainerType), nil
	case "config":
		return query.NewMapStringInterface(m.MachineInfo.Config), nil
	case "supported-containers":
		containerTypes := make([]string, len(m.MachineInfo.SupportedContainers))
		for i, v := range m.MachineInfo.SupportedContainers {
			containerTypes[i] = string(v)
		}
		return query.NewSliceString(containerTypes), nil
	}
	return nil, errors.Annotatef(query.ErrInvalidIdentifier(name), "Runtime Error: identifier %q not found on MachineInfo", name)
}

func outputMachineSummary(writer io.Writer, scopedContext ScopeContext, machineInfo *params.MachineInfo) {
	result := struct {
		Elements map[string]interface{} `yaml:"properties"`
	}{
		Elements: make(map[string]interface{}),
	}

	idents := scopedContext.RecordedIdents()
	for _, ident := range idents {
		scope := MakeMachineScope(scopedContext, machineInfo)
		box, err := scope.GetIdentValue(ident)
		if err != nil {
			continue
		}
		result.Elements[ident] = box.Value()
	}

	_ = yaml.NewEncoder(writer).Encode(result)
}
