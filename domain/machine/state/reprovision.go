// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"maps"
	"slices"
	"time"

	"github.com/canonical/sqlair"

	"github.com/juju/juju/domain/life"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	domainstatus "github.com/juju/juju/domain/status"
	domainstorage "github.com/juju/juju/domain/storage"
	"github.com/juju/juju/internal/errors"
)

// DetachLostMachineCloudInstance atomically rechecks the critical
// reprovisioning preconditions, clears stale provider-observed state, and
// moves the machine and its cloud instance back to pending. Machine-scoped
// storage provider state is reset so the normal provisioning paths can create
// empty replacement storage while preserving Juju identity and intent.
// Unsupported storage is rejected in the same transaction.
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
	existingReprovisionStmt, err := st.Prepare(`
SELECT mr.* AS &machineReprovision.*
FROM machine_reprovision AS mr
WHERE mr.machine_name = $machineName.name
`, machineNameParam, machineReprovision{})
	if err != nil {
		return errors.Errorf("preparing existing reprovision query: %w", err)
	}
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
	storageLifeStmt, err := st.prepareReprovisionStorageLifeStatement()
	if err != nil {
		return errors.Errorf("preparing storage lifecycle statement: %w", err)
	}
	storageTargetStmts, err := st.prepareReprovisionStorageTargetStatements()
	if err != nil {
		return errors.Errorf("preparing storage target statements: %w", err)
	}
	reprovisionStmt, err := st.Prepare(`
INSERT INTO machine_reprovision (*)
VALUES ($machineReprovision.*)
`, machineReprovision{})
	if err != nil {
		return errors.Errorf("preparing reprovision statement: %w", err)
	}
	storageResetStmts, err := st.prepareReprovisionStorageResetStatements()
	if err != nil {
		return errors.Errorf("preparing storage reset statements: %w", err)
	}

	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		var existingReprovision machineReprovision
		if err := tx.Query(ctx, existingReprovisionStmt, machineNameParam).Get(&existingReprovision); err == nil {
			return machineerrors.MachineReprovisionAlreadyExists
		} else if !errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("querying existing reprovision request for machine %q: %w", mName, err)
		}

		var target reprovisionDetachTarget
		if err := tx.Query(ctx, targetStmt, machineNameParam).Get(&target); errors.Is(err, sqlair.ErrNoRows) {
			return machineerrors.MachineNotFound
		} else if err != nil {
			return errors.Errorf("querying reprovision detach target %q: %w", mName, err)
		}

		if err := validateReprovisionDetachTarget(target, expectedInstanceID); err != nil {
			return errors.Capture(err)
		}

		storageParams := reprovisionStorageTargetParams{
			NetNodeUUID:    target.NetNodeUUID,
			AliveLifeID:    int(life.Alive),
			ModelScopeID:   int(domainstorage.ProvisionScopeModel),
			MachineScopeID: int(domainstorage.ProvisionScopeMachine),
		}
		if err := validateReprovisionStorageLives(
			ctx, tx, storageLifeStmt, storageParams,
		); err != nil {
			return errors.Capture(err)
		}
		storageTargets, err := st.getReprovisionStorageTargets(
			ctx, tx, storageTargetStmts, storageParams,
		)
		if err != nil {
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

		if err := st.resetReprovisionStorage(
			ctx, tx, storageResetStmts, storageTargets,
		); err != nil {
			return errors.Errorf("clearing machine-scoped storage: %w", err)
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
		if err := tx.Query(ctx, reprovisionStmt, machineReprovision{
			MachineName: mName,
			RequestedAt: updatedAt,
		}).Run(); err != nil {
			return errors.Errorf("recording reprovision wake-up: %w", err)
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

type reprovisionStorageTargets struct {
	volumes, filesystems                map[string]struct{}
	plans, volumeAttachments            map[string]struct{}
	filesystemAttachments, blockDevices map[string]struct{}
}

type reprovisionStorageTargetStatements struct {
	volumes, volumeLogicalAttachments         *sqlair.Statement
	volumeAttachments, volumePlans            *sqlair.Statement
	filesystems, filesystemLogicalAttachments *sqlair.Statement
	filesystemAttachments                     *sqlair.Statement
}

// prepareReprovisionStorageLifeStatement returns a query for the first
// non-alive lifecycle-managed storage row related to the target machine.
func (st *State) prepareReprovisionStorageLifeStatement() (*sqlair.Statement, error) {
	stmt, err := st.Prepare(`
WITH target_volume_uuids AS (
    SELECT mv.volume_uuid
    FROM machine AS m
    JOIN machine_volume AS mv ON m.uuid = mv.machine_uuid
    WHERE m.net_node_uuid = $reprovisionStorageTargetParams.net_node_uuid

    UNION

    SELECT sva.storage_volume_uuid
    FROM storage_volume_attachment AS sva
    WHERE sva.net_node_uuid = $reprovisionStorageTargetParams.net_node_uuid

    UNION

    SELECT svap.storage_volume_uuid
    FROM storage_volume_attachment_plan AS svap
    WHERE svap.net_node_uuid = $reprovisionStorageTargetParams.net_node_uuid

    UNION

    SELECT siv.storage_volume_uuid
    FROM storage_attachment AS sa
    JOIN unit AS u ON sa.unit_uuid = u.uuid
    JOIN storage_instance_volume AS siv
        ON sa.storage_instance_uuid = siv.storage_instance_uuid
    WHERE u.net_node_uuid = $reprovisionStorageTargetParams.net_node_uuid
),
target_filesystem_uuids AS (
    SELECT mf.filesystem_uuid
    FROM machine AS m
    JOIN machine_filesystem AS mf ON m.uuid = mf.machine_uuid
    WHERE m.net_node_uuid = $reprovisionStorageTargetParams.net_node_uuid

    UNION

    SELECT sfa.storage_filesystem_uuid
    FROM storage_filesystem_attachment AS sfa
    WHERE sfa.net_node_uuid = $reprovisionStorageTargetParams.net_node_uuid

    UNION

    SELECT sif.storage_filesystem_uuid
    FROM storage_attachment AS sa
    JOIN unit AS u ON sa.unit_uuid = u.uuid
    JOIN storage_instance_filesystem AS sif
        ON sa.storage_instance_uuid = sif.storage_instance_uuid
    WHERE u.net_node_uuid = $reprovisionStorageTargetParams.net_node_uuid
),
target_storage_instance_uuids AS (
    SELECT siv.storage_instance_uuid
    FROM storage_instance_volume AS siv
    JOIN target_volume_uuids AS tvu ON siv.storage_volume_uuid = tvu.volume_uuid

    UNION

    SELECT sif.storage_instance_uuid
    FROM storage_instance_filesystem AS sif
    JOIN target_filesystem_uuids AS tfu
        ON sif.storage_filesystem_uuid = tfu.filesystem_uuid
),
target_storage_lives AS (
    SELECT 'volume' AS entity_type, sv.uuid AS entity_uuid,
           sv.life_id AS life_id
    FROM storage_volume AS sv
    JOIN target_volume_uuids AS tvu ON sv.uuid = tvu.volume_uuid

    UNION ALL

    SELECT 'filesystem' AS entity_type, sf.uuid AS entity_uuid,
           sf.life_id AS life_id
    FROM storage_filesystem AS sf
    JOIN target_filesystem_uuids AS tfu ON sf.uuid = tfu.filesystem_uuid

    UNION ALL

    SELECT 'storage instance' AS entity_type, si.uuid AS entity_uuid,
           si.life_id AS life_id
    FROM storage_instance AS si
    JOIN target_storage_instance_uuids AS tsiu ON si.uuid = tsiu.storage_instance_uuid

    UNION ALL

    SELECT 'storage attachment' AS entity_type, sa.uuid AS entity_uuid,
           sa.life_id AS life_id
    FROM storage_attachment AS sa
    JOIN target_storage_instance_uuids AS tsiu
        ON sa.storage_instance_uuid = tsiu.storage_instance_uuid

    UNION ALL

    SELECT 'volume attachment' AS entity_type, sva.uuid AS entity_uuid,
           sva.life_id AS life_id
    FROM storage_volume_attachment AS sva
    JOIN target_volume_uuids AS tvu ON sva.storage_volume_uuid = tvu.volume_uuid

    UNION ALL

    SELECT 'filesystem attachment' AS entity_type, sfa.uuid AS entity_uuid,
           sfa.life_id AS life_id
    FROM storage_filesystem_attachment AS sfa
    JOIN target_filesystem_uuids AS tfu
        ON sfa.storage_filesystem_uuid = tfu.filesystem_uuid

    UNION ALL

    SELECT 'volume attachment plan' AS entity_type, svap.uuid AS entity_uuid,
           svap.life_id AS life_id
    FROM storage_volume_attachment_plan AS svap
    JOIN target_volume_uuids AS tvu ON svap.storage_volume_uuid = tvu.volume_uuid
)
SELECT tsl.entity_type AS &reprovisionStorageLife.entity_type,
       tsl.entity_uuid AS &reprovisionStorageLife.entity_uuid
FROM target_storage_lives AS tsl
WHERE tsl.life_id != $reprovisionStorageTargetParams.alive_life_id
LIMIT 1
`, reprovisionStorageTargetParams{}, reprovisionStorageLife{})
	if err != nil {
		return nil, errors.Capture(err)
	}
	return stmt, nil
}

func (st *State) prepareReprovisionStorageTargetStatements() (
	reprovisionStorageTargetStatements, error,
) {
	var statements reprovisionStorageTargetStatements
	var err error
	statements.volumes, err = st.Prepare(`
WITH target_volume_uuids AS (
    SELECT mv.volume_uuid
    FROM machine AS m
    JOIN machine_volume AS mv ON m.uuid = mv.machine_uuid
    WHERE m.net_node_uuid = $reprovisionStorageTargetParams.net_node_uuid

    UNION

    SELECT sva.storage_volume_uuid
    FROM storage_volume_attachment AS sva
    WHERE sva.net_node_uuid = $reprovisionStorageTargetParams.net_node_uuid

    UNION

    SELECT svap.storage_volume_uuid
    FROM storage_volume_attachment_plan AS svap
    WHERE svap.net_node_uuid = $reprovisionStorageTargetParams.net_node_uuid

    UNION

    SELECT siv.storage_volume_uuid
    FROM storage_attachment AS sa
    JOIN unit AS u ON sa.unit_uuid = u.uuid
    JOIN storage_instance_volume AS siv
        ON sa.storage_instance_uuid = siv.storage_instance_uuid
    WHERE u.net_node_uuid = $reprovisionStorageTargetParams.net_node_uuid
)
SELECT sv.uuid AS &reprovisionStorageEntityTarget.entity_uuid,
       sv.provision_scope_id AS &reprovisionStorageEntityTarget.scope_id,
       siv.storage_instance_uuid AS &reprovisionStorageEntityTarget.storage_instance_uuid
FROM target_volume_uuids AS tvu
JOIN storage_volume AS sv ON tvu.volume_uuid = sv.uuid
LEFT JOIN storage_instance_volume AS siv ON sv.uuid = siv.storage_volume_uuid
`, reprovisionStorageTargetParams{}, reprovisionStorageEntityTarget{})
	if err != nil {
		return reprovisionStorageTargetStatements{}, errors.Capture(err)
	}
	statements.volumeLogicalAttachments, err = st.Prepare(`
SELECT siv.storage_volume_uuid AS &reprovisionStorageLogicalAttachment.entity_uuid
FROM storage_attachment AS sa
JOIN unit AS u ON sa.unit_uuid = u.uuid
JOIN storage_instance_volume AS siv
    ON sa.storage_instance_uuid = siv.storage_instance_uuid
WHERE u.net_node_uuid = $reprovisionStorageTargetParams.net_node_uuid
`, reprovisionStorageTargetParams{}, reprovisionStorageLogicalAttachment{})
	if err != nil {
		return reprovisionStorageTargetStatements{}, errors.Capture(err)
	}
	statements.volumeAttachments, err = st.Prepare(`
SELECT sva.uuid AS &reprovisionStoragePhysicalAttachment.uuid,
       sva.storage_volume_uuid AS &reprovisionStoragePhysicalAttachment.entity_uuid,
       sva.provision_scope_id AS &reprovisionStoragePhysicalAttachment.scope_id,
       sva.block_device_uuid AS &reprovisionStoragePhysicalAttachment.block_device_uuid
FROM storage_volume_attachment AS sva
WHERE sva.net_node_uuid = $reprovisionStorageTargetParams.net_node_uuid
`, reprovisionStorageTargetParams{}, reprovisionStoragePhysicalAttachment{})
	if err != nil {
		return reprovisionStorageTargetStatements{}, errors.Capture(err)
	}
	statements.volumePlans, err = st.Prepare(`
SELECT svap.uuid AS &reprovisionStoragePlanTarget.uuid,
       svap.storage_volume_uuid AS &reprovisionStoragePlanTarget.entity_uuid,
       svap.provision_scope_id AS &reprovisionStoragePlanTarget.scope_id
FROM storage_volume_attachment_plan AS svap
WHERE svap.net_node_uuid = $reprovisionStorageTargetParams.net_node_uuid
`, reprovisionStorageTargetParams{}, reprovisionStoragePlanTarget{})
	if err != nil {
		return reprovisionStorageTargetStatements{}, errors.Capture(err)
	}
	statements.filesystems, err = st.Prepare(`
WITH target_filesystem_uuids AS (
    SELECT mf.filesystem_uuid
    FROM machine AS m
    JOIN machine_filesystem AS mf ON m.uuid = mf.machine_uuid
    WHERE m.net_node_uuid = $reprovisionStorageTargetParams.net_node_uuid

    UNION

    SELECT sfa.storage_filesystem_uuid
    FROM storage_filesystem_attachment AS sfa
    WHERE sfa.net_node_uuid = $reprovisionStorageTargetParams.net_node_uuid

    UNION

    SELECT sif.storage_filesystem_uuid
    FROM storage_attachment AS sa
    JOIN unit AS u ON sa.unit_uuid = u.uuid
    JOIN storage_instance_filesystem AS sif
        ON sa.storage_instance_uuid = sif.storage_instance_uuid
    WHERE u.net_node_uuid = $reprovisionStorageTargetParams.net_node_uuid
)
SELECT sf.uuid AS &reprovisionStorageEntityTarget.entity_uuid,
       sf.provision_scope_id AS &reprovisionStorageEntityTarget.scope_id,
       sif.storage_instance_uuid AS &reprovisionStorageEntityTarget.storage_instance_uuid
FROM target_filesystem_uuids AS tfu
JOIN storage_filesystem AS sf ON tfu.filesystem_uuid = sf.uuid
LEFT JOIN storage_instance_filesystem AS sif
    ON sf.uuid = sif.storage_filesystem_uuid
`, reprovisionStorageTargetParams{}, reprovisionStorageEntityTarget{})
	if err != nil {
		return reprovisionStorageTargetStatements{}, errors.Capture(err)
	}
	statements.filesystemLogicalAttachments, err = st.Prepare(`
SELECT sif.storage_filesystem_uuid AS &reprovisionStorageLogicalAttachment.entity_uuid
FROM storage_attachment AS sa
JOIN unit AS u ON sa.unit_uuid = u.uuid
JOIN storage_instance_filesystem AS sif
    ON sa.storage_instance_uuid = sif.storage_instance_uuid
WHERE u.net_node_uuid = $reprovisionStorageTargetParams.net_node_uuid
`, reprovisionStorageTargetParams{}, reprovisionStorageLogicalAttachment{})
	if err != nil {
		return reprovisionStorageTargetStatements{}, errors.Capture(err)
	}
	statements.filesystemAttachments, err = st.Prepare(`
SELECT sfa.uuid AS &reprovisionStoragePhysicalAttachment.uuid,
       sfa.storage_filesystem_uuid AS &reprovisionStoragePhysicalAttachment.entity_uuid,
       sfa.provision_scope_id AS &reprovisionStoragePhysicalAttachment.scope_id,
       NULL AS &reprovisionStoragePhysicalAttachment.block_device_uuid
FROM storage_filesystem_attachment AS sfa
WHERE sfa.net_node_uuid = $reprovisionStorageTargetParams.net_node_uuid
`, reprovisionStorageTargetParams{}, reprovisionStoragePhysicalAttachment{})
	if err != nil {
		return reprovisionStorageTargetStatements{}, errors.Capture(err)
	}
	return statements, nil
}

// validateReprovisionStorageLives rejects non-alive storage instances,
// volumes, filesystems, attachments, and attachment plans before reset targets
// are collected or any reprovisioning mutations are run.
func validateReprovisionStorageLives(
	ctx context.Context,
	tx *sqlair.TX,
	stmt *sqlair.Statement,
	params reprovisionStorageTargetParams,
) error {
	var invalidLife reprovisionStorageLife
	if err := tx.Query(ctx, stmt, params).Get(&invalidLife); err == nil {
		return errors.Errorf(
			"%s %q: %w", invalidLife.EntityType, invalidLife.EntityUUID,
			machineerrors.MachineStorageNotAlive,
		)
	} else if !errors.Is(err, sqlair.ErrNoRows) {
		return errors.Errorf("validating storage lifecycle: %w", err)
	}
	return nil
}

// getReprovisionStorageTargets captures storage associated with the machine
// after lifecycle validation. Model-scoped, inconsistent, or incomplete
// storage fails closed. Maps ensure each reset statement targets a UUID only
// once.
//
// storage_attachment, machine_volume, and machine_filesystem are retained as
// Juju associations. They participate in target discovery so incomplete
// provider state cannot be skipped.
func (st *State) getReprovisionStorageTargets(
	ctx context.Context, tx *sqlair.TX,
	statements reprovisionStorageTargetStatements,
	params reprovisionStorageTargetParams,
) (reprovisionStorageTargets, error) {
	var volumeRows []reprovisionStorageEntityTarget
	if err := tx.Query(ctx, statements.volumes, params).GetAll(&volumeRows); err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return reprovisionStorageTargets{}, errors.Errorf("querying attached volumes: %w", err)
	}
	var volumeLogicalRows []reprovisionStorageLogicalAttachment
	if err := tx.Query(ctx, statements.volumeLogicalAttachments, params).GetAll(&volumeLogicalRows); err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return reprovisionStorageTargets{}, errors.Errorf("querying logical volume attachments: %w", err)
	}
	var volumeAttachmentRows []reprovisionStoragePhysicalAttachment
	if err := tx.Query(ctx, statements.volumeAttachments, params).GetAll(&volumeAttachmentRows); err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return reprovisionStorageTargets{}, errors.Errorf("querying volume attachments: %w", err)
	}
	var volumePlanRows []reprovisionStoragePlanTarget
	if err := tx.Query(ctx, statements.volumePlans, params).GetAll(&volumePlanRows); err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return reprovisionStorageTargets{}, errors.Errorf("querying volume attachment plans: %w", err)
	}
	var filesystemRows []reprovisionStorageEntityTarget
	if err := tx.Query(ctx, statements.filesystems, params).GetAll(&filesystemRows); err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return reprovisionStorageTargets{}, errors.Errorf("querying attached filesystems: %w", err)
	}
	var filesystemLogicalRows []reprovisionStorageLogicalAttachment
	if err := tx.Query(ctx, statements.filesystemLogicalAttachments, params).GetAll(&filesystemLogicalRows); err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return reprovisionStorageTargets{}, errors.Errorf("querying logical filesystem attachments: %w", err)
	}
	var filesystemAttachmentRows []reprovisionStoragePhysicalAttachment
	if err := tx.Query(ctx, statements.filesystemAttachments, params).GetAll(&filesystemAttachmentRows); err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return reprovisionStorageTargets{}, errors.Errorf("querying filesystem attachments: %w", err)
	}
	logicalAttachments := func(rows []reprovisionStorageLogicalAttachment) map[string]int {
		result := make(map[string]int)
		for _, row := range rows {
			result[row.EntityUUID]++
		}
		return result
	}
	physicalAttachments := func(rows []reprovisionStoragePhysicalAttachment) map[string][]reprovisionStoragePhysicalAttachment {
		result := make(map[string][]reprovisionStoragePhysicalAttachment)
		for _, row := range rows {
			result[row.EntityUUID] = append(result[row.EntityUUID], row)
		}
		return result
	}
	volumeLogical := logicalAttachments(volumeLogicalRows)
	filesystemLogical := logicalAttachments(filesystemLogicalRows)
	volumeAttachments := physicalAttachments(volumeAttachmentRows)
	filesystemAttachments := physicalAttachments(filesystemAttachmentRows)
	volumePlans := make(map[string][]reprovisionStoragePlanTarget)
	for _, row := range volumePlanRows {
		volumePlans[row.EntityUUID] = append(volumePlans[row.EntityUUID], row)
	}

	targets := reprovisionStorageTargets{
		volumes:               make(map[string]struct{}),
		filesystems:           make(map[string]struct{}),
		plans:                 make(map[string]struct{}),
		volumeAttachments:     make(map[string]struct{}),
		filesystemAttachments: make(map[string]struct{}),
		blockDevices:          make(map[string]struct{}),
	}
	for _, row := range volumeRows {
		attachments := volumeAttachments[row.EntityUUID]
		plans := volumePlans[row.EntityUUID]
		if err := validateReprovisionStorageEntity(
			"volume", row, volumeLogical[row.EntityUUID], attachments, plans, params,
		); err != nil {
			return reprovisionStorageTargets{}, errors.Capture(err)
		}
		targets.volumes[row.EntityUUID] = struct{}{}
		for _, attachment := range attachments {
			targets.volumeAttachments[attachment.UUID] = struct{}{}
			if attachment.BlockDeviceUUID.Valid {
				targets.blockDevices[attachment.BlockDeviceUUID.V] = struct{}{}
			}
		}
		for _, plan := range plans {
			targets.plans[plan.UUID] = struct{}{}
		}
	}
	for _, row := range filesystemRows {
		attachments := filesystemAttachments[row.EntityUUID]
		if err := validateReprovisionStorageEntity(
			"filesystem", row, filesystemLogical[row.EntityUUID], attachments, nil, params,
		); err != nil {
			return reprovisionStorageTargets{}, errors.Capture(err)
		}
		targets.filesystems[row.EntityUUID] = struct{}{}
		for _, attachment := range attachments {
			targets.filesystemAttachments[attachment.UUID] = struct{}{}
		}
	}
	return targets, nil
}

