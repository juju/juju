// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"context"

	"github.com/canonical/sqlair"
	"github.com/juju/errors"
	"github.com/juju/names/v4"
	"github.com/juju/utils/v3"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/core/database"
	"github.com/juju/juju/domain/credential/state"
)

// InsertCredential inserts  a cloud credential into dqlite.
func InsertCredential(cloudCredTag names.CloudCredentialTag, credential cloud.Credential) func(context.Context, database.TxnRunner) error {
	return func(ctx context.Context, db database.TxnRunner) error {
		if cloudCredTag.Id() == "" {
			return nil
		}

		var (
			name      = cloudCredTag.Name()
			cloudName = cloudCredTag.Cloud().Id()
			owner     = cloudCredTag.Owner().Id()
		)
		credentialUUID, err := utils.NewUUID()
		if err != nil {
			return errors.Trace(err)
		}
		return errors.Trace(db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
			if err := state.CreateCredential(ctx, tx, credentialUUID.String(), name, cloudName, owner, credential); err != nil {
				return errors.Annotate(err, "creating bootstrap credential")
			}
			return nil
		}))
	}
}
