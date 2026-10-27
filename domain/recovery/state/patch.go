// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/canonical/sqlair"

	"github.com/juju/juju/core/network"
	"github.com/juju/juju/domain/ipaddress"
	domainrecovery "github.com/juju/juju/domain/recovery"
	"github.com/juju/juju/internal/errors"
)

// GetControllerMachineName resolves the archived machine the replacement
// maps onto: the lowest-named ALIVE machine hosting a unit of the
// controller application. An HA source may have lost its "0" machine; the
// patch target must be a machine that actually exists and is alive in the
// archive, never an assumed ordinal.
func (st *State) GetControllerMachineName(ctx context.Context) (string, error) {
	type machine struct {
		Name string `db:"name"`
	}
	stmt, err := st.Prepare(`
SELECT m.name AS &machine.name FROM machine AS m
JOIN v_machine_is_controller AS vic ON vic.machine_uuid = m.uuid
WHERE m.life_id = 0 ORDER BY m.name LIMIT 1`, machine{})
	if err != nil {
		return "", errors.Capture(err)
	}
	db, err := st.DB(ctx)
	if err != nil {
		return "", errors.Capture(err)
	}
	var found machine
	err = db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		return tx.Query(ctx, stmt).Get(&found)
	})
	if errors.Is(err, sqlair.ErrNoRows) {
		return "", errors.New("archive contains no live controller machine")
	}
	return found.Name, errors.Capture(err)
}

// PatchControllerMachine updates the cloud instance without changing logical
// identity. The instance ID and display name always take the replacement's
// values: the record describes the physical replacement, and an empty display
// name is a genuine observation on providers that do not set one. Zero arch,
// memory, core and root-disk observations leave the archived values in place.
func (st *State) PatchControllerMachine(ctx context.Context, patch domainrecovery.MachinePatch) error {
	type machinePatch struct {
		Name        string `db:"name"`
		UUID        string `db:"uuid"`
		InstanceID  string `db:"instance_id"`
		DisplayName string `db:"display_name"`
		Arch        string `db:"arch"`
		Mem         uint64 `db:"mem"`
		Cores       uint64 `db:"cores"`
		Disk        uint64 `db:"disk"`
	}
	input := machinePatch{Name: patch.MachineName, InstanceID: patch.InstanceID,
		DisplayName: patch.DisplayName, Arch: patch.Arch, Mem: patch.MemMB,
		Cores: patch.Cores, Disk: patch.RootDiskMB}
	get, err := st.Prepare(`SELECT m.uuid AS &machinePatch.uuid
FROM machine AS m WHERE m.name = $machinePatch.name`, input)
	if err != nil {
		return errors.Capture(err)
	}
	update, err := st.Prepare(`
UPDATE machine_cloud_instance AS mci
SET instance_id = $machinePatch.instance_id,
    display_name = $machinePatch.display_name,
    arch = CASE WHEN $machinePatch.arch != '' THEN $machinePatch.arch ELSE mci.arch END,
    mem = CASE WHEN $machinePatch.mem > 0 THEN $machinePatch.mem ELSE mci.mem END,
    cpu_cores = CASE WHEN $machinePatch.cores > 0 THEN $machinePatch.cores ELSE mci.cpu_cores END,
    root_disk = CASE WHEN $machinePatch.disk > 0 THEN $machinePatch.disk ELSE mci.root_disk END
WHERE mci.machine_uuid = $machinePatch.uuid`, input)
	if err != nil {
		return errors.Capture(err)
	}
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}
	return errors.Capture(db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		if err := tx.Query(ctx, get, input).Get(&input); errors.Is(err, sqlair.ErrNoRows) {
			return errors.New("archived controller machine not found")
		} else if err != nil {
			return errors.Capture(err)
		}
		var outcome sqlair.Outcome
		if err := tx.Query(ctx, update, input).Get(&outcome); err != nil {
			return errors.Capture(err)
		}
		affected, err := outcome.Result().RowsAffected()
		if err != nil {
			return errors.Capture(err)
		}
		if affected == 0 {
			return errors.New("archived controller machine has no cloud instance record")
		}
		return nil
	}))
}

