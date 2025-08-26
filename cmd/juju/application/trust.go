// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package application

import (
	"fmt"

	"github.com/juju/cmd/v3"
	"github.com/juju/errors"
	"github.com/juju/gnuflag"

	"github.com/juju/juju/api/client/application"
	appfacade "github.com/juju/juju/apiserver/facades/client/application"
	jujucmd "github.com/juju/juju/cmd"
	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/core/model"
)

const (
	trustSummary = `Sets the trust status of a deployed application to true.`
	trustDetails = `Sets the trust configuration value to true.

On Kubernetes models, the ` + "`trust`" + ` operation currently grants the charm full access to the cluster.
Until the permissions model is refined to grant more granular role-based access, the use of
` + "`--scope=cluster`" + ` is required to confirm this choice.
`

	trustExamples = `
    juju trust media-wiki
    juju trust metallb --scope=cluster
`
	clusterScopeError = `'juju trust' currently grants full access to the cluster itself.
Set the scope to 'cluster' using '--scope=cluster' to confirm this choice.
`
)

type trustCommand struct {
	modelcmd.ModelCommandBase
	api ApplicationAPI

	applicationName string
	removeTrust     bool
	scope           string
}

func NewTrustCommand() cmd.Command {
	return modelcmd.Wrap(&trustCommand{})
}

// Info is part of the cmd.Command interface.
func (c *trustCommand) Info() *cmd.Info {
	return jujucmd.Info(&cmd.Info{
		Name:     "trust",
		Args:     "<application name>",
		Purpose:  trustSummary,
		Doc:      trustDetails,
		Examples: trustExamples,
		SeeAlso: []string{
			"config",
		},
	})
}

// SetFlags is part of the cmd.Command interface.
func (c *trustCommand) SetFlags(f *gnuflag.FlagSet) {
	c.ModelCommandBase.SetFlags(f)
	f.BoolVar(&c.removeTrust, "remove", false, "Remove trusted access from a trusted application")
	f.StringVar(&c.scope, "scope", "", "(Kubernetes models only:) Needs to be set to `cluster`")
}

// getAPI either uses the fake API set at test time or that is nil, gets a real
// API and sets that as the API.
func (c *trustCommand) getAPI() (ApplicationAPI, error) {
	if c.api != nil {
		return c.api, nil
	}
	root, err := c.NewAPIRoot()
	if err != nil {
		return nil, errors.Trace(err)
	}
	client := application.NewClient(root)
	return client, nil
}

// Init is part of the cmd.Command interface.
func (c *trustCommand) Init(args []string) error {
	if len(args) == 0 {
		return errors.New("no application name specified")
	}
	c.applicationName = args[0]
	return nil
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

	// Set trust config value
	client, err := c.getAPI()
	if err != nil {
		return errors.Trace(err)
	}
	defer func() { _ = client.Close() }()

	err = client.SetConfig("", c.applicationName, "",
		map[string]string{appfacade.TrustConfigOptionName: fmt.Sprint(!c.removeTrust)},
	)
	return errors.Trace(block.ProcessBlockedError(err, block.BlockChange))
}
