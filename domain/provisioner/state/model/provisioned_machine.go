// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"context"
	"database/sql"
	"slices"
	"strings"

	"github.com/canonical/sqlair"

	domainmachine "github.com/juju/juju/domain/machine"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	networkerrors "github.com/juju/juju/domain/network/errors"
	"github.com/juju/juju/domain/provisioner"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/uuid"
)

// RecordProvisionedMachine persists the complete result of a successful
// provider StartInstance call in a single transaction, covering volumes,
// volume attachments, and cloud instance identity.
//
// The cloud-instance write is deliberately last: it emits the change-stream
// notification that wakes the instance-poller, so the poller never observes a
// newly registered instance before its provisioning state is available.
func (st *State) RecordProvisionedMachine(
	ctx context.Context,
	machineUUID string,
	info provisioner.ProvisionedMachineInfo,
) error {
	db, err := st.DB(ctx)
	if err != nil {
		return errors.Capture(err)
	}

	// ---- Statement preparation ----

	// Volume statements
	var (
		getVolumeUUIDStmt         *sqlair.Statement
		updateVolumeStmt          *sqlair.Statement
		getNetNodeStmt            *sqlair.Statement
		getAttachmentUUIDStmt     *sqlair.Statement
		getPlanUUIDStmt           *sqlair.Statement
		getBlockDevicesStmt       *sqlair.Statement
		getBlockDeviceLinksStmt   *sqlair.Statement
		insertBlockDeviceStmt     *sqlair.Statement
		insertBlockDeviceLinkStmt *sqlair.Statement
		updateAttachmentStmt      *sqlair.Statement
		updatePlanStmt            *sqlair.Statement
		deletePlanAttrsStmt       *sqlair.Statement
		insertPlanAttrStmt        *sqlair.Statement
	)
	if len(info.VolumeAttachments) > 0 {
		getNetNodeStmt, err = st.Prepare(`
SELECT &provNetNodeUUID.*
FROM   machine
WHERE  uuid = $provMachineUUIDParam.uuid
`, provNetNodeUUID{}, provMachineUUIDParam{})
		if err != nil {
			return errors.Errorf("preparing net node lookup: %w", err)
		}
	}
	if len(info.Volumes) > 0 || len(info.VolumeAttachments) > 0 {
		getVolumeUUIDStmt, err = st.Prepare(`
SELECT &provVolumeUUID.*
FROM   storage_volume
WHERE  volume_id = $provVolumeID.volume_id
`, provVolumeUUID{}, provVolumeID{})
		if err != nil {
			return errors.Errorf("preparing volume UUID lookup: %w", err)
		}
	}
	if len(info.Volumes) > 0 {
		updateVolumeStmt, err = st.Prepare(`
UPDATE storage_volume
SET    provider_id  = $provVolumeProvisionedInfo.provider_id,
       size_mib     = $provVolumeProvisionedInfo.size_mib,
       hardware_id  = $provVolumeProvisionedInfo.hardware_id,
       wwn          = $provVolumeProvisionedInfo.wwn,
       persistent   = $provVolumeProvisionedInfo.persistent
WHERE  uuid = $provVolumeProvisionedInfo.uuid
`, provVolumeProvisionedInfo{})
		if err != nil {
			return errors.Errorf("preparing volume update: %w", err)
		}
	}
	if len(info.VolumeAttachments) > 0 {
		getAttachmentUUIDStmt, err = st.Prepare(`
SELECT &provEntityUUID.*
FROM   storage_volume_attachment
WHERE  storage_volume_uuid = $provVolumeUUID.uuid
AND    net_node_uuid        = $provNetNodeUUID.net_node_uuid
`, provEntityUUID{}, provVolumeUUID{}, provNetNodeUUID{})
		if err != nil {
			return errors.Errorf("preparing attachment UUID lookup: %w", err)
		}
		getPlanUUIDStmt, err = st.Prepare(`
SELECT &provEntityUUID.*
FROM   storage_volume_attachment_plan
WHERE  storage_volume_uuid = $provVolumeUUID.uuid
AND    net_node_uuid        = $provNetNodeUUID.net_node_uuid
`, provEntityUUID{}, provVolumeUUID{}, provNetNodeUUID{})
		if err != nil {
			return errors.Errorf("preparing plan UUID lookup: %w", err)
		}
		getBlockDevicesStmt, err = st.Prepare(`
SELECT &provBlockDeviceRow.*
FROM   block_device
WHERE  machine_uuid = $provMachineUUIDParam.uuid
`, provBlockDeviceRow{}, provMachineUUIDParam{})
		if err != nil {
			return errors.Errorf("preparing block device lookup: %w", err)
		}
		getBlockDeviceLinksStmt, err = st.Prepare(`
SELECT &provBlockDeviceLinkRow.*
FROM   block_device_link_device
WHERE  machine_uuid = $provMachineUUIDParam.uuid
`, provBlockDeviceLinkRow{}, provMachineUUIDParam{})
		if err != nil {
			return errors.Errorf("preparing block device links lookup: %w", err)
		}
		insertBlockDeviceStmt, err = st.Prepare(`
INSERT INTO block_device (uuid, machine_uuid, name, bus_address)
VALUES      ($provNewBlockDeviceRow.*)
ON CONFLICT (uuid) DO NOTHING
`, provNewBlockDeviceRow{})
		if err != nil {
			return errors.Errorf("preparing block device insert: %w", err)
		}
		insertBlockDeviceLinkStmt, err = st.Prepare(`
INSERT INTO block_device_link_device (block_device_uuid, machine_uuid, name)
VALUES      ($provBlockDeviceLinkRow.*)
ON CONFLICT DO NOTHING
`, provBlockDeviceLinkRow{})
		if err != nil {
			return errors.Errorf("preparing block device link insert: %w", err)
		}
		updateAttachmentStmt, err = st.Prepare(`
UPDATE storage_volume_attachment
SET    read_only         = $provAttachmentProvisionedInfo.read_only,
       block_device_uuid = $provAttachmentProvisionedInfo.block_device_uuid
WHERE  uuid = $provAttachmentProvisionedInfo.uuid
`, provAttachmentProvisionedInfo{})
		if err != nil {
			return errors.Errorf("preparing attachment update: %w", err)
		}
		updatePlanStmt, err = st.Prepare(`
UPDATE storage_volume_attachment_plan
SET    device_type_id = $provPlanProvisionedInfo.device_type_id
WHERE  uuid = $provPlanProvisionedInfo.uuid
`, provPlanProvisionedInfo{})
		if err != nil {
			return errors.Errorf("preparing plan update: %w", err)
		}
		deletePlanAttrsStmt, err = st.Prepare(`
DELETE FROM storage_volume_attachment_plan_attr
WHERE       attachment_plan_uuid = $provPlanUUIDParam.uuid
`, provPlanUUIDParam{})
		if err != nil {
			return errors.Errorf("preparing plan attr delete: %w", err)
		}
		insertPlanAttrStmt, err = st.Prepare(`
INSERT INTO storage_volume_attachment_plan_attr (attachment_plan_uuid, key, value)
VALUES      ($provPlanAttrRow.*)
ON CONFLICT (attachment_plan_uuid, key) DO UPDATE SET value = EXCLUDED.value
`, provPlanAttrRow{})
		if err != nil {
			return errors.Errorf("preparing plan attr insert: %w", err)
		}
	}

	// Cloud instance statements.
	setInstanceDataStmt, err := st.Prepare(`
UPDATE machine_cloud_instance
SET
	  instance_id=$provInstanceData.instance_id,
	  display_name=$provInstanceData.display_name,
	  arch=$provInstanceData.arch,
	  mem=$provInstanceData.mem,
	  root_disk=$provInstanceData.root_disk,
	  root_disk_source=$provInstanceData.root_disk_source,
	  cpu_cores=$provInstanceData.cpu_cores,
	  cpu_power=$provInstanceData.cpu_power,
	  virt_type=$provInstanceData.virt_type,
	  availability_zone_uuid=$provInstanceData.availability_zone_uuid
WHERE machine_uuid=$provInstanceData.machine_uuid
`, provInstanceData{})
	if err != nil {
		return errors.Capture(err)
	}
	mNonce := provMachineNonce{
		MachineUUID: machineUUID,
		Nonce:       info.Nonce,
	}
	setNonceStmt, err := st.Prepare(`
UPDATE machine
SET    nonce = $provMachineNonce.nonce
WHERE  uuid = $provMachineNonce.machine_uuid
AND    (nonce IS NULL OR nonce = '')
`, mNonce)
	if err != nil {
		return errors.Capture(err)
	}
	setInstanceTagStmt, err := st.Prepare(`
INSERT INTO instance_tag (*)
VALUES ($provInstanceTag.*)
`, provInstanceTag{})
	if err != nil {
		return errors.Capture(err)
	}
	azName := provAZName{}
	if info.HardwareCharacteristics != nil && info.HardwareCharacteristics.AvailabilityZone != nil {
		azName = provAZName{Name: *info.HardwareCharacteristics.AvailabilityZone}
	}
	retrieveAZUUIDStmt, err := st.Prepare(`
SELECT &provAZName.uuid
FROM   availability_zone
WHERE  availability_zone.name = $provAZName.name
`, azName)
	if err != nil {
		return errors.Capture(err)
	}
	checkInstanceIDStmt, err := st.Prepare(`
SELECT &provInstanceID.instance_id
FROM   machine_cloud_instance
WHERE  machine_uuid = $provMachineUUIDParam.uuid;
`, provInstanceID{}, provMachineUUIDParam{})
	if err != nil {
		return errors.Capture(err)
	}
	setManualStmt, err := st.Prepare(`
INSERT INTO machine_manual (machine_uuid)
VALUES ($provMachineUUIDParam.uuid)
ON CONFLICT (machine_uuid) DO NOTHING
`, provMachineUUIDParam{})
	if err != nil {
		return errors.Capture(err)
	}

	// Pre-compute instanceID/displayName nullables.
	instanceID := info.InstanceID
	displayName := info.DisplayName

	var instID sql.Null[string]
	if v := instanceID.String(); v != "" {
		instID = sql.Null[string]{V: v, Valid: true}
	}
	var disName sql.Null[string]
	if v := displayName; v != "" {
		disName = sql.Null[string]{V: v, Valid: true}
	}

	// ---- Single transaction ----
	return db.Txn(ctx, func(ctx context.Context, tx *sqlair.TX) error {
		// 1. Volumes
		for _, v := range info.Volumes {
			vID := provVolumeID{VolumeID: v.VolumeID}
			var vUUID provVolumeUUID
			if err := tx.Query(ctx, getVolumeUUIDStmt, vID).Get(&vUUID); err != nil {
				if errors.Is(err, sqlair.ErrNoRows) {
					return errors.Errorf("volume %q does not exist", v.VolumeID)
				}
				return errors.Errorf("getting UUID for volume %q: %w", v.VolumeID, err)
			}
			volInfo := provVolumeProvisionedInfo{
				UUID:       vUUID.UUID,
				ProviderID: v.ProviderID,
				SizeMiB:    v.SizeMiB,
				HardwareID: v.HardwareID,
				WWN:        v.WWN,
				Persistent: v.Persistent,
			}
			if err := tx.Query(ctx, updateVolumeStmt, volInfo).Run(); err != nil {
				return errors.Errorf("updating provisioned info for volume %q: %w", v.VolumeID, err)
			}
		}

		// 2. Volume attachments
		if len(info.VolumeAttachments) > 0 {
			mUUIDParam := provMachineUUIDParam{UUID: machineUUID}
			var netNode provNetNodeUUID
			if err := tx.Query(ctx, getNetNodeStmt, mUUIDParam).Get(&netNode); err != nil {
				if errors.Is(err, sqlair.ErrNoRows) {
					return errors.Errorf("machine %q does not exist: %w", machineUUID, machineerrors.MachineNotFound)
				}
				return errors.Errorf("getting net node for machine %q: %w", machineUUID, err)
			}
			var existingDevices []provBlockDeviceRow
			if err := tx.Query(ctx, getBlockDevicesStmt, mUUIDParam).GetAll(&existingDevices); err != nil &&
				!errors.Is(err, sqlair.ErrNoRows) {
				return errors.Errorf("loading block devices for machine %q: %w", machineUUID, err)
			}
			var existingLinks []provBlockDeviceLinkRow
			if err := tx.Query(ctx, getBlockDeviceLinksStmt, mUUIDParam).GetAll(&existingLinks); err != nil &&
				!errors.Is(err, sqlair.ErrNoRows) {
				return errors.Errorf("loading block device links for machine %q: %w", machineUUID, err)
			}
			linksByDevice := make(map[string][]string, len(existingLinks))
			for _, l := range existingLinks {
				linksByDevice[l.BlockDeviceUUID] = append(linksByDevice[l.BlockDeviceUUID], l.LinkName)
			}
			for volumeID, attachment := range info.VolumeAttachments {
				vID := provVolumeID{VolumeID: volumeID}
				var vUUID provVolumeUUID
				if err := tx.Query(ctx, getVolumeUUIDStmt, vID).Get(&vUUID); err != nil {
					if errors.Is(err, sqlair.ErrNoRows) {
						return errors.Errorf("volume %q does not exist", volumeID)
					}
					return errors.Errorf("getting UUID for volume %q: %w", volumeID, err)
				}
				var attachmentUUID provEntityUUID
				if err := tx.Query(ctx, getAttachmentUUIDStmt, vUUID, netNode).Get(&attachmentUUID); err != nil {
					if errors.Is(err, sqlair.ErrNoRows) {
						return errors.Errorf("attachment for volume %q on machine %q does not exist", volumeID, machineUUID)
					}
					return errors.Errorf("getting attachment UUID for volume %q: %w", volumeID, err)
				}
				attachInfo := provAttachmentProvisionedInfo{
					UUID:     attachmentUUID.UUID,
					ReadOnly: attachment.ReadOnly,
				}
				if attachment.DeviceName != "" || attachment.DeviceLink != "" || attachment.BusAddress != "" {
					bdUUID, err := st.matchOrCreateBlockDevice(
						ctx, tx,
						machineUUID,
						attachment.DeviceName,
						attachment.BusAddress,
						attachment.DeviceLink,
						existingDevices, linksByDevice,
						insertBlockDeviceStmt, insertBlockDeviceLinkStmt,
					)
					if err != nil {
						return errors.Errorf("matching/creating block device for volume %q: %w", volumeID, err)
					}
					attachInfo.BlockDeviceUUID = sql.Null[string]{V: bdUUID, Valid: true}
				}
				if err := tx.Query(ctx, updateAttachmentStmt, attachInfo).Run(); err != nil {
					return errors.Errorf("updating attachment for volume %q: %w", volumeID, err)
				}
				if attachment.Plan == nil {
					continue
				}
				var planUUID provEntityUUID
				if err := tx.Query(ctx, getPlanUUIDStmt, vUUID, netNode).Get(&planUUID); err != nil {
					if errors.Is(err, sqlair.ErrNoRows) {
						return errors.Errorf("attachment plan for volume %q on machine %q does not exist", volumeID, machineUUID)
					}
					return errors.Errorf("getting plan UUID for volume %q: %w", volumeID, err)
				}
				deviceTypeID, err := volumeDeviceTypeToID(attachment.Plan.DeviceType)
				if err != nil {
					return errors.Errorf("parsing device type for volume %q plan: %w", volumeID, err)
				}
				planInfo := provPlanProvisionedInfo{
					UUID:         planUUID.UUID,
					DeviceTypeID: deviceTypeID,
				}
				if err := tx.Query(ctx, updatePlanStmt, planInfo).Run(); err != nil {
					return errors.Errorf("updating plan for volume %q: %w", volumeID, err)
				}
				if err := tx.Query(ctx, deletePlanAttrsStmt, provPlanUUIDParam{UUID: planUUID.UUID}).Run(); err != nil {
					return errors.Errorf("deleting plan attrs for volume %q: %w", volumeID, err)
				}
				if len(attachment.Plan.DeviceAttributes) > 0 {
					attrs := make([]provPlanAttrRow, 0, len(attachment.Plan.DeviceAttributes))
					for k, v := range attachment.Plan.DeviceAttributes {
						attrs = append(attrs, provPlanAttrRow{
							PlanUUID: planUUID.UUID,
							Key:      k,
							Value:    v,
						})
					}
					if err := tx.Query(ctx, insertPlanAttrStmt, attrs).Run(); err != nil {
						return errors.Errorf("inserting plan attrs for volume %q: %w", volumeID, err)
					}
				}
			}
		}

		// 3. Cloud instance (last, for change-stream notification).
		mUUIDParam := provMachineUUIDParam{UUID: machineUUID}
		var existing provInstanceID
		if err := tx.Query(ctx, checkInstanceIDStmt, mUUIDParam).Get(&existing); err != nil &&
			!errors.Is(err, sqlair.ErrNoRows) {
			return errors.Errorf("querying instance id for machine %q: %w", machineUUID, err)
		} else if existing.InstanceID != "" {
			return errors.Errorf("%w for machine %q", machineerrors.MachineCloudInstanceAlreadyExists, machineUUID)
		}
		if err := tx.Query(ctx, setNonceStmt, mNonce).Run(); err != nil {
			return errors.Errorf("setting nonce for machine %q: %w", machineUUID, err)
		}
		if strings.HasPrefix(instanceID.String(), domainmachine.ManualInstancePrefix) {
			if err := tx.Query(ctx, setManualStmt, mUUIDParam).Run(); err != nil {
				return errors.Errorf("inserting manual machine entry for machine %q: %w", machineUUID, err)
			}
		}
		instanceData := provInstanceData{
			MachineUUID: machineUUID,
			InstanceID:  instID,
			DisplayName: disName,
		}
		hc := info.HardwareCharacteristics
		if hc != nil {
			instanceData.Arch = hc.Arch
			instanceData.Mem = hc.Mem
			instanceData.RootDisk = hc.RootDisk
			instanceData.RootDiskSource = hc.RootDiskSource
			instanceData.CPUCores = hc.CpuCores
			instanceData.CPUPower = hc.CpuPower
			instanceData.VirtType = hc.VirtType
		}
		if hc != nil && hc.AvailabilityZone != nil && *hc.AvailabilityZone != "" {
			var azUUID provAZName
			if err := tx.Query(ctx, retrieveAZUUIDStmt, azName).Get(&azUUID); err != nil {
				if errors.Is(err, sqlair.ErrNoRows) {
					return errors.Errorf(
						"%w %q for machine %q",
						networkerrors.AvailabilityZoneNotFound,
						*hc.AvailabilityZone,
						machineUUID,
					)
				}
				return errors.Errorf(
					"retrieving availability zone %q for machine %q: %w",
					*hc.AvailabilityZone, machineUUID, err,
				)
			}
			instanceData.AvailabilityZoneUUID = &azUUID.UUID
		}
		if err := tx.Query(ctx, setInstanceDataStmt, instanceData).Run(); err != nil {
			return errors.Errorf("updating machine cloud instance for machine %q: %w", machineUUID, err)
		}
		if tags := provInstanceTagsFrom(machineUUID, hc); len(tags) > 0 {
			if err := tx.Query(ctx, setInstanceTagStmt, tags).Run(); err != nil {
				return errors.Errorf("inserting instance tags for machine %q: %w", machineUUID, err)
			}
		}
		return nil
	})
}

