// Copyright 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package commands

import (
	"fmt"

	"github.com/juju/cmd"
	"github.com/juju/errors"

	apicrossmodel "github.com/juju/juju/api/crossmodel"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/cmd/juju/block"
	"github.com/juju/juju/model/crossmodel"
)

const addRelationDoc = `
Add a relation between 2 local services or a local and a remote service.
Adding a relation between two remote services is not supported.

Services can be identified either by:
    <service name>[:<relation name>]
or
    <remote service url>

Examples:
    $ juju add-relation wordpress mysql
    $ juju add-relation wordpress local:/u/fred/db2
    $ juju add-relation wordpress vendor:/u/ibm/hosted-db2

`

func newAddRelationCommand() cmd.Command {
	addRelationCmd := &addRelationCommand{}
	addRelationCmd.newAPIFunc = func() (AddRelationAPI, error) {
		root, err := addRelationCmd.NewAPIRoot()
		if err != nil {
			return nil, err
		}
		return apicrossmodel.NewClient(root), nil
	}
	return envcmd.Wrap(addRelationCmd)
}

// addRelationCommand adds a relation between two service endpoints.
type addRelationCommand struct {
	envcmd.EnvCommandBase
	Endpoints []string

	newAPIFunc func() (AddRelationAPI, error)
}

func (c *addRelationCommand) Info() *cmd.Info {
	return &cmd.Info{
		Name:    "add-relation",
		Args:    "<service 1> <service2>",
		Purpose: "add a relation between two services",
		Doc:     addRelationDoc,
	}
}

func (c *addRelationCommand) Init(args []string) error {
	if len(args) != 2 {
		return fmt.Errorf("a relation must involve two services")
	}

	c.Endpoints = args
	return nil
}

func (c *addRelationCommand) Run(_ *cmd.Context) error {
	api, err := c.newAPIFunc()
	if err != nil {
		return err
	}
	defer api.Close()

	if api.BestAPIVersion() > 0 {
		_, err = api.AddRelation(c.Endpoints...)
		// if has access to new facade, use it; otherwise fall through to the old one further down..
		if !params.IsCodeNotImplemented(err) {
			return block.ProcessBlockedError(err, block.BlockChange)
		}
	}

	if c.hasRemoteServices() {
		// old client does not have cross-model capability.
		return errors.NotSupportedf("add relation between %v remote services", c.Endpoints)
	}

	oldAPI, err := c.NewAPIClient()
	if err != nil {
		return err
	}
	defer oldAPI.Close()

	_, err = oldAPI.AddRelation(c.Endpoints...)
	return block.ProcessBlockedError(err, block.BlockChange)
}

// hasRemoteServices determines if any of the command arguments is a remote service.
func (c *addRelationCommand) hasRemoteServices() bool {
	for _, endpoint := range c.Endpoints {
		if _, err := crossmodel.ParseServiceURL(endpoint); err == nil {
			return true
		}
	}
	return false
}

// AddRelationAPI defines the API methods that can add relation between services.
type AddRelationAPI interface {
	Close() error
	BestAPIVersion() int
	AddRelation(endpoints ...string) (crossmodel.AddRelationResults, error)
}
