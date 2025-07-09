// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"fmt"
	"testing"

	"github.com/juju/clock"
	"github.com/juju/tc"

	"github.com/juju/juju/core/instance"
	coremodel "github.com/juju/juju/core/model"
	modeltesting "github.com/juju/juju/core/model/testing"
	usertesting "github.com/juju/juju/core/user/testing"
	jujuversion "github.com/juju/juju/core/version"
	"github.com/juju/juju/domain/deployment"
	domainmachine "github.com/juju/juju/domain/machine"
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

func TestMigrationSuite(t *testing.T) {
	tc.Run(t, &migrationSuite{})
}

func (s *migrationSuite) SetUpTest(c *tc.C) {
	s.ModelSuite.SetUpTest(c)
	s.controllerUUID = uuid.MustNewUUID()

	runner := s.TxnRunnerFactory()
	state := modelstate.NewModelState(runner, loggertesting.WrapCheckLog(c))

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
}

// TestGetControllerUUID is asserting the happy path of getting the controller
// uuid from the database.
func (s *migrationSuite) TestGetControllerUUID(c *tc.C) {
	controllerId, err := New(s.TxnRunnerFactory()).GetControllerUUID(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(controllerId, tc.Equals, s.controllerUUID.String())
}

// TestGetAllInstanceIDs is asserting the happy path of getting all instance
// IDs for the model.
func (s *migrationSuite) TestGetAllInstanceIDs(c *tc.C) {
	// Add two different instances.
	db := s.DB()
	machineState := machinestate.NewState(s.TxnRunnerFactory(), clock.WallClock, loggertesting.WrapCheckLog(c))

	_, machineNames0, err := machineState.AddMachine(c.Context(), domainmachine.AddMachineArgs{
		Platform: deployment.Platform{
			Channel: "24.04",
			OSType:  deployment.Ubuntu,
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	machineUUID0, err := machineState.GetMachineUUID(c.Context(), machineNames0[0])
	c.Assert(err, tc.ErrorIsNil)

	// Add a reference AZ.
	_, err = db.ExecContext(c.Context(), fmt.Sprintf("INSERT INTO availability_zone VALUES(%q, 'az-1')", machineUUID0.String()))
	c.Assert(err, tc.ErrorIsNil)
	arch := "arm64"
	err = machineState.SetMachineCloudInstance(
		c.Context(),
		machineUUID0.String(),
		instance.Id("instance-0"),
		"",
		"nonce",
		&instance.HardwareCharacteristics{
			Arch: &arch,
		},
	)
	c.Assert(err, tc.ErrorIsNil)

	_, machineNames1, err := machineState.AddMachine(c.Context(), domainmachine.AddMachineArgs{
		Platform: deployment.Platform{
			Channel: "24.04",
			OSType:  deployment.Ubuntu,
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	machineUUID1, err := machineState.GetMachineUUID(c.Context(), machineNames1[0])
	c.Assert(err, tc.ErrorIsNil)

	err = machineState.SetMachineCloudInstance(
		c.Context(),
		machineUUID1.String(),
		instance.Id("instance-1"),
		"",
		"nonce",
		&instance.HardwareCharacteristics{
			Arch: &arch,
		},
	)
	c.Assert(err, tc.ErrorIsNil)

	instanceIDs, err := New(s.TxnRunnerFactory()).GetAllInstanceIDs(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(instanceIDs, tc.HasLen, 2)
	c.Check(instanceIDs.Values(), tc.SameContents, []string{"instance-0", "instance-1"})
}

// TestEmptyInstanceIDs tests that no error is returned when there are no
// instances in the model.
func (s *migrationSuite) TestEmptyInstanceIDs(c *tc.C) {
	instanceIDs, err := New(s.TxnRunnerFactory()).GetAllInstanceIDs(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Check(instanceIDs, tc.HasLen, 0)
}
