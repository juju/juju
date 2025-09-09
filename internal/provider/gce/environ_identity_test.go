// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce_test

import (
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/testing"
)

func (s *environSuite) TestSupportsInstanceRole(c *gc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)
	c.Assert(env.SupportsInstanceRoles(s.CallCtx), jc.IsTrue)
}

func (s *environSuite) TestCreateAutoInstanceRole(c *gc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)

	s.MockService.EXPECT().DefaultServiceAccount(gomock.Any()).Return("fred@googledev.com", nil)

	p := environs.BootstrapParams{
		ControllerConfig: map[string]interface{}{
			controller.ControllerUUIDKey: testing.ControllerTag.Id(),
		},
	}
	res, err := env.CreateAutoInstanceRole(s.CallCtx, p)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(res, gc.Equals, "fred@googledev.com")
}
