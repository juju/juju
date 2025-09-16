// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/domain/blockdevice"
	domainlife "github.com/juju/juju/domain/life"
	domainnetwork "github.com/juju/juju/domain/network"
	networkerrors "github.com/juju/juju/domain/network/errors"
	domainstorageprovisioning "github.com/juju/juju/domain/storageprovisioning"
	storageprovisioningerrors "github.com/juju/juju/domain/storageprovisioning/errors"
	domaintesting "github.com/juju/juju/domain/storageprovisioning/testing"
)

// volumeSuite provides a set of tests for asserting the state interface for
// volumes in the model.
type volumeSuite struct {
	baseSuite
}

// TestVolumeSuite runs the tests defined in [volumeSuite].
func TestVolumeSuite(t *testing.T) {
	tc.Run(t, &volumeSuite{})
}

// TestGetVolumeAttachmentIDsOnlyUnits tests that when requesting ids for a
// volume attachment and no machines are using the net node the unit name is
// reported.
func (s *volumeSuite) TestGetVolumeAttachmentIDsOnlyUnits(c *tc.C) {
	netNodeUUID := s.newNetNode(c)
	appUUID, _ := s.newApplication(c, "foo")
	_, unitName := s.newUnitWithNetNode(c, "foo/0", appUUID, netNodeUUID)

	vsUUID, vsID := s.newMachineVolume(c)
	vsaUUID := s.newMachineVolumeAttachment(c, vsUUID, netNodeUUID)

	st := NewState(s.TxnRunnerFactory())
	result, err := st.GetVolumeAttachmentIDs(c.Context(), []string{vsaUUID.String()})
	c.Check(err, tc.ErrorIsNil)
	c.Check(result, tc.DeepEquals, map[string]domainstorageprovisioning.VolumeAttachmentID{
		vsaUUID.String(): {
			VolumeID:    vsID,
			MachineName: nil,
			UnitName:    &unitName,
		},
	})
}

// TestGetVolumeAttachmentIDsOnlyMachines tests that when requesting ids for a
// volume attachment and the net node is attached to a machine the machine
// name is set.
func (s *volumeSuite) TestGetVolumeAttachmentIDsOnlyMachines(c *tc.C) {
	netNodeUUID := s.newNetNode(c)
	_, machineName := s.newMachineWithNetNode(c, netNodeUUID)

	vsUUID, vsID := s.newMachineVolume(c)
	vsaUUID := s.newMachineVolumeAttachment(c, vsUUID, netNodeUUID)

	st := NewState(s.TxnRunnerFactory())
	result, err := st.GetVolumeAttachmentIDs(c.Context(), []string{vsaUUID.String()})
	c.Check(err, tc.ErrorIsNil)
	c.Check(result, tc.DeepEquals, map[string]domainstorageprovisioning.VolumeAttachmentID{
		vsaUUID.String(): {
			VolumeID:    vsID,
			MachineName: &machineName,
			UnitName:    nil,
		},
	})
}

// TestGetVolumeAttachmentIDsMachineNotUnit tests that when requesting ids for a
// volume attachment and the net node is attached to a machine the machine
// name is set. This should remain true when the net node is also used by a
// unit. This is a valid case when units are assigned to a machine.
func (s *volumeSuite) TestGetVolumeAttachmentIDsMachineNotUnit(c *tc.C) {
	netNodeUUID := s.newNetNode(c)
	_, machineName := s.newMachineWithNetNode(c, netNodeUUID)
	appUUID, _ := s.newApplication(c, "foo")
	s.newUnitWithNetNode(c, "foo/0", appUUID, netNodeUUID)

	vsUUID, vsID := s.newMachineVolume(c)
	vsaUUID := s.newMachineVolumeAttachment(c, vsUUID, netNodeUUID)

	st := NewState(s.TxnRunnerFactory())
	result, err := st.GetVolumeAttachmentIDs(c.Context(), []string{vsaUUID.String()})
	c.Check(err, tc.ErrorIsNil)
	c.Check(result, tc.DeepEquals, map[string]domainstorageprovisioning.VolumeAttachmentID{
		vsaUUID.String(): {
			VolumeID:    vsID,
			MachineName: &machineName,
			UnitName:    nil,
		},
	})
}

// TestGetVolumeAttachmentIDsMixed tests that when requesting ids for a
// mixed set of volume attachments uuids the machine name and unit name are
// correctly set.
func (s *volumeSuite) TestGetVolumeAttachmentIDsMixed(c *tc.C) {
	netNodeUUID1 := s.newNetNode(c)
	netNodeUUID2 := s.newNetNode(c)
	_, machineName := s.newMachineWithNetNode(c, netNodeUUID1)
	appUUID, _ := s.newApplication(c, "foo")
	_, unitName := s.newUnitWithNetNode(c, "foo/0", appUUID, netNodeUUID2)

	vsOneUUID, vsOneID := s.newMachineVolume(c)
	vsaOneUUID := s.newMachineVolumeAttachment(c, vsOneUUID, netNodeUUID1)

	vsTwoUUID, vsTwoID := s.newMachineVolume(c)
	vsaTwoUUID := s.newMachineVolumeAttachment(c, vsTwoUUID, netNodeUUID2)

	st := NewState(s.TxnRunnerFactory())
	result, err := st.GetVolumeAttachmentIDs(c.Context(), []string{
		vsaOneUUID.String(), vsaTwoUUID.String(),
	})
	c.Check(err, tc.ErrorIsNil)
	c.Check(result, tc.DeepEquals, map[string]domainstorageprovisioning.VolumeAttachmentID{
		vsaOneUUID.String(): {
			VolumeID:    vsOneID,
			MachineName: &machineName,
			UnitName:    nil,
		},
		vsaTwoUUID.String(): {
			VolumeID:    vsTwoID,
			MachineName: nil,
			UnitName:    &unitName,
		},
	})
}

// TestGetVolumeAttachmentIDsNotMachineOrUnit tests that when requesting
// ids for a volume attachment that is using a net node not attached to a
// machine or unit the uuid is dropped from the final result.
func (s *volumeSuite) TestGetVolumeAttachmentIDsNotMachineOrUnit(c *tc.C) {
	netNodeUUID := s.newNetNode(c)
	vsUUID, _ := s.newMachineVolume(c)
	vsaUUID := s.newMachineVolumeAttachment(c, vsUUID, netNodeUUID)

	st := NewState(s.TxnRunnerFactory())
	result, err := st.GetVolumeAttachmentIDs(c.Context(), []string{vsaUUID.String()})
	c.Check(err, tc.ErrorIsNil)
	c.Check(result, tc.HasLen, 0)
}

