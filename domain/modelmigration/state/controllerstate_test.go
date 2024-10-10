// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/controller"
	controllertesting "github.com/juju/juju/core/controller/testing"
	coremodel "github.com/juju/juju/core/model"
	modeltesting "github.com/juju/juju/core/model/testing"
	usertesting "github.com/juju/juju/core/user/testing"
	jujuversion "github.com/juju/juju/core/version"
	"github.com/juju/juju/domain/model"
	modelstate "github.com/juju/juju/domain/model/state"
	schematesting "github.com/juju/juju/domain/schema/testing"
	loggertesting "github.com/juju/juju/internal/logger/testing"
)

type controllerSuite struct {
	schematesting.ModelSuite

	controllerUUID controller.UUID
}

var _ = gc.Suite(&controllerSuite{})

func (s *controllerSuite) SetUpTest(c *gc.C) {
	s.ModelSuite.SetUpTest(c)
	s.controllerUUID = controllertesting.GenControllerUUID(c)

	runner := s.TxnRunnerFactory()
	state := modelstate.NewModelState(runner, loggertesting.WrapCheckLog(c))

	id := modeltesting.GenModelUUID(c)
	args := model.ReadOnlyModelCreationArgs{
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
	}
	err := state.Create(context.Background(), args)
	c.Assert(err, jc.ErrorIsNil)
}
