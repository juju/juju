// Copyright 2013, 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client

import (
	"context"

	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/authentication"
	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/core/leadership"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/internal/docker"
	"github.com/juju/juju/internal/docker/registry"
	internallogger "github.com/juju/juju/internal/logger"
	"github.com/juju/juju/state"
)

var logger = internallogger.GetLogger("juju.apiserver.client")

// Client serves client-specific API methods.
type Client struct {
	stateAccessor    Backend
	storageAccessor  StorageInterface
	auth             facade.Authorizer
	resources        facade.Resources
	presence         facade.Presence
	leadershipReader leadership.Reader
	newEnviron       common.NewEnvironFunc
	check            *common.BlockChecker

	blockDeviceService      BlockDeviceService
	controllerConfigService ControllerConfigService
	networkService          NetworkService
	modelInfoService        ModelInfoService

	registryAPIFunc func(repoDetails docker.ImageRepoDetails) (registry.Registry, error)
}

// TODO(wallyworld) - remove this method
// state returns a state.State instance for this API.
// Until all code is refactored to use interfaces, we
// need this helper to keep older code happy.
func (c *Client) state() *state.State {
	return c.stateAccessor.(*stateShim).State
}

func (c *Client) checkCanRead(ctx context.Context) error {
	err := c.auth.HasPermission(ctx, permission.SuperuserAccess, c.stateAccessor.ControllerTag())
	if err != nil && !errors.Is(err, authentication.ErrorEntityMissingPermission) {
		return errors.Trace(err)
	}

	if err == nil {
		return nil
	}

	return c.auth.HasPermission(ctx, permission.ReadAccess, c.stateAccessor.ModelTag())
}

func (c *Client) checkIsAdmin(ctx context.Context) error {
	err := c.auth.HasPermission(ctx, permission.SuperuserAccess, c.stateAccessor.ControllerTag())
	if err != nil && !errors.Is(err, authentication.ErrorEntityMissingPermission) {
		return errors.Trace(err)
	}

	if err == nil {
		return nil
	}

	return c.auth.HasPermission(ctx, permission.AdminAccess, c.stateAccessor.ModelTag())
}

// NewClient creates a new instance of the Client Facade.
// TODO(aflynn): Create an args struct for this.
func NewClient(
	backend Backend,
	modelInfoService ModelInfoService,
	storageAccessor StorageInterface,
	blockDeviceService BlockDeviceService,
	controllerConfigService ControllerConfigService,
	resources facade.Resources,
	authorizer facade.Authorizer,
	presence facade.Presence,
	newEnviron common.NewEnvironFunc,
	blockChecker *common.BlockChecker,
	leadershipReader leadership.Reader,
	networkService NetworkService,
	registryAPIFunc func(docker.ImageRepoDetails) (registry.Registry, error),
) (*Client, error) {
	if !authorizer.AuthClient() {
		return nil, apiservererrors.ErrPerm
	}
	client := &Client{
		stateAccessor:           backend,
		storageAccessor:         storageAccessor,
		blockDeviceService:      blockDeviceService,
		controllerConfigService: controllerConfigService,
		auth:                    authorizer,
		resources:               resources,
		presence:                presence,
		leadershipReader:        leadershipReader,
		networkService:          networkService,
		modelInfoService:        modelInfoService,
		newEnviron:              newEnviron,
		check:                   blockChecker,
		registryAPIFunc:         registryAPIFunc,
	}
	return client, nil
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
