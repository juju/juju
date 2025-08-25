// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package setmeterstatus

import (
	"strings"

	"github.com/juju/cmd/v3"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"
	"github.com/juju/names/v5"

	"github.com/juju/juju/api/client/metricsdebug"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
)

const setMeterStatusDoc = `
Set meter status on the given application or unit. This command is used
to test the ` + "`meter-status-changed`" + ` hook for charms in development.
`

const setMeterStatusExamples = `
    juju set-meter-status myapp RED
    juju set-meter-status myapp/0 AMBER --info "my message"

`

// SetMeterStatusCommand sets the meter status on an application or unit. Useful for charm authors.
type SetMeterStatusCommand struct {
	modelcmd.ModelCommandBase
	Tag        names.Tag
	Status     string
	StatusInfo string
}

// New creates a new SetMeterStatusCommand.
func New() cmd.Command {
	return modelcmd.Wrap(&SetMeterStatusCommand{})
}

// Info implements Command.Info.
func (c *SetMeterStatusCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "set-meter-status",
		Args:     "[application or unit] status",
		Purpose:  "Sets the meter status on an application or unit.",
		Doc:      setMeterStatusDoc,
		Examples: setMeterStatusExamples,
	})
}

// Init reads and verifies the cli arguments for the SetMeterStatusCommand
func (c *SetMeterStatusCommand) Init(args []string) error {
	if len(args) != 2 {
		return errors.New("you need to specify an entity (application or unit) and a status")
	}
	if names.IsValidUnit(args[0]) {
		c.Tag = names.NewUnitTag(args[0])
	} else if names.IsValidApplication(args[0]) {
		c.Tag = names.NewApplicationTag(args[0])
	} else {
		return errors.Errorf("%q is not a valid unit or application", args[0])
	}
	c.Status = args[1]

	if err := cmd.CheckEmpty(args[2:]); err != nil {
		return errors.Errorf("unknown command line arguments: %s", strings.Join(args, ","))
	}
	return nil
}

// SetFlags implements Command.SetFlags.
func (c *SetMeterStatusCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ModelCommandBase.SetFlags(f)
	f.StringVar(&c.StatusInfo, "info", "", "Set the meter status info to this string")
}

// SetMeterStatusClient defines the juju api required by the command.
type SetMeterStatusClient interface {
	SetMeterStatus(tag, status, info string) error
	Close() error
}

var newClient = func(env modelcmd.ModelCommandBase) (SetMeterStatusClient, error) {
	state, err := env.NewAPIRoot()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return metricsdebug.NewClient(state), nil
}

// Run implements Command.Run.
func (c *SetMeterStatusCommand) Run(ctx *cmd.Context) error {
	client, err := newClient(c.ModelCommandBase)
	if err != nil {
		return errors.Trace(err)
	}
	defer client.Close()
	err = client.SetMeterStatus(c.Tag.String(), c.Status, c.StatusInfo)
	if err != nil {
		return errors.Trace(err)
	}
	return nil
}
