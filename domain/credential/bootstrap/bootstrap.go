// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"context"

	"github.com/canonical/sqlair"
	"github.com/juju/errors"
	"github.com/juju/utils/v3"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/database"
	"github.com/juju/juju/domain/credential/state"
)

// InsertInitialControllerCredentials inserts the initial
// controller credential into the database.
func InsertInitialControllerCredentials(name, cloudName, owner string, credential cloud.Credential) func(context.Context, database.TxnRunner) error {
	return func(ctx context.Context, db database.TxnRunner) error {
		return errors.Trace(db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
			credentialUUID, err := utils.NewUUID()
			if err != nil {
				return errors.Trace(err)
			}
			if err := state.CreateCredential(ctx, tx, credentialUUID.String(), name, cloudName, owner, credential); err != nil {
				return errors.Annotate(err, "creating bootstrap credential")
			}
			return nil
		}))
	}
}
