// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package all

import (
	"bytes"
	"io"
	"os"
	"reflect"

	"github.com/juju/errors"
	"github.com/juju/names"
	"gopkg.in/juju/charm.v6-unstable"
	charmresource "gopkg.in/juju/charm.v6-unstable/resource"
	"gopkg.in/juju/charmrepo.v2-unstable"

	jujucmd "github.com/juju/cmd"
	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/cmd/envcmd"
	"github.com/juju/juju/cmd/juju/commands"
	"github.com/juju/juju/resource"
	"github.com/juju/juju/resource/api/client"
	internalclient "github.com/juju/juju/resource/api/private/client"
	internalserver "github.com/juju/juju/resource/api/private/server"
	"github.com/juju/juju/resource/api/server"
	"github.com/juju/juju/resource/cmd"
	"github.com/juju/juju/resource/context"
	contextcmd "github.com/juju/juju/resource/context/cmd"
	"github.com/juju/juju/resource/persistence"
	"github.com/juju/juju/resource/state"
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
		r.newPublicFacade,
	)
}

// newPublicFacade is passed into common.RegisterStandardFacade
// in registerPublicFacade.
func (resources) newPublicFacade(st *corestate.State, _ *common.Resources, authorizer common.Authorizer) (*server.Facade, error) {
	if !authorizer.AuthClient() {
		return nil, common.ErrPerm
	}

	rst, err := st.Resources()
	//rst, err := state.NewState(&resourceState{raw: st})
	if err != nil {
		return nil, errors.Trace(err)
	}

	return server.NewFacade(rst), nil
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

// registerState registers the state functionality for resources.
func (resources) registerState() {
	if !markRegistered(resource.ComponentName, "state") {
		return
	}

	newResources := func(persist corestate.Persistence) corestate.Resources {
		st := state.NewState(&resourceState{persist: persist})
		return st
	}

	corestate.SetResourcesComponent(newResources)

	corestate.AddServicePostFuncs["resources"] = saveResourcesForDemo
}

// resourceState is a wrapper around state.State that supports the needs
// of resources.
type resourceState struct {
	persist corestate.Persistence
}

// Persistence implements resource/state.RawState.
func (st resourceState) Persistence() state.Persistence {
	return persistence.NewPersistence(st.persist)
}

// Storage implements resource/state.RawState.
func (st resourceState) Storage() state.Storage {
	return st.persist.NewStorage()
}

// registerPublicCommands adds the resources-related commands
// to the "juju" supercommand.
func (r resources) registerPublicCommands() {
	if !markRegistered(resource.ComponentName, "public-commands") {
		return
	}

	commands.RegisterEnvCommand(func() envcmd.EnvironCommand {
		return cmd.NewUploadCommand(cmd.UploadDeps{
			NewClient: func(c *cmd.UploadCommand) (cmd.UploadClient, error) {
				return r.newClient(c.NewAPIRoot)
			},
			OpenResource: func(s string) (cmd.ReadSeekCloser, error) {
				return os.Open(s)
			},
		})

	})

	commands.RegisterEnvCommand(func() envcmd.EnvironCommand {
		return cmd.NewShowServiceCommand(cmd.ShowServiceDeps{
			NewClient: func(c *cmd.ShowServiceCommand) (cmd.ShowServiceClient, error) {
				return r.newClient(c.NewAPIRoot)
			},
		})
	})
}

func newCharmstoreClient() charmrepo.Interface {
	// Also see apiserver/service/charmstore.go.
	var args charmrepo.NewCharmStoreParams
	client := charmrepo.NewCharmStore(args)
	return client
}

// TODO(ericsnow) Get rid of charmstoreClient one charmrepo.Interface grows the methods.

type charmstoreClient struct {
	charmrepo.Interface
}

func (charmstoreClient) ListResources(charmURLs []charm.URL) ([][]charmresource.Resource, error) {
	// TODO(natefinch): this is all demo stuff and should go away afterward.
	if len(charmURLs) != 1 || charmURLs[0].Name != "starsay" {
		res := make([][]charmresource.Resource, len(charmURLs))
		return res, nil
	}
	var fingerprint = []byte("123456789012345678901234567890123456789012345678")
	fp, err := charmresource.NewFingerprint(fingerprint)
	if err != nil {
		return nil, err
	}
	res := [][]charmresource.Resource{
		{
			{
				Meta: charmresource.Meta{
					Name:    "store-resource",
					Type:    charmresource.TypeFile,
					Path:    "filename.tgz",
					Comment: "One line that is useful when operators need to push it.",
				},
				Origin:      charmresource.OriginStore,
				Revision:    1,
				Fingerprint: fp,
				Size:        1,
			},
			{
				Meta: charmresource.Meta{
					Name:    "upload-resource",
					Type:    charmresource.TypeFile,
					Path:    "somename.xml",
					Comment: "Who uses xml anymore?",
				},
				Origin: charmresource.OriginUpload,
			},
		},
	}
	return res, nil
}

func (charmstoreClient) Close() error {
	return nil
}

type apicommand interface {
	NewAPIRoot() (api.Connection, error)
}

func (resources) newClient(newAPICaller func() (api.Connection, error)) (*client.Client, error) {
	apiCaller, err := newAPICaller()
	if err != nil {
		return nil, errors.Trace(err)
	}
	caller := base.NewFacadeCallerForVersion(apiCaller, resource.ComponentName, server.Version)
	doer, err := apiCaller.HTTPClient()
	if err != nil {
		return nil, errors.Trace(err)
	}

	// The apiCaller takes care of prepending /environment/<envUUID>.
	cl := client.NewClient(caller, doer, apiCaller)
	return cl, nil
}

// TODO(natefinch) DEMO CODE, revisit after demo!
func saveResourcesForDemo(st *corestate.State, args corestate.AddServiceArgs) error {
	resourceState, err := st.Resources()
	if err != nil {
		return errors.Annotate(err, "can't get resources from state")
	}

	for _, meta := range args.Charm.Meta().Resources {
		res := charmresource.Resource{
			Meta: meta,
			// TODO(natefinch): how do we determine this at deploy time?
			Origin: charmresource.OriginUpload,
		}

		// no data for you!
		r := &bytes.Buffer{}

		if _, err := resourceState.SetResource(args.Name, "", res, r); err != nil {
			return errors.Annotatef(err, "can't add resource %q", meta.Name)
		}
	}
	return nil
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

// OpenResource implements resource/api/private/server.UnitDataStore.
func (ds *resourcesUnitDataStore) OpenResource(name string) (resource.Resource, io.ReadCloser, error) {
	return ds.resources.OpenResource(ds.unit, name)
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