func validateReprovisionStorageEntity(
	entityType string,
	entity reprovisionStorageEntityTarget,
	logicalAttachmentCount int,
	attachments []reprovisionStoragePhysicalAttachment,
	plans []reprovisionStoragePlanTarget,
	params reprovisionStorageTargetParams,
) error {
	if !entity.StorageInstanceUUID.Valid {
		return errors.Errorf(
			"%s %q: %w", entityType, entity.EntityUUID,
			machineerrors.StorageScopeAmbiguous,
		)
	}
	if logicalAttachmentCount == 0 || len(attachments) == 0 {
		return errors.Errorf(
			"%s %q: %w", entityType, entity.EntityUUID,
			machineerrors.StorageScopeAmbiguous,
		)
	}

	if entity.ScopeID != params.ModelScopeID && entity.ScopeID != params.MachineScopeID {
		return errors.Errorf(
			"%s %q: %w", entityType, entity.EntityUUID,
			machineerrors.StorageScopeAmbiguous,
		)
	}
	attachmentScopeIDs := make([]int, 0, len(attachments)+len(plans))
	for _, attachment := range attachments {
		attachmentScopeIDs = append(attachmentScopeIDs, attachment.ScopeID)
	}
	for _, plan := range plans {
		attachmentScopeIDs = append(attachmentScopeIDs, plan.ScopeID)
	}
	for _, scopeID := range attachmentScopeIDs {
		if scopeID != entity.ScopeID {
			return errors.Errorf(
				"%s %q: %w", entityType, entity.EntityUUID,
				machineerrors.StorageScopeAmbiguous,
			)
		}
	}
	if entity.ScopeID == params.ModelScopeID {
		return errors.Errorf(
			"%s %q: %w", entityType, entity.EntityUUID,
			machineerrors.ModelScopedStorageAttached,
		)
	}
	return nil
}

