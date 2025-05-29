// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"testing"

	"github.com/juju/tc"

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

func TestModelSuite(t *testing.T) {
	tc.Run(t, &modelSuite{})
}

func (s *modelSuite) SetUpTest(c *tc.C) {
	s.ModelSuite.SetUpTest(c)
	s.controllerUUID = uuid.MustNewUUID()
}

func (s *modelSuite) createTestModel(c *tc.C) coremodel.UUID {
	runner := s.TxnRunnerFactory()
	state := NewModelState(runner, loggertesting.WrapCheckLog(c))

	id := modeltesting.GenModelUUID(c)
	args := model.ModelDetailArgs{
		UUID:            id,
		AgentStream:     modelagent.AgentStreamReleased,
		AgentVersion:    jujuversion.Current,
		ControllerUUID:  s.controllerUUID,
		Name:            "my-awesome-model",
		Qualifier:       "prod",
		Type:            coremodel.IAAS,
		Cloud:           "aws",
		CloudType:       "ec2",
		CloudRegion:     "myregion",
		CredentialOwner: usertesting.GenNewName(c, "myowner"),
		CredentialName:  "mycredential",
	}
	err := state.Create(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	return id
}

func (s *modelSuite) TestCreateAndReadModel(c *tc.C) {
	runner := s.TxnRunnerFactory()
	state := NewModelState(runner, loggertesting.WrapCheckLog(c))

	id := modeltesting.GenModelUUID(c)
	args := model.ModelDetailArgs{
		UUID:            id,
		AgentStream:     modelagent.AgentStreamReleased,
		AgentVersion:    jujuversion.Current,
		ControllerUUID:  s.controllerUUID,
		Name:            "my-awesome-model",
		Qualifier:       "prod",
		Type:            coremodel.IAAS,
		Cloud:           "aws",
		CloudType:       "ec2",
		CloudRegion:     "myregion",
		CredentialOwner: usertesting.GenNewName(c, "myowner"),
		CredentialName:  "mycredential",
	}
	err := state.Create(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)

	// Check that it was written correctly.
	model, err := state.GetModel(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(model, tc.DeepEquals, coremodel.ModelInfo{
		UUID:            id,
		AgentVersion:    jujuversion.Current,
		ControllerUUID:  s.controllerUUID,
		Name:            "my-awesome-model",
		Qualifier:       "prod",
		Type:            coremodel.IAAS,
		Cloud:           "aws",
		CloudType:       "ec2",
		CloudRegion:     "myregion",
		CredentialOwner: usertesting.GenNewName(c, "myowner"),
		CredentialName:  "mycredential",
	})
}

func (s *modelSuite) TestDeleteModel(c *tc.C) {
	runner := s.TxnRunnerFactory()
	state := NewModelState(runner, loggertesting.WrapCheckLog(c))

	id := modeltesting.GenModelUUID(c)
	args := model.ModelDetailArgs{
		UUID:            id,
		AgentStream:     modelagent.AgentStreamReleased,
		AgentVersion:    jujuversion.Current,
		ControllerUUID:  s.controllerUUID,
		Name:            "my-awesome-model",
		Qualifier:       "prod",
		Type:            coremodel.IAAS,
		Cloud:           "aws",
		CloudType:       "ec2",
		CloudRegion:     "myregion",
		CredentialOwner: usertesting.GenNewName(c, "myowner"),
		CredentialName:  "mycredential",
	}
	err := state.Create(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)

	err = state.Delete(c.Context(), id)
	c.Assert(err, tc.ErrorIsNil)

	err = state.Delete(c.Context(), id)
	c.Assert(err, tc.ErrorIs, modelerrors.NotFound)

	// Check that it was written correctly.
	_, err = state.GetModel(c.Context())
	c.Assert(err, tc.ErrorIs, modelerrors.NotFound)
}

func (s *modelSuite) TestCreateModelMultipleTimesWithSameUUID(c *tc.C) {
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
		Qualifier:      "prod",
		Type:           coremodel.IAAS,
		Cloud:          "aws",
		CloudType:      "ec2",
		CloudRegion:    "myregion",
	}
	err := state.Create(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)
	err = state.Create(c.Context(), args)
	c.Assert(err, tc.ErrorIs, modelerrors.AlreadyExists)
}

func (s *modelSuite) TestCreateModelMultipleTimesWithDifferentUUID(c *tc.C) {
	runner := s.TxnRunnerFactory()
	state := NewModelState(runner, loggertesting.WrapCheckLog(c))

	// Ensure that you can only ever insert one model.

	err := state.Create(c.Context(), model.ModelDetailArgs{
		UUID:         modeltesting.GenModelUUID(c),
		AgentStream:  modelagent.AgentStreamReleased,
		AgentVersion: jujuversion.Current,
		Name:         "my-awesome-model",
		Qualifier:    "prod",
		Type:         coremodel.IAAS,
		Cloud:        "aws",
		CloudType:    "ec2",
		CloudRegion:  "myregion",
	})
	c.Assert(err, tc.ErrorIsNil)

	err = state.Create(c.Context(), model.ModelDetailArgs{
		UUID:         modeltesting.GenModelUUID(c),
		AgentStream:  modelagent.AgentStreamReleased,
		AgentVersion: jujuversion.Current,
		Name:         "my-awesome-model",
		Qualifier:    "prod",
		Type:         coremodel.IAAS,
		Cloud:        "aws",
		CloudType:    "ec2",
		CloudRegion:  "myregion",
	})
	c.Assert(err, tc.ErrorIs, modelerrors.AlreadyExists)
}

func (s *modelSuite) TestCreateModelAndUpdate(c *tc.C) {
	runner := s.TxnRunnerFactory()
	state := NewModelState(runner, loggertesting.WrapCheckLog(c))

	// Ensure that you can't update it.

	id := modeltesting.GenModelUUID(c)
	err := state.Create(c.Context(), model.ModelDetailArgs{
		UUID:           id,
		AgentStream:    modelagent.AgentStreamReleased,
		AgentVersion:   jujuversion.Current,
		ControllerUUID: s.controllerUUID,
		Name:           "my-awesome-model",
		Qualifier:      "prod",
		Type:           coremodel.IAAS,
		Cloud:          "aws",
		CloudType:      "ec2",
		CloudRegion:    "myregion",
	})
	c.Assert(err, tc.ErrorIsNil)

	db := s.DB()
	_, err = db.ExecContext(c.Context(), "UPDATE model SET name = 'new-name' WHERE uuid = $1", id)
	c.Assert(err, tc.ErrorMatches, `model table is immutable, only insertions are allowed`)
}

func (s *modelSuite) TestCreateModelAndDelete(c *tc.C) {
	runner := s.TxnRunnerFactory()
	state := NewModelState(runner, loggertesting.WrapCheckLog(c))

	// Ensure that you can't update it.

	id := modeltesting.GenModelUUID(c)
	err := state.Create(c.Context(), model.ModelDetailArgs{
		UUID:         id,
		AgentStream:  modelagent.AgentStreamReleased,
		AgentVersion: jujuversion.Current,
		Name:         "my-awesome-model",
		Qualifier:    "prod",
		Type:         coremodel.IAAS,
		Cloud:        "aws",
		CloudType:    "ec2",
		CloudRegion:  "myregion",
	})
	c.Assert(err, tc.ErrorIsNil)

	db := s.DB()
	_, err = db.ExecContext(c.Context(), "DELETE FROM model WHERE uuid = $1", id)
	c.Assert(err, tc.ErrorMatches, `model table is immutable, only insertions are allowed`)
}

func (s *modelSuite) TestModelNotFound(c *tc.C) {
	runner := s.TxnRunnerFactory()
	state := NewModelState(runner, loggertesting.WrapCheckLog(c))

	_, err := state.GetModel(c.Context())
	c.Assert(err, tc.ErrorIs, modelerrors.NotFound)
}

func (s *modelSuite) TestGetModelMetrics(c *tc.C) {
	id := s.createTestModel(c)

	runner := s.TxnRunnerFactory()
	state := NewModelState(runner, loggertesting.WrapCheckLog(c))

	_, err := s.DB().ExecContext(c.Context(), `
		INSERT INTO charm (uuid, reference_name) VALUES ('456', 'foo');
	`)
	c.Assert(err, tc.ErrorIsNil)

	_, err = s.DB().ExecContext(c.Context(), `
		INSERT INTO application (uuid, name, life_id, charm_uuid, space_uuid) VALUES ('123', 'foo', 0, '456', ?);
		`, network.AlphaSpaceId)
	c.Assert(err, tc.ErrorIsNil)

	// Check that it was written correctly.
	model, err := state.GetModelMetrics(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(model, tc.DeepEquals, coremodel.ModelMetrics{
		Model: coremodel.ModelInfo{
			UUID:            id,
			AgentVersion:    jujuversion.Current,
			ControllerUUID:  s.controllerUUID,
			Name:            "my-awesome-model",
			Qualifier:       "prod",
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

func (s *modelSuite) TestGetModelMetricsNotFound(c *tc.C) {
	runner := s.TxnRunnerFactory()
	state := NewModelState(runner, loggertesting.WrapCheckLog(c))

	_, err := state.GetModelMetrics(c.Context())
	c.Assert(err, tc.ErrorIs, modelerrors.NotFound)
}

// TestSetModelConstraints is asserting the happy path of setting constraints on
// the model and having those values come back out as we expect from the state
// layer.
func (s *modelSuite) TestSetModelConstraints(c *tc.C) {
	s.createTestModel(c)

	runner := s.TxnRunnerFactory()
	state := NewModelState(runner, loggertesting.WrapCheckLog(c))

	_, err := s.DB().ExecContext(c.Context(), `
INSERT INTO space (uuid, name) VALUES
	(?, ?),
	(?, ?)`,
		uuid.MustNewUUID().String(), "space1",
		uuid.MustNewUUID().String(), "space2",
	)
	c.Assert(err, tc.ErrorIsNil)

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

	err = state.SetModelConstraints(c.Context(), cons)
	c.Assert(err, tc.ErrorIsNil)

	getCons, err := state.GetModelConstraints(c.Context())
	c.Check(err, tc.ErrorIsNil)
	c.Check(getCons, tc.DeepEquals, cons)
}

// TestSetModelConstraintsNullBools is a regression test for constraints to
// specifically assert that allocate public ip address can be null, false and
// true according to what the user wants.
//
// DQlite has a bug where null bool columns are reported back in select
// statements as false even thought the value in the database is NULL. To get
// around this bug we have updated the constraint table to strict and changed
// the type on "allocate_public_ip" to an integer.
func (s *modelSuite) TestSetModelConstraintsNullBools(c *tc.C) {
	s.createTestModel(c)

	runner := s.TxnRunnerFactory()
	state := NewModelState(runner, loggertesting.WrapCheckLog(c))

	// Nil Bool
	cons := constraints.Constraints{
		AllocatePublicIP: nil,
	}

	err := state.SetModelConstraints(c.Context(), cons)
	c.Assert(err, tc.ErrorIsNil)

	getCons, err := state.GetModelConstraints(c.Context())
	c.Check(err, tc.ErrorIsNil)
	c.Check(getCons.AllocatePublicIP, tc.IsNil)

	// False Bool
	cons.AllocatePublicIP = ptr(false)
	err = state.SetModelConstraints(c.Context(), cons)
	c.Assert(err, tc.ErrorIsNil)

	getCons, err = state.GetModelConstraints(c.Context())
	c.Check(err, tc.ErrorIsNil)
	c.Check(*getCons.AllocatePublicIP, tc.IsFalse)

	// True Bool
	cons.AllocatePublicIP = ptr(true)
	err = state.SetModelConstraints(c.Context(), cons)
	c.Assert(err, tc.ErrorIsNil)

	getCons, err = state.GetModelConstraints(c.Context())
	c.Check(err, tc.ErrorIsNil)
	c.Check(*getCons.AllocatePublicIP, tc.IsTrue)
}

// TestSetModelConstraintsOverwrites tests that after having set model
// constraints another subsequent call overwrites what has previously been set.
func (s *modelSuite) TestSetModelConstraintsOverwrites(c *tc.C) {
	s.createTestModel(c)

	runner := s.TxnRunnerFactory()
	state := NewModelState(runner, loggertesting.WrapCheckLog(c))

	_, err := s.DB().ExecContext(c.Context(), `
INSERT INTO space (uuid, name) VALUES
	(?, ?),
	(?, ?)`,
		uuid.MustNewUUID().String(), "space1",
		uuid.MustNewUUID().String(), "space2",
	)
	c.Assert(err, tc.ErrorIsNil)

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

	err = state.SetModelConstraints(c.Context(), cons)
	c.Assert(err, tc.ErrorIsNil)

	getCons, err := state.GetModelConstraints(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(getCons, tc.DeepEquals, cons)

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

	err = state.SetModelConstraints(c.Context(), cons)
	c.Assert(err, tc.ErrorIsNil)

	getCons, err = state.GetModelConstraints(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(getCons, tc.DeepEquals, cons)
}

// TestSetModelConstraintsFailedModelNotFound is asserting that if we set model
// constraints and the model does not exist we get back an error satisfying
// [modelerrors.NotFound].
func (s *modelSuite) TestSetModelConstraintFailedModelNotFound(c *tc.C) {
	runner := s.TxnRunnerFactory()
	state := NewModelState(runner, loggertesting.WrapCheckLog(c))

	err := state.SetModelConstraints(c.Context(), constraints.Constraints{
		Arch:      ptr("amd64"),
		Container: ptr(instance.NONE),
	})
	c.Assert(err, tc.ErrorIs, modelerrors.NotFound)
}

// TestSetModelConstraintsInvalidContainerType asserts that if we set model
// constraints with an unknown/invalid container type we get back an error
// satisfying [machineerrors.InvalidContainerType] and no changes are made to
// the database.
func (s *modelSuite) TestSetModelConstraintsInvalidContainerType(c *tc.C) {
	s.createTestModel(c)

	runner := s.TxnRunnerFactory()
	state := NewModelState(runner, loggertesting.WrapCheckLog(c))

	cons := constraints.Constraints{
		Container: ptr(instance.ContainerType("noexist")),
		ImageID:   ptr("image-id"),
	}

	err := state.SetModelConstraints(c.Context(), cons)
	c.Check(err, tc.ErrorIs, machineerrors.InvalidContainerType)

	_, err = state.GetModelConstraints(c.Context())
	c.Check(err, tc.ErrorIs, modelerrors.ConstraintsNotFound)
}

// TestSetModelConstraintFailedSpaceDoesNotExist asserts that if we set model
// constraints for a space that doesn't exist we get back an error satisfying
// [networkerrors.SpaceNotFound] and that no changes are made to the database.
func (s *modelSuite) TestSetModelConstraintFailedSpaceDoesNotExist(c *tc.C) {
	s.createTestModel(c)

	runner := s.TxnRunnerFactory()
	state := NewModelState(runner, loggertesting.WrapCheckLog(c))

	err := state.SetModelConstraints(c.Context(), constraints.Constraints{
		Spaces: ptr([]constraints.SpaceConstraint{
			{SpaceName: "space1", Exclude: false},
		}),
		ImageID: ptr("image-id"),
	})
	c.Check(err, tc.ErrorIs, networkerrors.SpaceNotFound)

	_, err = state.GetModelConstraints(c.Context())
	c.Check(err, tc.ErrorIs, modelerrors.ConstraintsNotFound)
}

// TestGetModelConstraintsNotFound asserts that if we ask for model constraints
// and they have not previously been set an error satisfying
// [modelerrors.ConstraintsNotFound].
func (s *modelSuite) TestGetModelConstraintsNotFound(c *tc.C) {
	s.createTestModel(c)

	runner := s.TxnRunnerFactory()
	state := NewModelState(runner, loggertesting.WrapCheckLog(c))

	_, err := state.GetModelConstraints(c.Context())
	c.Check(err, tc.ErrorIs, modelerrors.ConstraintsNotFound)
}

// TestGetModelConstraintsModelNotFound asserts that if we ask for model
// constraints for a model that doesn't exist we get back an error satisfying
// [modelerrors.NotFound].
func (s *modelSuite) TestGetModelConstraintsModelNotFound(c *tc.C) {
	runner := s.TxnRunnerFactory()
	state := NewModelState(runner, loggertesting.WrapCheckLog(c))

	_, err := state.GetModelConstraints(c.Context())
	c.Check(err, tc.ErrorIs, modelerrors.NotFound)
}

func (s *modelSuite) TestGetModelCloudType(c *tc.C) {
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
		Qualifier:       "prod",
		Type:            coremodel.IAAS,
		Cloud:           "aws",
		CloudType:       cloudType,
		CloudRegion:     "myregion",
		CredentialOwner: usertesting.GenNewName(c, "myowner"),
		CredentialName:  "mycredential",
	}
	err := state.Create(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)

	modelCloudType, err := state.GetModelCloudType(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(modelCloudType, tc.DeepEquals, cloudType)
}

func (s *modelSuite) TestGetModelCloudTypeNotFound(c *tc.C) {
	runner := s.TxnRunnerFactory()
	state := NewModelState(runner, loggertesting.WrapCheckLog(c))

	_, err := state.GetModelCloudType(c.Context())
	c.Assert(err, tc.ErrorIs, modelerrors.NotFound)
}

func (s *modelSuite) TestGetModelCloudRegionAndCredential(c *tc.C) {
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
		Qualifier:       "prod",
		Type:            coremodel.IAAS,
		Cloud:           "aws",
		CloudType:       cloudType,
		CloudRegion:     "myregion",
		CredentialOwner: usertesting.GenNewName(c, "myowner"),
		CredentialName:  "mycredential",
	}
	err := state.Create(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)

	owner, err := user.NewName("myowner")
	c.Assert(err, tc.ErrorIsNil)
	cloud, region, key, err := state.GetModelCloudRegionAndCredential(c.Context(), uuid)
	c.Assert(err, tc.ErrorIsNil)
	c.Check(cloud, tc.Equals, "aws")
	c.Check(region, tc.Equals, "myregion")
	c.Check(key, tc.DeepEquals, credential.Key{
		Name:  "mycredential",
		Cloud: "aws",
		Owner: owner,
	})
}

func (s *modelSuite) TestGetModelCloudRegionAndCredentialNotFound(c *tc.C) {
	runner := s.TxnRunnerFactory()
	state := NewModelState(runner, loggertesting.WrapCheckLog(c))

	uuid := modeltesting.GenModelUUID(c)
	_, _, _, err := state.GetModelCloudRegionAndCredential(c.Context(), uuid)
	c.Assert(err, tc.ErrorIs, modelerrors.NotFound)
}

func (s *modelSuite) TestIsControllerModelTrue(c *tc.C) {
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
		Qualifier:         "prod",
		Type:              coremodel.IAAS,
		Cloud:             "aws",
		CloudType:         cloudType,
		CloudRegion:       "myregion",
		CredentialOwner:   usertesting.GenNewName(c, "myowner"),
		CredentialName:    "mycredential",
		IsControllerModel: true,
	}
	err := state.Create(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)

	isControllerModel, err := state.IsControllerModel(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(isControllerModel, tc.IsTrue)
}

func (s *modelSuite) TestIsControllerModelFalse(c *tc.C) {
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
		Qualifier:         "prod",
		Type:              coremodel.IAAS,
		Cloud:             "aws",
		CloudType:         cloudType,
		CloudRegion:       "myregion",
		CredentialOwner:   usertesting.GenNewName(c, "myowner"),
		CredentialName:    "mycredential",
		IsControllerModel: false,
	}
	err := state.Create(c.Context(), args)
	c.Assert(err, tc.ErrorIsNil)

	isControllerModel, err := state.IsControllerModel(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(isControllerModel, tc.IsFalse)
}

func (s *modelSuite) TestIsControllerModelNotFound(c *tc.C) {
	runner := s.TxnRunnerFactory()
	state := NewModelState(runner, loggertesting.WrapCheckLog(c))

	_, err := state.IsControllerModel(c.Context())
	c.Assert(err, tc.ErrorIs, modelerrors.NotFound)
}

// TestGetControllerUUIDNotFound tests that if we ask for the controller uuid
// in the model database and no model record has been established an error
// satisfying [modelerrors.NotFound] is returned.
func (s *modelSuite) TestGetControllerUUIDNotFound(c *tc.C) {
	runner := s.TxnRunnerFactory()
	state := NewModelState(runner, loggertesting.WrapCheckLog(c))

	_, err := state.GetControllerUUID(c.Context())
	c.Assert(err, tc.ErrorIs, modelerrors.NotFound)
}

// TestGetControllerUUID tests that if we ask for the controller uuid in the
// model database and a model record has been established we get back the
// correct controller uuid.
func (s *modelSuite) TestGetControllerUUID(c *tc.C) {
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
		Qualifier:         "prod",
		Type:              coremodel.CAAS,
		Cloud:             "aws",
		CloudType:         cloudType,
		CloudRegion:       "myregion",
		CredentialOwner:   usertesting.GenNewName(c, "myowner"),
		CredentialName:    "mycredential",
		IsControllerModel: false,
	}
	err := state.Create(c.Context(), args)
	c.Check(err, tc.ErrorIsNil)

	controllerUUID, err := state.GetControllerUUID(c.Context())
	c.Check(err, tc.ErrorIsNil)
	c.Check(controllerUUID, tc.Equals, s.controllerUUID)
}

// TestGetModelType is testing the happy path of getting the model type for the
// current model.
func (s *modelSuite) TestGetModelType(c *tc.C) {
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
		Qualifier:         "prod",
		Type:              coremodel.CAAS,
		Cloud:             "aws",
		CloudType:         cloudType,
		CloudRegion:       "myregion",
		CredentialOwner:   usertesting.GenNewName(c, "myowner"),
		CredentialName:    "mycredential",
		IsControllerModel: false,
	}
	err := state.Create(c.Context(), args)
	c.Check(err, tc.ErrorIsNil)

	modelType, err := state.GetModelType(c.Context())
	c.Check(err, tc.ErrorIsNil)
	c.Check(modelType, tc.Equals, coremodel.CAAS)
}

// TestGetModelTypeNotFound is testing the error path of getting the model type
// when no model record has been created. This is expected to provide an error
// that satisfies [modelerrors.NotFound].
func (s *modelSuite) TestGetModelTypeNotFound(c *tc.C) {
	runner := s.TxnRunnerFactory()
	state := NewModelState(runner, loggertesting.WrapCheckLog(c))

	_, err := state.GetModelType(c.Context())
	c.Check(err, tc.ErrorIs, modelerrors.NotFound)
}

// TestGetModelInfoSummary is testing the happy path of getting the model info
// summary for the current model.
func (s *modelSuite) TestGetModelInfoSummary(c *tc.C) {
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
		Qualifier:         "prod",
		Type:              coremodel.CAAS,
		Cloud:             "aws",
		CloudType:         cloudType,
		CloudRegion:       "myregion",
		CredentialOwner:   usertesting.GenNewName(c, "myowner"),
		CredentialName:    "mycredential",
		IsControllerModel: false,
	}
	err := state.Create(c.Context(), args)
	c.Check(err, tc.ErrorIsNil)

	infoSummary, err := state.GetModelInfoSummary(c.Context())
	c.Check(err, tc.ErrorIsNil)
	c.Check(infoSummary, tc.DeepEquals, model.ModelInfoSummary{
		Name:           "mycontrollermodel",
		Qualifier:      "prod",
		UUID:           uuid,
		ModelType:      coremodel.CAAS,
		CloudName:      "aws",
		CloudType:      cloudType,
		CloudRegion:    "myregion",
		ControllerUUID: s.controllerUUID.String(),
		IsController:   false,
		AgentVersion:   jujuversion.Current,
		MachineCount:   0,
		UnitCount:      0,
		CoreCount:      0,
	})
}
