// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"time"

	"github.com/canonical/sqlair"

	"github.com/juju/juju/domain/life"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	domainstatus "github.com/juju/juju/domain/status"
	"github.com/juju/juju/internal/errors"
)

// DetachLostMachineCloudInstance atomically rechecks the critical
// reprovisioning preconditions, clears stale provider-observed state, and
// moves the machine and its cloud instance back to pending.
func (st *State) DetachLostMachineCloudInstance(
	ctx context.Context,
	mName string,
	expectedInstanceID string,
	statusMessage string,
	statusData []byte,
	updatedAt time.Time,
) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	machineNameParam := machineName{Name: mName}
	targetStmt, err := st.Prepare(`
SELECT     m.uuid AS &reprovisionDetachTarget.machine_uuid,
           m.net_node_uuid AS &reprovisionDetachTarget.net_node_uuid,
           mci.instance_id AS &reprovisionDetachTarget.instance_id,
           m.life_id AS &reprovisionDetachTarget.life_id,
           COUNT(DISTINCT mapr.machine_uuid) AS &reprovisionDetachTarget.agent_present
FROM       machine AS m
JOIN       machine_cloud_instance AS mci ON m.uuid = mci.machine_uuid
LEFT JOIN  machine_agent_presence AS mapr ON m.uuid = mapr.machine_uuid
WHERE      m.name = $machineName.name
GROUP BY   m.uuid, m.net_node_uuid, mci.instance_id, m.life_id
`, machineNameParam, reprovisionDetachTarget{})
	if err != nil {
		return errors.Errorf("preparing reprovision detach target query: %w", err)
	}

	machineStatusID, err := domainstatus.EncodeMachineStatus(domainstatus.MachineStatusPending)
	if err != nil {
		return errors.Capture(err)
	}
	instanceStatusID, err := domainstatus.EncodeCloudInstanceStatus(domainstatus.InstanceStatusPending)
	if err != nil {
		return errors.Capture(err)
	}
	networkStmts, err := st.prepareReprovisionNetworkStatements()
	if err != nil {
		return errors.Errorf("preparing network cleanup statements: %w", err)
	}
	blockDeviceStmts, err := st.prepareReprovisionBlockDeviceStatements()
	if err != nil {
		return errors.Errorf("preparing block device cleanup statements: %w", err)
	}
	machineDataStmts, err := st.prepareReprovisionMachineDataStatements()
	if err != nil {
		return errors.Errorf("preparing machine data cleanup statements: %w", err)
	}
	machineStatusStmt, instanceStatusStmt, err := st.prepareReprovisionStatusStatements()
	if err != nil {
		return errors.Errorf("preparing status statements: %w", err)
	}

	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var target reprovisionDetachTarget
		if err := tx.Query(ctx, targetStmt, machineNameParam).Get(&target); errors.Is(err, sqlair.ErrNoRows) {
			return machineerrors.MachineNotFound
		} else if err != nil {
			return errors.Errorf("querying reprovision detach target %q: %w", mName, err)
		}

		if err := validateReprovisionDetachTarget(target, expectedInstanceID); err != nil {
			return errors.Capture(err)
		}

		if err := runReprovisionStatements(
			ctx, tx, networkStmts, netNode{UUID: target.NetNodeUUID},
		); err != nil {
			return errors.Errorf("clearing network state: %w", err)
		}

		machineUUID := entityUUID{UUID: target.UUID}
		if err := runReprovisionStatements(ctx, tx, blockDeviceStmts, machineUUID); err != nil {
			return errors.Errorf("clearing block devices: %w", err)
		}
		if err := runReprovisionStatements(ctx, tx, machineDataStmts, machineUUID); err != nil {
			return errors.Errorf("clearing stale machine instance data: %w", err)
		}

		statusValue := setMachineStatus{
			MachineUUID: target.UUID,
			StatusID:    machineStatusID,
			Message:     statusMessage,
			Data:        statusData,
			Updated:     &updatedAt,
		}
		if err := tx.Query(ctx, machineStatusStmt, statusValue).Run(); err != nil {
			return errors.Errorf("setting reprovisioning machine status: %w", err)
		}
		statusValue.StatusID = instanceStatusID
		if err := tx.Query(ctx, instanceStatusStmt, statusValue).Run(); err != nil {
			return errors.Errorf("setting reprovisioning instance status: %w", err)
		}
		return nil
	})
}

