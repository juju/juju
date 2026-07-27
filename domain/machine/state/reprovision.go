// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
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
// physical storage realization is deleted while logical storage intent is
// preserved. Unsupported storage is rejected in the same transaction.
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
	volumeTargetStmt, filesystemTargetStmt, err := st.prepareReprovisionStorageTargetStatements()
	if err != nil {
		return errors.Errorf("preparing storage target statements: %w", err)
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

		storageTargets, err := st.getReprovisionStorageTargets(
			ctx, tx, volumeTargetStmt, filesystemTargetStmt,
			reprovisionStorageTargetParams{
				NetNodeUUID:    target.NetNodeUUID,
				ModelScopeID:   int(domainstorage.ProvisionScopeModel),
				MachineScopeID: int(domainstorage.ProvisionScopeMachine),
			},
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

		if err := st.removeReprovisionStorage(
			ctx, tx, storageTargets,
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
	volumes, filesystems                reprovisionUUIDs
	plans, volumeAttachments            reprovisionUUIDs
	filesystemAttachments, blockDevices reprovisionUUIDs
}

func (st *State) prepareReprovisionStorageTargetStatements() (
	*sqlair.Statement, *sqlair.Statement, error,
) {
	volumeStmt, err := st.Prepare(`
WITH volume_targets AS (
    SELECT DISTINCT sv.uuid AS volume_uuid,
           sv.provision_scope_id AS volume_scope_id,
           sva.uuid AS attachment_uuid,
           sva.provision_scope_id AS attachment_scope_id,
           svap.uuid AS plan_uuid,
           svap.provision_scope_id AS plan_scope_id,
           sva.block_device_uuid AS block_device_uuid
    FROM storage_volume AS sv
    LEFT JOIN storage_volume_attachment AS sva
        ON sv.uuid = sva.storage_volume_uuid
        AND sva.net_node_uuid = $reprovisionStorageTargetParams.net_node_uuid
    LEFT JOIN storage_volume_attachment_plan AS svap
        ON sv.uuid = svap.storage_volume_uuid
        AND svap.net_node_uuid = $reprovisionStorageTargetParams.net_node_uuid
    WHERE sva.uuid IS NOT NULL OR svap.uuid IS NOT NULL
),
classified_volume_targets AS (
    SELECT vt.volume_uuid,
           vt.attachment_uuid,
           vt.plan_uuid,
           vt.block_device_uuid,
           CASE
               WHEN vt.volume_scope_id NOT IN (
                   $reprovisionStorageTargetParams.model_scope_id,
                   $reprovisionStorageTargetParams.machine_scope_id
               ) THEN 'ambiguous'
               WHEN vt.attachment_uuid IS NOT NULL
                   AND vt.attachment_scope_id != vt.volume_scope_id
                   THEN 'ambiguous'
               WHEN vt.plan_uuid IS NOT NULL
                   AND vt.plan_scope_id != vt.volume_scope_id
                   THEN 'ambiguous'
               WHEN vt.volume_scope_id = $reprovisionStorageTargetParams.model_scope_id
                   THEN 'model'
               ELSE 'machine'
           END AS scope_class
    FROM volume_targets AS vt
)
SELECT cvt.volume_uuid AS &reprovisionVolumeTarget.volume_uuid,
       cvt.scope_class AS &reprovisionVolumeTarget.scope_class,
       cvt.attachment_uuid AS &reprovisionVolumeTarget.attachment_uuid,
       cvt.plan_uuid AS &reprovisionVolumeTarget.plan_uuid,
       cvt.block_device_uuid AS &reprovisionVolumeTarget.block_device_uuid
FROM classified_volume_targets AS cvt`, reprovisionStorageTargetParams{}, reprovisionVolumeTarget{})
	if err != nil {
		return nil, nil, errors.Capture(err)
	}
	filesystemStmt, err := st.Prepare(`
WITH filesystem_targets AS (
    SELECT sf.uuid AS filesystem_uuid,
           sf.provision_scope_id AS filesystem_scope_id,
           sfa.uuid AS attachment_uuid,
           sfa.provision_scope_id AS attachment_scope_id
    FROM storage_filesystem AS sf
    JOIN storage_filesystem_attachment AS sfa
        ON sf.uuid = sfa.storage_filesystem_uuid
    WHERE sfa.net_node_uuid = $reprovisionStorageTargetParams.net_node_uuid
),
classified_filesystem_targets AS (
    SELECT ft.filesystem_uuid,
           ft.attachment_uuid,
           CASE
               WHEN ft.filesystem_scope_id NOT IN (
                   $reprovisionStorageTargetParams.model_scope_id,
                   $reprovisionStorageTargetParams.machine_scope_id
               ) THEN 'ambiguous'
               WHEN ft.attachment_scope_id != ft.filesystem_scope_id
                   THEN 'ambiguous'
               WHEN ft.filesystem_scope_id = $reprovisionStorageTargetParams.model_scope_id
                   THEN 'model'
               ELSE 'machine'
           END AS scope_class
    FROM filesystem_targets AS ft
)
SELECT cft.filesystem_uuid AS &reprovisionFilesystemTarget.filesystem_uuid,
       cft.scope_class AS &reprovisionFilesystemTarget.scope_class,
       cft.attachment_uuid AS &reprovisionFilesystemTarget.attachment_uuid
FROM classified_filesystem_targets AS cft`, reprovisionStorageTargetParams{}, reprovisionFilesystemTarget{})
	if err != nil {
		return nil, nil, errors.Capture(err)
	}
	return volumeStmt, filesystemStmt, nil
}

func (st *State) getReprovisionStorageTargets(
	ctx context.Context, tx *sqlair.TX,
	volumeStmt, filesystemStmt *sqlair.Statement,
	params reprovisionStorageTargetParams,
) (reprovisionStorageTargets, error) {
	var volumeRows []reprovisionVolumeTarget
	if err := tx.Query(ctx, volumeStmt, params).GetAll(&volumeRows); err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return reprovisionStorageTargets{}, errors.Errorf("querying attached volumes: %w", err)
	}
	var filesystemRows []reprovisionFilesystemTarget
	if err := tx.Query(ctx, filesystemStmt, params).GetAll(&filesystemRows); err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return reprovisionStorageTargets{}, errors.Errorf("querying attached filesystems: %w", err)
	}

	var targets reprovisionStorageTargets
	add := func(ids reprovisionUUIDs, value string) reprovisionUUIDs {
		if slices.Contains(ids, value) {
			return ids
		}
		return append(ids, value)
	}
	for _, row := range volumeRows {
		switch row.ScopeClass {
		case "model":
			return reprovisionStorageTargets{}, errors.Errorf(
				"volume %q: %w", row.VolumeUUID, machineerrors.ModelScopedStorageAttached,
			)
		case "machine":
			// An empty Go case exits the switch; it does not fall through.
		case "ambiguous":
			return reprovisionStorageTargets{}, errors.Errorf(
				"volume %q: %w", row.VolumeUUID, machineerrors.StorageScopeAmbiguous,
			)
		default:
			return reprovisionStorageTargets{}, errors.Errorf(
				"volume %q has unknown scope classification %q: %w",
				row.VolumeUUID, row.ScopeClass, machineerrors.StorageScopeAmbiguous,
			)
		}
		targets.volumes = add(targets.volumes, row.VolumeUUID)
		if row.AttachmentUUID.Valid {
			targets.volumeAttachments = add(targets.volumeAttachments, row.AttachmentUUID.V)
		}
		if row.PlanUUID.Valid {
			targets.plans = add(targets.plans, row.PlanUUID.V)
		}
		if row.BlockDeviceUUID.Valid {
			targets.blockDevices = add(targets.blockDevices, row.BlockDeviceUUID.V)
		}
	}
	for _, row := range filesystemRows {
		switch row.ScopeClass {
		case "model":
			return reprovisionStorageTargets{}, errors.Errorf(
				"filesystem %q: %w", row.FilesystemUUID, machineerrors.ModelScopedStorageAttached,
			)
		case "machine":
			// An empty Go case exits the switch; it does not fall through.
		case "ambiguous":
			return reprovisionStorageTargets{}, errors.Errorf(
				"filesystem %q: %w", row.FilesystemUUID, machineerrors.StorageScopeAmbiguous,
			)
		default:
			return reprovisionStorageTargets{}, errors.Errorf(
				"filesystem %q has unknown scope classification %q: %w",
				row.FilesystemUUID, row.ScopeClass, machineerrors.StorageScopeAmbiguous,
			)
		}
		targets.filesystems = add(targets.filesystems, row.FilesystemUUID)
		targets.filesystemAttachments = add(targets.filesystemAttachments, row.AttachmentUUID)
	}
	return targets, nil
}

