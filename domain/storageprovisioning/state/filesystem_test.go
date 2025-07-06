// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/domain/life"
	domainlife "github.com/juju/juju/domain/life"
	domainnetwork "github.com/juju/juju/domain/network"
	"github.com/juju/juju/domain/storageprovisioning"
	"github.com/juju/juju/internal/uuid"
)

// filesystemSuite provides a set of tests for asserting the state interface
// for filesystems in the model.
type filesystemSuite struct {
	baseSuite
}

// TestFilesystemSuite runs the tests in [filesystemSuite].
func TestFilesystemSuite(t *testing.T) {
	tc.Run(t, &filesystemSuite{})
}

// TestGetFilesystemAttachmentIDsOnlyUnits tests that when requesting ids for a
// filesystem attachment and no machines are using the net node the unit name is
// reported.
func (s *filesystemSuite) TestGetFilesystemAttachmentIDsOnlyUnits(c *tc.C) {
	netNodeUUID := s.newNetNode(c)
	appUUID := s.newApplication(c, "foo")
	_, unitName := s.newUnitWithNetNode(c, "foo/0", appUUID, netNodeUUID)

	fsUUID, fsID := s.newMachineFilesystem(c)
	fsaUUID := s.newMachineFilesystemAttachment(c, fsUUID, netNodeUUID)

	st := NewState(s.TxnRunnerFactory())
	result, err := st.GetFilesystemAttachmentIDs(c.Context(), []string{fsaUUID})
	c.Check(err, tc.ErrorIsNil)
	c.Check(result, tc.DeepEquals, map[string]storageprovisioning.FilesystemAttachmentID{
		fsaUUID: {
			FilesystemID: fsID,
			MachineName:  nil,
			UnitName:     &unitName,
		},
	})
}

// TestGetFilesystemAttachmentIDsOnlyMachines tests that when requesting ids for a
// filesystem attachment and the net node is attached to a machine the machine
// name is set.
func (s *filesystemSuite) TestGetFilesystemAttachmentIDsOnlyMachines(c *tc.C) {
	netNodeUUID := s.newNetNode(c)
	_, machineName := s.newMachineWithNetNode(c, netNodeUUID)

	fsUUID, fsID := s.newMachineFilesystem(c)
	fsaUUID := s.newMachineFilesystemAttachment(c, fsUUID, netNodeUUID)

	st := NewState(s.TxnRunnerFactory())
	result, err := st.GetFilesystemAttachmentIDs(c.Context(), []string{fsaUUID})
	c.Check(err, tc.ErrorIsNil)
	c.Check(result, tc.DeepEquals, map[string]storageprovisioning.FilesystemAttachmentID{
		fsaUUID: {
			FilesystemID: fsID,
			MachineName:  &machineName,
			UnitName:     nil,
		},
	})
}

// TestGetFilesystemAttachmentIDsMachineNotUnit tests that when requesting ids for a
// filesystem attachment and the net node is attached to a machine the machine
// name is set. This should remain true when the net node is also used by a
// unit. This is a valid case when units are assigned to a machine.
func (s *filesystemSuite) TestGetFilesystemAttachmentIDsMachineNotUnit(c *tc.C) {
	netNodeUUID := s.newNetNode(c)
	_, machineName := s.newMachineWithNetNode(c, netNodeUUID)
	appUUID := s.newApplication(c, "foo")
	s.newUnitWithNetNode(c, "foo/0", appUUID, netNodeUUID)

	fsUUID, fsID := s.newMachineFilesystem(c)
	fsaUUID := s.newMachineFilesystemAttachment(c, fsUUID, netNodeUUID)

	st := NewState(s.TxnRunnerFactory())
	result, err := st.GetFilesystemAttachmentIDs(c.Context(), []string{fsaUUID})
	c.Check(err, tc.ErrorIsNil)
	c.Check(result, tc.DeepEquals, map[string]storageprovisioning.FilesystemAttachmentID{
		fsaUUID: {
			FilesystemID: fsID,
			MachineName:  &machineName,
			UnitName:     nil,
		},
	})
}

