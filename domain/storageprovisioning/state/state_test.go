// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"testing"

	"github.com/canonical/sqlair"
	"github.com/juju/tc"

	coreapplication "github.com/juju/juju/core/application"
	applicationtesting "github.com/juju/juju/core/application/testing"
	coremachine "github.com/juju/juju/core/machine"
	machinetesting "github.com/juju/juju/core/machine/testing"
	unittesting "github.com/juju/juju/core/unit/testing"
	applicationerrors "github.com/juju/juju/domain/application/errors"
	domainlife "github.com/juju/juju/domain/life"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	domainnetwork "github.com/juju/juju/domain/network"
	schematesting "github.com/juju/juju/domain/schema/testing"
	storagetesting "github.com/juju/juju/domain/storage/testing"
	"github.com/juju/juju/domain/storageprovisioning"
	storageprovisioningerrors "github.com/juju/juju/domain/storageprovisioning/errors"
	"github.com/juju/juju/internal/uuid"
)

// ddlAssumptionsSuite provides a test suite for testing assumption relied on in
// the DDL of the model. These tests exist to break when relied upon assumptions
// in the DDL change over time. When a test in this suite fails, it means code
// in this domain needs to be updated.
type ddlAssumptionsSuite struct {
	schematesting.ModelSuite
}

// stateSuite provides a test suite for testing the commonality parts of [State].
type stateSuite struct {
	baseSuite
}

// TestDDLAssumptionSuite registers and runs all of the tests located in
// [ddlAssumptionsSuite].
func TestDDLAssumptionsSuite(t *testing.T) {
	tc.Run(t, &ddlAssumptionsSuite{})
}

// TestStateSuite registers and runs all of the tests located in [stateSuite].
func TestStateSuite(t *testing.T) {
	tc.Run(t, &stateSuite{})
}

// TestCheckMachineIsDeadTrue tests that the [State.CheckMachineIsDead] method
// returns true when the machine is dead.
func (s *stateSuite) TestCheckMachineIsDeadTrue(c *tc.C) {
	netNode := s.newNetNode(c)
	machineUUID, _ := s.newMachineWithNetNode(c, netNode)
	s.changeMachineLife(c, machineUUID, domainlife.Dead)

	st := NewState(s.TxnRunnerFactory())
	isDead, err := st.CheckMachineIsDead(
		c.Context(), coremachine.UUID(machineUUID),
	)
	c.Check(err, tc.ErrorIsNil)
	c.Check(isDead, tc.IsTrue)
}

// TestCheckMachineIsDeadTrue tests that the [State.CheckMachineIsDead] method
// returns false when the machine is not dead.
func (s *stateSuite) TestCheckMachineIsDeadFalse(c *tc.C) {
	netNode := s.newNetNode(c)
	machineUUID, _ := s.newMachineWithNetNode(c, netNode)

	st := NewState(s.TxnRunnerFactory())
	isDead, err := st.CheckMachineIsDead(
		c.Context(), coremachine.UUID(machineUUID),
	)
	c.Check(err, tc.ErrorIsNil)
	c.Check(isDead, tc.IsFalse)
}

// TestCheckMachineIsDeadNotFound tests that check if a non-existent machine
// is dead results in a [machineerrors.MachineNotFound] error to the caller.
func (s *stateSuite) TestCheckMachineIsDeadNotFound(c *tc.C) {
	machineUUID := machinetesting.GenUUID(c)
	st := NewState(s.TxnRunnerFactory())
	_, err := st.CheckMachineIsDead(
		c.Context(), machineUUID,
	)
	c.Check(err, tc.ErrorIs, machineerrors.MachineNotFound)
}

// TestCheckNetNodeNotExist tests that the [State.checkNetNodeExists] method
// returns false when the net node does not exist.
func (s *stateSuite) TestCheckNetNodeNotExist(c *tc.C) {
	netNodeUUID, err := domainnetwork.NewNetNodeUUID()
	c.Assert(err, tc.ErrorIsNil)

	st := NewState(s.TxnRunnerFactory())

	var exists bool
	err = s.TxnRunner().Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		exists, err = st.checkNetNodeExists(ctx, tx, netNodeUUID)
		return err
	})
	c.Check(err, tc.ErrorIsNil)
	c.Check(exists, tc.Equals, false)
}