type reprovisionStorageResetStatements struct {
	planAttrs, plans                         *sqlair.Statement
	volumeAttachments, filesystemAttachments *sqlair.Statement
	blockDeviceLinks, blockDevices           *sqlair.Statement
	volumes, volumeStatuses                  *sqlair.Statement
	filesystems, filesystemStatuses          *sqlair.Statement
}

func (st *State) prepareReprovisionStorageResetStatements() (
	reprovisionStorageResetStatements, error,
) {
	var statements reprovisionStorageResetStatements
	queries := []struct {
		stmt  **sqlair.Statement
		query string
	}{
		// Plan attributes describe the old provider attachment and cannot be
		// reused.
		{stmt: &statements.planAttrs, query: `
DELETE FROM storage_volume_attachment_plan_attr
WHERE attachment_plan_uuid IN ($reprovisionUUIDs[:])`},
		// Attachment plans must be recalculated for the replacement machine.
		{stmt: &statements.plans, query: `
DELETE FROM storage_volume_attachment_plan
WHERE uuid IN ($reprovisionUUIDs[:])`},
		// Keep attachment intent, but remove evidence of the old provider
		// attachment.
		{stmt: &statements.volumeAttachments, query: `
UPDATE storage_volume_attachment
SET provider_id = NULL,
    block_device_uuid = NULL,
    read_only = NULL
WHERE uuid IN ($reprovisionUUIDs[:])`},
		// Keep attachment identity, but remove completion data reported for the
		// old mount.
		{stmt: &statements.filesystemAttachments, query: `
UPDATE storage_filesystem_attachment
SET provider_id = NULL,
    mount_point = NULL,
    read_only = NULL
WHERE uuid IN ($reprovisionUUIDs[:])`},
		// Attachment references are now clear, so old block-device links can
		// be removed.
		{stmt: &statements.blockDeviceLinks, query: `
DELETE FROM block_device_link_device
WHERE block_device_uuid IN ($reprovisionUUIDs[:])
AND NOT EXISTS (
    SELECT 1 FROM storage_volume_attachment AS sva
    WHERE sva.block_device_uuid = block_device_link_device.block_device_uuid
)`},
		// Remove old block-device evidence only when no attachment still
		// references it.
		{stmt: &statements.blockDevices, query: `
DELETE FROM block_device
WHERE uuid IN ($reprovisionUUIDs[:])
AND NOT EXISTS (
    SELECT 1 FROM storage_volume_attachment AS sva
    WHERE sva.block_device_uuid = block_device.uuid
)`},
		// Clear the old provider realization while retaining the Juju volume
		// identity.
		{stmt: &statements.volumes, query: `
UPDATE storage_volume
SET provider_id = NULL,
    size_mib = NULL,
    hardware_id = NULL,
    wwn = NULL,
    persistent = NULL,
    obliterate_on_cleanup = NULL
WHERE uuid IN ($reprovisionUUIDs[:])`},
		// Ensure the retained volume is visible to provisioning as pending work.
		{stmt: &statements.volumeStatuses, query: `
INSERT INTO storage_volume_status (volume_uuid, status_id, message, updated_at)
SELECT sv.uuid, 0, 'waiting for replacement machine', NULL
FROM storage_volume AS sv
WHERE sv.uuid IN ($reprovisionUUIDs[:])
ON CONFLICT (volume_uuid) DO UPDATE SET
    status_id = excluded.status_id,
    message = excluded.message,
    updated_at = excluded.updated_at`},
		// Clear the old provider realization while retaining the filesystem
		// identity.
		{stmt: &statements.filesystems, query: `
UPDATE storage_filesystem
SET provider_id = NULL,
    size_mib = NULL,
    obliterate_on_cleanup = NULL
WHERE uuid IN ($reprovisionUUIDs[:])`},
		// Ensure the retained filesystem is visible to provisioning as pending
		// work.
		{stmt: &statements.filesystemStatuses, query: `
INSERT INTO storage_filesystem_status (filesystem_uuid, status_id, message, updated_at)
SELECT sf.uuid, 0, 'waiting for replacement machine', NULL
FROM storage_filesystem AS sf
WHERE sf.uuid IN ($reprovisionUUIDs[:])
ON CONFLICT (filesystem_uuid) DO UPDATE SET
    status_id = excluded.status_id,
    message = excluded.message,
    updated_at = excluded.updated_at`},
	}
	for _, item := range queries {
		stmt, err := st.Prepare(item.query, reprovisionUUIDs{})
		if err != nil {
			return reprovisionStorageResetStatements{}, errors.Capture(err)
		}
		*item.stmt = stmt
	}
	return statements, nil
}