// TestGetFilesystemAttachmentIDsMixed tests that when requesting ids for a
// mixed set of filesystem attachments uuids the machine name and unit name are
// correctly set.
func (s *filesystemSuite) TestGetFilesystemAttachmentIDsMixed(c *tc.C) {
	netNodeUUID1 := s.newNetNode(c)
	netNodeUUID2 := s.newNetNode(c)
	_, machineName := s.newMachineWithNetNode(c, netNodeUUID1)
	appUUID := s.newApplication(c, "foo")
	_, unitName := s.newUnitWithNetNode(c, "foo/0", appUUID, netNodeUUID2)

	fs1UUID, fsID1 := s.newMachineFilesystem(c)
	fsa1UUID := s.newMachineFilesystemAttachment(c, fs1UUID, netNodeUUID1)

	fs2UUID, fsID2 := s.newMachineFilesystem(c)
	fsa2UUID := s.newMachineFilesystemAttachment(c, fs2UUID, netNodeUUID2)

	st := NewState(s.TxnRunnerFactory())
	result, err := st.GetFilesystemAttachmentIDs(c.Context(), []string{
		fsa1UUID, fsa2UUID,
	})
	c.Check(err, tc.ErrorIsNil)
	c.Check(result, tc.DeepEquals, map[string]storageprovisioning.FilesystemAttachmentID{
		fsa1UUID: {
			FilesystemID: fsID1,
			MachineName:  &machineName,
			UnitName:     nil,
		},
		fsa2UUID: {
			FilesystemID: fsID2,
			MachineName:  nil,
			UnitName:     &unitName,
		},
	})
}

// TestGetFilesystemAttachmentIDsNotMachineOrUnit tests that when requesting
// ids for a filesystem attachment that is using a net node not attached to a
// machine or unit the uuid is dropped from the final result.
func (s *filesystemSuite) TestGetFilesystemAttachmentIDsNotMachineOrUnit(c *tc.C) {
	netNodeUUID := s.newNetNode(c)
	fsUUID, _ := s.newMachineFilesystem(c)
	fsaUUID := s.newMachineFilesystemAttachment(c, fsUUID, netNodeUUID)

	st := NewState(s.TxnRunnerFactory())
	result, err := st.GetFilesystemAttachmentIDs(c.Context(), []string{fsaUUID})
	c.Check(err, tc.ErrorIsNil)
	c.Check(result, tc.HasLen, 0)
}

// TestGetFilesystemAttachmentIDsNotFound tests that when requesting ids for
// filesystem attachment uuids that don't exist the uuids are excluded from the
// result with no error returned.
func (s *filesystemSuite) TestGetFilesystemAttachmentIDsNotFound(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	result, err := st.GetFilesystemAttachmentIDs(c.Context(), []string{"no-exist"})
	c.Check(err, tc.ErrorIsNil)
	c.Check(result, tc.HasLen, 0)
}

// TestGetFilesystemAttachmentLifeForNetNode tests that the correct life is
// reported for each model provisioned filesystem attachment associated with the
// given net node.
//
// We also inject a life change during the test to make sure that it is
// reflected.
func (s *filesystemSuite) TestGetFilesystemAttachmentLifeForNetNode(c *tc.C) {
	netNodeUUID := s.newNetNode(c)
	fsUUID1, _ := s.newMachineFilesystem(c)
	fsUUID2, _ := s.newMachineFilesystem(c)
	fsUUID3, _ := s.newMachineFilesystem(c)
	fsaUUID1 := s.newMachineFilesystemAttachment(c, fsUUID1, netNodeUUID)
	fsaUUID2 := s.newMachineFilesystemAttachment(c, fsUUID2, netNodeUUID)
	fsaUUID3 := s.newMachineFilesystemAttachment(c, fsUUID3, netNodeUUID)

	st := NewState(s.TxnRunnerFactory())
	lives, err := st.GetFilesystemAttachmentLifeForNetNode(
		c.Context(), domainnetwork.NetNodeUUID(netNodeUUID),
	)
	c.Check(err, tc.ErrorIsNil)
	c.Check(lives, tc.DeepEquals, map[string]domainlife.Life{
		fsaUUID1: domainlife.Alive,
		fsaUUID2: domainlife.Alive,
		fsaUUID3: domainlife.Alive,
	})

	// Apply a life change to one of the attachments and check the change comes
	// out.
	s.changeFilesystemAttachmentLife(c, fsaUUID1, life.Dying)
	lives, err = st.GetFilesystemAttachmentLifeForNetNode(
		c.Context(), domainnetwork.NetNodeUUID(netNodeUUID),
	)
	c.Check(err, tc.ErrorIsNil)
	c.Check(lives, tc.DeepEquals, map[string]domainlife.Life{
		fsaUUID1: domainlife.Dying,
		fsaUUID2: domainlife.Alive,
		fsaUUID3: domainlife.Alive,
	})
}

