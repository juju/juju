// Copyright 2013, 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client

import (
	"context"

	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/authentication"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/core/leadership"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/environs"
	internallogger "github.com/juju/juju/internal/logger"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

var logger = internallogger.GetLogger("juju.apiserver.client")

type API struct {
	stateAccessor      Backend
	storageAccessor    StorageInterface
	blockDeviceService BlockDeviceService
	auth               facade.Authorizer
	presence           facade.Presence

	leadershipReader leadership.Reader
	networkService   NetworkService
}

// TODO(wallyworld) - remove this method
// state returns a state.State instance for this API.
// Until all code is refactored to use interfaces, we
// need this helper to keep older code happy.
func (api *API) state() *state.State {
	return api.stateAccessor.(*stateShim).State
}

// Client serves client-specific API methods.
type Client struct {
	api *API
}

// ClientV6 serves the (v6) client-specific API methods.
type ClientV6 struct {
	*Client
}

func (c *Client) checkCanRead(ctx context.Context) error {
	err := c.api.auth.HasPermission(ctx, permission.SuperuserAccess, c.api.stateAccessor.ControllerTag())
	if err != nil && !errors.Is(err, authentication.ErrorEntityMissingPermission) {
		return errors.Trace(err)
	}

	if err == nil {
		return nil
	}

	return c.api.auth.HasPermission(ctx, permission.ReadAccess, c.api.stateAccessor.ModelTag())
}

func (c *Client) checkCanWrite(ctx context.Context) error {
	err := c.api.auth.HasPermission(ctx, permission.SuperuserAccess, c.api.stateAccessor.ControllerTag())
	if err != nil && !errors.Is(err, authentication.ErrorEntityMissingPermission) {
		return errors.Trace(err)
	}

	if err == nil {
		return nil
	}

	return c.api.auth.HasPermission(ctx, permission.WriteAccess, c.api.stateAccessor.ModelTag())
}

func (c *Client) checkIsAdmin(ctx context.Context) error {
	err := c.api.auth.HasPermission(ctx, permission.SuperuserAccess, c.api.stateAccessor.ControllerTag())
	if err != nil && !errors.Is(err, authentication.ErrorEntityMissingPermission) {
		return errors.Trace(err)
	}

	if err == nil {
		return nil
	}

	return c.api.auth.HasPermission(ctx, permission.AdminAccess, c.api.stateAccessor.ModelTag())
}

// NewFacade creates a Client facade to handle API requests.
// Changes:
// - FindTools deals with CAAS models now;
func NewFacade(ctx facade.ModelContext) (*Client, error) {
	st := ctx.State()
	authorizer := ctx.Auth()
	presence := ctx.Presence()

	model, err := st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}

	serviceFactory := ctx.ServiceFactory()
	leadershipReader, err := ctx.LeadershipReader()
	if err != nil {
		return nil, errors.Trace(err)
	}

	storageAccessor, err := getStorageState(st)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return NewClient(
		&stateShim{
			State:                    st,
			model:                    model,
			session:                  nil,
			configSchemaSourceGetter: environs.ProviderConfigSchemaSource(serviceFactory.Cloud()),
		},
		storageAccessor,
		serviceFactory.BlockDevice(),
		authorizer,
		presence,
		leadershipReader,
		ctx.ServiceFactory().Network(),
	)
}

// NewClient creates a new instance of the Client Facade.
// TODO(aflynn): Create an args struct for this.
func NewClient(
	backend Backend,
	storageAccessor StorageInterface,
	blockDeviceService BlockDeviceService,
	authorizer facade.Authorizer,
	presence facade.Presence,
	leadershipReader leadership.Reader,
	networkService NetworkService,
) (*Client, error) {
	if !authorizer.AuthClient() {
		return nil, apiservererrors.ErrPerm
	}
	client := &Client{
		api: &API{
			stateAccessor:      backend,
			storageAccessor:    storageAccessor,
			blockDeviceService: blockDeviceService,
			auth:               authorizer,
			presence:           presence,
			leadershipReader:   leadershipReader,
			networkService:     networkService,
		},
	}
	return client, nil
}

// WatchAll initiates a watcher for entities in the connected model.
func (c *Client) WatchAll(ctx context.Context) (params.AllWatcherId, error) {
	return params.AllWatcherId{}, errors.NotImplementedf("WatchAll")
}

// NOTE: this is necessary for the other packages that do upgrade tests.
// Really they should be using a mocked out api server, but that is outside
// the scope of this fix.
var skipReplicaCheck = false

// SkipReplicaCheck is required for tests only as the test mongo isn't a replica.
func SkipReplicaCheck(patcher Patcher) {
	patcher.PatchValue(&skipReplicaCheck, true)
}

// Patcher is provided by the test suites to temporarily change values.
type Patcher interface {
	PatchValue(dest, value interface{})
}
