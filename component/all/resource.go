// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package all

import (
	"io"
	"os"
	"reflect"

	jujucmd "github.com/juju/cmd"
	"github.com/juju/errors"
	"github.com/juju/names"

	coreapi "github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/apiserver/charmrevisionupdater"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/common/apihttp"
	"github.com/juju/juju/cmd/juju/charmcmd"
	"github.com/juju/juju/cmd/juju/commands"
	"github.com/juju/juju/cmd/modelcmd"
	"github.com/juju/juju/resource"
	"github.com/juju/juju/resource/api"
	"github.com/juju/juju/resource/api/client"
	internalclient "github.com/juju/juju/resource/api/private/client"
	internalserver "github.com/juju/juju/resource/api/private/server"
	"github.com/juju/juju/resource/api/server"
	"github.com/juju/juju/resource/cmd"
	"github.com/juju/juju/resource/context"
	contextcmd "github.com/juju/juju/resource/context/cmd"
	"github.com/juju/juju/resource/resourceadapters"
	corestate "github.com/juju/juju/state"
	unitercontext "github.com/juju/juju/worker/uniter/runner/context"
	"github.com/juju/juju/worker/uniter/runner/jujuc"
)

// resources exposes the registration methods needed
// for the top-level component machinery.
type resources struct{}