// PatchControllerUnitAddresses replaces the controller unit's IP and DNS
// observations. Kubernetes pod names live in fqdn_address, independently of the
// application's Service addresses. Only the unit's observations are replaced;
// shared DNS rows and Service addresses belonging to other nodes are preserved.
func (st *State) PatchControllerUnitAddresses(ctx context.Context, patch domainrecovery.ControllerUnitAddressPatch) error {
	if len(patch.Addresses) == 0 {
		return nil
	}
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}
	return errors.Capture(db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {

		type observation struct {
			UUID        string `db:"uuid"`
			NetNodeUUID string `db:"net_node_uuid"`
			DeviceUUID  string `db:"device_uuid"`
			Value       string `db:"address_value"`
			TypeID      int    `db:"type_id"`
			OriginID    int    `db:"origin_id"`
			ScopeID     int    `db:"scope_id"`
		}
		query, err := sqlair.Prepare(`
SELECT u.net_node_uuid AS &observation.net_node_uuid
FROM unit AS u
JOIN application_controller AS ac ON ac.application_uuid = u.application_uuid
WHERE u.name = 'controller/0'`, observation{})
		if err != nil {
			return errors.Capture(err)
		}
		var node observation
		if err := tx.Query(ctx, query).Get(&node); errors.Is(err, sqlair.ErrNoRows) {
			return errors.New("controller unit not found")
		} else if err != nil {
			return errors.Errorf("finding controller unit: %w", err)
		}

		type addressUUIDs []string
		selectNames, err := sqlair.Prepare(`
SELECT nnfa.address_uuid AS &observation.uuid
FROM net_node_fqdn_address AS nnfa
WHERE nnfa.net_node_uuid = $observation.net_node_uuid`, observation{})
		if err != nil {
			return errors.Capture(err)
		}
		var oldNames []observation
		if err := tx.Query(ctx, selectNames, node).GetAll(&oldNames); err != nil && !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Capture(err)
		}
		for _, statement := range []string{
			`WITH old_ips AS (
    SELECT ipa.uuid AS uuid FROM ip_address AS ipa
    WHERE ipa.net_node_uuid = $observation.net_node_uuid
)
DELETE FROM provider_ip_address AS pia WHERE pia.address_uuid IN old_ips`,
			`DELETE FROM ip_address AS ipa WHERE ipa.net_node_uuid = $observation.net_node_uuid`,
			`DELETE FROM net_node_fqdn_address AS nnfa WHERE nnfa.net_node_uuid = $observation.net_node_uuid`,
		} {
			query, err := sqlair.Prepare(statement, observation{})
			if err != nil {
				return errors.Capture(err)
			}
			if err := tx.Query(ctx, query, node).Run(); err != nil {
				return errors.Errorf("removing stale controller unit addresses: %w", err)
			}
		}
		if len(oldNames) > 0 {
			ids := make(addressUUIDs, len(oldNames))
			for i, name := range oldNames {
				ids[i] = name.UUID
			}
			query, err := sqlair.Prepare(`
WITH referenced AS (
    SELECT nnfa.address_uuid AS uuid FROM net_node_fqdn_address AS nnfa
)
DELETE FROM fqdn_address AS fa
WHERE fa.uuid IN ($addressUUIDs[:]) AND fa.uuid NOT IN referenced`, ids)
			if err != nil {
				return errors.Capture(err)
			}
			if err := tx.Query(ctx, query, ids).Run(); err != nil {
				return errors.Errorf("removing stale controller unit DNS names: %w", err)
			}
		}

		for _, address := range patch.Addresses {
			input := observation{
				UUID: address.UUID, NetNodeUUID: node.NetNodeUUID, Value: address.Value,
				TypeID:   int(ipaddress.MarshallAddressType(address.Type)),
				OriginID: int(ipaddress.MarshallOrigin(network.OriginProvider)),
				ScopeID:  int(ipaddress.MarshallScope(network.ScopeCloudLocal)),
			}
			if address.Type == network.HostName {
				// Reuse an existing name with the same scope. Other nodes may
				// reference it, so mutating an archived DNS row is unsafe.
				query, err := sqlair.Prepare(`
INSERT INTO fqdn_address (uuid, address, scope_id)
VALUES ($observation.uuid, $observation.address_value, 1)
ON CONFLICT (address, scope_id) DO NOTHING`, input)
				if err != nil {
					return errors.Capture(err)
				}
				if err := tx.Query(ctx, query, input).Run(); err != nil {
					return errors.Errorf("inserting controller unit DNS address: %w", err)
				}
				query, err = sqlair.Prepare(`
INSERT INTO net_node_fqdn_address (net_node_uuid, address_uuid)
SELECT $observation.net_node_uuid, fa.uuid FROM fqdn_address AS fa
WHERE fa.address = $observation.address_value AND fa.scope_id = 1`, input)
				if err != nil {
					return errors.Capture(err)
				}
				if err := tx.Query(ctx, query, input).Run(); err != nil {
					return errors.Errorf("linking controller unit DNS address: %w", err)
				}
				continue
			}

			if node.DeviceUUID == "" {
				query, err := sqlair.Prepare(`
SELECT lld.uuid AS &observation.device_uuid FROM link_layer_device AS lld
WHERE lld.net_node_uuid = $observation.net_node_uuid ORDER BY lld.uuid LIMIT 1`, node)
				if err != nil {
					return errors.Capture(err)
				}
				if err := tx.Query(ctx, query, node).Get(&node); errors.Is(err, sqlair.ErrNoRows) {
					node.DeviceUUID = patch.DeviceUUID
					query, err := sqlair.Prepare(`
INSERT INTO link_layer_device
    (uuid, net_node_uuid, name, device_type_id, virtual_port_type_id)
VALUES ($observation.device_uuid, $observation.net_node_uuid, '', 0, 0)`, node)
					if err != nil {
						return errors.Capture(err)
					}
					if err := tx.Query(ctx, query, node).Run(); err != nil {
						return errors.Errorf("creating controller unit device: %w", err)
					}
				} else if err != nil {
					return errors.Capture(err)
				}
			}
			input.DeviceUUID = node.DeviceUUID
			query, err := sqlair.Prepare(`
INSERT INTO ip_address
    (uuid, net_node_uuid, device_uuid, address_value,
     type_id, config_type_id, origin_id, scope_id)
VALUES ($observation.uuid, $observation.net_node_uuid, $observation.device_uuid,
        $observation.address_value, $observation.type_id, 0,
        $observation.origin_id, $observation.scope_id)`, input)
			if err != nil {
				return errors.Capture(err)
			}
			if err := tx.Query(ctx, query, input).Run(); err != nil {
				return errors.Errorf("inserting controller unit IP address: %w", err)
			}
		}
		return nil
	}))
}