// TestCheckNetNodeExists tests that when a net node exists
// [State.checkNetNodeExists] returns true.
func (s *stateSuite) TestCheckNetNodeExists(c *tc.C) {
	netNodeUUID, err := domainnetwork.NewNetNodeUUID()
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.DB().ExecContext(
		c.Context(),
		"INSERT INTO net_node (uuid) VALUES (?)",
		netNodeUUID.String(),
	)
	c.Assert(err, tc.ErrorIsNil)

	st := NewState(s.TxnRunnerFactory())

	var exists bool
	err = s.TxnRunner().Txn(c.Context(), func(ctx context.Context, tx *sqlair.TX) error {
		var err error
		exists, err = st.checkNetNodeExists(ctx, tx, netNodeUUID)
		return err
	})
	c.Check(err, tc.ErrorIsNil)
	c.Check(exists, tc.Equals, true)
}

// TestGetMachineNetNodeUUID tests that the [State.GetMachineNetNodeUUID]
// returns the correct net node uuid for a machine.
func (s *stateSuite) TestGetMachineNetNodeUUID(c *tc.C) {
	netNodeUUID := s.newNetNode(c)
	machineUUID, _ := s.newMachineWithNetNode(c, netNodeUUID)

	st := NewState(s.TxnRunnerFactory())
	rval, err := st.GetMachineNetNodeUUID(
		c.Context(), coremachine.UUID(machineUUID),
	)
	c.Check(err, tc.ErrorIsNil)
	c.Check(rval, tc.Equals, netNodeUUID)
}

// TestGetMachineNetNodeUUIDNotFound tests that asking for the net node of a
// machine that does not exist returns a [machineerrors.MachineNotFound] error
// to the caller.
func (s *stateSuite) TestGetMachineNetNodeUUIDNotFound(c *tc.C) {
	machineUUID := machinetesting.GenUUID(c)

	st := NewState(s.TxnRunnerFactory())
	_, err := st.GetMachineNetNodeUUID(
		c.Context(), machineUUID,
	)
	c.Check(err, tc.ErrorIs, machineerrors.MachineNotFound)
}

// TestGetUnitNetNodeUUID tests the happy path of [State.GetUnitNetNodeUUID].
func (s *stateSuite) TestGetUnitNetNodeUUID(c *tc.C) {
	netNodeUUID := s.newNetNode(c)
	appUUID, _ := s.newApplication(c, "foo")
	unitUUID, _ := s.newUnitWithNetNode(c, "foo/0", appUUID, netNodeUUID)

	st := NewState(s.TxnRunnerFactory())
	rval, err := st.GetUnitNetNodeUUID(
		c.Context(), unitUUID,
	)
	c.Check(err, tc.ErrorIsNil)
	c.Check(rval, tc.Equals, netNodeUUID)
}

// TestGetUnitNetNodeUUIDNotFound tests that asking for the net node of a unit
// that does not exist returns a [applicationerrors.UnitNotFound] error to the
// caller.
func (s *stateSuite) TestGetUnitNetNodeUUIDNotFound(c *tc.C) {
	unitUUID := unittesting.GenUnitUUID(c)
	st := NewState(s.TxnRunnerFactory())

	_, err := st.GetUnitNetNodeUUID(c.Context(), unitUUID)
	c.Check(err, tc.ErrorIs, applicationerrors.UnitNotFound)
}