func (st *State) removeReprovisionStorage(
	ctx context.Context, tx *sqlair.TX,
	targets reprovisionStorageTargets,
) error {
	run := func(query string, ids reprovisionUUIDs) error {
		if len(ids) == 0 {
			return nil
		}
		stmt, err := st.Prepare(query, reprovisionUUIDs{})
		if err != nil {
			return errors.Capture(err)
		}
		return errors.Capture(tx.Query(ctx, stmt, ids).Run())
	}
	steps := []struct {
		query string
		ids   reprovisionUUIDs
	}{
		{query: `DELETE FROM storage_volume_attachment_plan_attr WHERE attachment_plan_uuid IN ($reprovisionUUIDs[:])`, ids: targets.plans},
		{query: `DELETE FROM storage_volume_attachment_plan WHERE uuid IN ($reprovisionUUIDs[:])`, ids: targets.plans},
		{query: `DELETE FROM storage_volume_attachment WHERE uuid IN ($reprovisionUUIDs[:])`, ids: targets.volumeAttachments},
		{query: `DELETE FROM storage_filesystem_attachment WHERE uuid IN ($reprovisionUUIDs[:])`, ids: targets.filesystemAttachments},
		{query: `
DELETE FROM block_device_link_device
WHERE block_device_uuid IN ($reprovisionUUIDs[:])
AND NOT EXISTS (
    SELECT 1 FROM storage_volume_attachment AS sva
    WHERE sva.block_device_uuid = block_device_link_device.block_device_uuid
)`, ids: targets.blockDevices},
		{query: `
DELETE FROM block_device
WHERE uuid IN ($reprovisionUUIDs[:])
AND NOT EXISTS (
    SELECT 1 FROM storage_volume_attachment AS sva
    WHERE sva.block_device_uuid = block_device.uuid
)`, ids: targets.blockDevices},
		{query: `DELETE FROM machine_volume WHERE volume_uuid IN ($reprovisionUUIDs[:])`, ids: targets.volumes},
		{query: `DELETE FROM storage_volume_status WHERE volume_uuid IN ($reprovisionUUIDs[:])`, ids: targets.volumes},
		{query: `DELETE FROM storage_instance_volume WHERE storage_volume_uuid IN ($reprovisionUUIDs[:])`, ids: targets.volumes},
		{`DELETE FROM storage_volume WHERE uuid IN ($reprovisionUUIDs[:])`, targets.volumes},
		{query: `DELETE FROM machine_filesystem WHERE filesystem_uuid IN ($reprovisionUUIDs[:])`, ids: targets.filesystems},
		{query: `DELETE FROM storage_filesystem_status WHERE filesystem_uuid IN ($reprovisionUUIDs[:])`, ids: targets.filesystems},
		{query: `DELETE FROM storage_instance_filesystem WHERE storage_filesystem_uuid IN ($reprovisionUUIDs[:])`, ids: targets.filesystems},
		{query: `DELETE FROM storage_filesystem WHERE uuid IN ($reprovisionUUIDs[:])`, ids: targets.filesystems},
	}
	for _, step := range steps {
		if err := run(step.query, step.ids); err != nil {
			return errors.Capture(err)
		}
	}
	return nil
}
