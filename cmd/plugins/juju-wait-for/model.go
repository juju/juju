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
	cmd := &modelCommand{}
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

func (c *modelCommand) waitFor(name string, deltas []params.Delta, q query.Query) (bool, error) {
	for _, delta := range deltas {
		logger.Tracef("delta %T: %v", delta.Entity, delta.Entity)

		switch entityInfo := delta.Entity.(type) {
		case *params.ModelUpdate:
			if entityInfo.Name == name {
				scope := MakeModelScope(entityInfo)
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
		logger.Infof("model %q not found, waiting...", name)
		return false, nil
	}

	logger.Infof("model %q found, waiting...", name)
	return false, nil
}

// ModelScope allows the query to introspect a model entity.
type ModelScope struct {
	GenericScope
	ModelInfo *params.ModelUpdate
}

// MakeModelScope creates an ModelScope from an ModelUpdate
func MakeModelScope(info *params.ModelUpdate) ModelScope {
	return ModelScope{
		GenericScope: GenericScope{
			Info: info,
		},
		ModelInfo: info,
	}
}

// GetIdentValue returns the value of the identifier in a given scope.
func (m ModelScope) GetIdentValue(name string) (query.Ord, error) {
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
	}
	return nil, errors.Annotatef(query.ErrInvalidIdentifier(name), "Runtime Error: identifier %q not found on ModelInfo", name)
}
