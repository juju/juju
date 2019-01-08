// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"

	"github.com/juju/juju/api/modelgeneration"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
)

const (
	cancelGenerationSummary = "Cancels the new generation to the model."
	cancelGenerationDoc     = `
Cancel the next generation. This will abort anything in the next
generation and return the active target to the current generation. 

Examples:
    juju cancel-generation

See also:
    add-generation
    advance-generation
    switch-generation
`
)

// NewCancelGenerationCommand wraps cancelGenerationCommand with sane model settings.
func NewCancelGenerationCommand() cmd.Command {
	return modelcmd.Wrap(&cancelGenerationCommand{})
}

// cancelGenerationCommand is the simplified command for accessing and setting
// attributes related to canceling model generations.
type cancelGenerationCommand struct {
	modelcmd.ModelCommandBase

	api CancelGenerationCommandAPI
}

// CancelGenerationCommandAPI defines an API interface to be used during testing.
//go:generate mockgen -package model_test -destination ./cancelgenerationmock_test.go github.com/juju/juju/cmd/juju/model CancelGenerationCommandAPI
type CancelGenerationCommandAPI interface {
	Close() error
	CancelGeneration() error
}

// Info implements part of the cmd.Command interface.
func (c *cancelGenerationCommand) Info() *cmd.Info {
	info := &cmd.Info{
		Name:    "cancel-generation",
		Purpose: cancelGenerationSummary,
		Doc:     cancelGenerationDoc,
	}
	return jujucmd.Info(info)
}

// SetFlags implements part of the cmd.Command interface.
func (c *cancelGenerationCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ModelCommandBase.SetFlags(f)
}

// Init implements part of the cmd.Command interface.
func (c *cancelGenerationCommand) Init(args []string) error {
	if len(args) != 0 {
		return errors.Errorf("No arguments allowed")
	}
	return nil
}

// getAPI returns the API. This allows passing in a test CancelGenerationCommandAPI
// implementation.
func (c *cancelGenerationCommand) getAPI() (CancelGenerationCommandAPI, error) {
	if c.api != nil {
		return c.api, nil
	}
	api, err := c.NewAPIRoot()
	if err != nil {
		return nil, errors.Annotate(err, "opening API connection")
	}
	client := modelgeneration.NewClient(api)
	return client, nil
}

// Run implements the meaty part of the cmd.Command interface.
func (c *cancelGenerationCommand) Run(ctx *cmd.Context) error {
	client, err := c.getAPI()
	if err != nil {
		return err
	}
	defer client.Close()

	// TODO (hml) 20-12-2018
	// update to check err when CancelGeneration() is implemented in the
	// apiserver.
	client.CancelGeneration()

	ctx.Stdout.Write([]byte("changes dropped and target generation set to current\n"))
	return nil
}