// RegisterForServer is the top-level registration method
// for the component in a jujud context.
func (r resources) registerForServer() error {
	r.registerState()
	r.registerAgentWorkers()
	r.registerPublicFacade()
	r.registerHookContext()
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
	if !markRegistered(resource.ComponentName, "public-facade") {
		return
	}

	common.RegisterStandardFacade(
		resource.ComponentName,
		server.Version,
		resourceadapters.NewPublicFacade,
	)
	coreapi.RegisterFacadeVersion(resource.ComponentName, server.Version)

	common.RegisterAPIModelEndpoint(api.HTTPEndpointPattern, apihttp.HandlerSpec{
		Constraints: apihttp.HandlerConstraints{
			AuthKind:            names.UserTagKind,
			StrictValidation:    true,
			ControllerModelOnly: false,
		},
		NewHandler: resourceadapters.NewUploadHandler,
	})
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

// registerAgentWorkers adds the resources workers to the agents.
func (r resources) registerAgentWorkers() {
	if !markRegistered(resource.ComponentName, "agent-workers") {
		return
	}

	charmrevisionupdater.RegisterLatestCharmHandler("resources", resourceadapters.NewLatestCharmHandler)
}

// registerState registers the state functionality for resources.
func (resources) registerState() {
	if !markRegistered(resource.ComponentName, "state") {
		return
	}

	corestate.SetResourcesComponent(resourceadapters.NewResourceState)
	corestate.SetResourcesPersistence(resourceadapters.NewResourcePersistence)
	corestate.RegisterCleanupHandler(corestate.CleanupKindResourceBlob, resourceadapters.CleanUpBlob)
}

// registerPublicCommands adds the resources-related commands
// to the "juju" supercommand.
func (r resources) registerPublicCommands() {
	if !markRegistered(resource.ComponentName, "public-commands") {
		return
	}

	charmcmd.RegisterSubCommand(func(spec charmcmd.CharmstoreSpec) jujucmd.Command {
		base := charmcmd.NewCommandBase(spec)
		resBase := &resourceadapters.CharmCmdBase{base}
		return cmd.NewListCharmResourcesCommand(resBase)
	})

	commands.RegisterEnvCommand(func() modelcmd.ModelCommand {
		return cmd.NewUploadCommand(cmd.UploadDeps{
			NewClient: func(c *cmd.UploadCommand) (cmd.UploadClient, error) {
				return resourceadapters.NewAPIClient(c.NewAPIRoot)
			},
			OpenResource: func(s string) (cmd.ReadSeekCloser, error) {
				return os.Open(s)
			},
		})

	})

	commands.RegisterEnvCommand(func() modelcmd.ModelCommand {
		return cmd.NewShowServiceCommand(cmd.ShowServiceDeps{
			NewClient: func(c *cmd.ShowServiceCommand) (cmd.ShowServiceClient, error) {
				return resourceadapters.NewAPIClient(c.NewAPIRoot)
			},
		})
	})
}

// TODO(katco): This seems to be common across components. Pop up a
// level and genericize?
func (r resources) registerHookContext() {
	if markRegistered(resource.ComponentName, "hook-context") == false {
		return
	}

	unitercontext.RegisterComponentFunc(
		resource.ComponentName,
		func(config unitercontext.ComponentConfig) (jujuc.ContextComponent, error) {
			unitID := names.NewUnitTag(config.UnitName).String()
			hctxClient, err := r.newUnitFacadeClient(unitID, config.APICaller)
			if err != nil {
				return nil, errors.Trace(err)
			}
			// TODO(ericsnow) Pass the unit's tag through to the component?
			return context.NewContextAPI(hctxClient, config.DataDir), nil
		},
	)

	r.registerHookContextCommands()
	r.registerHookContextFacade()
}

func (c resources) registerHookContextCommands() {
	if markRegistered(resource.ComponentName, "hook-context-commands") == false {
		return
	}

	jujuc.RegisterCommand(
		contextcmd.GetCmdName,
		func(ctx jujuc.Context) (jujucmd.Command, error) {
			compCtx, err := ctx.Component(resource.ComponentName)
			if err != nil {
				return nil, errors.Trace(err)
			}
			typedCtx, ok := compCtx.(*context.Context)
			if !ok {
				return nil, errors.Trace(err)
			}
			cmd, err := contextcmd.NewGetCmd(typedCtx)
			if err != nil {
				return nil, errors.Trace(err)
			}
			return cmd, nil
		},
	)
}

func (r resources) registerHookContextFacade() {
	common.RegisterHookContextFacade(
		context.HookContextFacade,
		internalserver.FacadeVersion,
		r.newHookContextFacade,
		reflect.TypeOf(&internalserver.UnitFacade{}),
	)
	coreapi.RegisterFacadeVersion(context.HookContextFacade, internalserver.FacadeVersion)
}

// resourcesUnitDatastore is a shim to elide serviceName from
// ListResources.
type resourcesUnitDataStore struct {
	resources corestate.Resources
	unit      *corestate.Unit
}

// ListResources implements resource/api/private/server.UnitDataStore.
func (ds *resourcesUnitDataStore) ListResources() (resource.ServiceResources, error) {
	return ds.resources.ListResources(ds.unit.ServiceName())
}

// GetResource implements resource/api/private/server.UnitDataStore.
func (ds *resourcesUnitDataStore) GetResource(name string) (resource.Resource, error) {
	return ds.resources.GetResource(ds.unit.ServiceName(), name)
}

// OpenResource implements resource/api/private/server.UnitDataStore.
func (ds *resourcesUnitDataStore) OpenResource(name string) (resource.Resource, io.ReadCloser, error) {
	return ds.resources.OpenResourceForUniter(ds.unit, name)
}

func (r resources) newHookContextFacade(st *corestate.State, unit *corestate.Unit) (interface{}, error) {
	res, err := st.Resources()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return internalserver.NewUnitFacade(&resourcesUnitDataStore{res, unit}), nil
}

func (r resources) newUnitFacadeClient(unitName string, caller base.APICaller) (context.APIClient, error) {

	facadeCaller := base.NewFacadeCallerForVersion(caller, context.HookContextFacade, internalserver.FacadeVersion)
	httpClient, err := caller.HTTPClient()
	if err != nil {
		return nil, errors.Trace(err)
	}
	unitHTTPClient := internalclient.NewUnitHTTPClient(httpClient, unitName)

	return internalclient.NewUnitFacadeClient(facadeCaller, unitHTTPClient), nil
}
