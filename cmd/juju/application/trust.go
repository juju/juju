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
	"github.com/juju/juju/core/model"
)

const (
	trustSummary = `Sets the trust status of a deployed application to true.`
	trustDetails = `Sets the trust configuration value to true.

On k8s models, the trust operation currently grants the charm full access to the cluster.
Until the permissions model is refined to grant more granular role based access, the use of
'--scope=cluster' is required to confirm this choice.

Examples:
    juju trust media-wiki
    juju trust metallb --scope=cluster

See also:
    config
`
	clusterScopeError = `'juju trust' currently grants full access to the cluster itself.
Set the scope to 'cluster' using '--scope=cluster' to confirm this choice.
`
)

type trustCommand struct {
	configCommand
	removeTrust bool
	scope       string
}

//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/applicationapi_mock.go github.com/juju/juju/cmd/juju/application ApplicationAPI

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
	f.StringVar(&c.scope, "scope", "", "k8s models only - needs to be set to 'cluster'")
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

func (c *trustCommand) Run(ctx *cmd.Context) error {
	modelType, err := c.ModelType()
	if err != nil {
		return errors.Trace(err)
	}
	if modelType == model.CAAS && !c.removeTrust {
		if c.scope == "" {
			return errors.New(clusterScopeError)
		}
		if c.scope != "cluster" {
			return errors.NotValidf("scope %q", c.scope)
		}
	}
	return c.configCommand.Run(ctx)
}
