// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	controllertesting "github.com/juju/juju/core/controller/testing"
	controllerstatetesting "github.com/juju/juju/domain/controller/state/testing"
	modelstatetesting "github.com/juju/juju/domain/model/state/testing"
	schematesting "github.com/juju/juju/domain/schema/testing"
)

type controllerSuite struct {
	schematesting.ControllerSuite
}

var _ = gc.Suite(&controllerSuite{})

func (s *controllerSuite) TestModelAvailable(c *gc.C) {
	runner := s.TxnRunnerFactory()
	state := NewControllerState(runner)

	modelUUID := modelstatetesting.CreateTestModel(c, runner, "test")

	available, err := state.ModelAvailable(context.Background(), modelUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(available, jc.IsTrue)
}

func (s *controllerSuite) TestModelMigrationInfoIsControllerModel(c *gc.C) {
	runner := s.TxnRunnerFactory()
	state := NewControllerState(runner)

	controllerUUID := controllertesting.GenControllerUUID(c)

	modelUUID := modelstatetesting.CreateTestModel(c, runner, "test")
	controllerstatetesting.CreateTestController(c, runner, controllerUUID, modelUUID)

	info, err := state.ModelMigrationInfo(context.Background(), modelUUID)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(info.ControllerUUID, gc.Equals, controllerUUID)
	c.Check(info.IsControllerModel, jc.IsTrue)
	c.Check(info.MigrationActive, jc.IsFalse)
}

func (s *controllerSuite) TestModelMigrationInfoIsNotControllerModel(c *gc.C) {
	runner := s.TxnRunnerFactory()
	state := NewControllerState(runner)

	controllerUUID := controllertesting.GenControllerUUID(c)

	modelUUID0 := modelstatetesting.CreateTestModel(c, runner, "test0")
	modelUUID1 := modelstatetesting.CreateTestModel(c, runner, "test1")
	controllerstatetesting.CreateTestController(c, runner, controllerUUID, modelUUID0)

	info, err := state.ModelMigrationInfo(context.Background(), modelUUID1)
	c.Assert(err, jc.ErrorIsNil)
	c.Check(info.ControllerUUID, gc.Equals, controllerUUID)
	c.Check(info.IsControllerModel, jc.IsFalse)
	c.Check(info.MigrationActive, jc.IsFalse)
}
