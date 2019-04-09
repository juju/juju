// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"os"
	"strconv"
	"time"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"

	"github.com/juju/juju/api/modelgeneration"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/juju/common"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/cmd/output"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/juju/osenv"
)

const (
	showGenerationSummary = `Displays a summary of the active "next" generation.`
	showGenerationDoc     = `
Summary information includes each application that has changes made under the
generation, with any units that have been advanced, and any changed config
values made to the generations.

Examples:
    juju show-generation
    juju show-generation --utc

See also:
    add-generation
    advance-generation
    cancel-generation
    switch-generation
`
)

// ShowGenerationCommandAPI defines an API interface to be used during testing.
//go:generate mockgen -package mocks -destination ./mocks/showgeneration_mock.go github.com/juju/juju/cmd/juju/model ShowGenerationCommandAPI
type ShowGenerationCommandAPI interface {
	Close() error
	GenerationInfo(string, string, func(time.Time) string) (model.GenerationSummaries, error)
}

// addGenerationCommand is the simplified command for accessing and setting
// attributes related to adding model generations.
type showGenerationCommand struct {
	modelcmd.ModelCommandBase

	api ShowGenerationCommandAPI
	out cmd.Output

	isoTime    bool
	branchName string
}

// NewShowGenerationCommand wraps showGenerationCommand with sane model settings.
func NewShowGenerationCommand() cmd.Command {
	return modelcmd.Wrap(&showGenerationCommand{})
}

// Info implements part of the cmd.Command interface.
func (c *showGenerationCommand) Info() *cmd.Info {
	info := &cmd.Info{
		Name:    "show-generation",
		Args:    "<branch name>",
		Purpose: showGenerationSummary,
		Doc:     showGenerationDoc,
	}
	return jujucmd.Info(info)
}

// SetFlags implements part of the cmd.Command interface.
func (c *showGenerationCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ModelCommandBase.SetFlags(f)
	f.BoolVar(&c.isoTime, "utc", false, "Display time as UTC in RFC3339 format")
	c.out.AddFlags(f, "yaml", output.DefaultFormatters)
}

// Init implements part of the cmd.Command interface.
func (c *showGenerationCommand) Init(args []string) error {
	if len(args) != 1 {
		return errors.Errorf("must specify a branch name")
	}
	c.branchName = args[0]

	// If use of ISO time not specified on command line, check env var.
	if !c.isoTime {
		var err error
		envVarValue := os.Getenv(osenv.JujuStatusIsoTimeEnvKey)
		if envVarValue != "" {
			if c.isoTime, err = strconv.ParseBool(envVarValue); err != nil {
				return errors.Annotatef(err, "invalid %s env var, expected true|false", osenv.JujuStatusIsoTimeEnvKey)
			}
		}
	}
	return nil
}

// getAPI returns the API. This allows passing in a test ShowGenerationCommandAPI
// implementation.
func (c *showGenerationCommand) getAPI() (ShowGenerationCommandAPI, error) {
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
func (c *showGenerationCommand) Run(ctx *cmd.Context) error {
	client, err := c.getAPI()
	if err != nil {
		return err
	}
	defer func() { _ = client.Close() }()

	_, modelDetails, err := c.ModelCommandBase.ModelDetails()
	if err != nil {
		return errors.Annotate(err, "getting model details")
	}

	// Partially apply our time format
	formatTime := func(t time.Time) string {
		return common.FormatTime(&t, c.isoTime)
	}

	deltas, err := client.GenerationInfo(modelDetails.ModelUUID, c.branchName, formatTime)
	if err != nil {
		return errors.Trace(err)
	}
	return errors.Trace(c.out.Write(ctx, deltas))
}
