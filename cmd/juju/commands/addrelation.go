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

		if !addRelationCmd.HasRemoteService {
			return root.Client(), nil
		}
		// Only return cross-model-capable client if needed
		return apicrossmodel.NewClient(root), nil
	}
	return envcmd.Wrap(addRelationCmd)
}

// addRelationCommand adds a relation between two service endpoints.
type addRelationCommand struct {
	envcmd.EnvCommandBase
	Endpoints []string

	// HasRemoteService indicates if one of the command arguments is a remote service.
	HasRemoteService bool

	// newAPIFunc is func to get API that is capable of adding relation between services.
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

	// Is 1st service remote?
	_, err := crossmodel.ParseServiceURL(args[0])
	firstRemote := err == nil

	// Is 2nd service remote?
	_, err2 := crossmodel.ParseServiceURL(args[1])
	secondRemote := err2 == nil

	if firstRemote && secondRemote {
		// Can't have relation between 2 remote services... yet
		return errors.NotSupportedf("add-relation between 2 remote services")
	}

	c.HasRemoteService = firstRemote || secondRemote

	c.Endpoints = args
	return nil
}

func (c *addRelationCommand) Run(_ *cmd.Context) error {
	api, err := c.newAPIFunc()
	if err != nil {
		return err
	}
	defer api.Close()

	_, err = api.AddRelation(c.Endpoints...)

	if params.IsCodeNotImplemented(err) {
		// Client does not support add relation for given services. This may happen if the client is old
		// and does not have cross-model capability.
		// Return a user friendly message :D
		return errors.Errorf("cannot add relation for services %v: not supported by the API server", c.Endpoints)
	}
	return block.ProcessBlockedError(err, block.BlockChange)
}

// AddRelationAPI defines the API methods that can add relation between services.
type AddRelationAPI interface {
	Close() error
	AddRelation(endpoints ...string) (*params.AddRelationResults, error)
}
