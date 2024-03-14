// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"context"

	"github.com/canonical/sqlair"
	"github.com/juju/errors"

	"github.com/juju/juju/cloud"
	corecredential "github.com/juju/juju/core/credential"
	"github.com/juju/juju/core/database"
	"github.com/juju/juju/domain/credential"
	"github.com/juju/juju/domain/credential/state"
	"github.com/juju/juju/internal/uuid"
)

// InsertCredential inserts  a cloud credential into dqlite.
func InsertCredential(id corecredential.ID, cred cloud.Credential) func(context.Context, database.TxnRunner) error {
	return func(ctx context.Context, db database.TxnRunner) error {
		if id.IsZero() {
			return nil
		}

		credentialUUID, err := uuid.NewUUID()
		if err != nil {
			return errors.Trace(err)
		}
		return errors.Trace(db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
			if err := state.CreateCredential(ctx, tx, credentialUUID.String(), id, credential.CloudCredentialInfo{
				AuthType:      string(cred.AuthType()),
				Attributes:    cred.Attributes(),
				Revoked:       cred.Revoked,
				Label:         cred.Label,
				Invalid:       cred.Invalid,
				InvalidReason: cred.InvalidReason,
			}); err != nil {
				return errors.Annotate(err, "creating bootstrap credential")
			}
			return nil
		}))
	}
}
