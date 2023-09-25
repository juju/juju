// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/juju/errors"

	"github.com/juju/juju/core/database"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/domain/credential"
	modelerrors "github.com/juju/juju/domain/model/errors"
	"github.com/juju/juju/domain/modelmanager/service"
	jujudb "github.com/juju/juju/internal/database"
)

// State represents a type for interacting with the underlying model state.
type State struct {
	*domain.StateBase
}

// NewState returns a new State for interacting with the underlying model state.
func NewState(
	factory database.TxnRunnerFactory,
) *State {
	return &State{
		StateBase: domain.NewStateBase(factory),
	}
}

// SetCloudCredential sets the cloud credential that will be used by this model
// identified by the cloud, owner and name of the cloud credential. If the cloud
// credential or the model do not exist then an error that satisfies NotFound is
// returned.
func (s *State) SetCloudCredential(
	ctx context.Context,
	modelUUID service.UUID,
	id credential.ID,
) error {
	db, err := s.DB()
	if err != nil {
		return errors.Trace(err)
	}

	cloudCredUUIDStmt := `
SELECT cloud_credential.uuid,
       cloud.uuid
FROM cloud_credential
INNER JOIN cloud
ON cloud.uuid = cloud_credential.cloud_uuid
WHERE cloud.name = ?
AND cloud_credential.owner_uuid = ?
AND cloud_credential.name = ?
`

	stmt := `
INSERT INTO model_metadata (model_uuid, cloud_uuid, cloud_credential_uuid) VALUES (?, ?, ?)
ON CONFLICT(model_uuid) DO UPDATE
SET cloud_uuid = excluded.cloud_uuid,
    cloud_credential_uuid = excluded.cloud_credential_uuid
WHERE model_uuid = excluded.model_uuid
`

	return db.StdTxn(ctx, func(ctx context.Context, tx *sql.Tx) error {
		var cloudCredUUID, cloudUUID string
		err := tx.QueryRowContext(ctx, cloudCredUUIDStmt, id.Cloud, id.Owner, id.Name).
			Scan(&cloudCredUUID, &cloudUUID)
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf(
				"%w cloud credential %q%w",
				errors.NotFound, id, errors.Hide(err),
			)
		} else if err != nil {
			return fmt.Errorf(
				"getting cloud credential uuid for %q: %w",
				id, err,
			)
		}

		_, err = tx.ExecContext(ctx, stmt, modelUUID, cloudUUID, cloudCredUUID)
		if jujudb.IsErrConstraintForeignKey(err) {
			return fmt.Errorf(
				"%w %q when setting cloud credential %q%w",
				modelerrors.NotFound, modelUUID, id, errors.Hide(err))
		} else if err != nil {
			return fmt.Errorf(
				"setting cloud credential %q for model %q: %w",
				id, modelUUID, err)
		}
		return nil
	})
}