// TestGetVolumeAttachmentIDsNotFound tests that when requesting ids for
// volume attachment uuids that don't exist the uuids are excluded from the
// result with no error returned.
func (s *volumeSuite) TestGetVolumeAttachmentIDsNotFound(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	result, err := st.GetVolumeAttachmentIDs(c.Context(), []string{"no-exist"})
	c.Check(err, tc.ErrorIsNil)
	c.Check(result, tc.HasLen, 0)
}

// TestGetVolumeAttachmentLifeForNetNode tests that the correct life is
// reported for each model provisioned volume attachment associated with the
// given net node.
//
// We also inject a life change during the test to make sure that it is
// reflected.
func (s *volumeSuite) TestGetVolumeAttachmentLifeForNetNode(c *tc.C) {
	netNodeUUID := s.newNetNode(c)
	vsUUID1, _ := s.newMachineVolume(c)
	vsUUID2, _ := s.newMachineVolume(c)
	vsUUID3, _ := s.newMachineVolume(c)
	vsaUUID1 := s.newMachineVolumeAttachment(c, vsUUID1, netNodeUUID)
	vsaUUID2 := s.newMachineVolumeAttachment(c, vsUUID2, netNodeUUID)
	vsaUUID3 := s.newMachineVolumeAttachment(c, vsUUID3, netNodeUUID)

	st := NewState(s.TxnRunnerFactory())
	lives, err := st.GetVolumeAttachmentLifeForNetNode(
		c.Context(), netNodeUUID,
	)
	c.Check(err, tc.ErrorIsNil)
	c.Check(lives, tc.DeepEquals, map[string]domainlife.Life{
		vsaUUID1.String(): domainlife.Alive,
		vsaUUID2.String(): domainlife.Alive,
		vsaUUID3.String(): domainlife.Alive,
	})

	// Apply a life change to one of the attachments and check the change comes
	// out.
	s.changeVolumeAttachmentLife(c, vsaUUID1, domainlife.Dying)
	lives, err = st.GetVolumeAttachmentLifeForNetNode(
		c.Context(), netNodeUUID,
	)
	c.Check(err, tc.ErrorIsNil)
	c.Check(lives, tc.DeepEquals, map[string]domainlife.Life{
		vsaUUID1.String(): domainlife.Dying,
		vsaUUID2.String(): domainlife.Alive,
		vsaUUID3.String(): domainlife.Alive,
	})
}

// TestGetVolumeAttachmentLifeNoResults tests that when no attachment lives
// exist for a net node an empty result is returned with no error.
func (s *volumeSuite) TestGetVolumeAttachmentLifeNoResults(c *tc.C) {
	netNodeUUID := s.newNetNode(c)
	st := NewState(s.TxnRunnerFactory())
	lives, err := st.GetVolumeAttachmentLifeForNetNode(
		c.Context(), netNodeUUID,
	)
	c.Check(err, tc.ErrorIsNil)
	c.Check(lives, tc.HasLen, 0)
}

// TestGetVolumeAttachment tests that a volume attachment is returned without
// block device information when it is not available.
func (s *volumeSuite) TestGetVolumeAttachment(c *tc.C) {
	netNodeUUID := s.newNetNode(c)

	volUUID, vsID := s.newMachineVolume(c)
	vaUUID := s.newMachineVolumeAttachment(c, volUUID, netNodeUUID)

	st := NewState(s.TxnRunnerFactory())
	result, err := st.GetVolumeAttachment(c.Context(), vaUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.DeepEquals, domainstorageprovisioning.VolumeAttachment{
		VolumeID: vsID,
	})
}