// TestGetFilesystemAttachmentLifeNoResults tests that when no attachment lives
// exist for a net node an empty result is returned with no error.
func (s *filesystemSuite) TestGetFilesystemAttachmentLifeNoResults(c *tc.C) {
	netNodeUUID := s.newNetNode(c)
	st := NewState(s.TxnRunnerFactory())
	lives, err := st.GetFilesystemAttachmentLifeForNetNode(
		c.Context(), domainnetwork.NetNodeUUID(netNodeUUID),
	)
	c.Check(err, tc.ErrorIsNil)
	c.Check(lives, tc.HasLen, 0)
}

// TestGetFilesystemLifeForNetNode tests if we can get the filesystem life for
// filesystems attached to a specified machine's net node.
func (s *filesystemSuite) TestGetFilesystemLifeForNetNode(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

	netNodeUUID := s.newNetNode(c)
	fsOneUUID, fsOneID := s.newMachineFilesystem(c)
	_ = s.newMachineFilesystemAttachment(c, fsOneUUID, netNodeUUID)
	fsTwoUUID, fsTwoID := s.newMachineFilesystem(c)
	_ = s.newMachineFilesystemAttachment(c, fsTwoUUID, netNodeUUID)
	fsThreeUUID, fsThreeID := s.newMachineFilesystem(c)
	_ = s.newMachineFilesystemAttachment(c, fsThreeUUID, netNodeUUID)

	s.changeFilesystemLife(c, fsTwoUUID, domainlife.Dying)
	s.changeFilesystemLife(c, fsThreeUUID, domainlife.Dead)

	// Add unrelated filesystems.
	_, _ = s.newModelFilesystem(c)
	fsIDOtherMachine, _ := s.newMachineFilesystem(c)
	_ = s.newMachineFilesystemAttachment(c, fsIDOtherMachine, s.newNetNode(c))

	fsUUIDs, err := st.GetFilesystemLifeForNetNode(
		c.Context(), domainnetwork.NetNodeUUID(netNodeUUID))
	c.Assert(err, tc.ErrorIsNil)
	c.Check(fsUUIDs, tc.DeepEquals, map[string]domainlife.Life{
		fsOneID:   domainlife.Alive,
		fsTwoID:   domainlife.Dying,
		fsThreeID: domainlife.Dead,
	})
}

// TestInitialWatchStatementMachineProvisionedFilesystems tests the initial query
// for machine provisioned filesystems watcher returns only the filesystem UUIDs
// attached to the specified machine net node.
func (s *filesystemSuite) TestInitialWatchStatementMachineProvisionedFilesystems(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

	netNodeUUID := s.newNetNode(c)
	fsOneUUID, fsOneID := s.newMachineFilesystem(c)
	_ = s.newMachineFilesystemAttachment(c, fsOneUUID, netNodeUUID)
	fsTwoUUID, fsTwoID := s.newMachineFilesystem(c)
	_ = s.newMachineFilesystemAttachment(c, fsTwoUUID, netNodeUUID)

	// Add unrelated filesystems.
	_, _ = s.newModelFilesystem(c)
	fsIDOtherMachine, _ := s.newMachineFilesystem(c)
	_ = s.newMachineFilesystemAttachment(c, fsIDOtherMachine, s.newNetNode(c))

	ns, initialQuery := st.InitialWatchStatementMachineProvisionedFilesystems(
		domainnetwork.NetNodeUUID(netNodeUUID),
	)
	c.Check(ns, tc.Equals, "storage_filesystem_life_machine_provisioning")

	db := s.TxnRunner()
	fsUUIDs, err := initialQuery(c.Context(), db)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(fsUUIDs, tc.DeepEquals, map[string]domainlife.Life{
		fsOneID: domainlife.Alive,
		fsTwoID: domainlife.Alive,
	})
}

