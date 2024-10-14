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
	modelstatetesting "github.com/juju/juju/domain/model/state/testing"
	schematesting "github.com/juju/juju/domain/schema/testing"
)

type controllerSuite struct {
	schematesting.ModelSuite

	controllerUUID controller.UUID
	modelUUID      coremodel.UUID
}

var _ = gc.Suite(&controllerSuite{})

func (s *controllerSuite) SetUpTest(c *gc.C) {
	s.ModelSuite.SetUpTest(c)
	s.controllerUUID = controllertesting.GenControllerUUID(c)

	runner := s.TxnRunnerFactory()
	s.modelUUID = modelstatetesting.CreateTestModel(c, runner, "test")
}

func (s *controllerSuite) TestModelAvailable(c *gc.C) {
	state := NewControllerState(s.TxnRunnerFactory())

	available, err := state.ModelAvailable(context.Background(), s.modelUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(available, jc.IsTrue)
}
