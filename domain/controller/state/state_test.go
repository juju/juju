// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"context"

	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"

	coremodel "github.com/juju/juju/core/model"
	schematesting "github.com/juju/juju/domain/schema/testing"
	jujutesting "github.com/juju/juju/internal/testing"
)

type stateSuite struct {
	schematesting.ControllerSuite
	controllerModelUUID coremodel.UUID
}

var _ = tc.Suite(&stateSuite{})

func (s *stateSuite) SetUpTest(c *tc.C) {
	s.controllerModelUUID = coremodel.UUID(jujutesting.ModelTag.Id())
	s.ControllerSuite.SetUpTest(c)
	_ = s.ControllerSuite.SeedControllerTable(c, s.controllerModelUUID)
}

func (s *stateSuite) TestControllerModelUUID(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	uuid, err := st.ControllerModelUUID(context.Background())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(uuid, tc.Equals, s.controllerModelUUID)
}
