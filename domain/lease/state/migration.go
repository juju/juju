// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/canonical/sqlair"

	"github.com/juju/juju/core/database"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/domain"
	"github.com/juju/juju/internal/errors"
)

// MigrationState provides the lease read capabilities.
type MigrationState struct {
	*domain.StateBase
}

// NewMigrationState returns a new migration state reference.
func NewMigrationState(factory database.TxnRunnerFactory) *MigrationState {
	return &MigrationState{
		StateBase: domain.NewStateBase(factory),
	}
}

// GetApplicationLeadershipForModel returns the leadership information for
// the model applications.
//
// This replicates some of the functionality of the lease state, but with key
// differences in the implementation. We only return the
// "application-leadership" leases and we also check if the lease has expired
// and remove it if it has.
func (s *MigrationState) GetApplicationLeadershipForModel(ctx context.Context, modelUUID model.UUID) (map[string]string, error) {
	db, err := s.DB(ctx)
	if err != nil {
		return nil, errors.Capture(err)
	}

	lease := leadership{
		ModelUUID: modelUUID.String(),
	}

	stmt, err := s.Prepare(`
SELECT &leadership.*
FROM   lease
WHERE  lease_type_id = 1
AND    model_uuid = $leadership.model_uuid
AND    expiry >= date('now');`, lease)
	if err != nil {
		return nil, errors.Errorf("preparing delete lease statement: %w", err)
	}

	var result map[string]string
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var leases []leadership
		err := tx.Query(ctx, stmt, lease).GetAll(&leases)
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil
		} else if err != nil {
			return errors.Capture(err)
		}

		result = map[string]string{}
		for _, lease := range leases {
			result[lease.Name] = lease.Holder
		}
		return nil
	})
	return result, errors.Capture(err)
}
