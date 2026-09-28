// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/canonical/sqlair"

	"github.com/juju/juju/core/network"
	"github.com/juju/juju/domain/network/internal"
	"github.com/juju/juju/internal/errors"
)

// CreateK8sServices creates cloud service in state.
// It creates the associated net node uuid and links it to the application
// through the provided application name. Hostname scopes must be validated by
// the service layer before calling this method.
func (st *State) CreateK8sServices(ctx context.Context, k8sServices []internal.ImportK8sService) error {

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
		for _, svc := range k8sServices {
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
			for _, addr := range svc.Addresses {
				if network.AddressType(addr.Type) != network.HostName {
					continue
				}
				if err := st.importK8sServiceFQDN(ctx, tx, svc.NetNodeUUID, addr); err != nil {
					return errors.Capture(err)
				}
			}
		}
		return nil
	})

	return errors.Capture(err)
}

func (st *State) importK8sServiceFQDN(ctx context.Context, tx *sqlair.TX, netNodeUUID string, addr internal.ImportK8sServiceAddress) error {
	type hostname struct {
		UUID        string `db:"uuid"`
		Address     string `db:"address"`
		Scope       string `db:"scope"`
		NetNodeUUID string `db:"net_node_uuid"`
	}
	input := hostname{UUID: addr.UUID, Address: addr.Value, Scope: addr.Scope, NetNodeUUID: netNodeUUID}
	insert, err := st.Prepare(`
INSERT INTO fqdn_address (uuid, address, scope_id)
SELECT $hostname.uuid, $hostname.address, nas.id
FROM network_address_scope AS nas WHERE nas.name = $hostname.scope
ON CONFLICT (address, scope_id) DO NOTHING`, input)
	if err != nil {
		return errors.Capture(err)
	}
	if err := tx.Query(ctx, insert, input).Run(); err != nil {
		return errors.Errorf("inserting service hostname: %w", err)
	}
	link, err := st.Prepare(`
INSERT INTO net_node_fqdn_address (net_node_uuid, address_uuid)
SELECT $hostname.net_node_uuid, fa.uuid
FROM fqdn_address AS fa
JOIN network_address_scope AS nas ON nas.id = fa.scope_id
WHERE fa.address = $hostname.address AND nas.name = $hostname.scope
ON CONFLICT DO NOTHING`, input)
	if err != nil {
		return errors.Capture(err)
	}
	return errors.Capture(tx.Query(ctx, link, input).Run())
}
