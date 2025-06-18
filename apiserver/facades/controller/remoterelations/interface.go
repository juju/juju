// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remoterelations

import (
	"context"

	"github.com/juju/juju/rpc/params"
)

// ControllerConfigAPI provides the subset of common.ControllerConfigAPI
// required by the remote firewaller facade
type ControllerConfigAPI interface {
	// ControllerConfig returns the controller's configuration.
	ControllerConfig(context.Context) (params.ControllerConfigResult, error)

	// ControllerAPIInfoForModels returns the controller api connection details for the specified models.
	ControllerAPIInfoForModels(ctx context.Context, args params.Entities) (params.ControllerAPIInfoResults, error)
}
