// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"fmt"

	"github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"

	"github.com/juju/juju/apiserver/facades/client/application"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/modelcmd"
)

const (
	trustSummary = `Sets the trust status of a deployed application to true.`
	trustDetails = `Sets the trust configuration value to true.

Examples:
    juju trust media-wiki

See also:
    config
`
)

type trustCommand struct {
	configCommand
	removeTrust bool
}

func NewTrustCommand() cmd.Command {
	return modelcmd.Wrap(&trustCommand{})
}

// Info is part of the cmd.Command interface.
func (c *trustCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:    "trust",
		Args:    "<application name>",
		Purpose: trustSummary,
		Doc:     trustDetails,
	})
}

// SetFlags is part of the cmd.Command interface.
func (c *trustCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ModelCommandBase.SetFlags(f)
	f.BoolVar(&c.removeTrust, "remove", false, "Remove trusted access from a trusted application")
}

// Init is part of the cmd.Command interface.
func (c *trustCommand) Init(args []string) error {
	if len(args) == 0 {
		return errors.New("no application name specified")
	}
	c.applicationName = args[0]
	var trustOptionPair string
	trustOptionPair = fmt.Sprintf("%s=%t", application.TrustConfigOptionName, !c.removeTrust)
	return c.parseSet([]string{trustOptionPair})
}
