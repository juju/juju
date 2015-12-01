// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package all

import (
	"github.com/juju/errors"
	"gopkg.in/juju/charm.v6-unstable"

	coreapi "github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/cmd/juju/commands"
	"github.com/juju/juju/resource"
	"github.com/juju/juju/resource/api/client"
	"github.com/juju/juju/resource/api/server"
	"github.com/juju/juju/resource/cmd"
	"github.com/juju/juju/resource/state"
	corestate "github.com/juju/juju/state"
	"github.com/juju/juju/state/utils"
)

type resources struct{}

func (c resources) registerForServer() error {
	c.registerPublicFacade()
	return nil
}

func (c resources) registerForClient() error {
	c.registerPublicCommands()
	return nil
}

func (c resources) registerPublicFacade() error {
	common.RegisterStandardFacade(
		resource.ComponentName,
		server.Version,
		c.newPublicFacade,
	)
	return nil
}

func (resources) newPublicFacade(st *corestate.State, _ *common.Resources, _ common.Authorizer) (*server.Facade, error) {
	rst := state.NewState(&resourceState{raw: st})
	return server.NewFacade(rst), nil
}

type resourceState struct {
	raw *corestate.State
}

func (st resourceState) CharmMetadata(serviceID string) (*charm.Meta, error) {
	meta, err := utils.CharmMetadata(st.raw, serviceID)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return meta, nil
}

type resourcesAPIClient struct {
	*client.Client
	closeFunc func() error
}

func (c resourcesAPIClient) Close() error {
	return c.closeFunc()
}

func (resources) newAPIClient(apiCaller coreapi.Connection) (*resourcesAPIClient, error) {
	caller := base.NewFacadeCallerForVersion(apiCaller, resource.ComponentName, server.Version)

	cl := &resourcesAPIClient{
		Client:    client.NewClient(caller),
		closeFunc: apiCaller.Close,
	}

	return cl, nil
}

func (c resources) registerPublicCommands() {
	if !markRegistered(resource.ComponentName, "public-commands") {
		return
	}

	commands.RegisterEnvCommand(func() envcmd.EnvironCommand {
		return cmd.NewShowCommand(c.newShowAPIClient)
	})
}

func (c resources) newShowAPIClient(command *cmd.ShowCommand) (cmd.ShowAPI, error) {
	apiCaller, err := command.NewAPIRoot()
	if err != nil {
		return nil, errors.Trace(err)
	}

	return c.newAPIClient(apiCaller)
}
