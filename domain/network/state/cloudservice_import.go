// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/canonical/sqlair"

	"github.com/juju/juju/domain/network/internal"
	"github.com/juju/juju/internal/errors"
)

// CreateCloudServices creates cloud service in state.
// It creates the associated net node uuid and links it to the application
// through the provided application name.
func (st *State) CreateCloudServices(ctx context.Context, cloudservices []internal.ImportCloudService) error {

	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	type service struct {
		UUID            string `db:"uuid"`
		ApplicationName string `db:"application_name"`
		NetNodeUUID     string `db:"net_node_uuid"`
		ProviderID      string `db:"provider_id"`
	}

	insertNetNodesStmt, err := st.Prepare(`
INSERT INTO net_node (uuid) VALUES ($service.net_node_uuid)`, service{})
	if err != nil {
		return errors.Capture(err)
	}

	insertServiceStmt, err := st.Prepare(`
INSERT INTO k8s_service (uuid, application_uuid, net_node_uuid,provider_id) 
SELECT 
    $service.uuid, 
    a.uuid AS application_uuid,
    $service.net_node_uuid,
    $service.provider_id
FROM application AS a
WHERE a.name = $service.application_name
`, service{})
	if err != nil {
		return errors.Capture(err)
	}

	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		for _, svc := range cloudservices {
			var outcome sqlair.Outcome
			input := service{
				UUID:            svc.UUID,
				ApplicationName: svc.ApplicationName,
				NetNodeUUID:     svc.NetNodeUUID,
				ProviderID:      svc.ProviderID,
			}
			if err := tx.Query(ctx, insertNetNodesStmt, input).Get(&outcome); err != nil {
				return errors.Errorf("inserting net nodes: %w", err)
			}
			if affected, err := outcome.Result().RowsAffected(); err != nil {
				return errors.Errorf("getting rows affected: %w", err)
			} else if affected != 1 {
				return errors.Errorf("inserting net nodes: expected 1 row affected, got %d", affected)
			}

			if err := tx.Query(ctx, insertServiceStmt, input).Get(&outcome); err != nil {
				return errors.Errorf("inserting services: %w", err)
			}
			if affected, err := outcome.Result().RowsAffected(); err != nil {
				return errors.Errorf("getting rows affected: %w", err)
			} else if affected != 1 {
				return errors.Errorf("inserting cloud services: expected 1 row affected, got %d", affected)
			}
		}
		return nil
	})

	return errors.Capture(err)
}
