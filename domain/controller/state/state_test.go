// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state

import (
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/controller"
	coremodel "github.com/juju/juju/core/model"
	schematesting "github.com/juju/juju/domain/schema/testing"
	jujutesting "github.com/juju/juju/internal/testing"
)

type stateSuite struct {
	schematesting.ControllerSuite
	controllerModelUUID coremodel.UUID
}

func TestStateSuite(t *testing.T) {
	tc.Run(t, &stateSuite{})
}

func (s *stateSuite) SetUpTest(c *tc.C) {
	s.controllerModelUUID = coremodel.UUID(jujutesting.ModelTag.Id())
	s.ControllerSuite.SetUpTest(c)
	_ = s.ControllerSuite.SeedControllerTable(c, s.controllerModelUUID)
}

func (s *stateSuite) TestControllerModelUUID(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())
	uuid, err := st.GetControllerModelUUID(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(uuid, tc.Equals, s.controllerModelUUID)
}

func (s *stateSuite) TestGetControllerAgentInfo(c *tc.C) {
	st := NewState(s.TxnRunnerFactory())

	servingInfo, err := st.GetControllerAgentInfo(c.Context())
	c.Assert(err, tc.ErrorIsNil)

	c.Check(servingInfo, tc.DeepEquals, controller.ControllerAgentInfo{
		APIPort:        17070,
		Cert:           "test-cert",
		PrivateKey:     "test-private-key",
		CAPrivateKey:   "test-ca-private-key",
		SystemIdentity: "test-system-identity",
	})
}
