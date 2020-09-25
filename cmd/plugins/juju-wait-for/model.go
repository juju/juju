// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/names/v4"

	"github.com/juju/juju/apiserver/params"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
)

func newModelCommand() cmd.Command {
	cmd := &modelCommand{}
	cmd.newWatchAllAPIFunc = func() (WatchAllAPI, error) {
		client, err := cmd.NewAPIClient()
		if err != nil {
			return nil, errors.Trace(err)
		}
		return watchAllAPIShim{
			Client: client,
		}, nil
	}
	cmd.waitForFn = cmd.waitFor
	return modelcmd.Wrap(cmd)
}

const modelCommandDoc = `
Wait for a given model to reach a goal state.

arguments:
name
   model name identifier

options:
--status (= "available")
   status of the model to wait-for
--life (= "alive")
   life of the model to wait-for
`

// modelCommand stores image metadata in Juju environment.
type modelCommand struct {
	waitForCommandBase

	life   string
	status string

	predicate Predicate
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

	predicates := map[string]Predicate{
		"life":   LifePredicate("alive"),
		"status": StatusPredicate("available"),
	}
	if c.life != "" {
		predicates["life"] = LifePredicate(c.life)
	}
	if c.status != "" {
		predicates["status"] = StatusPredicate(c.status)
	}
	c.predicate = ComposePredicates(predicates)

	return nil
}

// Info implements Command.Info.
func (c *modelCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "model",
		Args:    "[<name>]",
		Purpose: "wait for an application to reach a goal state",
		Doc:     modelCommandDoc,
	})
}

// SetFlags implements Command.SetFlags.
func (c *modelCommand) SetFlags(f *gnuflag.FlagSet) {
	c.waitForCommandBase.SetFlags(f)
	f.StringVar(&c.life, "life", "", "goal state for the life of a model")
	f.StringVar(&c.status, "status", "", "goal state for the status of a model")
}

func (c *modelCommand) waitFor(name string, state State, deltas []params.Delta) State {
	for _, delta := range deltas {
		switch entityInfo := delta.Entity.(type) {
		case *params.ModelUpdate:
			if entityInfo.Name == name {
				if c.predicate(entityInfo) {
					state.Complete = true
					return state
				}
			}
			state.Found = true
			break
		}
	}

	if !state.Found {
		logger.Infof("model %q not found, waiting...", name)
		return state
	}

	logger.Infof("model %q found, waiting...", name)
	return state
}
