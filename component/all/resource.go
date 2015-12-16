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

// resources exposes the registration methods needed
// for the top-level component machinery.
type resources struct{}

// RegisterForServer is the top-level registration method
// for the component in a jujud context.
func (r resources) registerForServer() error {
	r.registerPublicFacade()
	return nil
}

// RegisterForClient is the top-level registration method
// for the component in a "juju" command context.
func (r resources) registerForClient() error {
	r.registerPublicCommands()
	return nil
}

// registerPublicFacade adds the resources public API facade
// to the API server.
func (r resources) registerPublicFacade() {
	common.RegisterStandardFacade(
		resource.ComponentName,
		server.Version,
		r.newPublicFacade,
	)
}

// newPublicFacade is passed into common.RegisterStandardFacade
// in registerPublicFacade.
func (resources) newPublicFacade(st *corestate.State, _ *common.Resources, authorizer common.Authorizer) (*server.Facade, error) {
	if !authorizer.AuthClient() {
		return nil, common.ErrPerm
	}

	rst := state.NewState(&resourceState{raw: st})
	return server.NewFacade(rst), nil
}

// resourceState is a wrapper around state.State that supports the needs
// of resources.
type resourceState struct {
	raw *corestate.State
}

// CharmMetadata implements resource/state.RawState.
func (st resourceState) CharmMetadata(serviceID string) (*charm.Meta, error) {
	meta, err := utils.CharmMetadata(st.raw, serviceID)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return meta, nil
}

// resourcesApiClient adds a Close() method to the resources public API client.
type resourcesAPIClient struct {
	*client.Client
	closeConnFunc func() error
}

// Close implements io.Closer.
func (client resourcesAPIClient) Close() error {
	return client.closeConnFunc()
}

// newAPIClient builds a new resources public API client from
// the provided API caller.
func (resources) newAPIClient(apiCaller coreapi.Connection) (*resourcesAPIClient, error) {
	caller := base.NewFacadeCallerForVersion(apiCaller, resource.ComponentName, server.Version)

	cl := &resourcesAPIClient{
		Client:        client.NewClient(caller),
		closeConnFunc: apiCaller.Close,
	}

	return cl, nil
}

// registerPublicCommands adds the resources-related commands
// to the "juju" supercommand.
func (r resources) registerPublicCommands() {
	if !markRegistered(resource.ComponentName, "public-commands") {
		return
	}

	newShowAPIClient := func(command *cmd.ShowCommand) (cmd.CharmResourceLister, error) {
		//apiCaller, err := command.NewAPIRoot()
		//if err != nil {
		//	return nil, errors.Trace(err)
		//}
		//return r.newAPIClient(apiCaller)
		// TODO(ericsnow) finish!
		return nil, errors.Errorf("not implemented")
	}
	commands.RegisterEnvCommand(func() envcmd.EnvironCommand {
		return cmd.NewShowCommand(newShowAPIClient)
	})
}
