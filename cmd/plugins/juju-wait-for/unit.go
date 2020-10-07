// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"fmt"
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
	f.StringVar(&c.query, "query", `life=="alive" && status=="active"`, "query the goal state")
	f.DurationVar(&c.timeout, "timeout", time.Minute*10, "how long to wait, before timing out")
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
	client, err := c.newWatchAllAPIFunc()
	if err != nil {
		return errors.Trace(err)
	}

	strategy := &Strategy{
		Client:  client,
		Timeout: c.timeout,
	}
	err = strategy.Run(c.name, c.query, c.waitFor)
	return errors.Trace(err)
}

func (c *unitCommand) waitFor(name string, deltas []params.Delta, q query.Query) (bool, error) {
	for _, delta := range deltas {
		logger.Tracef("delta %T: %v", delta.Entity, delta.Entity)

		switch entityInfo := delta.Entity.(type) {
		case *params.UnitInfo:
			if entityInfo.Name == name {
				scope := MakeUnitScope(entityInfo)
				if res, err := q.Run(scope); query.IsInvalidIdentifierErr(err) {
					return false, invalidIdentifierError(scope, err)
				} else if res && err == nil {
					return true, nil
				}
				c.found = entityInfo.Life != life.Dead
			}
			break
		}
	}

	if !c.found {
		logger.Infof("unit %q not found, waiting...", name)
		return false, nil
	}

	logger.Infof("unit %q found, waiting...", name)
	return false, nil
}

// UnitScope allows the query to introspect a unit entity.
type UnitScope struct {
	GenericScope
	UnitInfo *params.UnitInfo
}

// MakeUnitScope creates an UnitScope from an UnitInfo
func MakeUnitScope(info *params.UnitInfo) UnitScope {
	return UnitScope{
		GenericScope: GenericScope{
			Info: info,
		},
		UnitInfo: info,
	}
}

// GetIdentValue returns the value of the identifier in a given scope.
func (m UnitScope) GetIdentValue(name string) (query.Ord, error) {
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
	case "agent-status":
		return query.NewString(string(m.UnitInfo.AgentStatus.Current)), nil
	}
	fmt.Println(">>", name)
	return nil, errors.Annotatef(query.ErrInvalidIdentifier(name), "Runtime Error: identifier %q not found on UnitInfo", name)
}
