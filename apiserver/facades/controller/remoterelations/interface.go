// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remoterelations

import (
	"context"

	"github.com/juju/names/v6"
	"gopkg.in/macaroon.v2"

	common "github.com/juju/juju/apiserver/common/crossmodel"
	"github.com/juju/juju/rpc/params"
)

// RemoteRelationsState provides the subset of global state required by the
// remote relations facade.
type RemoteRelationsState interface {
	common.Backend

	// RemoveRemoteEntity removes the specified entity from the remote entities collection.
	RemoveRemoteEntity(entity names.Tag) error

	// SaveMacaroon saves the given macaroon for the specified entity.
	SaveMacaroon(entity names.Tag, mac *macaroon.Macaroon) error
}

// ControllerConfigAPI provides the subset of common.ControllerConfigAPI
// required by the remote firewaller facade
type ControllerConfigAPI interface {
	// ControllerConfig returns the controller's configuration.
	ControllerConfig(context.Context) (params.ControllerConfigResult, error)

	// ControllerAPIInfoForModels returns the controller api connection details for the specified models.
	ControllerAPIInfoForModels(ctx context.Context, args params.Entities) (params.ControllerAPIInfoResults, error)
}
