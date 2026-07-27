// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"database/sql"
	"time"

	"github.com/juju/tc"

	"github.com/juju/juju/core/network"
	"github.com/juju/juju/domain/life"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	domainstatus "github.com/juju/juju/domain/status"
)

func (s *stateSuite) TestDetachLostMachineCloudInstance(c *tc.C) {
	machineUUID, machineName := s.ensureInstance(c)
	netNodeUUID := s.machineNetNodeUUID(c, machineUUID.String())
	s.addReprovisionNetworkState(c, netNodeUUID)
	s.addReprovisionBlockDeviceState(c, machineUUID.String(), netNodeUUID)
	s.addReprovisionUnit(c, netNodeUUID)
	preservedCounts := map[string]int{
		"application":        s.rowCount(c, "application"),
		"unit":               s.rowCount(c, "unit"),
		"machine_platform":   s.rowCount(c, "machine_platform"),
		"machine_constraint": s.rowCount(c, "machine_constraint"),
		"machine_placement":  s.rowCount(c, "machine_placement"),
	}

	updatedAt := time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)
	statusData := []byte(`{"old-instance-id":"123"}`)
	err := s.state.DetachLostMachineCloudInstance(
		c.Context(), machineName.String(), "123",
		"reprovisioning requested", statusData, updatedAt,
	)
	c.Assert(err, tc.ErrorIsNil)

	var (
		instanceID, displayName, arch, availabilityZone, nonce, hostname sql.Null[string]
		machineStatusID, instanceStatusID                                int
		machineMessage, instanceMessage                                  string
		machineData, instanceData                                        []byte
	)
	db := s.DB()
	err = db.QueryRowContext(c.Context(), `
SELECT mci.instance_id, mci.display_name, mci.arch,
       mci.availability_zone_uuid, m.nonce, m.hostname
FROM machine AS m
JOIN machine_cloud_instance AS mci ON m.uuid = mci.machine_uuid
WHERE m.uuid = ?`, machineUUID.String()).Scan(
		&instanceID, &displayName, &arch, &availabilityZone, &nonce, &hostname,
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(instanceID.Valid, tc.IsFalse)
	c.Check(displayName.Valid, tc.IsFalse)
	c.Check(arch.Valid, tc.IsFalse)
	c.Check(availabilityZone.Valid, tc.IsFalse)
	c.Check(nonce.Valid, tc.IsFalse)
	c.Check(hostname.Valid, tc.IsFalse)

	err = db.QueryRowContext(c.Context(), `
SELECT ms.status_id, ms.message, ms.data,
       mcis.status_id, mcis.message, mcis.data
FROM machine_status AS ms
JOIN machine_cloud_instance_status AS mcis
  ON ms.machine_uuid = mcis.machine_uuid
WHERE ms.machine_uuid = ?`, machineUUID.String()).Scan(
		&machineStatusID, &machineMessage, &machineData,
		&instanceStatusID, &instanceMessage, &instanceData,
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(machineStatusID, tc.Equals, int(domainstatus.MachineStatusPending))
	c.Check(instanceStatusID, tc.Equals, int(domainstatus.InstanceStatusPending))
	c.Check(machineMessage, tc.Equals, "reprovisioning requested")
	c.Check(instanceMessage, tc.Equals, "reprovisioning requested")
	c.Check(machineData, tc.DeepEquals, statusData)
	c.Check(instanceData, tc.DeepEquals, statusData)

	for _, table := range []string{
		"instance_tag",
		"provider_ip_address",
		"ip_address",
		"link_layer_device_parent",
		"provider_link_layer_device",
		"link_layer_device_dns_domain",
		"link_layer_device_dns_address",
		"link_layer_device_route",
		"link_layer_device",
		"net_node_fqdn_address",
		"net_node_hostname_address",
	} {
		c.Check(s.rowCount(c, table), tc.Equals, 0, tc.Commentf("table %s", table))
	}
	c.Check(s.rowCountWhere(c, "fqdn_address", "uuid = ?", "fqdn-uuid"), tc.Equals, 1)
	c.Check(s.rowCountWhere(c, "hostname_address", "uuid = ?", "hostname-uuid"), tc.Equals, 1)
	c.Check(s.rowCountWhere(c, "block_device", "uuid = ?", "unreferenced-block"), tc.Equals, 0)
	c.Check(s.rowCountWhere(c, "block_device_link_device", "block_device_uuid = ?", "unreferenced-block"), tc.Equals, 0)
	c.Check(s.rowCountWhere(c, "block_device", "uuid = ?", "referenced-block"), tc.Equals, 0)
	c.Check(s.rowCountWhere(c, "block_device_link_device", "block_device_uuid = ?", "referenced-block"), tc.Equals, 0)
	c.Check(s.rowCountWhere(c, "storage_volume", "uuid = ?", "volume-uuid"), tc.Equals, 0)
	c.Check(s.rowCountWhere(c, "storage_volume_attachment", "uuid = ?", "attachment-uuid"), tc.Equals, 0)

	var name, preservedNetNode string
	err = db.QueryRowContext(c.Context(),
		"SELECT name, net_node_uuid FROM machine WHERE uuid = ?", machineUUID.String(),
	).Scan(&name, &preservedNetNode)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(name, tc.Equals, machineName.String())
	c.Check(preservedNetNode, tc.Equals, netNodeUUID)
	for table, count := range preservedCounts {
		c.Check(s.rowCount(c, table), tc.Equals, count, tc.Commentf("table %s", table))
	}
	c.Check(s.rowCountWhere(c, "unit", "net_node_uuid = ?", netNodeUUID), tc.Equals, 1)
}

func (s *stateSuite) TestDetachLostMachineCloudInstanceRechecksLife(c *tc.C) {
	machineUUID, machineName := s.ensureInstance(c)
	s.runQuery(c, "UPDATE machine SET life_id = ? WHERE uuid = ?", life.Dying, machineUUID.String())

	err := s.state.DetachLostMachineCloudInstance(
		c.Context(), machineName.String(), "123", "message", nil, time.Now(),
	)
	c.Assert(err, tc.ErrorIs, machineerrors.MachineNotAlive)
	s.checkInstanceID(c, machineUUID.String(), "123")
}

func (s *stateSuite) TestDetachLostMachineCloudInstanceRechecksPresence(c *tc.C) {
	machineUUID, machineName := s.ensureInstance(c)
	s.runQuery(c, "INSERT INTO machine_agent_presence (machine_uuid) VALUES (?)", machineUUID.String())

	err := s.state.DetachLostMachineCloudInstance(
		c.Context(), machineName.String(), "123", "message", nil, time.Now(),
	)
	c.Assert(err, tc.ErrorIs, machineerrors.MachineAgentPresent)
	s.checkInstanceID(c, machineUUID.String(), "123")
}

func (s *stateSuite) TestDetachLostMachineCloudInstanceRechecksInstance(c *tc.C) {
	machineUUID, machineName := s.ensureInstance(c)

	err := s.state.DetachLostMachineCloudInstance(
		c.Context(), machineName.String(), "old-instance", "message", nil, time.Now(),
	)
	c.Assert(err, tc.ErrorIs, machineerrors.MachineCloudInstanceChanged)
	s.checkInstanceID(c, machineUUID.String(), "123")
}

func (s *stateSuite) TestDetachLostMachineCloudInstanceRepeated(c *tc.C) {
	machineUUID, machineName := s.ensureInstance(c)
	err := s.state.DetachLostMachineCloudInstance(
		c.Context(), machineName.String(), "123", "message", nil, time.Now(),
	)
	c.Assert(err, tc.ErrorIsNil)

	err = s.state.DetachLostMachineCloudInstance(
		c.Context(), machineName.String(), "123", "message", nil, time.Now(),
	)
	c.Assert(err, tc.ErrorIs, machineerrors.NotProvisioned)
	s.checkInstanceID(c, machineUUID.String(), "")
}

func (s *stateSuite) TestDetachLostMachineCloudInstanceRejectsModelScopedStorage(c *tc.C) {
	machineUUID, machineName := s.ensureInstance(c)
	netNodeUUID := s.machineNetNodeUUID(c, machineUUID.String())
	s.addReprovisionUnit(c, netNodeUUID)
	s.addReprovisionVolumeStorage(c, machineUUID.String(), netNodeUUID, 0, 0)

	err := s.state.DetachLostMachineCloudInstance(
		c.Context(), machineName.String(), "123", "message", nil, time.Now(),
	)
	c.Assert(err, tc.ErrorIs, machineerrors.ModelScopedStorageAttached)
	s.checkInstanceID(c, machineUUID.String(), "123")
	c.Check(s.rowCountWhere(c, "storage_volume", "uuid = ?", "storage-volume"), tc.Equals, 1)
}

func (s *stateSuite) TestDetachLostMachineCloudInstanceRejectsAmbiguousStorage(c *tc.C) {
	machineUUID, machineName := s.ensureInstance(c)
	netNodeUUID := s.machineNetNodeUUID(c, machineUUID.String())
	s.addReprovisionUnit(c, netNodeUUID)
	s.addReprovisionVolumeStorage(c, machineUUID.String(), netNodeUUID, 1, 0)

	err := s.state.DetachLostMachineCloudInstance(
		c.Context(), machineName.String(), "123", "message", nil, time.Now(),
	)
	c.Assert(err, tc.ErrorIs, machineerrors.StorageScopeAmbiguous)
	s.checkInstanceID(c, machineUUID.String(), "123")
	c.Check(s.rowCountWhere(c, "storage_volume", "uuid = ?", "storage-volume"), tc.Equals, 1)
}

func (s *stateSuite) TestDetachLostMachineCloudInstanceRejectsModelScopedFilesystem(c *tc.C) {
	machineUUID, machineName := s.ensureInstance(c)
	netNodeUUID := s.machineNetNodeUUID(c, machineUUID.String())
	s.addReprovisionUnit(c, netNodeUUID)
	s.addReprovisionFilesystemStorage(c, machineUUID.String(), netNodeUUID, 0, 0)

	err := s.state.DetachLostMachineCloudInstance(
		c.Context(), machineName.String(), "123", "message", nil, time.Now(),
	)
	c.Assert(err, tc.ErrorIs, machineerrors.ModelScopedStorageAttached)
	s.checkInstanceID(c, machineUUID.String(), "123")
	c.Check(s.rowCountWhere(c, "storage_filesystem", "uuid = ?", "storage-filesystem"), tc.Equals, 1)
}

func (s *stateSuite) TestDetachLostMachineCloudInstanceRejectsAmbiguousFilesystem(c *tc.C) {
	machineUUID, machineName := s.ensureInstance(c)
	netNodeUUID := s.machineNetNodeUUID(c, machineUUID.String())
	s.addReprovisionUnit(c, netNodeUUID)
	s.addReprovisionFilesystemStorage(c, machineUUID.String(), netNodeUUID, 1, 0)

	err := s.state.DetachLostMachineCloudInstance(
		c.Context(), machineName.String(), "123", "message", nil, time.Now(),
	)
	c.Assert(err, tc.ErrorIs, machineerrors.StorageScopeAmbiguous)
	s.checkInstanceID(c, machineUUID.String(), "123")
	c.Check(s.rowCountWhere(c, "storage_filesystem", "uuid = ?", "storage-filesystem"), tc.Equals, 1)
}

func (s *stateSuite) TestDetachLostMachineCloudInstanceCleansVolumeAndPreservesIntent(c *tc.C) {
	machineUUID, machineName := s.ensureInstance(c)
	netNodeUUID := s.machineNetNodeUUID(c, machineUUID.String())
	s.addReprovisionUnit(c, netNodeUUID)
	s.addReprovisionVolumeStorage(c, machineUUID.String(), netNodeUUID, 1, 1)

	err := s.state.DetachLostMachineCloudInstance(
		c.Context(), machineName.String(), "123", "message", nil, time.Now(),
	)
	c.Assert(err, tc.ErrorIsNil)

	for _, table := range []string{
		"storage_volume", "storage_instance_volume", "storage_volume_status",
		"storage_volume_attachment", "storage_volume_attachment_plan",
		"storage_volume_attachment_plan_attr", "machine_volume",
	} {
		c.Check(s.rowCount(c, table), tc.Equals, 0, tc.Commentf("table %s", table))
	}
	for _, table := range []string{
		"storage_instance", "storage_attachment", "storage_unit_owner",
	} {
		c.Check(s.rowCount(c, table), tc.Equals, 1, tc.Commentf("table %s", table))
	}
}

func (s *stateSuite) TestDetachLostMachineCloudInstanceCleansFilesystemAndPreservesIntent(c *tc.C) {
	machineUUID, machineName := s.ensureInstance(c)
	netNodeUUID := s.machineNetNodeUUID(c, machineUUID.String())
	s.addReprovisionUnit(c, netNodeUUID)
	s.addReprovisionFilesystemStorage(c, machineUUID.String(), netNodeUUID, 1, 1)

	err := s.state.DetachLostMachineCloudInstance(
		c.Context(), machineName.String(), "123", "message", nil, time.Now(),
	)
	c.Assert(err, tc.ErrorIsNil)

	for _, table := range []string{
		"storage_filesystem", "storage_instance_filesystem",
		"storage_filesystem_status", "storage_filesystem_attachment",
		"machine_filesystem",
	} {
		c.Check(s.rowCount(c, table), tc.Equals, 0, tc.Commentf("table %s", table))
	}
	for _, table := range []string{
		"storage_instance", "storage_attachment", "storage_unit_owner",
	} {
		c.Check(s.rowCount(c, table), tc.Equals, 1, tc.Commentf("table %s", table))
	}
}

func (s *stateSuite) TestDetachLostMachineCloudInstanceCleansPlanOnlyVolume(c *tc.C) {
	machineUUID, machineName := s.ensureInstance(c)
	netNodeUUID := s.machineNetNodeUUID(c, machineUUID.String())
	s.addReprovisionUnit(c, netNodeUUID)
	s.addReprovisionVolumePlanStorage(c, machineUUID.String(), netNodeUUID, 1)

	err := s.state.DetachLostMachineCloudInstance(
		c.Context(), machineName.String(), "123", "message", nil, time.Now(),
	)
	c.Assert(err, tc.ErrorIsNil)

	for _, table := range []string{
		"storage_volume", "storage_instance_volume", "storage_volume_status",
		"storage_volume_attachment_plan", "storage_volume_attachment_plan_attr",
		"machine_volume",
	} {
		c.Check(s.rowCount(c, table), tc.Equals, 0, tc.Commentf("table %s", table))
	}
	for _, table := range []string{
		"storage_instance", "storage_attachment", "storage_unit_owner",
	} {
		c.Check(s.rowCount(c, table), tc.Equals, 1, tc.Commentf("table %s", table))
	}
}

func (s *stateSuite) TestDetachLostMachineCloudInstancePreservesOtherMachineStorage(c *tc.C) {
	machineUUID, machineName := s.ensureInstance(c)
	netNodeUUID := s.machineNetNodeUUID(c, machineUUID.String())
	s.addReprovisionUnit(c, netNodeUUID)
	s.addReprovisionVolumeStorage(c, machineUUID.String(), netNodeUUID, 1, 1)

	otherMachineUUID, _ := s.addMachine(c)
	otherNetNodeUUID := s.machineNetNodeUUID(c, otherMachineUUID.String())
	s.runQuery(c, `
INSERT INTO storage_volume (uuid, volume_id, life_id, provision_scope_id)
VALUES (?, ?, 0, 1)`, "other-volume", "other-volume/0")
	s.runQuery(c, `
INSERT INTO storage_volume_attachment
    (uuid, storage_volume_uuid, net_node_uuid, life_id, provision_scope_id)
VALUES (?, ?, ?, 0, 1)`, "other-volume-attachment", "other-volume", otherNetNodeUUID)
	s.runQuery(c, "INSERT INTO machine_volume VALUES (?, ?)",
		otherMachineUUID.String(), "other-volume")
	s.runQuery(c, `
INSERT INTO storage_filesystem (uuid, filesystem_id, life_id, provision_scope_id)
VALUES (?, ?, 0, 1)`, "other-filesystem", "other-filesystem/0")
	s.runQuery(c, `
INSERT INTO storage_filesystem_attachment
    (uuid, storage_filesystem_uuid, net_node_uuid, provision_scope_id, life_id)
VALUES (?, ?, ?, 1, 0)`, "other-filesystem-attachment", "other-filesystem",
		otherNetNodeUUID)
	s.runQuery(c, "INSERT INTO machine_filesystem VALUES (?, ?)",
		otherMachineUUID.String(), "other-filesystem")

	err := s.state.DetachLostMachineCloudInstance(
		c.Context(), machineName.String(), "123", "message", nil, time.Now(),
	)
	c.Assert(err, tc.ErrorIsNil)

	checks := []struct {
		table  string
		column string
		uuid   string
	}{
		{table: "storage_volume", column: "uuid", uuid: "other-volume"},
		{table: "storage_volume_attachment", column: "uuid", uuid: "other-volume-attachment"},
		{table: "machine_volume", column: "volume_uuid", uuid: "other-volume"},
		{table: "storage_filesystem", column: "uuid", uuid: "other-filesystem"},
		{table: "storage_filesystem_attachment", column: "uuid", uuid: "other-filesystem-attachment"},
		{table: "machine_filesystem", column: "filesystem_uuid", uuid: "other-filesystem"},
	}
	for _, check := range checks {
		c.Check(s.rowCountWhere(c, check.table, check.column+" = ?", check.uuid),
			tc.Equals, 1, tc.Commentf("table %s", check.table))
	}
}

func (s *stateSuite) TestDetachLostMachineCloudInstanceRollsBack(c *tc.C) {
	machineUUID, machineName := s.ensureInstance(c)
	netNodeUUID := s.machineNetNodeUUID(c, machineUUID.String())
	s.addReprovisionNetworkState(c, netNodeUUID)
	s.addReprovisionUnit(c, netNodeUUID)
	s.addReprovisionVolumeStorage(c, machineUUID.String(), netNodeUUID, 1, 1)
	s.runQuery(c, `
CREATE TRIGGER fail_reprovision_detach
BEFORE UPDATE ON machine_cloud_instance
WHEN NEW.instance_id IS NULL
BEGIN
    SELECT RAISE(ABORT, 'detach failed');
END`)

	err := s.state.DetachLostMachineCloudInstance(
		c.Context(), machineName.String(), "123", "message", nil, time.Now(),
	)
	c.Assert(err, tc.ErrorMatches, ".*detach failed.*")
	s.checkInstanceID(c, machineUUID.String(), "123")
	c.Check(s.rowCount(c, "instance_tag"), tc.Equals, 2)
	c.Check(s.rowCount(c, "link_layer_device"), tc.Equals, 2)
	c.Check(s.rowCount(c, "link_layer_device_parent"), tc.Equals, 1)
	c.Check(s.rowCount(c, "ip_address"), tc.Equals, 1)
	c.Check(s.rowCount(c, "net_node_fqdn_address"), tc.Equals, 1)
	c.Check(s.rowCount(c, "net_node_hostname_address"), tc.Equals, 1)
	for _, table := range []string{
		"storage_instance", "storage_attachment", "storage_unit_owner",
		"storage_volume", "storage_instance_volume", "storage_volume_status",
		"storage_volume_attachment", "storage_volume_attachment_plan",
		"storage_volume_attachment_plan_attr", "machine_volume",
	} {
		c.Check(s.rowCount(c, table), tc.Equals, 1, tc.Commentf("table %s", table))
	}
}

func (s *stateSuite) machineNetNodeUUID(c *tc.C, machineUUID string) string {
	var netNodeUUID string
	err := s.DB().QueryRowContext(c.Context(),
		"SELECT net_node_uuid FROM machine WHERE uuid = ?", machineUUID,
	).Scan(&netNodeUUID)
	c.Assert(err, tc.ErrorIsNil)
	return netNodeUUID
}

func (s *stateSuite) addReprovisionNetworkState(c *tc.C, netNodeUUID string) {
	s.runQuery(c, `
INSERT INTO link_layer_device
    (uuid, net_node_uuid, name, device_type_id, virtual_port_type_id)
VALUES (?, ?, 'eth0', 2, 0)`, "device-uuid", netNodeUUID)
	s.runQuery(c, `
INSERT INTO link_layer_device
    (uuid, net_node_uuid, name, device_type_id, virtual_port_type_id)
VALUES (?, ?, 'eth1', 2, 0)`, "child-device-uuid", netNodeUUID)
	s.runQuery(c, "INSERT INTO link_layer_device_parent VALUES (?, ?)",
		"child-device-uuid", "device-uuid")
	s.runQuery(c, "INSERT INTO provider_link_layer_device VALUES (?, ?)", "provider-device", "device-uuid")
	s.runQuery(c, "INSERT INTO link_layer_device_dns_domain VALUES (?, ?)", "device-uuid", "example.test")
	s.runQuery(c, "INSERT INTO link_layer_device_dns_address VALUES (?, ?)", "device-uuid", "10.0.0.2")
	s.runQuery(c, "INSERT INTO link_layer_device_route VALUES (?, ?, ?, ?)", "device-uuid", "0.0.0.0/0", "10.0.0.1", 1)
	s.runQuery(c, `
INSERT INTO ip_address
    (uuid, net_node_uuid, device_uuid, address_value, type_id,
     config_type_id, origin_id, scope_id)
VALUES (?, ?, ?, '10.0.0.2/24', 0, 1, 1, 2)`, "address-uuid", netNodeUUID, "device-uuid")
	s.runQuery(c, "INSERT INTO provider_ip_address VALUES (?, ?)", "provider-address", "address-uuid")
	s.runQuery(c, "INSERT INTO fqdn_address VALUES (?, ?, ?)",
		"fqdn-uuid", "machine.example.test", 1)
	s.runQuery(c, "INSERT INTO net_node_fqdn_address VALUES (?, ?)",
		netNodeUUID, "fqdn-uuid")
	s.runQuery(c, "INSERT INTO hostname_address VALUES (?, ?, ?)",
		"hostname-uuid", "machine", 0)
	s.runQuery(c, "INSERT INTO net_node_hostname_address VALUES (?, ?)",
		netNodeUUID, "hostname-uuid")
}

func (s *stateSuite) addReprovisionBlockDeviceState(c *tc.C, machineUUID, netNodeUUID string) {
	s.runQuery(c, "INSERT INTO block_device (uuid, machine_uuid) VALUES (?, ?)", "unreferenced-block", machineUUID)
	s.runQuery(c, "INSERT INTO block_device_link_device VALUES (?, ?, ?)", "unreferenced-block", machineUUID, "sda")
	s.runQuery(c, "INSERT INTO block_device (uuid, machine_uuid) VALUES (?, ?)", "referenced-block", machineUUID)
	s.runQuery(c, "INSERT INTO block_device_link_device VALUES (?, ?, ?)", "referenced-block", machineUUID, "sdb")
	s.runQuery(c, "INSERT INTO storage_volume (uuid, volume_id, life_id, provision_scope_id) VALUES (?, ?, 0, 1)", "volume-uuid", "0")
	s.runQuery(c, `
INSERT INTO storage_volume_attachment
    (uuid, storage_volume_uuid, net_node_uuid, life_id,
     provision_scope_id, block_device_uuid)
VALUES (?, ?, ?, 0, 1, ?)`, "attachment-uuid", "volume-uuid", netNodeUUID, "referenced-block")
}

func (s *stateSuite) addReprovisionUnit(c *tc.C, netNodeUUID string) {
	s.runQuery(c, "INSERT INTO charm (uuid, reference_name, source_id) VALUES (?, ?, 0)", "reprovision-charm", "reprovision")
	s.runQuery(c, "INSERT INTO charm_metadata (charm_uuid, name, subordinate) VALUES (?, ?, false)", "reprovision-charm", "reprovision")
	s.runQuery(c, `
INSERT INTO application (uuid, charm_uuid, name, life_id, space_uuid)
VALUES (?, ?, ?, 0, ?)`, "reprovision-application", "reprovision-charm", "reprovision", network.AlphaSpaceId)
	s.runQuery(c, `
INSERT INTO unit
    (uuid, name, life_id, application_uuid, net_node_uuid, charm_uuid)
VALUES (?, ?, 0, ?, ?, ?)`, "reprovision-unit", "reprovision/0",
		"reprovision-application", netNodeUUID, "reprovision-charm")
}

func (s *stateSuite) addReprovisionStorageIntent(c *tc.C, kind int) {
	s.runQuery(c, "INSERT INTO storage_pool (uuid, name, type) VALUES (?, ?, ?)",
		"reprovision-pool", "reprovision-pool", "reprovision")
	s.runQuery(c, `
INSERT INTO storage_instance
    (uuid, charm_name, storage_name, storage_kind_id, storage_id, life_id,
     storage_pool_uuid, requested_size_mib)
VALUES (?, ?, ?, ?, ?, 0, ?, 1024)`, "storage-instance", "reprovision",
		"data", kind, "data/0", "reprovision-pool")
	s.runQuery(c, `
INSERT INTO storage_attachment (uuid, storage_instance_uuid, unit_uuid, life_id)
VALUES (?, ?, ?, 0)`, "storage-attachment", "storage-instance", "reprovision-unit")
	s.runQuery(c, `
INSERT INTO storage_unit_owner (storage_instance_uuid, unit_uuid)
VALUES (?, ?)`, "storage-instance", "reprovision-unit")
}

func (s *stateSuite) addReprovisionVolumeStorage(
	c *tc.C, machineUUID, netNodeUUID string, volumeScope, attachmentScope int,
) {
	s.addReprovisionStorageIntent(c, 0)
	s.runQuery(c, `
INSERT INTO storage_volume (uuid, volume_id, life_id, provision_scope_id)
VALUES (?, ?, 0, ?)`, "storage-volume", "storage-volume/0", volumeScope)
	s.runQuery(c, "INSERT INTO storage_instance_volume VALUES (?, ?)",
		"storage-instance", "storage-volume")
	s.runQuery(c, "INSERT INTO storage_volume_status VALUES (?, 3, ?, NULL)",
		"storage-volume", "attached")
	s.runQuery(c, "INSERT INTO block_device (uuid, machine_uuid) VALUES (?, ?)",
		"storage-block", machineUUID)
	s.runQuery(c, "INSERT INTO block_device_link_device VALUES (?, ?, ?)",
		"storage-block", machineUUID, "sdc")
	s.runQuery(c, `
INSERT INTO storage_volume_attachment
    (uuid, storage_volume_uuid, net_node_uuid, life_id,
     provision_scope_id, block_device_uuid)
VALUES (?, ?, ?, 0, ?, ?)`, "storage-volume-attachment", "storage-volume",
		netNodeUUID, attachmentScope, "storage-block")
	s.runQuery(c, `
INSERT INTO storage_volume_attachment_plan
    (uuid, storage_volume_uuid, net_node_uuid, life_id, provision_scope_id)
VALUES (?, ?, ?, 0, ?)`, "storage-volume-plan", "storage-volume",
		netNodeUUID, attachmentScope)
	s.runQuery(c, "INSERT INTO storage_volume_attachment_plan_attr VALUES (?, ?, ?)",
		"storage-volume-plan", "device", "sdc")
	s.runQuery(c, "INSERT INTO machine_volume VALUES (?, ?)", machineUUID, "storage-volume")
}

func (s *stateSuite) addReprovisionVolumePlanStorage(
	c *tc.C, machineUUID, netNodeUUID string, scope int,
) {
	s.addReprovisionStorageIntent(c, 0)
	s.runQuery(c, `
INSERT INTO storage_volume (uuid, volume_id, life_id, provision_scope_id)
VALUES (?, ?, 0, ?)`, "storage-volume", "storage-volume/0", scope)
	s.runQuery(c, "INSERT INTO storage_instance_volume VALUES (?, ?)",
		"storage-instance", "storage-volume")
	s.runQuery(c, "INSERT INTO storage_volume_status VALUES (?, 0, ?, NULL)",
		"storage-volume", "pending")
	s.runQuery(c, `
INSERT INTO storage_volume_attachment_plan
    (uuid, storage_volume_uuid, net_node_uuid, life_id, provision_scope_id)
VALUES (?, ?, ?, 0, ?)`, "storage-volume-plan", "storage-volume", netNodeUUID, scope)
	s.runQuery(c, "INSERT INTO storage_volume_attachment_plan_attr VALUES (?, ?, ?)",
		"storage-volume-plan", "device", "sdc")
	s.runQuery(c, "INSERT INTO machine_volume VALUES (?, ?)", machineUUID, "storage-volume")
}

func (s *stateSuite) addReprovisionFilesystemStorage(
	c *tc.C, machineUUID, netNodeUUID string, filesystemScope, attachmentScope int,
) {
	s.addReprovisionStorageIntent(c, 1)
	s.runQuery(c, `
INSERT INTO storage_filesystem (uuid, filesystem_id, life_id, provision_scope_id)
VALUES (?, ?, 0, ?)`, "storage-filesystem", "storage-filesystem/0", filesystemScope)
	s.runQuery(c, "INSERT INTO storage_instance_filesystem VALUES (?, ?)",
		"storage-instance", "storage-filesystem")
	s.runQuery(c, "INSERT INTO storage_filesystem_status VALUES (?, 3, ?, NULL)",
		"storage-filesystem", "attached")
	s.runQuery(c, `
INSERT INTO storage_filesystem_attachment
    (uuid, storage_filesystem_uuid, net_node_uuid, provision_scope_id, life_id)
VALUES (?, ?, ?, ?, 0)`, "storage-filesystem-attachment", "storage-filesystem",
		netNodeUUID, attachmentScope)
	s.runQuery(c, "INSERT INTO machine_filesystem VALUES (?, ?)",
		machineUUID, "storage-filesystem")
}

func (s *stateSuite) checkInstanceID(c *tc.C, machineUUID, expected string) {
	var instanceID sql.Null[string]
	err := s.DB().QueryRowContext(c.Context(),
		"SELECT instance_id FROM machine_cloud_instance WHERE machine_uuid = ?", machineUUID,
	).Scan(&instanceID)
	c.Assert(err, tc.ErrorIsNil)
	if expected == "" {
		c.Check(instanceID.Valid, tc.IsFalse)
		return
	}
	c.Check(instanceID.V, tc.Equals, expected)
	c.Check(instanceID.Valid, tc.IsTrue)
}

func (s *stateSuite) rowCount(c *tc.C, table string) int {
	return s.rowCountWhere(c, table, "1 = 1")
}

func (s *stateSuite) rowCountWhere(c *tc.C, table, where string, args ...any) int {
	var count int
	err := s.DB().QueryRowContext(c.Context(),
		"SELECT COUNT(*) FROM "+table+" WHERE "+where, args...,
	).Scan(&count)
	c.Assert(err, tc.ErrorIsNil)
	return count
}
