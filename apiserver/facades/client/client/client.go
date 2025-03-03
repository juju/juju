// Copyright 2013, 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client

import (
	"context"

	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/authentication"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/core/leadership"
	"github.com/juju/juju/core/permission"
	internallogger "github.com/juju/juju/internal/logger"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

var logger = internallogger.GetLogger("juju.apiserver.client")

// Client serves client-specific API methods.
type Client struct {
	stateAccessor    Backend
	storageAccessor  StorageInterface
	auth             facade.Authorizer
	leadershipReader leadership.Reader

	blockDeviceService BlockDeviceService
	networkService     NetworkService
	modelInfoService   ModelInfoService
	machineService     MachineService
	applicationService ApplicationService
	portService        PortService
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
