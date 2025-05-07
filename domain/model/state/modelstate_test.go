// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"
	"database/sql"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/credential"
	"github.com/juju/juju/core/instance"
	coremodel "github.com/juju/juju/core/model"
	modeltesting "github.com/juju/juju/core/model/testing"
	"github.com/juju/juju/core/network"
	"github.com/juju/juju/core/user"
	usertesting "github.com/juju/juju/core/user/testing"
	jujuversion "github.com/juju/juju/core/version"
	"github.com/juju/juju/domain/constraints"
	machineerrors "github.com/juju/juju/domain/machine/errors"
	"github.com/juju/juju/domain/model"
	modelerrors "github.com/juju/juju/domain/model/errors"
	"github.com/juju/juju/domain/modelagent"
	networkerrors "github.com/juju/juju/domain/network/errors"
	schematesting "github.com/juju/juju/domain/schema/testing"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/uuid"
)

type modelSuite struct {
	schematesting.ModelSuite

	controllerUUID uuid.UUID
}

var _ = gc.Suite(&modelSuite{})

func (s *modelSuite) SetUpTest(c *gc.C) {
	s.ModelSuite.SetUpTest(c)
	s.controllerUUID = uuid.MustNewUUID()
}

func (s *modelSuite) createTestModel(c *gc.C) coremodel.UUID {
	runner := s.TxnRunnerFactory()
	state := NewModelState(runner, loggertesting.WrapCheckLog(c))

	id := modeltesting.GenModelUUID(c)
	args := model.ModelDetailArgs{
		UUID:            id,
		AgentStream:     modelagent.AgentStreamReleased,
		AgentVersion:    jujuversion.Current,
		ControllerUUID:  s.controllerUUID,
		Name:            "my-awesome-model",
		Type:            coremodel.IAAS,
		Cloud:           "aws",
		CloudType:       "ec2",
		CloudRegion:     "myregion",
		CredentialOwner: usertesting.GenNewName(c, "myowner"),
		CredentialName:  "mycredential",
	}
	err := state.Create(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	return id
}

func (s *modelSuite) TestCreateAndReadModel(c *gc.C) {
	runner := s.TxnRunnerFactory()
	state := NewModelState(runner, loggertesting.WrapCheckLog(c))

	id := modeltesting.GenModelUUID(c)
	args := model.ModelDetailArgs{
		UUID:            id,
		AgentStream:     modelagent.AgentStreamReleased,
		AgentVersion:    jujuversion.Current,
		ControllerUUID:  s.controllerUUID,
		Name:            "my-awesome-model",
		Type:            coremodel.IAAS,
		Cloud:           "aws",
		CloudType:       "ec2",
		CloudRegion:     "myregion",
		CredentialOwner: usertesting.GenNewName(c, "myowner"),
		CredentialName:  "mycredential",
	}
	err := state.Create(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)

	// Check that it was written correctly.
	model, err := state.GetModel(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(model, jc.DeepEquals, coremodel.ModelInfo{
		UUID:            id,
		AgentVersion:    jujuversion.Current,
		ControllerUUID:  s.controllerUUID,
		Name:            "my-awesome-model",
		Type:            coremodel.IAAS,
		Cloud:           "aws",
		CloudType:       "ec2",
		CloudRegion:     "myregion",
		CredentialOwner: usertesting.GenNewName(c, "myowner"),
		CredentialName:  "mycredential",
	})
}

func (s *modelSuite) TestDeleteModel(c *gc.C) {
	runner := s.TxnRunnerFactory()
	state := NewModelState(runner, loggertesting.WrapCheckLog(c))

	id := modeltesting.GenModelUUID(c)
	args := model.ModelDetailArgs{
		UUID:            id,
		AgentStream:     modelagent.AgentStreamReleased,
		AgentVersion:    jujuversion.Current,
		ControllerUUID:  s.controllerUUID,
		Name:            "my-awesome-model",
		Type:            coremodel.IAAS,
		Cloud:           "aws",
		CloudType:       "ec2",
		CloudRegion:     "myregion",
		CredentialOwner: usertesting.GenNewName(c, "myowner"),
		CredentialName:  "mycredential",
	}
	err := state.Create(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)

	err = state.Delete(context.Background(), id)
	c.Assert(err, jc.ErrorIsNil)

	err = state.Delete(context.Background(), id)
	c.Assert(err, jc.ErrorIs, modelerrors.NotFound)

	// Check that it was written correctly.
	_, err = state.GetModel(context.Background())
	c.Assert(err, jc.ErrorIs, modelerrors.NotFound)
}

func (s *modelSuite) TestCreateModelMultipleTimesWithSameUUID(c *gc.C) {
	runner := s.TxnRunnerFactory()
	state := NewModelState(runner, loggertesting.WrapCheckLog(c))

	// Ensure that we can't create the same model twice.

	id := modeltesting.GenModelUUID(c)
	args := model.ModelDetailArgs{
		UUID:           id,
		AgentStream:    modelagent.AgentStreamReleased,
		AgentVersion:   jujuversion.Current,
		ControllerUUID: s.controllerUUID,
		Name:           "my-awesome-model",
		Type:           coremodel.IAAS,
		Cloud:          "aws",
		CloudType:      "ec2",
		CloudRegion:    "myregion",
	}
	err := state.Create(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
	err = state.Create(context.Background(), args)
	c.Assert(err, jc.ErrorIs, modelerrors.AlreadyExists)
}

func (s *modelSuite) TestCreateModelMultipleTimesWithDifferentUUID(c *gc.C) {
	runner := s.TxnRunnerFactory()
	state := NewModelState(runner, loggertesting.WrapCheckLog(c))

	// Ensure that you can only ever insert one model.

	err := state.Create(context.Background(), model.ModelDetailArgs{
		UUID:         modeltesting.GenModelUUID(c),
		AgentStream:  modelagent.AgentStreamReleased,
		AgentVersion: jujuversion.Current,
		Name:         "my-awesome-model",
		Type:         coremodel.IAAS,
		Cloud:        "aws",
		CloudType:    "ec2",
		CloudRegion:  "myregion",
	})
	c.Assert(err, jc.ErrorIsNil)

	err = state.Create(context.Background(), model.ModelDetailArgs{
		UUID:         modeltesting.GenModelUUID(c),
		AgentStream:  modelagent.AgentStreamReleased,
		AgentVersion: jujuversion.Current,
		Name:         "my-awesome-model",
		Type:         coremodel.IAAS,
		Cloud:        "aws",
		CloudType:    "ec2",
		CloudRegion:  "myregion",
	})
	c.Assert(err, jc.ErrorIs, modelerrors.AlreadyExists)
}

func (s *modelSuite) TestCreateModelAndUpdate(c *gc.C) {
	runner := s.TxnRunnerFactory()
	state := NewModelState(runner, loggertesting.WrapCheckLog(c))

	// Ensure that you can't update it.

	id := modeltesting.GenModelUUID(c)
	err := state.Create(context.Background(), model.ModelDetailArgs{
		UUID:           id,
		AgentStream:    modelagent.AgentStreamReleased,
		AgentVersion:   jujuversion.Current,
		ControllerUUID: s.controllerUUID,
		Name:           "my-awesome-model",
		Type:           coremodel.IAAS,
		Cloud:          "aws",
		CloudType:      "ec2",
		CloudRegion:    "myregion",
	})
	c.Assert(err, jc.ErrorIsNil)

	db := s.DB()
	_, err = db.ExecContext(context.Background(), "UPDATE model SET name = 'new-name' WHERE uuid = $1", id)
	c.Assert(err, gc.ErrorMatches, `model table is immutable, only insertions are allowed`)
}

// CreateModelWithEmptyCloudRegion is a regression test to make sure that we set
// cloud region to null in the database when the supplied value is not set.
// Cloud region should be a null field in the database when it is not set for
// the value. Due to the way with which this value is retrieved from the
// controller database [ModelState.Create] was getting fed with the zero value
// string for cloud region and this is what we ended up being set. In this case
// we want to set the column to NULL so that is has correct meaning in the DDL.
func (s *modelSuite) CreateModelWithEmptyCloudRegion(c *gc.C) {
	runner := s.TxnRunnerFactory()
	state := NewModelState(runner, loggertesting.WrapCheckLog(c))

	id := modeltesting.GenModelUUID(c)
	err := state.Create(context.Background(), model.ModelDetailArgs{
		UUID:         id,
		AgentStream:  modelagent.AgentStreamReleased,
		AgentVersion: jujuversion.Current,
		Name:         "my-awesome-model",
		Type:         coremodel.IAAS,
		Cloud:        "aws",
		CloudType:    "ec2",
	})
	c.Assert(err, jc.ErrorIsNil)

	var cloudRegionVal sql.NullString
	err = s.DB().QueryRowContext(
		context.Background(),
		`SELECT cloud_region FROM model`,
	).Scan(&cloudRegionVal)
	c.Check(err, jc.ErrorIsNil)
	c.Check(cloudRegionVal.Valid, jc.IsFalse)
}

func (s *modelSuite) TestCreateModelAndDelete(c *gc.C) {
	runner := s.TxnRunnerFactory()
	state := NewModelState(runner, loggertesting.WrapCheckLog(c))

	// Ensure that you can't update it.

	id := modeltesting.GenModelUUID(c)
	err := state.Create(context.Background(), model.ModelDetailArgs{
		UUID:         id,
		AgentStream:  modelagent.AgentStreamReleased,
		AgentVersion: jujuversion.Current,
		Name:         "my-awesome-model",
		Type:         coremodel.IAAS,
		Cloud:        "aws",
		CloudType:    "ec2",
		CloudRegion:  "myregion",
	})
	c.Assert(err, jc.ErrorIsNil)

	db := s.DB()
	_, err = db.ExecContext(context.Background(), "DELETE FROM model WHERE uuid = $1", id)
	c.Assert(err, gc.ErrorMatches, `model table is immutable, only insertions are allowed`)
}

func (s *modelSuite) TestModelNotFound(c *gc.C) {
	runner := s.TxnRunnerFactory()
	state := NewModelState(runner, loggertesting.WrapCheckLog(c))

	_, err := state.GetModel(context.Background())
	c.Assert(err, jc.ErrorIs, modelerrors.NotFound)
}

func (s *modelSuite) TestGetModelMetrics(c *gc.C) {
	id := s.createTestModel(c)

	runner := s.TxnRunnerFactory()
	state := NewModelState(runner, loggertesting.WrapCheckLog(c))

	_, err := s.DB().ExecContext(context.Background(), `
		INSERT INTO charm (uuid, reference_name) VALUES ('456', 'foo');
	`)
	c.Assert(err, jc.ErrorIsNil)

	_, err = s.DB().ExecContext(context.Background(), `
		INSERT INTO application (uuid, name, life_id, charm_uuid, space_uuid) VALUES ('123', 'foo', 0, '456', ?);
		`, network.AlphaSpaceId)
	c.Assert(err, jc.ErrorIsNil)

	// Check that it was written correctly.
	model, err := state.GetModelMetrics(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(model, jc.DeepEquals, coremodel.ModelMetrics{
		Model: coremodel.ModelInfo{
			UUID:            id,
			AgentVersion:    jujuversion.Current,
			ControllerUUID:  s.controllerUUID,
			Name:            "my-awesome-model",
			Type:            coremodel.IAAS,
			Cloud:           "aws",
			CloudType:       "ec2",
			CloudRegion:     "myregion",
			CredentialOwner: usertesting.GenNewName(c, "myowner"),
			CredentialName:  "mycredential",
		},
		ApplicationCount: 1,
		MachineCount:     0,
		UnitCount:        0,
	})
}

func (s *modelSuite) TestGetModelMetricsNotFound(c *gc.C) {
	runner := s.TxnRunnerFactory()
	state := NewModelState(runner, loggertesting.WrapCheckLog(c))

	_, err := state.GetModelMetrics(context.Background())
	c.Assert(err, jc.ErrorIs, modelerrors.NotFound)
}

// TestSetModelConstraints is asserting the happy path of setting constraints on
// the model and having those values come back out as we expect from the state
// layer.
func (s *modelSuite) TestSetModelConstraints(c *gc.C) {
	s.createTestModel(c)

	runner := s.TxnRunnerFactory()
	state := NewModelState(runner, loggertesting.WrapCheckLog(c))

	_, err := s.DB().ExecContext(context.Background(), `
INSERT INTO space (uuid, name) VALUES
	(?, ?),
	(?, ?)`,
		uuid.MustNewUUID().String(), "space1",
		uuid.MustNewUUID().String(), "space2",
	)
	c.Assert(err, jc.ErrorIsNil)

	cons := constraints.Constraints{
		Arch:           ptr("amd64"),
		Container:      ptr(instance.LXD),
		CpuCores:       ptr(uint64(4)),
		Mem:            ptr(uint64(1024)),
		RootDisk:       ptr(uint64(1024)),
		RootDiskSource: ptr("root-disk-source"),
		Tags:           ptr([]string{"tag1", "tag2"}),
		InstanceRole:   ptr("instance-role"),
		InstanceType:   ptr("instance-type"),
		Spaces: ptr([]constraints.SpaceConstraint{
			{SpaceName: "space1", Exclude: false},
		}),
		VirtType:         ptr("virt-type"),
		Zones:            ptr([]string{"zone1", "zone2"}),
		AllocatePublicIP: ptr(true),
		ImageID:          ptr("image-id"),
	}

	err = state.SetModelConstraints(context.Background(), cons)
	c.Assert(err, jc.ErrorIsNil)

	getCons, err := state.GetModelConstraints(context.Background())
	c.Check(err, jc.ErrorIsNil)
	c.Check(getCons, jc.DeepEquals, cons)
}

// TestSetModelConstraintsNullBools is a regression test for constraints to
// specifically assert that allocate public ip address can be null, false and
// true according to what the user wants.
//
// DQlite has a bug where null bool columns are reported back in select
// statements as false even thought the value in the database is NULL. To get
// around this bug we have updated the constraint table to strict and changed
// the type on "allocate_public_ip" to an integer.
func (s *modelSuite) TestSetModelConstraintsNullBools(c *gc.C) {
	s.createTestModel(c)

	runner := s.TxnRunnerFactory()
	state := NewModelState(runner, loggertesting.WrapCheckLog(c))

	// Nil Bool
	cons := constraints.Constraints{
		AllocatePublicIP: nil,
	}

	err := state.SetModelConstraints(context.Background(), cons)
	c.Assert(err, jc.ErrorIsNil)

	getCons, err := state.GetModelConstraints(context.Background())
	c.Check(err, jc.ErrorIsNil)
	c.Check(getCons.AllocatePublicIP, gc.IsNil)

	// False Bool
	cons.AllocatePublicIP = ptr(false)
	err = state.SetModelConstraints(context.Background(), cons)
	c.Assert(err, jc.ErrorIsNil)

	getCons, err = state.GetModelConstraints(context.Background())
	c.Check(err, jc.ErrorIsNil)
	c.Check(*getCons.AllocatePublicIP, jc.IsFalse)

	// True Bool
	cons.AllocatePublicIP = ptr(true)
	err = state.SetModelConstraints(context.Background(), cons)
	c.Assert(err, jc.ErrorIsNil)

	getCons, err = state.GetModelConstraints(context.Background())
	c.Check(err, jc.ErrorIsNil)
	c.Check(*getCons.AllocatePublicIP, jc.IsTrue)
}

// TestSetModelConstraintsOverwrites tests that after having set model
// constraints another subsequent call overwrites what has previously been set.
func (s *modelSuite) TestSetModelConstraintsOverwrites(c *gc.C) {
	s.createTestModel(c)

	runner := s.TxnRunnerFactory()
	state := NewModelState(runner, loggertesting.WrapCheckLog(c))

	_, err := s.DB().ExecContext(context.Background(), `
INSERT INTO space (uuid, name) VALUES
	(?, ?),
	(?, ?)`,
		uuid.MustNewUUID().String(), "space1",
		uuid.MustNewUUID().String(), "space2",
	)
	c.Assert(err, jc.ErrorIsNil)

	cons := constraints.Constraints{
		Arch:           ptr("amd64"),
		Container:      ptr(instance.LXD),
		CpuCores:       ptr(uint64(4)),
		Mem:            ptr(uint64(1024)),
		RootDisk:       ptr(uint64(1024)),
		RootDiskSource: ptr("root-disk-source"),
		Tags:           ptr([]string{"tag1", "tag2"}),
		InstanceRole:   ptr("instance-role"),
		InstanceType:   ptr("instance-type"),
		Spaces: ptr([]constraints.SpaceConstraint{
			{SpaceName: "space1", Exclude: false},
		}),
		VirtType:         ptr("virt-type"),
		Zones:            ptr([]string{"zone1", "zone2"}),
		AllocatePublicIP: ptr(true),
		ImageID:          ptr("image-id"),
	}

	err = state.SetModelConstraints(context.Background(), cons)
	c.Assert(err, jc.ErrorIsNil)

	getCons, err := state.GetModelConstraints(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(getCons, jc.DeepEquals, cons)

	// This is the update that should overwrite anything previously set.
	// We explicitly only setting zone as one of the external tables to
	// constraints. This helps validates the internal implementation that
	// previously set tags and spaces are removed correctly.
	cons = constraints.Constraints{
		Arch:    ptr("amd64"),
		Zones:   ptr([]string{"zone2"}),
		ImageID: ptr("image-id"),
		Spaces: ptr([]constraints.SpaceConstraint{
			{SpaceName: "space1", Exclude: true},
		}),
	}

	err = state.SetModelConstraints(context.Background(), cons)
	c.Assert(err, jc.ErrorIsNil)

	getCons, err = state.GetModelConstraints(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(getCons, jc.DeepEquals, cons)
}

// TestSetModelConstraintsFailedModelNotFound is asserting that if we set model
// constraints and the model does not exist we get back an error satisfying
// [modelerrors.NotFound].
func (s *modelSuite) TestSetModelConstraintFailedModelNotFound(c *gc.C) {
	runner := s.TxnRunnerFactory()
	state := NewModelState(runner, loggertesting.WrapCheckLog(c))

	err := state.SetModelConstraints(context.Background(), constraints.Constraints{
		Arch:      ptr("amd64"),
		Container: ptr(instance.NONE),
	})
	c.Assert(err, jc.ErrorIs, modelerrors.NotFound)
}

// TestSetModelConstraintsInvalidContainerType asserts that if we set model
// constraints with an unknown/invalid container type we get back an error
// satisfying [machineerrors.InvalidContainerType] and no changes are made to
// the database.
func (s *modelSuite) TestSetModelConstraintsInvalidContainerType(c *gc.C) {
	s.createTestModel(c)

	runner := s.TxnRunnerFactory()
	state := NewModelState(runner, loggertesting.WrapCheckLog(c))

	cons := constraints.Constraints{
		Container: ptr(instance.ContainerType("noexist")),
		ImageID:   ptr("image-id"),
	}

	err := state.SetModelConstraints(context.Background(), cons)
	c.Check(err, jc.ErrorIs, machineerrors.InvalidContainerType)

	_, err = state.GetModelConstraints(context.Background())
	c.Check(err, jc.ErrorIs, modelerrors.ConstraintsNotFound)
}

// TestSetModelConstraintFailedSpaceDoesNotExist asserts that if we set model
// constraints for a space that doesn't exist we get back an error satisfying
// [networkerrors.SpaceNotFound] and that no changes are made to the database.
func (s *modelSuite) TestSetModelConstraintFailedSpaceDoesNotExist(c *gc.C) {
	s.createTestModel(c)

	runner := s.TxnRunnerFactory()
	state := NewModelState(runner, loggertesting.WrapCheckLog(c))

	err := state.SetModelConstraints(context.Background(), constraints.Constraints{
		Spaces: ptr([]constraints.SpaceConstraint{
			{SpaceName: "space1", Exclude: false},
		}),
		ImageID: ptr("image-id"),
	})
	c.Check(err, jc.ErrorIs, networkerrors.SpaceNotFound)

	_, err = state.GetModelConstraints(context.Background())
	c.Check(err, jc.ErrorIs, modelerrors.ConstraintsNotFound)
}

// TestGetModelConstraintsNotFound asserts that if we ask for model constraints
// and they have not previously been set an error satisfying
// [modelerrors.ConstraintsNotFound].
func (s *modelSuite) TestGetModelConstraintsNotFound(c *gc.C) {
	s.createTestModel(c)

	runner := s.TxnRunnerFactory()
	state := NewModelState(runner, loggertesting.WrapCheckLog(c))

	_, err := state.GetModelConstraints(context.Background())
	c.Check(err, jc.ErrorIs, modelerrors.ConstraintsNotFound)
}

// TestGetModelConstraintsModelNotFound asserts that if we ask for model
// constraints for a model that doesn't exist we get back an error satisfying
// [modelerrors.NotFound].
func (s *modelSuite) TestGetModelConstraintsModelNotFound(c *gc.C) {
	runner := s.TxnRunnerFactory()
	state := NewModelState(runner, loggertesting.WrapCheckLog(c))

	_, err := state.GetModelConstraints(context.Background())
	c.Check(err, jc.ErrorIs, modelerrors.NotFound)
}

func (s *modelSuite) TestGetModelCloudType(c *gc.C) {
	runner := s.TxnRunnerFactory()
	state := NewModelState(runner, loggertesting.WrapCheckLog(c))

	id := modeltesting.GenModelUUID(c)
	cloudType := "ec2"
	args := model.ModelDetailArgs{
		UUID:            id,
		AgentStream:     modelagent.AgentStreamReleased,
		AgentVersion:    jujuversion.Current,
		ControllerUUID:  s.controllerUUID,
		Name:            "mymodel",
		Type:            coremodel.IAAS,
		Cloud:           "aws",
		CloudType:       cloudType,
		CloudRegion:     "myregion",
		CredentialOwner: usertesting.GenNewName(c, "myowner"),
		CredentialName:  "mycredential",
	}
	err := state.Create(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)

	modelCloudType, err := state.GetModelCloudType(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(modelCloudType, jc.DeepEquals, cloudType)
}

func (s *modelSuite) TestGetModelCloudTypeNotFound(c *gc.C) {
	runner := s.TxnRunnerFactory()
	state := NewModelState(runner, loggertesting.WrapCheckLog(c))

	_, err := state.GetModelCloudType(context.Background())
	c.Assert(err, jc.ErrorIs, modelerrors.NotFound)
}

func (s *modelSuite) TestGetModelCloudRegionAndCredential(c *gc.C) {
	runner := s.TxnRunnerFactory()
	state := NewModelState(runner, loggertesting.WrapCheckLog(c))

	uuid := modeltesting.GenModelUUID(c)
	cloudType := "ec2"
	args := model.ModelDetailArgs{
		UUID:            uuid,
		AgentStream:     modelagent.AgentStreamReleased,
		AgentVersion:    jujuversion.Current,
		ControllerUUID:  s.controllerUUID,
		Name:            "mymodel",
		Type:            coremodel.IAAS,
		Cloud:           "aws",
		CloudType:       cloudType,
		CloudRegion:     "myregion",
		CredentialOwner: usertesting.GenNewName(c, "myowner"),
		CredentialName:  "mycredential",
	}
	err := state.Create(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)

	owner, err := user.NewName("myowner")
	c.Assert(err, jc.ErrorIsNil)
	cloud, region, key, err := state.GetModelCloudRegionAndCredential(context.Background(), uuid)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(cloud, gc.Equals, "aws")
	c.Check(region, gc.Equals, "myregion")
	c.Check(key, jc.DeepEquals, credential.Key{
		Name:  "mycredential",
		Cloud: "aws",
		Owner: owner,
	})
}

func (s *modelSuite) TestGetModelCloudRegionAndCredentialNotFound(c *gc.C) {
	runner := s.TxnRunnerFactory()
	state := NewModelState(runner, loggertesting.WrapCheckLog(c))

	uuid := modeltesting.GenModelUUID(c)
	_, _, _, err := state.GetModelCloudRegionAndCredential(context.Background(), uuid)
	c.Assert(err, jc.ErrorIs, modelerrors.NotFound)
}

func (s *modelSuite) TestIsControllerModelTrue(c *gc.C) {
	runner := s.TxnRunnerFactory()
	state := NewModelState(runner, loggertesting.WrapCheckLog(c))

	uuid := modeltesting.GenModelUUID(c)
	cloudType := "ec2"
	args := model.ModelDetailArgs{
		UUID:              uuid,
		AgentStream:       modelagent.AgentStreamReleased,
		AgentVersion:      jujuversion.Current,
		ControllerUUID:    s.controllerUUID,
		Name:              "mycontrollermodel",
		Type:              coremodel.IAAS,
		Cloud:             "aws",
		CloudType:         cloudType,
		CloudRegion:       "myregion",
		CredentialOwner:   usertesting.GenNewName(c, "myowner"),
		CredentialName:    "mycredential",
		IsControllerModel: true,
	}
	err := state.Create(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)

	isControllerModel, err := state.IsControllerModel(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(isControllerModel, jc.IsTrue)
}

func (s *modelSuite) TestIsControllerModelFalse(c *gc.C) {
	runner := s.TxnRunnerFactory()
	state := NewModelState(runner, loggertesting.WrapCheckLog(c))

	uuid := modeltesting.GenModelUUID(c)
	cloudType := "ec2"
	args := model.ModelDetailArgs{
		UUID:              uuid,
		AgentStream:       modelagent.AgentStreamReleased,
		AgentVersion:      jujuversion.Current,
		ControllerUUID:    s.controllerUUID,
		Name:              "mycontrollermodel",
		Type:              coremodel.IAAS,
		Cloud:             "aws",
		CloudType:         cloudType,
		CloudRegion:       "myregion",
		CredentialOwner:   usertesting.GenNewName(c, "myowner"),
		CredentialName:    "mycredential",
		IsControllerModel: false,
	}
	err := state.Create(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)

	isControllerModel, err := state.IsControllerModel(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(isControllerModel, jc.IsFalse)
}

func (s *modelSuite) TestIsControllerModelNotFound(c *gc.C) {
	runner := s.TxnRunnerFactory()
	state := NewModelState(runner, loggertesting.WrapCheckLog(c))

	_, err := state.IsControllerModel(context.Background())
	c.Assert(err, jc.ErrorIs, modelerrors.NotFound)
}

// TestGetControllerUUIDNotFound tests that if we ask for the controller uuid
// in the model database and no model record has been established an error
// satisfying [modelerrors.NotFound] is returned.
func (s *modelSuite) TestGetControllerUUIDNotFound(c *gc.C) {
	runner := s.TxnRunnerFactory()
	state := NewModelState(runner, loggertesting.WrapCheckLog(c))

	_, err := state.GetControllerUUID(context.Background())
	c.Assert(err, jc.ErrorIs, modelerrors.NotFound)
}

// TestGetControllerUUID tests that if we ask for the controller uuid in the
// model database and a model record has been established we get back the
// correct controller uuid.
func (s *modelSuite) TestGetControllerUUID(c *gc.C) {
	runner := s.TxnRunnerFactory()
	state := NewModelState(runner, loggertesting.WrapCheckLog(c))

	uuid := modeltesting.GenModelUUID(c)
	cloudType := "ec2"
	args := model.ModelDetailArgs{
		UUID:              uuid,
		AgentStream:       modelagent.AgentStreamReleased,
		AgentVersion:      jujuversion.Current,
		ControllerUUID:    s.controllerUUID,
		Name:              "mycontrollermodel",
		Type:              coremodel.CAAS,
		Cloud:             "aws",
		CloudType:         cloudType,
		CloudRegion:       "myregion",
		CredentialOwner:   usertesting.GenNewName(c, "myowner"),
		CredentialName:    "mycredential",
		IsControllerModel: false,
	}
	err := state.Create(context.Background(), args)
	c.Check(err, jc.ErrorIsNil)

	controllerUUID, err := state.GetControllerUUID(context.Background())
	c.Check(err, jc.ErrorIsNil)
	c.Check(controllerUUID, gc.Equals, s.controllerUUID)
}

// TestGetModelType is testing the happy path of getting the model type for the
// current model.
func (s *modelSuite) TestGetModelType(c *gc.C) {
	runner := s.TxnRunnerFactory()
	state := NewModelState(runner, loggertesting.WrapCheckLog(c))

	uuid := modeltesting.GenModelUUID(c)
	cloudType := "ec2"
	args := model.ModelDetailArgs{
		UUID:              uuid,
		AgentStream:       modelagent.AgentStreamReleased,
		AgentVersion:      jujuversion.Current,
		ControllerUUID:    s.controllerUUID,
		Name:              "mycontrollermodel",
		Type:              coremodel.CAAS,
		Cloud:             "aws",
		CloudType:         cloudType,
		CloudRegion:       "myregion",
		CredentialOwner:   usertesting.GenNewName(c, "myowner"),
		CredentialName:    "mycredential",
		IsControllerModel: false,
	}
	err := state.Create(context.Background(), args)
	c.Check(err, jc.ErrorIsNil)

	modelType, err := state.GetModelType(context.Background())
	c.Check(err, jc.ErrorIsNil)
	c.Check(modelType, gc.Equals, coremodel.CAAS)
}

// TestGetModelTypeNotFound is testing the error path of getting the model type
// when no model record has been created. This is expected to provide an error
// that satisfies [modelerrors.NotFound].
func (s *modelSuite) TestGetModelTypeNotFound(c *gc.C) {
	runner := s.TxnRunnerFactory()
	state := NewModelState(runner, loggertesting.WrapCheckLog(c))

	_, err := state.GetModelType(context.Background())
	c.Check(err, jc.ErrorIs, modelerrors.NotFound)
}