// TestInitialWatchStatementMachineProvisionedFilesystemsNone tests the initial
// query for machine provisioned filesystems watcher does not return an error
// when no machine provisioned filesystems are attached to the specified machine
// net node.
func (s *filesystemSuite) TestInitialWatchStatementMachineProvisionedFilesystemsNone(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

	netNodeUUID := s.newNetNode(c)

	// Add unrelated filesystems.
	_, _ = s.newModelFilesystem(c)
	fsIDOtherMachine, _ := s.newMachineFilesystem(c)
	s.newMachineFilesystemAttachment(c, fsIDOtherMachine, s.newNetNode(c))

	ns, initialQuery := st.InitialWatchStatementMachineProvisionedFilesystems(
		domainnetwork.NetNodeUUID(netNodeUUID),
	)
	c.Check(ns, tc.Equals, "storage_filesystem_life_machine_provisioning")

	db := s.TxnRunner()
	fsUUIDs, err := initialQuery(c.Context(), db)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(fsUUIDs, tc.HasLen, 0)
}

// TestInitialWatchStatementMachineProvisionedFilesystemsNetNodeMissing tests
// the initial query for machine provisioned filesystems watcher errors when the
// net node specified is not found.
func (s *filesystemSuite) TestInitialWatchStatementMachineProvisionedFilesystemsNetNodeMissing(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

	netNodeUUID := uuid.MustNewUUID().String()

	ns, initialQuery := st.InitialWatchStatementMachineProvisionedFilesystems(
		domainnetwork.NetNodeUUID(netNodeUUID),
	)
	c.Check(ns, tc.Equals, "storage_filesystem_life_machine_provisioning")

	db := s.TxnRunner()
	_, err := initialQuery(c.Context(), db)
	c.Assert(err, tc.NotNil)
}

// TestInitialWatchStatementModelProvisionedFilesystemsNone tests the initial
// query for a model provisioned filsystem watcher returns no error when there
// is not any model provisioned filesystems.
func (s *filesystemSuite) TestInitialWatchStatementModelProvisionedFilesystemsNone(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	_, _ = s.newMachineFilesystem(c)

	ns, initialQuery := st.InitialWatchStatementModelProvisionedFilesystems()
	c.Check(ns, tc.Equals, "storage_filesystem_life_model_provisioning")

	db := s.TxnRunner()
	fsIDs, err := initialQuery(c.Context(), db)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(fsIDs, tc.HasLen, 0)
}

// TestInitialWatchStatementModelProvisionedFilesystems tests the initial query
// for a model provisioned filsystem watcher returns only the filesystem IDs for
// the model provisoned filesystems.
func (s *filesystemSuite) TestInitialWatchStatementModelProvisionedFilesystems(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	_, fsOneID := s.newModelFilesystem(c)
	_, fsTwoID := s.newModelFilesystem(c)
	_, _ = s.newMachineFilesystem(c)

	ns, initialQuery := st.InitialWatchStatementModelProvisionedFilesystems()
	c.Check(ns, tc.Equals, "storage_filesystem_life_model_provisioning")

	db := s.TxnRunner()
	fsIDs, err := initialQuery(c.Context(), db)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(fsIDs, tc.SameContents, []string{fsOneID, fsTwoID})
}

// TestInitialWatchStatementMachineProvisionedFilesystemAttachments tests the
// initial query for machine provisioned filesystem attachments watcher returns
// only the filesystem attachment UUIDs attached to the specified net node.
func (s *filesystemSuite) TestInitialWatchStatementMachineProvisionedFilesystemAttachments(c *tc.C) {
	netNodeUUID := s.newNetNode(c)
	fsOneUUID, _ := s.newMachineFilesystem(c)
	fsaOneUUID := s.newMachineFilesystemAttachment(c, fsOneUUID, netNodeUUID)
	fsTwoUUID, _ := s.newMachineFilesystem(c)
	fsaTwoUUID := s.newMachineFilesystemAttachment(c, fsTwoUUID, netNodeUUID)

	// Add unrelated filesystems.
	_, _ = s.newModelFilesystem(c)
	fsIDOtherMachine, _ := s.newMachineFilesystem(c)
	_ = s.newMachineFilesystemAttachment(c, fsIDOtherMachine, s.newNetNode(c))

	st := NewState(s.TxnRunnerFactory())
	ns, initialQuery := st.InitialWatchStatementMachineProvisionedFilesystemAttachments(
		domainnetwork.NetNodeUUID(netNodeUUID),
	)
	c.Check(ns, tc.Equals, "storage_filesystem_attachment_life_machine_provisioning")

	db := s.TxnRunner()
	fsaUUIDs, err := initialQuery(c.Context(), db)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(fsaUUIDs, tc.DeepEquals, map[string]domainlife.Life{
		fsaTwoUUID: domainlife.Alive,
		fsaOneUUID: domainlife.Alive,
	})
}

