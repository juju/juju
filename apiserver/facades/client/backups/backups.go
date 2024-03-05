// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"context"

	"github.com/juju/names/v5"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/controller"
	corebackups "github.com/juju/juju/core/backups"
)

// ControllerConfigService is an interface that provides the controller config.
type ControllerConfigService interface {
	// ControllerConfig returns the controller config.
	ControllerConfig(context.Context) (controller.Config, error)
}

// API provides backup-specific API methods.
type API struct {
	controllerConfigService ControllerConfigService
	paths                   *corebackups.Paths

	// machineID is the ID of the machine where the API server is running.
	machineID string
}

// NewAPI creates a new instance of the Backups API facade.
func NewAPI(
	controllerConfigService ControllerConfigService,
	authorizer facade.Authorizer,
	machineTag names.Tag,
	dataDir, logDir string,
) (*API, error) {
	// TODO (manadart 2023-10-11) Restore this assignment based on super-user
	// access to the controller when re-implemented for Dqlite.
	isControllerAdmin := true

	if !authorizer.AuthClient() || !isControllerAdmin {
		return nil, apiservererrors.ErrPerm
	}

	paths := corebackups.Paths{
		DataDir: dataDir,
		LogsDir: logDir,
	}

	b := API{
		controllerConfigService: controllerConfigService,
		paths:                   &paths,
		machineID:               machineTag.Id(),
	}
	return &b, nil
}