// TestGetStorageResourceTagInfoForApplication tests that values expected values
// are obtained from model info, model config and application.
func (s *stateSuite) TestGetStorageResourceTagInfoForApplication(c *tc.C) {
	controllerUUID := uuid.MustNewUUID().String()
	appUUID, _ := s.newApplication(c, "foo")

	_, err := s.DB().ExecContext(c.Context(),
		`INSERT INTO model_config (key, value) VALUES (?, ?)`, "resource_tags", "a=x b=y")
	c.Assert(err, tc.ErrorIsNil)
	_, err = s.DB().ExecContext(c.Context(),
		`INSERT INTO model (uuid, controller_uuid, name, qualifier, type, cloud, cloud_type) VALUES (?, ?, "", "", "", "", "")`,
		s.ModelUUID(), controllerUUID)
	c.Assert(err, tc.ErrorIsNil)

	st := NewState(s.TxnRunnerFactory())
	resourceTags, err := st.GetStorageResourceTagInfoForApplication(
		c.Context(), coreapplication.ID(appUUID), "resource_tags",
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(resourceTags, tc.DeepEquals, storageprovisioning.ApplicationResourceTagInfo{
		ModelResourceTagInfo: storageprovisioning.ModelResourceTagInfo{
			BaseResourceTags: "a=x b=y",
			ModelUUID:        s.ModelUUID(),
			ControllerUUID:   controllerUUID,
		},
		ApplicationName: "foo",
	})
}

func (s *stateSuite) TestGetStorageResourceTagInfoForApplicationNotFound(c *tc.C) {
	appUUID := applicationtesting.GenApplicationUUID(c)
	controllerUUID := uuid.MustNewUUID().String()
	_, err := s.DB().ExecContext(
		c.Context(),
		`
INSERT INTO model (uuid, controller_uuid, name, qualifier, type, cloud, cloud_type)
VALUES (?, ?, "", "", "", "", "")
`,
		s.ModelUUID(),
		controllerUUID,
	)
	c.Assert(err, tc.ErrorIsNil)

	st := NewState(s.TxnRunnerFactory())
	_, err = st.GetStorageResourceTagInfoForApplication(
		c.Context(), appUUID, "resource_tags",
	)
	c.Check(err, tc.ErrorIs, applicationerrors.ApplicationNotFound)
}

func (s *stateSuite) TestGetStorageResourceTagInfoForModel(c *tc.C) {
	controllerUUID := uuid.MustNewUUID().String()

	_, err := s.DB().ExecContext(
		c.Context(),
		"INSERT INTO model_config (key, value) VALUES (?, ?)",
		"resource_tags",
		"a=x b=y",
	)
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.DB().ExecContext(
		c.Context(),
		`
INSERT INTO model (uuid, controller_uuid, name, qualifier, type, cloud, cloud_type)
VALUES (?, ?, "", "", "", "", "")
`,
		s.ModelUUID(),
		controllerUUID,
	)
	c.Assert(err, tc.ErrorIsNil)

	st := NewState(s.TxnRunnerFactory())
	resourceTags, err := st.GetStorageResourceTagInfoForModel(
		c.Context(), "resource_tags",
	)
	c.Check(err, tc.ErrorIsNil)
	c.Check(resourceTags, tc.DeepEquals, storageprovisioning.ModelResourceTagInfo{
		BaseResourceTags: "a=x b=y",
		ModelUUID:        s.ModelUUID(),
		ControllerUUID:   controllerUUID,
	})
}

func (s *stateSuite) TestGetStorageAttachmentIDsForUnit(c *tc.C) {
	netNodeUUID := s.newNetNode(c)
	appUUID, charmUUID := s.newApplication(c, "foo")
	unitUUID, _ := s.newUnitWithNetNode(c, "foo/0", appUUID, netNodeUUID)
	storageInstanceUUID := s.newStorageInstance(c, charmUUID)
	storageID := s.getStorageID(c, storageInstanceUUID)
	s.newStorageAttachment(c, storageInstanceUUID, unitUUID, 0)

	st := NewState(s.TxnRunnerFactory())
	storageIDs, err := st.GetStorageAttachmentIDsForUnit(c.Context(), unitUUID.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(storageIDs, tc.DeepEquals, []string{
		storageID,
	})
}

func (s *stateSuite) TestGetStorageAttachmentIDsForUnitWithUnitNotFound(c *tc.C) {
	unitUUID := unittesting.GenUnitUUID(c)

	st := NewState(s.TxnRunnerFactory())
	_, err := st.GetStorageAttachmentIDsForUnit(c.Context(), unitUUID.String())
	c.Assert(err, tc.ErrorIs, applicationerrors.UnitNotFound)
}

func (s *stateSuite) TestGetStorageInstanceUUIDByID(c *tc.C) {
	_, charmUUID := s.newApplication(c, "foo")
	storageInstanceUUID := s.newStorageInstance(c, charmUUID)
	storageID := s.getStorageID(c, storageInstanceUUID)

	st := NewState(s.TxnRunnerFactory())
	rval, err := st.GetStorageInstanceUUIDByID(
		c.Context(), storageID,
	)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(rval, tc.Equals, storageInstanceUUID.String())
}

func (s *stateSuite) TestGetStorageInstanceUUIDByIDWithStorageInstanceNotFound(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	_, err := st.GetStorageInstanceUUIDByID(c.Context(), "foo/1")
	c.Assert(err, tc.ErrorIs, storageprovisioningerrors.StorageInstanceNotFound)
}

func (s *stateSuite) TestGetAttachmentLife(c *tc.C) {
	netNodeUUID := s.newNetNode(c)
	appUUID, charmUUID := s.newApplication(c, "foo")
	unitUUID, _ := s.newUnitWithNetNode(c, "foo/0", appUUID, netNodeUUID)
	storageInstanceUUID := s.newStorageInstance(c, charmUUID)
	s.newStorageAttachment(c, storageInstanceUUID, unitUUID, 0)

	st := NewState(s.TxnRunnerFactory())

	life, err := st.GetStorageAttachmentLife(c.Context(), unitUUID.String(), storageInstanceUUID.String())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(life, tc.Equals, domainlife.Alive)
}

func (s *stateSuite) TestGetAttachmentLifeWithUnitNotFound(c *tc.C) {
	unitUUID := unittesting.GenUnitUUID(c)
	storageInstanceUUID := storagetesting.GenStorageInstanceUUID(c)

	st := NewState(s.TxnRunnerFactory())
	_, err := st.GetStorageAttachmentLife(c.Context(), unitUUID.String(), storageInstanceUUID.String())
	c.Assert(err, tc.ErrorIs, applicationerrors.UnitNotFound)
}

func (s *stateSuite) TestGetAttachmentLifeWithStorageInstanceNotFound(c *tc.C) {
	netNodeUUID := s.newNetNode(c)
	appUUID, _ := s.newApplication(c, "foo")
	unitUUID, _ := s.newUnitWithNetNode(c, "foo/0", appUUID, netNodeUUID)
	storageInstanceUUID := storagetesting.GenStorageInstanceUUID(c)

	st := NewState(s.TxnRunnerFactory())
	_, err := st.GetStorageAttachmentLife(c.Context(), unitUUID.String(), storageInstanceUUID.String())
	c.Assert(err, tc.ErrorIs, storageprovisioningerrors.StorageInstanceNotFound)
}

func (s *stateSuite) TestGetAttachmentLifeWithStorageAttachmentNotFound(c *tc.C) {
	netNodeUUID := s.newNetNode(c)
	appUUID, charmUUID := s.newApplication(c, "foo")
	unitUUID, _ := s.newUnitWithNetNode(c, "foo/0", appUUID, netNodeUUID)
	storageInstanceUUID := s.newStorageInstance(c, charmUUID)

	st := NewState(s.TxnRunnerFactory())
	_, err := st.GetStorageAttachmentLife(c.Context(), unitUUID.String(), storageInstanceUUID.String())
	c.Assert(err, tc.ErrorIs, storageprovisioningerrors.StorageAttachmentNotFound)
}

// TestMachineProvisionScopeValue tests that the value of machine provision
// scope in the storage_provision_scope table is 1. This is an assumption that
// is made in this state layer.
func (s *ddlAssumptionsSuite) TestMachineProvisionScopeValue(c *tc.C) {
	var idVal int
	err := s.DB().QueryRowContext(
		c.Context(),
		"SELECT id from storage_provision_scope WHERE scope = 'machine'",
	).Scan(&idVal)

	c.Check(err, tc.ErrorIsNil)
	c.Check(idVal, tc.Equals, 1)
}

// TestModelProvisionScopeValue tests that the value of model provision
// scope in the storage_provision_scope table is 0. This is an assumption that
// is made in this state layer.
func (s *ddlAssumptionsSuite) TestModelProvisionScopeValue(c *tc.C) {
	var idVal int
	err := s.DB().QueryRowContext(
		c.Context(),
		"SELECT id from storage_provision_scope WHERE scope = 'model'",
	).Scan(&idVal)

	c.Check(err, tc.ErrorIsNil)
	c.Check(idVal, tc.Equals, 0)
}
