// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"context"

	"github.com/canonical/sqlair"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/user"
	"github.com/juju/juju/domain/cloud/state"
	internaldatabase "github.com/juju/juju/internal/database"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/uuid"
)

// InsertCloud inserts the initial cloud during bootstrap.
func InsertCloud(ownerName user.Name, cloud cloud.Cloud) internaldatabase.BootstrapOpt {
	return func(ctx context.Context, controller, model database.TxnRunner) error {
		cloudUUID, err := uuid.NewUUID()
		if err != nil {
			return errors.Capture(err)
		}
		return errors.Capture(controller.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
			if err := state.CreateCloud(ctx, tx, ownerName, cloudUUID.String(), cloud); err != nil {
				return errors.Errorf("creating bootstrap cloud: %w", err)
			}
			return nil
		}))

	}
}
