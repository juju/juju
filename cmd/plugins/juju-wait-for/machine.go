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
Wait for a given machine to reach a goal state.

arguments:
name
   machine name identifier

options:
--query (= 'life=="alive" && status=="started")
   query represents the goal state of a given machine
`

// machineCommand defines a command for waiting for models.
type machineCommand struct {
	waitForCommandBase

	id      string
	query   string
	timeout time.Duration
	found   bool
}

// Info implements Command.Info.
func (c *machineCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "machine",
		Args:    "[<id>]",
		Purpose: "wait for an machine to reach a goal state",
		Doc:     machineCommandDoc,
	})
}

// SetFlags implements Command.SetFlags.
func (c *machineCommand) SetFlags(f *gnuflag.FlagSet) {
	c.waitForCommandBase.SetFlags(f)
	f.StringVar(&c.query, "query", `life=="alive" && status=="started"`, "query the goal state")
	f.DurationVar(&c.timeout, "timeout", time.Minute*10, "how long to wait, before timing out")
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
	client, err := c.newWatchAllAPIFunc()
	if err != nil {
		return errors.Trace(err)
	}

	strategy := &Strategy{
		Client:  client,
		Timeout: c.timeout,
	}
	err = strategy.Run(c.id, c.query, c.waitFor)
	return errors.Trace(err)
}

func (c *machineCommand) waitFor(id string, deltas []params.Delta, q query.Query) (bool, error) {
	for _, delta := range deltas {
		logger.Tracef("delta %T: %v", delta.Entity, delta.Entity)

		switch entityInfo := delta.Entity.(type) {
		case *params.MachineInfo:
			if entityInfo.Id == id {
				scope := MakeMachineScope(entityInfo)
				if res, err := q.BuiltinsRun(scope); query.IsInvalidIdentifierErr(err) {
					return false, invalidIdentifierError(scope, err)
				} else if query.IsRuntimeError(err) {
					return false, errors.Trace(err)
				} else if res && err == nil {
					return true, nil
				} else if err != nil {
					logger.Errorf("%v", err)
				}
				c.found = entityInfo.Life != life.Dead
			}
			break
		}
	}

	if !c.found {
		logger.Infof("machine %q not found, waiting...", id)
		return false, nil
	}

	logger.Infof("machine %q found, waiting...", id)
	return false, nil
}

// MachineScope allows the query to introspect a model entity.
type MachineScope struct {
	GenericScope
	MachineInfo *params.MachineInfo
}

// MakeMachineScope creates an MachineScope from an MachineInfo
func MakeMachineScope(info *params.MachineInfo) MachineScope {
	return MachineScope{
		GenericScope: GenericScope{
			Info: info,
		},
		MachineInfo: info,
	}
}

// GetIdentValue returns the value of the identifier in a given scope.
func (m MachineScope) GetIdentValue(name string) (query.Ord, error) {
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
	}
	return nil, errors.Annotatef(query.ErrInvalidIdentifier(name), "Runtime Error: identifier %q not found on MachineInfo", name)
}