// resetReprovisionStorage discards provider-observed storage state for the
// lost machine while preserving Juju storage identities and intent. Volume,
// filesystem, instance-link, machine-link, and attachment rows remain so the
// normal provisioning paths can create empty replacement storage.
//
// Provider-specific attachment plans are deleted first. Attachments are then
// marked unprovisioned before their old block devices are removed. Finally the
// volume and filesystem provider fields are cleared and their statuses are
// moved back to pending.
func (st *State) resetReprovisionStorage(
	ctx context.Context, tx *sqlair.TX,
	statements reprovisionStorageResetStatements,
	targets reprovisionStorageTargets,
) error {
	ids := func(values map[string]struct{}) reprovisionUUIDs {
		return reprovisionUUIDs(slices.Collect(maps.Keys(values)))
	}

	var (
		plans                 = ids(targets.plans)
		volumeAttachments     = ids(targets.volumeAttachments)
		filesystemAttachments = ids(targets.filesystemAttachments)
		blockDevices          = ids(targets.blockDevices)
		volumes               = ids(targets.volumes)
		filesystems           = ids(targets.filesystems)
	)

	steps := []struct {
		stmt *sqlair.Statement
		ids  reprovisionUUIDs
	}{
		{stmt: statements.planAttrs, ids: plans},
		{stmt: statements.plans, ids: plans},
		{stmt: statements.volumeAttachments, ids: volumeAttachments},
		{stmt: statements.filesystemAttachments, ids: filesystemAttachments},
		{stmt: statements.blockDeviceLinks, ids: blockDevices},
		{stmt: statements.blockDevices, ids: blockDevices},
		{stmt: statements.volumes, ids: volumes},
		{stmt: statements.volumeStatuses, ids: volumes},
		{stmt: statements.filesystems, ids: filesystems},
		{stmt: statements.filesystemStatuses, ids: filesystems},
	}
	for _, step := range steps {
		if len(step.ids) == 0 {
			continue
		}
		if err := tx.Query(ctx, step.stmt, step.ids).Run(); err != nil {
			return errors.Capture(err)
		}
	}
	return nil
}