// matchOrCreateBlockDevice finds an existing block device on the machine that
// matches the given device name, bus address, and device links; or creates a
// new one if no match is found. Returns the UUID of the matched or created
// block device.
//
// This is an inline equivalent of blockdevice/service.MatchOrCreateBlockDevice,
// operating within the caller's transaction.
func (st *State) matchOrCreateBlockDevice(
	ctx context.Context,
	tx *sqlair.TX,
	machineUUID string,
	deviceName, busAddress, deviceLink string,
	existing []provBlockDeviceRow,
	linksByDevice map[string][]string,
	insertDeviceStmt, insertLinkStmt *sqlair.Statement,
) (string, error) {
	// Match against existing devices by name, bus address, or device link.
	for _, d := range existing {
		if deviceName != "" && d.Name == deviceName {
			return d.UUID, nil
		}
		if busAddress != "" && d.BusAddress == busAddress {
			return d.UUID, nil
		}
		if deviceLink != "" {
			if slices.Contains(linksByDevice[d.UUID], deviceLink) {
				return d.UUID, nil
			}
		}
	}

	// No match — create a new block device.
	newUUID, err := uuid.NewUUID()
	if err != nil {
		return "", errors.Errorf("generating block device UUID: %w", err)
	}
	bdUUID := newUUID.String()

	row := provNewBlockDeviceRow{
		UUID:        bdUUID,
		MachineUUID: machineUUID,
		Name:        deviceName,
		BusAddress:  busAddress,
	}
	if err := tx.Query(ctx, insertDeviceStmt, row).Run(); err != nil {
		return "", errors.Errorf("inserting block device: %w", err)
	}

	if deviceLink != "" {
		linkRow := provBlockDeviceLinkRow{
			BlockDeviceUUID: bdUUID,
			MachineUUID:     machineUUID,
			LinkName:        deviceLink,
		}
		if err := tx.Query(ctx, insertLinkStmt, linkRow).Run(); err != nil {
			return "", errors.Errorf("inserting block device link: %w", err)
		}
	}

	return bdUUID, nil
}

// volumeDeviceTypeToID converts the string device type from a provisioned
// volume attachment plan to its integer ID used in the database.
func volumeDeviceTypeToID(deviceType string) (int, error) {
	switch deviceType {
	case "local", "":
		return 0, nil
	case "iscsi":
		return 1, nil
	default:
		return 0, errors.Errorf("unsupported volume device type %q", deviceType)
	}
}
