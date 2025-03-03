// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	coremodel "github.com/juju/juju/core/model"
	modeltesting "github.com/juju/juju/domain/model/state/testing"
	schematesting "github.com/juju/juju/domain/schema/testing"
	jujutesting "github.com/juju/juju/internal/testing"
)

type stateSuite struct {
	schematesting.ControllerSuite
	controllerModelUUID coremodel.UUID
}

var _ = gc.Suite(&stateSuite{})

func (s *stateSuite) SetUpTest(c *gc.C) {
	s.controllerModelUUID = coremodel.UUID(jujutesting.ModelTag.Id())
	s.ControllerSuite.SetUpTest(c)
	_ = s.ControllerSuite.SeedControllerTable(c, s.controllerModelUUID)
}

func (s *stateSuite) TestControllerModelUUID(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	uuid, err := st.ControllerModelUUID(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(uuid, gc.Equals, s.controllerModelUUID)
}

func (s *stateSuite) TestGetModelActivationStatus(c *gc.C) {
	st := NewState(s.TxnRunnerFactory())
	uuid := modeltesting.CreateTestModel(c, s.TxnRunnerFactory(), "test-controller")
	activated, err := st.GetModelActivationStatus(context.Background(), uuid.String())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(activated, jc.IsTrue)
}