// TestInitialWatchStatementMachineProvisionedFilesystemAttachmentsNone tests
// the initial query for machine provisioned filesystem attachments watcher does
// not return an error when no machine provisioned filesystem attachments are
// attached to the specified net node.
func (s *filesystemSuite) TestInitialWatchStatementMachineProvisionedFilesystemAttachmentsNone(c *tc.C) {
	netNodeUUID := s.newNetNode(c)

	// Add unrelated filesystems.
	_, _ = s.newModelFilesystem(c)
	fsIDOtherMachine, _ := s.newMachineFilesystem(c)
	s.newMachineFilesystemAttachment(c, fsIDOtherMachine, s.newNetNode(c))

	st := NewState(s.TxnRunnerFactory())
	ns, initialQuery := st.InitialWatchStatementMachineProvisionedFilesystemAttachments(
		domainnetwork.NetNodeUUID(netNodeUUID),
	)
	c.Check(ns, tc.Equals, "storage_filesystem_attachment_life_machine_provisioning")

	db := s.TxnRunner()
	fsaUUIDs, err := initialQuery(c.Context(), db)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(fsaUUIDs, tc.HasLen, 0)
}

// TestInitialWatchStatementMachineProvisionedFilesystemAttachmentsNetNodeMissing
// tests the initial query for machine provisioned filesystem attachmewnts
// watcher errors when the net node specified is not found.
func (s *filesystemSuite) TestInitialWatchStatementMachineProvisionedFilesystemAttachmentsNetNodeMissing(c *tc.C) {
	netNodeUUID, err := domainnetwork.NewNetNodeUUID()
	c.Assert(err, tc.ErrorIsNil)

	st := NewState(s.TxnRunnerFactory())
	ns, initialQuery := st.InitialWatchStatementMachineProvisionedFilesystemAttachments(
		domainnetwork.NetNodeUUID(netNodeUUID),
	)
	c.Check(ns, tc.Equals, "storage_filesystem_attachment_life_machine_provisioning")

	db := s.TxnRunner()
	_, err = initialQuery(c.Context(), db)
	// We don't focus on what the error is as no specific error type is offered
	// as part of the contract. We just care that an error occured.
	c.Assert(err, tc.NotNil)
}

// TestInitialWatchStatementModelProvisionedFilesystemAttachmentsNone tests the
// initial query for a model provisioned filsystem attachment watcher returns no
// error when there is no model provisioned filesystem attachments.
func (s *filesystemSuite) TestInitialWatchStatementModelProvisionedFilesystemAttachmentsNone(c *tc.C) {
	// Create a machine based filesystem attachment to assert  this doesn't show
	// up.
	netNode := s.newNetNode(c)
	fsUUID, _ := s.newMachineFilesystem(c)
	s.newMachineFilesystemAttachment(c, fsUUID, netNode)

	st := NewState(s.TxnRunnerFactory())
	ns, initialQuery := st.InitialWatchStatementModelProvisionedFilesystemAttachments()
	c.Check(ns, tc.Equals, "storage_filesystem_attachment_life_model_provisioning")

	db := s.TxnRunner()
	fsaUUIDs, err := initialQuery(c.Context(), db)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(fsaUUIDs, tc.HasLen, 0)
}

// TestInitialWatchStatementModelProvisionedFilesystemAttachments tests the
// initial query for a model provisioned filsystem attachment watcher returns
// only the filesystem attachment uuids for the model provisoned filesystem
// attachments.
func (s *filesystemSuite) TestInitialWatchStatementModelProvisionedFilesystemAttachments(c *tc.C) {
	netNodeUUID := s.newNetNode(c)
	st := NewState(s.TxnRunnerFactory())
	fsOneUUID, _ := s.newModelFilesystem(c)
	fsTwoUUID, _ := s.newModelFilesystem(c)
	fsThreeUUID, _ := s.newMachineFilesystem(c)
	fsaOneUUID := s.newModelFilesystemAttachment(c, fsOneUUID, netNodeUUID)
	fsaTwoUUID := s.newModelFilesystemAttachment(c, fsTwoUUID, netNodeUUID)
	s.newMachineFilesystemAttachment(c, fsThreeUUID, netNodeUUID)

	ns, initialQuery := st.InitialWatchStatementModelProvisionedFilesystemAttachments()
	c.Check(ns, tc.Equals, "storage_filesystem_attachment_life_model_provisioning")

	db := s.TxnRunner()
	fsaUUIDs, err := initialQuery(c.Context(), db)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(fsaUUIDs, tc.SameContents, []string{fsaOneUUID, fsaTwoUUID})
}