func validateReprovisionDetachTarget(target reprovisionDetachTarget, expectedInstanceID string) error {
	if target.LifeID != life.Alive {
		return machineerrors.MachineNotAlive
	}
	if target.AgentPresent > 0 {
		return machineerrors.MachineAgentPresent
	}
	if !target.InstanceID.Valid || target.InstanceID.V == "" {
		return machineerrors.NotProvisioned
	}
	if target.InstanceID.V != expectedInstanceID {
		return machineerrors.MachineCloudInstanceChanged
	}
	return nil
}

func (st *State) prepareReprovisionNetworkStatements() ([]*sqlair.Statement, error) {
	queries := []string{
		// Delete provider identifiers for IP addresses observed on the old
		// machine instance.
		`
WITH target_addresses AS (
    SELECT ipa.uuid AS address_uuid
    FROM ip_address AS ipa
    WHERE ipa.net_node_uuid = $netNode.net_node_uuid
)
DELETE FROM provider_ip_address
WHERE address_uuid IN (
    SELECT ta.address_uuid FROM target_addresses AS ta
)`,
		// Delete all IP addresses associated with the machine net node.
		`
DELETE FROM ip_address
WHERE net_node_uuid = $netNode.net_node_uuid`,
		// Delete parent-child relationships for the old link-layer devices.
		`
WITH target_devices AS (
    SELECT lld.uuid AS device_uuid
    FROM link_layer_device AS lld
    WHERE lld.net_node_uuid = $netNode.net_node_uuid
)
DELETE FROM link_layer_device_parent
WHERE device_uuid IN (
    SELECT td.device_uuid FROM target_devices AS td
)
OR parent_uuid IN (
    SELECT td.device_uuid FROM target_devices AS td
)`,
		// Delete provider identifiers for the old link-layer devices.
		`
WITH target_devices AS (
    SELECT lld.uuid AS device_uuid
    FROM link_layer_device AS lld
    WHERE lld.net_node_uuid = $netNode.net_node_uuid
)
DELETE FROM provider_link_layer_device
WHERE device_uuid IN (
    SELECT td.device_uuid FROM target_devices AS td
)`,
		// Delete DNS search domains reported for the old link-layer devices.
		`
WITH target_devices AS (
    SELECT lld.uuid AS device_uuid
    FROM link_layer_device AS lld
    WHERE lld.net_node_uuid = $netNode.net_node_uuid
)
DELETE FROM link_layer_device_dns_domain
WHERE device_uuid IN (
    SELECT td.device_uuid FROM target_devices AS td
)`,
		// Delete DNS server addresses reported for the old link-layer devices.
		`
WITH target_devices AS (
    SELECT lld.uuid AS device_uuid
    FROM link_layer_device AS lld
    WHERE lld.net_node_uuid = $netNode.net_node_uuid
)
DELETE FROM link_layer_device_dns_address
WHERE device_uuid IN (
    SELECT td.device_uuid FROM target_devices AS td
)`,
		// Delete routes reported for the old link-layer devices.
		`
WITH target_devices AS (
    SELECT lld.uuid AS device_uuid
    FROM link_layer_device AS lld
    WHERE lld.net_node_uuid = $netNode.net_node_uuid
)
DELETE FROM link_layer_device_route
WHERE device_uuid IN (
    SELECT td.device_uuid FROM target_devices AS td
)`,
		// Delete the old link-layer devices after their references are removed.
		`
DELETE FROM link_layer_device
WHERE net_node_uuid = $netNode.net_node_uuid`,
		// Delete FQDN associations observed for the old machine instance.
		`
DELETE FROM net_node_fqdn_address
WHERE net_node_uuid = $netNode.net_node_uuid`,
		// Delete hostname associations observed for the old machine instance.
		`
DELETE FROM net_node_hostname_address
WHERE net_node_uuid = $netNode.net_node_uuid`,
	}
	return st.prepareReprovisionStatements(queries, netNode{})
}

