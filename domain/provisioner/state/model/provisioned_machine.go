// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model

import (
	"context"
	"database/sql"
	"slices"
	"strings"

	"github.com/canonical/sqlair"

	corenetwork "github.com/juju/juju/core/network"
	domainmachine "github.com/juju/juju/domain/machine"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	domainnetwork "github.com/juju/juju/domain/network"
	networkerrors "github.com/juju/juju/domain/network/errors"
	"github.com/juju/juju/domain/provisioner"
	"github.com/juju/juju/internal/errors"
	"github.com/juju/juju/internal/uuid"
)

// RecordProvisionedMachine persists the complete result of a successful
// provider StartInstance call in a single transaction, covering network
// configuration, volumes, volume attachments, and cloud instance identity.
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

	nics := domainnetwork.ProviderNetInterfaces(info.NetworkConfig)

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
	if len(info.VolumeAttachments) > 0 || len(nics) > 0 {
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

	var (
		upsertDeviceStmt            *sqlair.Statement
		upsertAddrStmt              *sqlair.Statement
		upsertProviderDeviceStmt    *sqlair.Statement
		upsertProviderAddrStmt      *sqlair.Statement
		upsertDNSDomainStmt         *sqlair.Statement
		upsertDNSAddrStmt           *sqlair.Statement
		upsertDeviceParentStmt      *sqlair.Statement
		getSubnetByProviderIDStmt   *sqlair.Statement
		validateProviderDeviceStmt  *sqlair.Statement
		validateProviderAddrStmt    *sqlair.Statement
		deleteStaleProviderDevStmt  *sqlair.Statement
		deleteStaleProviderAddrStmt *sqlair.Statement
		deleteStaleDNSDomainStmt    *sqlair.Statement
		deleteStaleDNSAddrStmt      *sqlair.Statement
		deleteStaleParentStmt       *sqlair.Statement
		deleteStaleAddrStmt         *sqlair.Statement
		deleteStaleDeviceStmt       *sqlair.Statement
	)
	if len(nics) > 0 {
		upsertDeviceStmt, err = st.Prepare(`
INSERT INTO link_layer_device (*) VALUES ($provLLDRow.*)
ON CONFLICT (uuid) DO UPDATE SET
    device_type_id       = EXCLUDED.device_type_id,
    mac_address          = EXCLUDED.mac_address,
    mtu                  = EXCLUDED.mtu,
    gateway_address      = EXCLUDED.gateway_address,
    is_default_gateway   = EXCLUDED.is_default_gateway,
    is_auto_start        = EXCLUDED.is_auto_start,
    is_enabled           = EXCLUDED.is_enabled,
    virtual_port_type_id = EXCLUDED.virtual_port_type_id,
    vlan_tag             = EXCLUDED.vlan_tag
`, provLLDRow{})
		if err != nil {
			return errors.Errorf("preparing device upsert: %w", err)
		}
		upsertAddrStmt, err = st.Prepare(`
INSERT INTO ip_address (*) VALUES ($provIPAddrRow.*)
ON CONFLICT (uuid) DO UPDATE SET
    device_uuid    = EXCLUDED.device_uuid,
    address_value  = EXCLUDED.address_value,
    config_type_id = EXCLUDED.config_type_id,
    type_id        = EXCLUDED.type_id,
    subnet_uuid    = EXCLUDED.subnet_uuid,
    scope_id       = EXCLUDED.scope_id,
    origin_id      = EXCLUDED.origin_id,
    is_secondary   = EXCLUDED.is_secondary,
    is_shadow      = EXCLUDED.is_shadow
`, provIPAddrRow{})
		if err != nil {
			return errors.Errorf("preparing address upsert: %w", err)
		}
		upsertProviderDeviceStmt, err = st.Prepare(`
INSERT INTO provider_link_layer_device (provider_id, device_uuid)
VALUES      ($provProviderLLDRow.*)
ON CONFLICT (provider_id) DO UPDATE SET device_uuid = EXCLUDED.device_uuid
`, provProviderLLDRow{})
		if err != nil {
			return errors.Errorf("preparing provider device upsert: %w", err)
		}
		upsertProviderAddrStmt, err = st.Prepare(`
INSERT INTO provider_ip_address (provider_id, address_uuid)
VALUES      ($provProviderIPRow.*)
ON CONFLICT (provider_id) DO UPDATE SET address_uuid = EXCLUDED.address_uuid
`, provProviderIPRow{})
		if err != nil {
			return errors.Errorf("preparing provider address upsert: %w", err)
		}
		upsertDNSDomainStmt, err = st.Prepare(`
INSERT INTO link_layer_device_dns_domain (device_uuid, search_domain)
VALUES      ($provDNSDomainRow.*)
ON CONFLICT (device_uuid, search_domain) DO NOTHING
`, provDNSDomainRow{})
		if err != nil {
			return errors.Errorf("preparing DNS domain upsert: %w", err)
		}
		upsertDNSAddrStmt, err = st.Prepare(`
INSERT INTO link_layer_device_dns_address (device_uuid, dns_address)
VALUES      ($provDNSAddrRow.*)
ON CONFLICT (device_uuid, dns_address) DO NOTHING
`, provDNSAddrRow{})
		if err != nil {
			return errors.Errorf("preparing DNS address upsert: %w", err)
		}
		upsertDeviceParentStmt, err = st.Prepare(`
INSERT INTO link_layer_device_parent (device_uuid, parent_uuid)
VALUES      ($provLLDParentRow.*)
ON CONFLICT (device_uuid) DO UPDATE SET parent_uuid = EXCLUDED.parent_uuid
`, provLLDParentRow{})
		if err != nil {
			return errors.Errorf("preparing device parent upsert: %w", err)
		}
		getSubnetByProviderIDStmt, err = st.Prepare(`
SELECT subnet_uuid AS &provSubnetUUID.uuid
FROM   provider_subnet
WHERE  provider_id = $provProviderID.provider_id
`, provSubnetUUID{}, provProviderID{})
		if err != nil {
			return errors.Errorf("preparing subnet by provider ID lookup: %w", err)
		}
		validateProviderDeviceStmt, err = st.Prepare(`
SELECT &provProviderLLDRow.*
FROM   provider_link_layer_device
WHERE  provider_id IN ($provProviderIDs[:])
`, provProviderLLDRow{}, provProviderIDs{})
		if err != nil {
			return errors.Errorf("preparing provider device validation: %w", err)
		}
		validateProviderAddrStmt, err = st.Prepare(`
SELECT &provProviderIPRow.*
FROM   provider_ip_address
WHERE  provider_id IN ($provProviderIDs[:])
`, provProviderIPRow{}, provProviderIDs{})
		if err != nil {
			return errors.Errorf("preparing provider address validation: %w", err)
		}
		deleteStaleProviderDevStmt, err = st.Prepare(`
DELETE FROM provider_link_layer_device
WHERE       device_uuid IN ($provLLDUUIDs[:])
`, provLLDUUIDs{})
		if err != nil {
			return errors.Errorf("preparing stale provider device delete: %w", err)
		}
		deleteStaleProviderAddrStmt, err = st.Prepare(`
DELETE FROM provider_ip_address
WHERE       address_uuid IN ($provLLDUUIDs[:])
`, provLLDUUIDs{})
		if err != nil {
			return errors.Errorf("preparing stale provider address delete: %w", err)
		}
		deleteStaleDNSDomainStmt, err = st.Prepare(`
DELETE FROM link_layer_device_dns_domain
WHERE       device_uuid IN ($provLLDUUIDs[:])
`, provLLDUUIDs{})
		if err != nil {
			return errors.Errorf("preparing stale DNS domain delete: %w", err)
		}
		deleteStaleDNSAddrStmt, err = st.Prepare(`
DELETE FROM link_layer_device_dns_address
WHERE       device_uuid IN ($provLLDUUIDs[:])
`, provLLDUUIDs{})
		if err != nil {
			return errors.Errorf("preparing stale DNS address delete: %w", err)
		}
		deleteStaleParentStmt, err = st.Prepare(`
DELETE FROM link_layer_device_parent
WHERE       device_uuid IN ($provLLDUUIDs[:])
`, provLLDUUIDs{})
		if err != nil {
			return errors.Errorf("preparing stale parent delete: %w", err)
		}
		deleteStaleAddrStmt, err = st.Prepare(`
DELETE FROM ip_address
WHERE       uuid IN ($provLLDUUIDs[:])
`, provLLDUUIDs{})
		if err != nil {
			return errors.Errorf("preparing stale address delete: %w", err)
		}
		deleteStaleDeviceStmt, err = st.Prepare(`
DELETE FROM link_layer_device
WHERE       uuid IN ($provLLDUUIDs[:])
`, provLLDUUIDs{})
		if err != nil {
			return errors.Errorf("preparing stale device delete: %w", err)
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

		// 3. Network config
		if len(nics) > 0 {
			lookups, err := st.getProvNetConfigLookups(ctx, tx)
			if err != nil {
				return errors.Errorf("getting network config lookups: %w", err)
			}
			mUUIDParam := provMachineUUIDParam{UUID: machineUUID}
			var netNode provNetNodeUUID
			if err := tx.Query(ctx, getNetNodeStmt, mUUIDParam).Get(&netNode); err != nil {
				if errors.Is(err, sqlair.ErrNoRows) {
					return errors.Errorf("machine %q does not exist: %w", machineUUID, machineerrors.MachineNotFound)
				}
				return errors.Errorf("getting net node for machine %q: %w", machineUUID, err)
			}
			nodeUUID := netNode.NetNodeUUID
			existingDevUUIDs, err := st.getExistingDeviceUUIDs(ctx, tx, nodeUUID)
			if err != nil {
				return errors.Errorf("reading existing devices for machine %q: %w", machineUUID, err)
			}
			nameToUUID := make(map[string]string, len(nics))
			for _, nic := range nics {
				if existingUUID, ok := existingDevUUIDs[nic.Name]; ok {
					nameToUUID[nic.Name] = existingUUID
					continue
				}
				newUUID, err := uuid.NewUUID()
				if err != nil {
					return errors.Errorf("generating device UUID: %w", err)
				}
				nameToUUID[nic.Name] = newUUID.String()
			}
			existingAddrUUIDs, err := st.getExistingAddressUUIDs(ctx, tx, nodeUUID)
			if err != nil {
				return errors.Errorf("reading existing addresses for machine %q: %w", machineUUID, err)
			}
			var (
				deviceRows    []provLLDRow
				addrRows      []provIPAddrRow
				provDevRows   []provProviderLLDRow
				provAddrRows  []provProviderIPRow
				dnsDomainRows []provDNSDomainRow
				dnsAddrRows   []provDNSAddrRow
				parentRows    []provLLDParentRow
			)
			for _, nic := range nics {
				devUUID := nameToUUID[nic.Name]
				devTypeID, ok := lookups.deviceType[nic.Type]
				if !ok {
					return errors.Errorf("unsupported device type %q for NIC %q", nic.Type, nic.Name)
				}
				portTypeID, ok := lookups.virtualPortType[nic.VirtualPortType]
				if !ok {
					return errors.Errorf("unsupported virtual port type %q for NIC %q", nic.VirtualPortType, nic.Name)
				}
				deviceRows = append(deviceRows, provLLDRow{
					UUID:              devUUID,
					NetNodeUUID:       nodeUUID,
					Name:              nic.Name,
					MTU:               nic.MTU,
					MACAddress:        nic.MACAddress,
					DeviceTypeID:      devTypeID,
					VirtualPortTypeID: portTypeID,
					IsAutoStart:       nic.IsAutoStart,
					IsEnabled:         nic.IsEnabled,
					IsDefaultGateway:  nic.IsDefaultGateway,
					GatewayAddress:    nic.GatewayAddress,
					VlanTag:           nic.VLANTag,
				})
				if nic.ProviderID != nil && *nic.ProviderID != "" {
					provDevRows = append(provDevRows, provProviderLLDRow{
						ProviderID: string(*nic.ProviderID),
						DeviceUUID: devUUID,
					})
				}
				for _, sd := range nic.DNSSearchDomains {
					dnsDomainRows = append(dnsDomainRows, provDNSDomainRow{
						DeviceUUID:   devUUID,
						SearchDomain: sd,
					})
				}
				for _, da := range nic.DNSAddresses {
					dnsAddrRows = append(dnsAddrRows, provDNSAddrRow{
						DeviceUUID: devUUID,
						Address:    da,
					})
				}
				if nic.ParentDeviceName != "" {
					if parentUUID, ok := nameToUUID[nic.ParentDeviceName]; ok {
						parentRows = append(parentRows, provLLDParentRow{
							DeviceUUID: devUUID,
							ParentUUID: parentUUID,
						})
					} else {
						st.logger.Warningf(ctx,
							"parent device %q for NIC %q not found in incoming data",
							nic.ParentDeviceName, nic.Name,
						)
					}
				}
				for _, addr := range nic.Addrs {
					var addrUUIDStr string
					if existingUUID, ok := existingAddrUUIDs[addr.AddressValue]; ok {
						addrUUIDStr = existingUUID
					} else {
						newUUID, err := uuid.NewUUID()
						if err != nil {
							return errors.Errorf("generating address UUID: %w", err)
						}
						addrUUIDStr = newUUID.String()
					}
					typeID, ok := lookups.addrType[addr.AddressType]
					if !ok {
						return errors.Errorf("unsupported address type %q", addr.AddressType)
					}
					configTypeID, ok := lookups.addrConfigType[addr.ConfigType]
					if !ok {
						return errors.Errorf("unsupported address config type %q", addr.ConfigType)
					}
					originID, ok := lookups.origin[addr.Origin]
					if !ok {
						return errors.Errorf("unsupported address origin %q", addr.Origin)
					}
					scopeID, ok := lookups.scope[addr.Scope]
					if !ok {
						return errors.Errorf("unsupported address scope %q", addr.Scope)
					}
					addrRow := provIPAddrRow{
						UUID:         addrUUIDStr,
						NodeUUID:     nodeUUID,
						DeviceUUID:   devUUID,
						AddressValue: addr.AddressValue,
						TypeID:       typeID,
						ConfigTypeID: configTypeID,
						OriginID:     originID,
						ScopeID:      scopeID,
						IsSecondary:  addr.IsSecondary,
						IsShadow:     addr.IsShadow,
					}
					if addr.ProviderSubnetID != nil && *addr.ProviderSubnetID != "" {
						pID := provProviderID{ProviderID: string(*addr.ProviderSubnetID)}
						var sUUID provSubnetUUID
						if err := tx.Query(ctx, getSubnetByProviderIDStmt, pID).Get(&sUUID); err != nil &&
							!errors.Is(err, sqlair.ErrNoRows) {
							return errors.Errorf("looking up subnet for provider ID %q: %w", *addr.ProviderSubnetID, err)
						}
						if sUUID.UUID != "" {
							addrRow.SubnetUUID = &sUUID.UUID
						}
					}
					addrRows = append(addrRows, addrRow)
					if addr.ProviderID != nil && *addr.ProviderID != "" {
						provAddrRows = append(provAddrRows, provProviderIPRow{
							ProviderID:  string(*addr.ProviderID),
							AddressUUID: addrUUIDStr,
						})
					}
				}
			}

			// Clean up stale rows for devices/addresses no longer
			// in the incoming network config, e.g. after a retry.
			staleDevUUIDs := computeStaleDeviceUUIDs(existingDevUUIDs, nameToUUID)
			staleAddrUUIDs := computeStaleAddressUUIDs(existingAddrUUIDs, addrRows)
			if len(staleAddrUUIDs) > 0 {
				sa := provLLDUUIDs(staleAddrUUIDs)
				if err := tx.Query(ctx, deleteStaleProviderAddrStmt, sa).Run(); err != nil {
					return errors.Errorf("deleting stale provider address IDs: %w", err)
				}
				if err := tx.Query(ctx, deleteStaleAddrStmt, sa).Run(); err != nil {
					return errors.Errorf("deleting stale addresses: %w", err)
				}
			}
			if len(staleDevUUIDs) > 0 {
				sd := provLLDUUIDs(staleDevUUIDs)
				if err := tx.Query(ctx, deleteStaleProviderDevStmt, sd).Run(); err != nil {
					return errors.Errorf("deleting stale provider device IDs: %w", err)
				}
				if err := tx.Query(ctx, deleteStaleDNSDomainStmt, sd).Run(); err != nil {
					return errors.Errorf("deleting stale DNS domains: %w", err)
				}
				if err := tx.Query(ctx, deleteStaleDNSAddrStmt, sd).Run(); err != nil {
					return errors.Errorf("deleting stale DNS addresses: %w", err)
				}
				if err := tx.Query(ctx, deleteStaleParentStmt, sd).Run(); err != nil {
					return errors.Errorf("deleting stale parent links: %w", err)
				}
				if err := tx.Query(ctx, deleteStaleDeviceStmt, sd).Run(); err != nil {
					return errors.Errorf("deleting stale devices: %w", err)
				}
			}

			if len(deviceRows) > 0 {
				if err := tx.Query(ctx, upsertDeviceStmt, deviceRows).Run(); err != nil {
					return errors.Errorf("upserting link layer devices: %w", err)
				}
			}
			if len(addrRows) > 0 {
				if err := tx.Query(ctx, upsertAddrStmt, addrRows).Run(); err != nil {
					return errors.Errorf("upserting IP addresses: %w", err)
				}
			}
			if len(provDevRows) > 0 {
				if err := validateProviderDeviceIDs(ctx, tx, validateProviderDeviceStmt, provDevRows); err != nil {
					return errors.Capture(err)
				}
				if err := tx.Query(ctx, upsertProviderDeviceStmt, provDevRows).Run(); err != nil {
					return errors.Errorf("upserting provider device IDs: %w", err)
				}
			}
			if len(provAddrRows) > 0 {
				if err := validateProviderAddressIDs(ctx, tx, validateProviderAddrStmt, provAddrRows); err != nil {
					return errors.Capture(err)
				}
				if err := tx.Query(ctx, upsertProviderAddrStmt, provAddrRows).Run(); err != nil {
					return errors.Errorf("upserting provider address IDs: %w", err)
				}
			}
			if len(dnsDomainRows) > 0 {
				if err := tx.Query(ctx, upsertDNSDomainStmt, dnsDomainRows).Run(); err != nil {
					return errors.Errorf("upserting DNS search domains: %w", err)
				}
			}
			if len(dnsAddrRows) > 0 {
				if err := tx.Query(ctx, upsertDNSAddrStmt, dnsAddrRows).Run(); err != nil {
					return errors.Errorf("upserting DNS addresses: %w", err)
				}
			}
			if len(parentRows) > 0 {
				if err := tx.Query(ctx, upsertDeviceParentStmt, parentRows).Run(); err != nil {
					return errors.Errorf("upserting device parents: %w", err)
				}
			}
		}

		// 4. Cloud instance (last, for change-stream notification).
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

// getExistingDeviceUUIDs reads existing link_layer_device rows for a net node
// and returns a map of device name → UUID. Used to reuse UUIDs on retry.
func (st *State) getExistingDeviceUUIDs(
	ctx context.Context, tx *sqlair.TX, nodeUUID string,
) (map[string]string, error) {
	nUUID := provNetNodeUUID{NetNodeUUID: nodeUUID}
	stmt, err := st.Prepare(
		"SELECT &provLLDNameUUID.* FROM link_layer_device WHERE net_node_uuid = $provNetNodeUUID.net_node_uuid",
		nUUID, provLLDNameUUID{},
	)
	if err != nil {
		return nil, errors.Errorf("preparing existing device UUID query: %w", err)
	}

	var rows []provLLDNameUUID
	if err := tx.Query(ctx, stmt, nUUID).GetAll(&rows); err != nil {
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil, nil
		}
		return nil, errors.Errorf("querying existing device UUIDs: %w", err)
	}

	result := make(map[string]string, len(rows))
	for _, r := range rows {
		result[r.Name] = r.UUID
	}
	return result, nil
}

// getExistingAddressUUIDs reads existing ip_address rows for a net node and
// returns a map of address_value → UUID. Used to reuse UUIDs on retry.
func (st *State) getExistingAddressUUIDs(
	ctx context.Context, tx *sqlair.TX, nodeUUID string,
) (map[string]string, error) {
	nUUID := provNetNodeUUID{NetNodeUUID: nodeUUID}
	stmt, err := st.Prepare(
		"SELECT &provIPAddrNameUUID.* FROM ip_address WHERE net_node_uuid = $provNetNodeUUID.net_node_uuid",
		nUUID, provIPAddrNameUUID{},
	)
	if err != nil {
		return nil, errors.Errorf("preparing existing address UUID query: %w", err)
	}

	var rows []provIPAddrNameUUID
	if err := tx.Query(ctx, stmt, nUUID).GetAll(&rows); err != nil {
		if errors.Is(err, sqlair.ErrNoRows) {
			return nil, nil
		}
		return nil, errors.Errorf("querying existing address UUIDs: %w", err)
	}

	result := make(map[string]string, len(rows))
	for _, r := range rows {
		result[r.Value] = r.UUID
	}
	return result, nil
}

// getProvNetConfigLookups reads the enum ID tables needed for inserting
// network config rows.
func (st *State) getProvNetConfigLookups(
	ctx context.Context, tx *sqlair.TX,
) (provNetConfigLookups, error) {
	var lookups provNetConfigLookups
	var err error

	lookups.deviceType, err = getProvLookup[corenetwork.LinkLayerDeviceType](ctx, tx, st, "link_layer_device_type")
	if err != nil {
		return lookups, errors.Capture(err)
	}
	lookups.virtualPortType, err = getProvLookup[corenetwork.VirtualPortType](ctx, tx, st, "virtual_port_type")
	if err != nil {
		return lookups, errors.Capture(err)
	}
	lookups.addrType, err = getProvLookup[corenetwork.AddressType](ctx, tx, st, "ip_address_type")
	if err != nil {
		return lookups, errors.Capture(err)
	}
	lookups.addrConfigType, err = getProvLookup[corenetwork.AddressConfigType](ctx, tx, st, "ip_address_config_type")
	if err != nil {
		return lookups, errors.Capture(err)
	}
	lookups.origin, err = getProvLookup[corenetwork.Origin](ctx, tx, st, "ip_address_origin")
	if err != nil {
		return lookups, errors.Capture(err)
	}
	lookups.scope, err = getProvLookup[corenetwork.Scope](ctx, tx, st, "ip_address_scope")
	if err != nil {
		return lookups, errors.Capture(err)
	}

	return lookups, nil
}

// getProvLookup reads a lookup table and returns a name→ID map.
func getProvLookup[T ~string](
	ctx context.Context, tx *sqlair.TX, st *State, tableName string,
) (map[T]int, error) {
	stmt, err := st.Prepare(
		"SELECT &provLookupRow.* FROM "+tableName,
		provLookupRow{},
	)
	if err != nil {
		return nil, errors.Errorf("preparing lookup for %s: %w", tableName, err)
	}

	var rows []provLookupRow
	if err := tx.Query(ctx, stmt).GetAll(&rows); err != nil {
		return nil, errors.Errorf("querying %s lookup: %w", tableName, err)
	}

	result := make(map[T]int, len(rows))
	for _, r := range rows {
		result[T(r.Name)] = r.ID
	}
	return result, nil
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

// validateProviderDeviceIDs checks for duplicate provider device IDs in the
// incoming data and for conflicts with existing provider_link_layer_device
// rows that map a different device_uuid.
func validateProviderDeviceIDs(ctx context.Context, tx *sqlair.TX, stmt *sqlair.Statement, rows []provProviderLLDRow) error {
	// Check for duplicates within the incoming data.
	seen := make(map[string]string, len(rows))
	for _, r := range rows {
		if existingUUID, ok := seen[r.ProviderID]; ok && existingUUID != r.DeviceUUID {
			return errors.Errorf("duplicate provider device ID %q assigned to multiple devices in incoming data", r.ProviderID)
		}
		seen[r.ProviderID] = r.DeviceUUID
	}

	// Check for conflicts with existing provider mappings.
	ids := make([]string, 0, len(seen))
	for id := range seen {
		ids = append(ids, id)
	}
	pIDs := provProviderIDs(ids)
	var existing []provProviderLLDRow
	if err := tx.Query(ctx, stmt, pIDs).GetAll(&existing); err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return errors.Errorf("checking existing provider device mappings: %w", err)
	}
	for _, e := range existing {
		if incomingUUID, ok := seen[e.ProviderID]; ok && incomingUUID != e.DeviceUUID {
			return errors.Errorf("provider device ID %q already mapped to device %q", e.ProviderID, e.DeviceUUID)
		}
	}
	return nil
}

// validateProviderAddressIDs checks for duplicate provider address IDs in the
// incoming data and for conflicts with existing provider_ip_address
// rows that map a different address_uuid.
func validateProviderAddressIDs(ctx context.Context, tx *sqlair.TX, stmt *sqlair.Statement, rows []provProviderIPRow) error {
	// Check for duplicates within the incoming data.
	seen := make(map[string]string, len(rows))
	for _, r := range rows {
		if existingUUID, ok := seen[r.ProviderID]; ok && existingUUID != r.AddressUUID {
			return errors.Errorf("duplicate provider address ID %q assigned to multiple addresses in incoming data", r.ProviderID)
		}
		seen[r.ProviderID] = r.AddressUUID
	}

	// Check for conflicts with existing provider mappings.
	ids := make([]string, 0, len(seen))
	for id := range seen {
		ids = append(ids, id)
	}
	pIDs := provProviderIDs(ids)
	var existing []provProviderIPRow
	if err := tx.Query(ctx, stmt, pIDs).GetAll(&existing); err != nil && !errors.Is(err, sqlair.ErrNoRows) {
		return errors.Errorf("checking existing provider address mappings: %w", err)
	}
	for _, e := range existing {
		if incomingUUID, ok := seen[e.ProviderID]; ok && incomingUUID != e.AddressUUID {
			return errors.Errorf("provider address ID %q already mapped to address %q", e.ProviderID, e.AddressUUID)
		}
	}
	return nil
}

// computeStaleDeviceUUIDs returns UUIDs of devices that exist in the DB but
// are not present in the incoming network config.
func computeStaleDeviceUUIDs(existing map[string]string, incoming map[string]string) []string {
	incomingSet := make(map[string]bool, len(incoming))
	for _, uuid := range incoming {
		incomingSet[uuid] = true
	}
	var stale []string
	for _, uuid := range existing {
		if !incomingSet[uuid] {
			stale = append(stale, uuid)
		}
	}
	return stale
}

// computeStaleAddressUUIDs returns UUIDs of addresses that exist in the DB
// but are not present in the incoming network config.
func computeStaleAddressUUIDs(existing map[string]string, incoming []provIPAddrRow) []string {
	incomingSet := make(map[string]bool, len(incoming))
	for _, r := range incoming {
		incomingSet[r.UUID] = true
	}
	var stale []string
	for _, uuid := range existing {
		if !incomingSet[uuid] {
			stale = append(stale, uuid)
		}
	}
	return stale
}
