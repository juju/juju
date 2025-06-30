// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"testing"

	"github.com/juju/tc"

	domainlife "github.com/juju/juju/domain/life"
	schematesting "github.com/juju/juju/domain/schema/testing"
	"github.com/juju/juju/internal/uuid"
)

// filesystemSuite provides a test suite for testing the filesystem
// provisioning interface of state.
type filesystemSuite struct {
	schematesting.ModelSuite
}

func TestFilesystemSuite(t *testing.T) {
	tc.Run(t, &filesystemSuite{})
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

	ns, initialQuery := st.InitialWatchStatementMachineProvisionedFilesystems(netNodeUUID)
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

	ns, initialQuery := st.InitialWatchStatementMachineProvisionedFilesystems(netNodeUUID)
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

	ns, initialQuery := st.InitialWatchStatementMachineProvisionedFilesystems(netNodeUUID)
	c.Check(ns, tc.Equals, "storage_filesystem_life_machine_provisioning")

	db := s.TxnRunner()
	_, err := initialQuery(c.Context(), db)
	c.Assert(err, tc.NotNil)
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

	fsUUIDs, err := st.GetFilesystemLifeForNetNode(c.Context(), netNodeUUID)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(fsUUIDs, tc.DeepEquals, map[string]domainlife.Life{
		fsOneID:   domainlife.Alive,
		fsTwoID:   domainlife.Dying,
		fsThreeID: domainlife.Dead,
	})
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

// deleteFilesystem is a utility function for deleting a filesystem in the
// model.
func (s *filesystemSuite) deleteFilesystem(c *tc.C, uuid string) {
	_, err := s.DB().Exec(`
DELETE FROM storage_filesystem
WHERE  uuid = ?
`,
		uuid)
	c.Assert(err, tc.ErrorIsNil)
}

// deleteFilesystemAttachment is a utility function for deleting a filesystem
// attachment in the model.
func (s *filesystemSuite) deleteFilesystemAttachment(c *tc.C, uuid string) {
	_, err := s.DB().Exec(`
DELETE FROM storage_filesystem_attachment
WHERE  uuid = ?
`,
		uuid)
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

// newNetNode creates a new net node in the model for referencing to storage
// entity attachments. The net node is not associated with any machine or units.
func (s *filesystemSuite) newNetNode(c *tc.C) string {
	nodeUUID, err := uuid.NewUUID()
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.DB().ExecContext(
		c.Context(),
		"INSERT INTO net_node VALUES (?)",
		nodeUUID.String(),
	)
	c.Assert(err, tc.ErrorIsNil)

	return nodeUUID.String()
}