func (st *State) prepareReprovisionBlockDeviceStatements() ([]*sqlair.Statement, error) {
	queries := []string{
		// Delete device links for old block devices that are not referenced by
		// a storage volume attachment.
		`
WITH unreferenced_block_devices AS (
    SELECT bd.uuid
    FROM block_device AS bd
    LEFT JOIN storage_volume_attachment AS sva
    ON bd.uuid = sva.block_device_uuid
    WHERE bd.machine_uuid = $entityUUID.uuid
    AND sva.uuid IS NULL
)
DELETE FROM block_device_link_device
WHERE block_device_uuid IN (
    SELECT uuid FROM unreferenced_block_devices
)`,
		// Delete old block devices that are safe to remove because no storage
		// volume attachment references them.
		`
WITH attached_block_devices AS (
    SELECT sva.block_device_uuid
    FROM storage_volume_attachment AS sva
    WHERE sva.block_device_uuid IS NOT NULL
)
DELETE FROM block_device
WHERE machine_uuid = $entityUUID.uuid
AND uuid NOT IN (
    SELECT block_device_uuid FROM attached_block_devices
)`,
	}
	return st.prepareReprovisionStatements(queries, entityUUID{})
}

func (st *State) prepareReprovisionMachineDataStatements() ([]*sqlair.Statement, error) {
	queries := []string{
		// Delete provider tags reported for the old machine instance.
		`
DELETE FROM instance_tag
WHERE machine_uuid = $entityUUID.uuid`,
		// Clear runtime identity data reported by the old machine instance.
		`
UPDATE machine
SET nonce = NULL,
    hostname = NULL
WHERE uuid = $entityUUID.uuid`,
		// Clear the old provider instance association and its observed hardware
		// characteristics.
		`
UPDATE machine_cloud_instance
SET instance_id = NULL,
    display_name = NULL,
    arch = NULL,
    availability_zone_uuid = NULL,
    cpu_cores = NULL,
    cpu_power = NULL,
    mem = NULL,
    root_disk = NULL,
    root_disk_source = NULL,
    virt_type = NULL
WHERE machine_uuid = $entityUUID.uuid`,
	}
	return st.prepareReprovisionStatements(queries, entityUUID{})
}

func (st *State) prepareReprovisionStatusStatements() (*sqlair.Statement, *sqlair.Statement, error) {
	// Move the machine agent status back to pending and record the
	// reprovisioning context.
	machineStatusStmt, err := st.Prepare(`
INSERT INTO machine_status (*)
VALUES ($setMachineStatus.*)
ON CONFLICT (machine_uuid)
DO UPDATE SET
    status_id = excluded.status_id,
    message = excluded.message,
    updated_at = excluded.updated_at,
    data = excluded.data
`, setMachineStatus{})
	if err != nil {
		return nil, nil, errors.Capture(err)
	}
	// Move the cloud instance status back to pending and record the
	// reprovisioning context.
	instanceStatusStmt, err := st.Prepare(`
UPDATE machine_cloud_instance_status
SET status_id = $setMachineStatus.status_id,
    message = $setMachineStatus.message,
    data = $setMachineStatus.data,
    updated_at = $setMachineStatus.updated_at
WHERE machine_uuid = $setMachineStatus.machine_uuid
`, setMachineStatus{})
	if err != nil {
		return nil, nil, errors.Capture(err)
	}
	return machineStatusStmt, instanceStatusStmt, nil
}

func (st *State) prepareReprovisionStatements(
	queries []string, typeSample any,
) ([]*sqlair.Statement, error) {
	statements := make([]*sqlair.Statement, len(queries))
	for i, query := range queries {
		stmt, err := st.Prepare(query, typeSample)
		if err != nil {
			return nil, errors.Capture(err)
		}
		statements[i] = stmt
	}
	return statements, nil
}

func runReprovisionStatements(
	ctx context.Context, tx *sqlair.TX, statements []*sqlair.Statement, arg any,
) error {
	for _, stmt := range statements {
		if err := tx.Query(ctx, stmt, arg).Run(); err != nil {
			return errors.Capture(err)
		}
	}
	return nil
}
