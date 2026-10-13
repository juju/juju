// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	corelogger "github.com/juju/juju/core/logger"
)

// API provides backup-specific API methods.
type API struct {
	authorizer     facade.Authorizer
	controllerUUID string
	logger         corelogger.Logger
}

// NewAPI creates a new instance of the Backups API facade.
func NewAPI(
	authorizer facade.Authorizer,
	controllerUUID string,
	logger corelogger.Logger,
) (*API, error) {
	if !authorizer.AuthClient() {
		return nil, apiservererrors.ErrPerm
	}

	return &API{
		authorizer:     authorizer,
		controllerUUID: controllerUUID,
		logger:         logger,
	}, nil
}