// TestGetVolumeAttachmentWithBlockDevice tests that a volume attachment with a
// block device returns relevant block device information.
func (s *volumeSuite) TestGetVolumeAttachmentWithBlockDevice(c *tc.C) {
	netNodeUUID := s.newNetNode(c)

	machineUUID, _ := s.newMachineWithNetNode(c, netNodeUUID)
	volUUID, vsID := s.newMachineVolume(c)
	vaUUID := s.newMachineVolumeAttachment(c, volUUID, netNodeUUID)
	bdUUID := s.newBlockDevice(c, machineUUID, "blocky", "blockyhwid",
		"blocky:addr", []string{
			"/dev/a",
			"/dev/b",
		})
	s.changeVolumeAttachmentInfo(c, vaUUID, bdUUID, true)

	st := NewState(s.TxnRunnerFactory())
	result, err := st.GetVolumeAttachment(c.Context(), vaUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(result, tc.DeepEquals, domainstorageprovisioning.VolumeAttachment{
		VolumeID:              vsID,
		ReadOnly:              true,
		BlockDeviceName:       "blocky",
		BlockDeviceLinks:      []string{"/dev/a", "/dev/b"},
		BlockDeviceBusAddress: "blocky:addr",
	})
}

// TestGetVolumeAttachmentNotFound tests that get volume attachment returns a
// volume attachment not found error.
func (s *volumeSuite) TestGetVolumeAttachmentNotFound(c *tc.C) {
	vaUUID := domaintesting.GenVolumeAttachmentUUID(c)

	st := NewState(s.TxnRunnerFactory())
	_, err := st.GetVolumeAttachment(c.Context(), vaUUID)
	c.Assert(err, tc.ErrorIs, storageprovisioningerrors.VolumeAttachmentNotFound)
}

// TestGetVolumeAttachmentPlanLifeFornetNode tests that the correct life is
// reported for each volume attachment plan associated with the given net node.
// We expect in this test that it is the volume id for the attachment plan
// that is returned and not the uuid for the attachment plan.
//
// We also inject a life change during the test to make sure that it is
// reflected.
func (s *volumeSuite) TestGetVolumeAttachmentPlanLifeForNetNode(c *tc.C) {
	netNodeUUID := s.newNetNode(c)
	vsOneUUID, vOneID := s.newMachineVolume(c)
	vsTwoUUID, vTwoID := s.newMachineVolume(c)
	vsThreeUUID, vThreeID := s.newMachineVolume(c)
	vsapOneUUID := s.newVolumeAttachmentPlan(c, vsOneUUID, netNodeUUID)
	s.newVolumeAttachmentPlan(c, vsTwoUUID, netNodeUUID)
	s.newVolumeAttachmentPlan(c, vsThreeUUID, netNodeUUID)

	st := NewState(s.TxnRunnerFactory())
	lives, err := st.GetVolumeAttachmentPlanLifeForNetNode(
		c.Context(), netNodeUUID,
	)
	c.Check(err, tc.ErrorIsNil)
	c.Check(lives, tc.DeepEquals, map[string]domainlife.Life{
		vOneID:   domainlife.Alive,
		vTwoID:   domainlife.Alive,
		vThreeID: domainlife.Alive,
	})

	// Apply a life change to one of the attachments and check the change comes
	// out.
	s.changeVolumeAttachmentPlanLife(c, vsapOneUUID, domainlife.Dying)
	lives, err = st.GetVolumeAttachmentPlanLifeForNetNode(
		c.Context(), netNodeUUID,
	)
	c.Check(err, tc.ErrorIsNil)
	c.Check(lives, tc.DeepEquals, map[string]domainlife.Life{
		vOneID:   domainlife.Dying,
		vTwoID:   domainlife.Alive,
		vThreeID: domainlife.Alive,
	})
}

// TestGetVolumeAttachmentPlanLifeNoResults tests that when no attachment plan
// lives exist for a net node an empty result is returned with no error.
func (s *volumeSuite) TestGetVolumeAttachmentPlanLifeNoResults(c *tc.C) {
	netNodeUUID := s.newNetNode(c)
	st := NewState(s.TxnRunnerFactory())
	lives, err := st.GetVolumeAttachmentPlanLifeForNetNode(
		c.Context(), netNodeUUID,
	)
	c.Check(err, tc.ErrorIsNil)
	c.Check(lives, tc.HasLen, 0)
}

// TestGetVolumeLifeForNetNode tests if we can get the volume life for
// volumes attached to a specified machine's net node.
func (s *volumeSuite) TestGetVolumeLifeForNetNode(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

	netNodeUUID := s.newNetNode(c)
	vsOneUUID, vsOneID := s.newMachineVolume(c)
	_ = s.newMachineVolumeAttachment(c, vsOneUUID, netNodeUUID)
	vsTwoUUID, vsTwoID := s.newMachineVolume(c)
	_ = s.newMachineVolumeAttachment(c, vsTwoUUID, netNodeUUID)
	vsThreeUUID, vsThreeID := s.newMachineVolume(c)
	_ = s.newMachineVolumeAttachment(c, vsThreeUUID, netNodeUUID)

	s.changeVolumeLife(c, vsTwoUUID, domainlife.Dying)
	s.changeVolumeLife(c, vsThreeUUID, domainlife.Dead)

	// Add unrelated volumes.
	_, _ = s.newModelVolume(c)
	vsIDOtherMachine, _ := s.newMachineVolume(c)
	_ = s.newMachineVolumeAttachment(c, vsIDOtherMachine, s.newNetNode(c))

	vsUUIDs, err := st.GetVolumeLifeForNetNode(
		c.Context(), netNodeUUID,
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(vsUUIDs, tc.DeepEquals, map[string]domainlife.Life{
		vsOneID:   domainlife.Alive,
		vsTwoID:   domainlife.Dying,
		vsThreeID: domainlife.Dead,
	})
}

// TestInitialWatchStatementMachineProvisionedVolumes tests the initial query
// for machine provisioned volumes watcher returns only the volume UUIDs
// attached to the specified machine net node.
func (s *volumeSuite) TestInitialWatchStatementMachineProvisionedVolumes(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

	netNodeUUID := s.newNetNode(c)
	vsOneUUID, vsOneID := s.newMachineVolume(c)
	_ = s.newMachineVolumeAttachment(c, vsOneUUID, netNodeUUID)
	vsTwoUUID, vsTwoID := s.newMachineVolume(c)
	_ = s.newMachineVolumeAttachment(c, vsTwoUUID, netNodeUUID)

	// Add unrelated volumes.
	_, _ = s.newModelVolume(c)
	vsIDOtherMachine, _ := s.newMachineVolume(c)
	_ = s.newMachineVolumeAttachment(c, vsIDOtherMachine, s.newNetNode(c))

	ns, initialQuery := st.InitialWatchStatementMachineProvisionedVolumes(
		netNodeUUID,
	)
	c.Check(ns, tc.Equals, "storage_volume_life_machine_provisioning")

	db := s.TxnRunner()
	vsUUIDs, err := initialQuery(c.Context(), db)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(vsUUIDs, tc.DeepEquals, map[string]domainlife.Life{
		vsOneID: domainlife.Alive,
		vsTwoID: domainlife.Alive,
	})
}

// TestInitialWatchStatementMachineProvisionedVolumesNone tests the initial
// query for machine provisioned volumes watcher does not return an error
// when no machine provisioned volumes are attached to the specified machine
// net node.
func (s *volumeSuite) TestInitialWatchStatementMachineProvisionedVolumesNone(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

	netNodeUUID := s.newNetNode(c)

	// Add unrelated volumes.
	_, _ = s.newModelVolume(c)
	vsIDOtherMachine, _ := s.newMachineVolume(c)
	s.newMachineVolumeAttachment(c, vsIDOtherMachine, s.newNetNode(c))

	ns, initialQuery := st.InitialWatchStatementMachineProvisionedVolumes(
		netNodeUUID,
	)
	c.Check(ns, tc.Equals, "storage_volume_life_machine_provisioning")

	db := s.TxnRunner()
	vsUUIDs, err := initialQuery(c.Context(), db)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(vsUUIDs, tc.HasLen, 0)
}

// TestInitialWatchStatementMachineProvisionedVolumesNetNodeMissing tests
// the initial query for machine provisioned volumes watcher errors when the
// net node specified is not found.
func (s *volumeSuite) TestInitialWatchStatementMachineProvisionedVolumesNetNodeMissing(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

	netNodeUUID, err := domainnetwork.NewNetNodeUUID()
	c.Assert(err, tc.ErrorIsNil)

	ns, initialQuery := st.InitialWatchStatementMachineProvisionedVolumes(
		netNodeUUID,
	)
	c.Check(ns, tc.Equals, "storage_volume_life_machine_provisioning")

	db := s.TxnRunner()
	_, err = initialQuery(c.Context(), db)
	c.Assert(err, tc.NotNil)
}

// TestInitialWatchStatementModelProvisionedVolumesNone tests the initial
// query for a model provisioned filsystem watcher returns no error when there
// is not any model provisioned volumes.
func (s *volumeSuite) TestInitialWatchStatementModelProvisionedVolumesNone(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	_, _ = s.newMachineVolume(c)

	ns, initialQuery := st.InitialWatchStatementModelProvisionedVolumes()
	c.Check(ns, tc.Equals, "storage_volume_life_model_provisioning")

	db := s.TxnRunner()
	vsIDs, err := initialQuery(c.Context(), db)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(vsIDs, tc.HasLen, 0)
}

// TestInitialWatchStatementModelProvisionedVolumes tests the initial query
// for a model provisioned filsystem watcher returns only the volume IDs for
// the model provisoned volumes.
func (s *volumeSuite) TestInitialWatchStatementModelProvisionedVolumes(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	_, vsOneID := s.newModelVolume(c)
	_, vsTwoID := s.newModelVolume(c)
	_, _ = s.newMachineVolume(c)

	ns, initialQuery := st.InitialWatchStatementModelProvisionedVolumes()
	c.Check(ns, tc.Equals, "storage_volume_life_model_provisioning")

	db := s.TxnRunner()
	vsIDs, err := initialQuery(c.Context(), db)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(vsIDs, tc.SameContents, []string{vsOneID, vsTwoID})
}

// TestInitialWatchStatementMachineProvisionedVolumeAttachments tests the
// initial query for machine provisioned volume attachments watcher returns
// only the volume attachment UUIDs attached to the specified net node.
func (s *volumeSuite) TestInitialWatchStatementMachineProvisionedVolumeAttachments(c *tc.C) {
	netNodeUUID := s.newNetNode(c)
	vsOneUUID, _ := s.newMachineVolume(c)
	vsaOneUUID := s.newMachineVolumeAttachment(c, vsOneUUID, netNodeUUID)
	vsTwoUUID, _ := s.newMachineVolume(c)
	vsaTwoUUID := s.newMachineVolumeAttachment(c, vsTwoUUID, netNodeUUID)

	// Add unrelated volumes.
	_, _ = s.newModelVolume(c)
	vsIDOtherMachine, _ := s.newMachineVolume(c)
	_ = s.newMachineVolumeAttachment(c, vsIDOtherMachine, s.newNetNode(c))

	st := NewState(s.TxnRunnerFactory())
	ns, initialQuery := st.InitialWatchStatementMachineProvisionedVolumeAttachments(
		netNodeUUID,
	)
	c.Check(ns, tc.Equals, "storage_volume_attachment_life_machine_provisioning")

	db := s.TxnRunner()
	vsaUUIDs, err := initialQuery(c.Context(), db)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(vsaUUIDs, tc.DeepEquals, map[string]domainlife.Life{
		vsaTwoUUID.String(): domainlife.Alive,
		vsaOneUUID.String(): domainlife.Alive,
	})
}

// TestInitialWatchStatementMachineProvisionedVolumeAttachmentsNone tests
// the initial query for machine provisioned volume attachments watcher does
// not return an error when no machine provisioned volume attachments are
// attached to the specified net node.
func (s *volumeSuite) TestInitialWatchStatementMachineProvisionedVolumeAttachmentsNone(c *tc.C) {
	netNodeUUID := s.newNetNode(c)

	// Add unrelated volumes.
	_, _ = s.newModelVolume(c)
	vsIDOtherMachine, _ := s.newMachineVolume(c)
	s.newMachineVolumeAttachment(c, vsIDOtherMachine, s.newNetNode(c))

	st := NewState(s.TxnRunnerFactory())
	ns, initialQuery := st.InitialWatchStatementMachineProvisionedVolumeAttachments(
		netNodeUUID,
	)
	c.Check(ns, tc.Equals, "storage_volume_attachment_life_machine_provisioning")

	db := s.TxnRunner()
	vsaUUIDs, err := initialQuery(c.Context(), db)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(vsaUUIDs, tc.HasLen, 0)
}

// TestInitialWatchStatementMachineProvisionedVolumeAttachmentsNetNodeMissing
// tests the initial query for machine provisioned volume attachmewnts
// watcher errors when the net node specified is not found.
func (s *volumeSuite) TestInitialWatchStatementMachineProvisionedVolumeAttachmentsNetNodeMissing(c *tc.C) {
	netNodeUUID, err := domainnetwork.NewNetNodeUUID()
	c.Assert(err, tc.ErrorIsNil)

	st := NewState(s.TxnRunnerFactory())
	ns, initialQuery := st.InitialWatchStatementMachineProvisionedVolumeAttachments(
		netNodeUUID,
	)
	c.Check(ns, tc.Equals, "storage_volume_attachment_life_machine_provisioning")

	db := s.TxnRunner()
	_, err = initialQuery(c.Context(), db)
	// We don't focus on what the error is as no specific error type is offered
	// as part of the contract. We just care that an error occurred.
	c.Assert(err, tc.NotNil)
}

// TestInitialWatchStatementModelProvisionedVolumeAttachmentsNone tests the
// initial query for a model provisioned filsystem attachment watcher returns no
// error when there is no model provisioned volume attachments.
func (s *volumeSuite) TestInitialWatchStatementModelProvisionedVolumeAttachmentsNone(c *tc.C) {
	// Create a machine based volume attachment to assert  this doesn't show
	// up.
	netNode := s.newNetNode(c)
	vsUUID, _ := s.newMachineVolume(c)
	s.newMachineVolumeAttachment(c, vsUUID, netNode)

	st := NewState(s.TxnRunnerFactory())
	ns, initialQuery := st.InitialWatchStatementModelProvisionedVolumeAttachments()
	c.Check(ns, tc.Equals, "storage_volume_attachment_life_model_provisioning")

	db := s.TxnRunner()
	vsaUUIDs, err := initialQuery(c.Context(), db)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(vsaUUIDs, tc.HasLen, 0)
}

// TestInitialWatchStatementModelProvisionedVolumeAttachments tests the
// initial query for a model provisioned filsystem attachment watcher returns
// only the volume attachment uuids for the model provisoned volume
// attachments.
func (s *volumeSuite) TestInitialWatchStatementModelProvisionedVolumeAttachments(c *tc.C) {
	netNodeUUID := s.newNetNode(c)
	st := NewState(s.TxnRunnerFactory())
	vsOneUUID, _ := s.newModelVolume(c)
	vsTwoUUID, _ := s.newModelVolume(c)
	vsThreeUUID, _ := s.newMachineVolume(c)
	vsaOneUUID := s.newModelVolumeAttachment(c, vsOneUUID, netNodeUUID)
	vsaTwoUUID := s.newModelVolumeAttachment(c, vsTwoUUID, netNodeUUID)
	s.newMachineVolumeAttachment(c, vsThreeUUID, netNodeUUID)

	ns, initialQuery := st.InitialWatchStatementModelProvisionedVolumeAttachments()
	c.Check(ns, tc.Equals, "storage_volume_attachment_life_model_provisioning")

	db := s.TxnRunner()
	vsaUUIDs, err := initialQuery(c.Context(), db)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(vsaUUIDs, tc.SameContents, []string{
		vsaOneUUID.String(), vsaTwoUUID.String(),
	})
}

// TestInitialWatchStatementMachineProvisionedVolumeAttachmentPlans tests the
// initial query for machine provisioned volume attachment plans watcher returns
// only the volume ids attached to the specified net node.
func (s *volumeSuite) TestInitialWatchStatementMachineProvisionedVolumeAttachmentPlans(c *tc.C) {
	netNodeUUID := s.newNetNode(c)
	vsOneUUID, vOneID := s.newMachineVolume(c)
	s.newVolumeAttachmentPlan(c, vsOneUUID, netNodeUUID)
	vsTwoUUID, vTwoID := s.newMachineVolume(c)
	s.newVolumeAttachmentPlan(c, vsTwoUUID, netNodeUUID)

	// Add unrelated volumes.
	_, _ = s.newModelVolume(c)
	vsIDOtherMachine, _ := s.newMachineVolume(c)
	_ = s.newVolumeAttachmentPlan(c, vsIDOtherMachine, s.newNetNode(c))

	st := NewState(s.TxnRunnerFactory())
	ns, initialQuery := st.InitialWatchStatementVolumeAttachmentPlans(
		netNodeUUID,
	)
	c.Check(ns, tc.Equals, "storage_volume_attachment_plan_life_machine_provisioning")

	db := s.TxnRunner()
	vsaUUIDs, err := initialQuery(c.Context(), db)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(vsaUUIDs, tc.DeepEquals, map[string]domainlife.Life{
		vOneID: domainlife.Alive,
		vTwoID: domainlife.Alive,
	})
}

// TestInitialWatchStatementMachineProvisionedVolumeAttachmentPlansNone tests
// the initial query for machine provisioned volume attachment plans watcher
// does not return an error when no machine provisioned volume attachments are
// attached to the specified net node.
func (s *volumeSuite) TestInitialWatchStatementMachineProvisionedVolumeAttachmentPlansNone(c *tc.C) {
	netNodeUUID := s.newNetNode(c)
	// Add unrelated volumes.
	vsUUID, _ := s.newMachineVolume(c)
	s.newVolumeAttachmentPlan(c, vsUUID, s.newNetNode(c))

	st := NewState(s.TxnRunnerFactory())
	ns, initialQuery := st.InitialWatchStatementVolumeAttachmentPlans(
		netNodeUUID,
	)
	c.Check(ns, tc.Equals, "storage_volume_attachment_plan_life_machine_provisioning")

	db := s.TxnRunner()
	planUUIDs, err := initialQuery(c.Context(), db)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(planUUIDs, tc.HasLen, 0)
}

// TestInitialWatchStatementMachineProvisionedVolumeAttachmentPlansNetNodeMissing
// tests the initial query for machine provisioned volume attachmewnt plans
// watcher errors when the net node specified is not found.
func (s *volumeSuite) TestInitialWatchStatementMachineProvisionedVolumeAttachmentPlansNetNodeMissing(c *tc.C) {
	netNodeUUID, err := domainnetwork.NewNetNodeUUID()
	c.Assert(err, tc.ErrorIsNil)

	st := NewState(s.TxnRunnerFactory())
	ns, initialQuery := st.InitialWatchStatementVolumeAttachmentPlans(
		netNodeUUID,
	)
	c.Check(ns, tc.Equals, "storage_volume_attachment_plan_life_machine_provisioning")

	db := s.TxnRunner()
	_, err = initialQuery(c.Context(), db)
	// We don't focus on what the error is as no specific error type is offered
	// as part of the contract. We just care that an error occurred.
	c.Assert(err, tc.NotNil)
}

// TestGetVolumeAttachmentLife tests that asking for the life of a volume
// attachment that doesn't exist returns to the caller an error satisfying
// [storageprovisioningerrors.VolumeAttachmentNotFound].
func (s *volumeSuite) TestGetVolumeAttachmentLifeNotFound(c *tc.C) {
	uuid := domaintesting.GenVolumeAttachmentUUID(c)
	st := NewState(s.TxnRunnerFactory())

	_, err := st.GetVolumeAttachmentLife(c.Context(), uuid)
	c.Check(err, tc.ErrorIs, storageprovisioningerrors.VolumeAttachmentNotFound)
}

func (s *volumeSuite) TestGetVolumeAttachmentLife(c *tc.C) {
	fsUUID, _ := s.newModelVolume(c)
	netNodeUUID := s.newNetNode(c)
	uuid := s.newModelVolumeAttachment(c, fsUUID, netNodeUUID)
	st := NewState(s.TxnRunnerFactory())

	life, err := st.GetVolumeAttachmentLife(c.Context(), uuid)
	c.Check(err, tc.ErrorIsNil)
	c.Check(life, tc.Equals, domainlife.Alive)

	// Update the life and confirm that it is reflected out again.
	s.changeVolumeAttachmentLife(c, uuid, domainlife.Dying)
	life, err = st.GetVolumeAttachmentLife(c.Context(), uuid)
	c.Check(err, tc.ErrorIsNil)
	c.Check(life, tc.Equals, domainlife.Dying)
}

func (s *volumeSuite) TestGetVolumeAttachmentPlanUUIDForVolumeNetNode(c *tc.C) {
	netNodeUUID := s.newNetNode(c)
	volUUID, _ := s.newMachineVolume(c)
	vapUUID := s.newVolumeAttachmentPlan(c, volUUID, netNodeUUID)

	st := NewState(s.TxnRunnerFactory())
	uuid, err := st.GetVolumeAttachmentPlanUUIDForVolumeNetNode(
		c.Context(), volUUID, netNodeUUID,
	)
	c.Check(err, tc.ErrorIsNil)
	c.Check(uuid.String(), tc.Equals, vapUUID)
}

// TestGetVolumeAttachmentPlanUUIDForVolumeNetNodeVolNotFound tests that the
// caller get backs a [storageprovisioningerrors.VolumeNotFound] error when
// asking for an attachment using a volume uuid that does not exist in the
// model.
func (s *volumeSuite) TestGetVolumeAttachmentPlanUUIDForVolumeNetNodeVolNotFound(c *tc.C) {
	netNodeUUID := s.newNetNode(c)
	notFoundVol := domaintesting.GenVolumeUUID(c)
	st := NewState(s.TxnRunnerFactory())

	_, err := st.GetVolumeAttachmentPlanUUIDForVolumeNetNode(
		c.Context(), notFoundVol, netNodeUUID,
	)

	c.Check(err, tc.ErrorIs, storageprovisioningerrors.VolumeNotFound)
}

// TestGetVolumeAttachmentPlanUUIDForVolumeNetNodeNetNodeNotFound tests that the
// caller get backs a [networkerrors.NetNodeNotFound] error when asking
// for an attachment using a net node uuid that does not exist in the model.
func (s *volumeSuite) TestGetVolumeAttachmentPlanUUIDForVolumeNetNodeNetNodeNotFound(c *tc.C) {
	notFoundNodeUUID, err := domainnetwork.NewNetNodeUUID()
	c.Assert(err, tc.ErrorIsNil)
	volUUID, _ := s.newModelVolume(c)
	st := NewState(s.TxnRunnerFactory())

	_, err = st.GetVolumeAttachmentPlanUUIDForVolumeNetNode(
		c.Context(), volUUID, notFoundNodeUUID,
	)

	c.Check(err, tc.ErrorIs, networkerrors.NetNodeNotFound)
}

// TestGetVolumeAttachmentPlanUUIDForVolumeNetNodeUnrelated tests that if the
// volume uuid and net node uuid exist but are unrelated within an
// attachment an error satisfying
// [storageprovisioningerrors.VolumeAttachmentPlanNotFound] is returned.
func (s *volumeSuite) TestGetVolumeAttachmentPlanUUIDForVolumeNetNodeUnrelated(c *tc.C) {
	nnUUIDOne := s.newNetNode(c)
	nnUUIDTwo := s.newNetNode(c)
	volUUIDOne, _ := s.newMachineVolume(c)
	volUUIDTwo, _ := s.newMachineVolume(c)
	s.newVolumeAttachmentPlan(c, volUUIDOne, nnUUIDOne)
	s.newVolumeAttachmentPlan(c, volUUIDTwo, nnUUIDTwo)
	st := NewState(s.TxnRunnerFactory())

	_, err := st.GetVolumeAttachmentPlanUUIDForVolumeNetNode(
		c.Context(), volUUIDOne, nnUUIDTwo,
	)

	c.Check(err, tc.ErrorIs, storageprovisioningerrors.VolumeAttachmentPlanNotFound)
}

func (s *volumeSuite) TestGetVolumeAttachmentUUIDForVolumeNetNode(c *tc.C) {
	netNodeUUID := s.newNetNode(c)
	fsUUID, _ := s.newMachineVolume(c)
	fsaUUID := s.newMachineVolumeAttachment(c, fsUUID, netNodeUUID)

	st := NewState(s.TxnRunnerFactory())
	uuid, err := st.GetVolumeAttachmentUUIDForVolumeNetNode(
		c.Context(), fsUUID, netNodeUUID,
	)
	c.Check(err, tc.ErrorIsNil)
	c.Check(uuid.String(), tc.Equals, fsaUUID.String())
}

// TestGetVolumeAttachmentUUIDForVolumeNetNodeVolNotFound tests that the caller
// get backs a [storageprovisioningerrors.VolumeNotFound] error when asking
// for an attachment using a volume uuid that does not exist in the model.
func (s *volumeSuite) TestGetVolumeAttachmentUUIDForVolumeNetNodeVolNotFound(c *tc.C) {
	netNodeUUID := s.newNetNode(c)
	notFoundFS := domaintesting.GenVolumeUUID(c)
	st := NewState(s.TxnRunnerFactory())

	_, err := st.GetVolumeAttachmentUUIDForVolumeNetNode(
		c.Context(), notFoundFS, netNodeUUID,
	)

	c.Check(err, tc.ErrorIs, storageprovisioningerrors.VolumeNotFound)
}

// TestGetVolumeAttachmentUUIDForVolumeNetNodeNetNodeNotFound tests that the
// caller get backs a [networkerrors.NetNodeNotFound] error when asking
// for an attachment using a net node uuid that does not exist in the model.
func (s *volumeSuite) TestGetVolumeAttachmentUUIDForVolumeNetNodeNetNodeNotFound(c *tc.C) {
	notFoundNodeUUID, err := domainnetwork.NewNetNodeUUID()
	c.Assert(err, tc.ErrorIsNil)
	volUUID, _ := s.newModelVolume(c)
	st := NewState(s.TxnRunnerFactory())

	_, err = st.GetVolumeAttachmentUUIDForVolumeNetNode(
		c.Context(), volUUID, notFoundNodeUUID,
	)

	c.Check(err, tc.ErrorIs, networkerrors.NetNodeNotFound)
}

// TestGetVolumeAttachmentUUIDForVolumeNetNodeUnrelated tests that if the
// volume uuid and net node uuid exist but are unrelated within an
// attachment an error satisfying
// [storageprovisioningerrors.VolumeAttachmentNotFound] is returned.
func (s *volumeSuite) TestGetVolumeAttachmentUUIDForVolumeNetNodeUnrelated(c *tc.C) {
	nnUUIDOne := s.newNetNode(c)
	nnUUIDTwo := s.newNetNode(c)
	volUUIDOne, _ := s.newMachineVolume(c)
	volUUIDTwo, _ := s.newMachineVolume(c)
	s.newMachineVolumeAttachment(c, volUUIDOne, nnUUIDOne)
	s.newMachineVolumeAttachment(c, volUUIDTwo, nnUUIDTwo)
	st := NewState(s.TxnRunnerFactory())

	_, err := st.GetVolumeAttachmentUUIDForVolumeNetNode(
		c.Context(), volUUIDOne, nnUUIDTwo,
	)

	c.Check(err, tc.ErrorIs, storageprovisioningerrors.VolumeAttachmentNotFound)
}

// TestGetVolumeLifeNotFound tests that asking for the life of a volume
// attachment that doesn't exist returns to the caller an error satisfying
// [storageprovisioningerrors.VolumeNotFound].
func (s *volumeSuite) TestGetVolumeLifeNotFound(c *tc.C) {
	uuid := domaintesting.GenVolumeUUID(c)
	st := NewState(s.TxnRunnerFactory())

	_, err := st.GetVolumeLife(c.Context(), uuid)
	c.Check(err, tc.ErrorIs, storageprovisioningerrors.VolumeNotFound)
}

func (s *volumeSuite) TestGetVolumeLife(c *tc.C) {
	volUUID, _ := s.newModelVolume(c)
	st := NewState(s.TxnRunnerFactory())

	life, err := st.GetVolumeLife(c.Context(), volUUID)
	c.Check(err, tc.ErrorIsNil)
	c.Check(life, tc.Equals, domainlife.Alive)

	// Update the life and confirm that it is reflected out again.
	s.changeVolumeLife(c, volUUID, domainlife.Dying)
	life, err = st.GetVolumeLife(c.Context(), volUUID)
	c.Check(err, tc.ErrorIsNil)
	c.Check(life, tc.Equals, domainlife.Dying)
}

// TestGetVolumeUUIDForIDNotFound tests that asking for the uuid of a
// volume using an id that does not exist returns an error satisfying
// [storageprovisioningerrors.VolumeNotFound] to the caller.
func (s *volumeSuite) TestGetVolumeUUIDForIDNotFound(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

	_, err := st.GetVolumeUUIDForID(c.Context(), "no-exist")
	c.Check(err, tc.ErrorIs, storageprovisioningerrors.VolumeNotFound)
}

func (s *volumeSuite) TestGetVolumeUUIDForID(c *tc.C) {
	volUUID, fsID := s.newModelVolume(c)
	st := NewState(s.TxnRunnerFactory())

	gotUUID, err := st.GetVolumeUUIDForID(c.Context(), fsID)

	c.Check(err, tc.ErrorIsNil)
	c.Check(gotUUID.String(), tc.Equals, volUUID.String())
}

// TestGetVolumeParamsNotFound checks that when asking for volume params and the
// volume doesn't exist, the caller gets back an error satisfying a volume not
// found error.
func (s *volumeSuite) TestGetVolumeParamsNotFound(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	volUUID := domaintesting.GenVolumeUUID(c)

	_, err := st.GetVolumeParams(c.Context(), volUUID)
	c.Check(err, tc.ErrorIs, storageprovisioningerrors.VolumeNotFound)
}

func (s *volumeSuite) TestGetVolumeParams(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	poolUUID := s.newStoragePool(c, "mypool", "mypoolprovider", map[string]string{
		"foo": "bar",
	})
	charmUUID := s.newCharm(c)
	s.newCharmStorage(c, charmUUID, "mystorage", "block", false, "")
	suuid := s.newStorageInstanceForCharmWithPool(c, charmUUID, poolUUID, "mystorage")
	volUUID, volID := s.newMachineVolume(c)
	s.newStorageInstanceVolume(c, suuid, volUUID)

	params, err := st.GetVolumeParams(c.Context(), volUUID)

	c.Check(err, tc.ErrorIsNil)
	c.Check(params, tc.DeepEquals, domainstorageprovisioning.VolumeParams{
		Attributes: map[string]string{
			"foo": "bar",
		},
		ID:       volID,
		Provider: "mypoolprovider",
		SizeMiB:  100,
	})
}

// TestGetVolumeAttachmentParamsNotFound ensures a volume attachment not found
// error is returned when the volume attachment does not exist.
func (s *volumeSuite) TestGetVolumeAttachmentParamsNotFound(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	vaUUID := domaintesting.GenVolumeAttachmentUUID(c)

	_, err := st.GetVolumeAttachmentParams(c.Context(), vaUUID)
	c.Check(err, tc.ErrorIs, storageprovisioningerrors.VolumeAttachmentNotFound)
}

func (s *volumeSuite) TestGetVolumeAttachmentParams(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	netNodeUUID := s.newNetNode(c)
	machineUUID, _ := s.newMachineWithNetNode(c, netNodeUUID)
	s.newMachineCloudInstanceWithID(c, machineUUID, "machine-id-123")
	poolUUID := s.newStoragePool(c, "thebigpool", "canonical", map[string]string{
		"foo": "bar",
	})
	charmUUID := s.newCharm(c)
	s.newCharmStorage(c, charmUUID, "mystorage", "block", true, "")
	suuid := s.newStorageInstanceForCharmWithPool(c, charmUUID, poolUUID, "mystorage")
	volUUID, _ := s.newMachineVolume(c)
	s.setVolumeProviderID(c, volUUID, "provider-id")
	vaUUID := s.newMachineVolumeAttachment(c, volUUID, netNodeUUID)
	s.newStorageInstanceVolume(c, suuid, volUUID)

	params, err := st.GetVolumeAttachmentParams(c.Context(), vaUUID)

	c.Check(err, tc.ErrorIsNil)
	c.Check(params, tc.DeepEquals, domainstorageprovisioning.VolumeAttachmentParams{
		MachineInstanceID: "machine-id-123",
		Provider:          "canonical",
		ProviderID:        "provider-id",
		ReadOnly:          true,
	})
}

func (s *volumeSuite) TestGetVolume(c *tc.C) {
	volUUID, volID := s.newModelVolume(c)
	s.changeVolumeInfo(c, volUUID, "vol-123", 1234, "hwid", "wwn", true)
	st := NewState(s.TxnRunnerFactory())

	vol, err := st.GetVolume(c.Context(), volUUID)
	c.Check(err, tc.ErrorIsNil)
	c.Check(vol, tc.DeepEquals, domainstorageprovisioning.Volume{
		VolumeID:   volID,
		ProviderID: "vol-123",
		SizeMiB:    1234,
		HardwareID: "hwid",
		WWN:        "wwn",
		Persistent: true,
	})
}

func (s *volumeSuite) TestGetVolumeNotFound(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

	uuid := domaintesting.GenVolumeUUID(c)

	_, err := st.GetVolume(c.Context(), uuid)
	c.Check(err, tc.ErrorIs, storageprovisioningerrors.VolumeNotFound)
}

func (s *volumeSuite) TestSetVolumeProvisionedInfo(c *tc.C) {
	volUUID, volID := s.newMachineVolume(c)

	st := NewState(s.TxnRunnerFactory())

	info := domainstorageprovisioning.VolumeProvisionedInfo{
		ProviderID: "vol-123",
		SizeMiB:    1234,
		HardwareID: "hwid",
		WWN:        "wwn",
		Persistent: true,
	}
	err := st.SetVolumeProvisionedInfo(c.Context(), volUUID, info)
	c.Assert(err, tc.ErrorIsNil)

	vol, err := st.GetVolume(c.Context(), volUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(vol, tc.DeepEquals, domainstorageprovisioning.Volume{
		VolumeID:   volID,
		ProviderID: "vol-123",
		SizeMiB:    1234,
		HardwareID: "hwid",
		WWN:        "wwn",
		Persistent: true,
	})
}

func (s *volumeSuite) TestSetVolumeProvisionedInfoNotFound(c *tc.C) {
	volUUID := domaintesting.GenVolumeUUID(c)

	st := NewState(s.TxnRunnerFactory())

	info := domainstorageprovisioning.VolumeProvisionedInfo{}
	err := st.SetVolumeProvisionedInfo(c.Context(), volUUID, info)
	c.Assert(err, tc.ErrorIs, storageprovisioningerrors.VolumeNotFound)
}

func (s *volumeSuite) TestSetVolumeAttachmentProvisionedInfo(c *tc.C) {
	nnUUID := s.newNetNode(c)
	machineUUID, _ := s.newMachineWithNetNode(c, nnUUID)
	bdUUID := s.newBlockDevice(c, machineUUID,
		"sda", "", "busaddr", []string{"/dev/disk/by-id/mysda"})
	volUUID, volID := s.newMachineVolume(c)
	vaUUID := s.newModelVolumeAttachment(c, volUUID, nnUUID)

	st := NewState(s.TxnRunnerFactory())

	blockDeviceUUID := blockdevice.BlockDeviceUUID(bdUUID)
	info := domainstorageprovisioning.VolumeAttachmentProvisionedInfo{
		ReadOnly:        true,
		BlockDeviceUUID: &blockDeviceUUID,
	}
	err := st.SetVolumeAttachmentProvisionedInfo(c.Context(), vaUUID, info)
	c.Assert(err, tc.ErrorIsNil)

	vol, err := st.GetVolumeAttachment(c.Context(), vaUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(vol, tc.DeepEquals, domainstorageprovisioning.VolumeAttachment{
		VolumeID:              volID,
		ReadOnly:              true,
		BlockDeviceName:       "sda",
		BlockDeviceLinks:      []string{"/dev/disk/by-id/mysda"},
		BlockDeviceBusAddress: "busaddr",
	})
}

func (s *volumeSuite) TestSetVolumeAttachmentProvisionedInfoNoBlockDevice(c *tc.C) {
	nnUUID := s.newNetNode(c)
	volUUID, volID := s.newMachineVolume(c)
	vaUUID := s.newModelVolumeAttachment(c, volUUID, nnUUID)

	st := NewState(s.TxnRunnerFactory())

	info := domainstorageprovisioning.VolumeAttachmentProvisionedInfo{
		ReadOnly: true,
	}
	err := st.SetVolumeAttachmentProvisionedInfo(c.Context(), vaUUID, info)
	c.Assert(err, tc.ErrorIsNil)

	vol, err := st.GetVolumeAttachment(c.Context(), vaUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(vol, tc.DeepEquals, domainstorageprovisioning.VolumeAttachment{
		VolumeID: volID,
		ReadOnly: true,
	})
}

func (s *volumeSuite) TestSetVolumeAttachmentProvisionedInfoBlockDeviceNotFound(c *tc.C) {
	nnUUID := s.newNetNode(c)
	volUUID, _ := s.newMachineVolume(c)
	vaUUID := s.newModelVolumeAttachment(c, volUUID, nnUUID)

	st := NewState(s.TxnRunnerFactory())

	blockDeviceUUID := tc.Must(c, blockdevice.NewBlockDeviceUUID)
	info := domainstorageprovisioning.VolumeAttachmentProvisionedInfo{
		ReadOnly:        true,
		BlockDeviceUUID: &blockDeviceUUID,
	}
	err := st.SetVolumeAttachmentProvisionedInfo(c.Context(), vaUUID, info)
	c.Assert(err, tc.ErrorIs, storageprovisioningerrors.BlockDeviceNotFound)
}

func (s *volumeSuite) TestSetVolumeAttachmentProvisionedInfoNotFound(c *tc.C) {
	vaUUID := domaintesting.GenVolumeAttachmentUUID(c)

	st := NewState(s.TxnRunnerFactory())

	info := domainstorageprovisioning.VolumeAttachmentProvisionedInfo{}
	err := st.SetVolumeAttachmentProvisionedInfo(c.Context(), vaUUID, info)
	c.Assert(err, tc.ErrorIs, storageprovisioningerrors.VolumeAttachmentNotFound)
}

// changeVolumeLife is a utility function for updating the life value of a
// volume.
func (s *volumeSuite) changeVolumeLife(
	c *tc.C, uuid domainstorageprovisioning.VolumeUUID, life domainlife.Life,
) {
	_, err := s.DB().Exec(`
UPDATE storage_volume
SET    life_id = ?
WHERE  uuid = ?
`,
		int(life), uuid.String())
	c.Assert(err, tc.ErrorIsNil)
}

// changeVolumeAttachmentLife is a utility function for updating the life
// value of a volume attachment.
func (s *volumeSuite) changeVolumeAttachmentLife(
	c *tc.C, uuid domainstorageprovisioning.VolumeAttachmentUUID, life domainlife.Life,
) {
	_, err := s.DB().Exec(`
UPDATE storage_volume_attachment
SET    life_id = ?
WHERE  uuid = ?
`,
		int(life), uuid.String())
	c.Assert(err, tc.ErrorIsNil)
}

// changeVolumeAttachmentPlanLife is a utility function for updating the life
// value of a volume attachment plan.
func (s *volumeSuite) changeVolumeAttachmentPlanLife(
	c *tc.C, uuid string, life domainlife.Life,
) {
	_, err := s.DB().Exec(`
UPDATE storage_volume_attachment_plan
SET    life_id = ?
WHERE  uuid = ?
`,
		int(life), uuid)
	c.Assert(err, tc.ErrorIsNil)

}

func (s *volumeSuite) setVolumeProviderID(
	c *tc.C,
	volUUID domainstorageprovisioning.VolumeUUID,
	providerID string,
) {
	_, err := s.DB().Exec(`
UPDATE storage_volume
SET    provider_id = ?
WHERE  uuid = ?
`,
		providerID, volUUID.String(),
	)
	c.Assert(err, tc.ErrorIsNil)
}
