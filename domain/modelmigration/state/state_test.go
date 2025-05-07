// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/juju/clock"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/instance"
	coremodel "github.com/juju/juju/core/model"
	modeltesting "github.com/juju/juju/core/model/testing"
	usertesting "github.com/juju/juju/core/user/testing"
	jujuversion "github.com/juju/juju/core/version"
	machinestate "github.com/juju/juju/domain/machine/state"
	"github.com/juju/juju/domain/model"
	modelstate "github.com/juju/juju/domain/model/state"
	"github.com/juju/juju/domain/modelagent"
	schematesting "github.com/juju/juju/domain/schema/testing"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/uuid"
)

type migrationSuite struct {
	schematesting.ModelSuite

	controllerUUID uuid.UUID
}

var _ = gc.Suite(&migrationSuite{})

func (s *migrationSuite) SetUpTest(c *gc.C) {
	s.ModelSuite.SetUpTest(c)
	s.controllerUUID = uuid.MustNewUUID()

	runner := s.TxnRunnerFactory()
	state := modelstate.NewModelState(runner, loggertesting.WrapCheckLog(c))

	uuid := modeltesting.GenModelUUID(c)
	ownerName := usertesting.GenNewName(c, "tlm")
	ownerUUID := usertesting.GenUserUUID(c)
	args := model.ModelDetailArgs{
		UUID:            uuid,
		AgentStream:     modelagent.AgentStreamReleased,
		AgentVersion:    jujuversion.Current,
		ControllerUUID:  s.controllerUUID,
		Name:            "my-awesome-model",
		OwnerName:       ownerName,
		Owner:           ownerUUID,
		Type:            coremodel.IAAS,
		Cloud:           "aws",
		CloudType:       "ec2",
		CloudRegion:     "myregion",
		CredentialOwner: usertesting.GenNewName(c, "myowner"),
		CredentialName:  "mycredential",
	}
	err := state.Create(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
}

// TestGetControllerUUID is asserting the happy path of getting the controller
// uuid from the database.
func (s *migrationSuite) TestGetControllerUUID(c *gc.C) {
	controllerId, err := New(s.TxnRunnerFactory()).GetControllerUUID(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(controllerId, gc.Equals, s.controllerUUID.String())
}

// TestGetAllInstanceIDs is asserting the happy path of getting all instance
// IDs for the model.
func (s *migrationSuite) TestGetAllInstanceIDs(c *gc.C) {
	// Add two different instances.
	db := s.DB()
	machineState := machinestate.NewState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c))

	err := machineState.CreateMachine(context.Background(), "666", "0", "deadbeef")
	c.Assert(err, jc.ErrorIsNil)
	// Add a reference AZ.
	_, err = db.ExecContext(context.Background(), "INSERT INTO availability_zone VALUES('deadbeef', 'az-1')")
	c.Assert(err, jc.ErrorIsNil)
	arch := "arm64"
	err = machineState.SetMachineCloudInstance(
		context.Background(),
		"deadbeef",
		instance.Id("instance-0"),
		"",
		&instance.HardwareCharacteristics{
			Arch: &arch,
		},
	)
	c.Assert(err, jc.ErrorIsNil)
	err = machineState.CreateMachine(context.Background(), "667", "1", "deadbeef-2")
	c.Assert(err, jc.ErrorIsNil)
	err = machineState.SetMachineCloudInstance(
		context.Background(),
		"deadbeef-2",
		instance.Id("instance-1"),
		"",
		&instance.HardwareCharacteristics{
			Arch: &arch,
		},
	)
	c.Assert(err, jc.ErrorIsNil)

	instanceIDs, err := New(s.TxnRunnerFactory()).GetAllInstanceIDs(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(instanceIDs, gc.HasLen, 2)
	c.Check(instanceIDs.Values(), jc.SameContents, []string{"instance-0", "instance-1"})
}

// TestEmptyInstanceIDs tests that no error is returned when there are no
// instances in the model.
func (s *migrationSuite) TestEmptyInstanceIDs(c *gc.C) {
	instanceIDs, err := New(s.TxnRunnerFactory()).GetAllInstanceIDs(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Check(instanceIDs, gc.HasLen, 0)
}