// changeFilesystemLife is a utility function for updating the life value of a
// filesystem.
func (s *filesystemSuite) changeFilesystemLife(
	c *tc.C, uuid string, life domainlife.Life,
) {
	_, err := s.DB().Exec(`
UPDATE storage_filesystem
SET    life_id = ?
WHERE  uuid = ?
`,
		int(life), uuid)
	c.Assert(err, tc.ErrorIsNil)
}

// changeFilesystemAttachmentLife is a utility function for updating the life
// value of a filesystem attachment. This is used to trigger an update trigger
// for a filesystem attachment.
func (s *filesystemSuite) changeFilesystemAttachmentLife(
	c *tc.C, uuid string, life domainlife.Life,
) {
	_, err := s.DB().Exec(`
UPDATE storage_filesystem_attachment
SET    life_id = ?
WHERE  uuid = ?
`,
		int(life), uuid)
	c.Assert(err, tc.ErrorIsNil)
}

// newMachineFilesystem creates a new filesystem in the model with machine
// provision scope. Returned is the uuid and filesystem id of the entity.
func (s *filesystemSuite) newMachineFilesystem(c *tc.C) (string, string) {
	fsUUID, err := uuid.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	fsID := fmt.Sprintf("foo/%s", fsUUID.String())

	_, err = s.DB().Exec(`
INSERT INTO storage_filesystem (uuid, filesystem_id, life_id, provision_scope_id)
VALUES (?, ?, 0, 1)
	`,
		fsUUID.String(), fsID)
	c.Assert(err, tc.ErrorIsNil)

	return fsUUID.String(), fsID
}

// newModelFilesystem creates a new filesystem in the model with model
// provision scope. Return is the uuid and filesystem id of the entity.
func (s *filesystemSuite) newModelFilesystem(c *tc.C) (string, string) {
	fsUUID, err := uuid.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	fsID := fmt.Sprintf("foo/%s", fsUUID.String())

	_, err = s.DB().Exec(`
INSERT INTO storage_filesystem (uuid, filesystem_id, life_id, provision_scope_id)
VALUES (?, ?, 0, 0)
	`,
		fsUUID.String(), fsID)
	c.Assert(err, tc.ErrorIsNil)

	return fsUUID.String(), fsID
}

// newMachineFilesystemAttachment creates a new filesystem attachment that has
// machine provision scope. The attachment is associated with the provided
// filesystem uuid and net node uuid.
func (s *filesystemSuite) newMachineFilesystemAttachment(
	c *tc.C, fsUUID string, netNodeUUID string,
) string {
	attachmentUUID, err := uuid.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.DB().ExecContext(
		c.Context(),
		`
INSERT INTO storage_filesystem_attachment (uuid,
                                           storage_filesystem_uuid,
                                           net_node_uuid,
                                           life_id,
                                           provision_scope_id)
VALUES (?, ?, ?, 0, 1)
`,
		attachmentUUID.String(), fsUUID, netNodeUUID)
	c.Assert(err, tc.ErrorIsNil)

	return attachmentUUID.String()
}

// newModelFilesystemAttachment creates a new filesystem attachment that has
// model provision scope. The attachment is associated with the provided
// filesystem uuid and net node uuid.
func (s *filesystemSuite) newModelFilesystemAttachment(
	c *tc.C, fsUUID string, netNodeUUID string,
) string {
	attachmentUUID, err := uuid.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.DB().ExecContext(
		c.Context(),
		`
INSERT INTO storage_filesystem_attachment (uuid,
                                           storage_filesystem_uuid,
                                           net_node_uuid,
                                           life_id,
                                           provision_scope_id)
VALUES (?, ?, ?, 0, 0)
`,
		attachmentUUID.String(), fsUUID, netNodeUUID)
	c.Assert(err, tc.ErrorIsNil)

	return attachmentUUID.String()
}
