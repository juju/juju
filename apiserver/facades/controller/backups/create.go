// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"context"

	"github.com/juju/names/v6"

	coreerrors "github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/rpc/params"
)

// Create is the API method that requests juju to create a new backup
// of its state. As of 4.1 the RPC method no longer creates anything:
// backup creation and download happen in a single request against the
// controller's backups HTTP endpoint, so this method only remains for
// older clients, which are rejected with a clear upgrade error before
// any archive is created.
func (a *API) Create(ctx context.Context, args params.BackupsCreateArgs) (params.BackupsMetadataResult, error) {
	// Creating a backup requires superuser access to the controller. This
	// mirrors the long-standing access gate for backup creation.
	if err := a.authorizer.HasPermission(
		ctx, permission.SuperuserAccess, names.NewControllerTag(a.controllerUUID),
	); err != nil {
		return params.BackupsMetadataResult{}, errors.Capture(err)
	}

	return params.BackupsMetadataResult{}, errors.Errorf(
		"create-backup from clients older than 4.1 is not supported; use a 4.1 or newer client",
	).Add(coreerrors.NotSupported)
}
