// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"strings"

	"github.com/juju/cmd/v3"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/names/v5"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/client/application"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/cmd/modelcmd"
	corebase "github.com/juju/juju/core/base"
)

// NewSetApplicationBaseCommand returns a command which updates the base of
// an application.
func NewSetApplicationBaseCommand() cmd.Command {
	return modelcmd.Wrap(&setApplicationBase{})
}

// setApplicationBaseAPI defines a subset of the application facade, as required
// by the set-application-base command.
type setApplicationBaseAPI interface {
	Close() error
	UpdateApplicationBase(string, corebase.Base, bool) error
}

// setApplicationBase is responsible for updating the base of an application.
type setApplicationBase struct {
	modelcmd.ModelCommandBase
	modelcmd.IAASOnlyCommand

	apiClient setApplicationBaseAPI

	applicationName string
	releaseArg      string
}

var setApplicationBaseDoc = `
The specified application's base value will be set within juju. Any subordinates
of the application will also have their base set to the provided value. A base
can be specified using the OS name and the version of the OS, separated by ` + "`@`" + `.

This will not change the base of any existing units, rather new units will use
the new base when deployed.

It is recommended to only do this after upgrade-machine has been run for
machine containing all existing units of the application.

To ensure correct binaries, run ` + "`juju refresh`" + ` before running ` + "`juju add-unit`" + `.
`

const setApplicationBaseExamples = `
Set the base for the ` + "`ubuntu`" + ` application to ` + "`ubuntu@20.04`" + `:

	juju set-application-base ubuntu ubuntu@20.04
`

func (c *setApplicationBase) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "set-application-base",
		Args:     "<application> <base>",
		Purpose:  "Set an application's base.",
		Doc:      setApplicationBaseDoc,
		Examples: setApplicationBaseExamples,
		SeeAlso: []string{
			"status",
			"refresh",
			"upgrade-machine",
		},
	})
}

func (c *setApplicationBase) SetFlags(f *gnuflag.FlagSet) {
	c.ModelCommandBase.SetFlags(f)
}

// Init implements cmd.Command.
func (c *setApplicationBase) Init(args []string) error {
	switch len(args) {
	case 2:
		if names.IsValidApplication(args[0]) {
			c.applicationName = args[0]
		} else {
			return errors.Errorf("invalid application name %q", args[0])
		}
		c.releaseArg = args[1]
	case 1:
		if strings.Contains(args[0], "@") {
			return errors.Errorf("no application name")
		} else {
			return errors.Errorf("no base specified")
		}
	case 0:
		return errors.Errorf("application name and base required")
	default:
		return cmd.CheckEmpty(args[2:])
	}
	return nil
}

// Run implements cmd.Run.
func (c *setApplicationBase) Run(ctx *cmd.Context) error {
	var apiRoot api.Connection
	if c.apiClient == nil {
		var err error
		apiRoot, err = c.NewAPIRoot()
		if err != nil {
			return errors.Trace(err)
		}
		defer func() { _ = apiRoot.Close() }()
	}

	if c.applicationName != "" {
		if c.apiClient == nil {
			c.apiClient = application.NewClient(apiRoot)
			defer func() { _ = c.apiClient.Close() }()
		}

		base, err := c.parseBase(ctx, c.releaseArg)
		if err != nil {
			return errors.Trace(err)
		}

		err = c.updateApplicationBase(base)
		if err == nil {
			// TODO hmlanigan 2022-01-18
			// Remove warning once improvements to develop are made, where by
			// set-application-base downloads the new charm. Or this command is removed.
			// subordinate
			ctx.Warningf("To ensure the correct charm binaries are installed when add-unit is next called, please first run `juju refresh` for this application and any related subordinates.")
		}
		return err
	}

	// This should never happen...
	return errors.New("no application name specified")
}

func (c *setApplicationBase) updateApplicationBase(base corebase.Base) error {
	err := block.ProcessBlockedError(
		c.apiClient.UpdateApplicationBase(c.applicationName, base, false),
		block.BlockChange)

	return err
}

func (c *setApplicationBase) parseBase(ctx *cmd.Context, arg string) (corebase.Base, error) {
	// If this doesn't contain an @ then it's a series and not a base.
	if strings.Contains(arg, "@") {
		return corebase.ParseBaseFromString(arg)
	}

	ctx.Warningf("series argument is deprecated, use base instead")
	return corebase.GetBaseFromSeries(arg)
}
