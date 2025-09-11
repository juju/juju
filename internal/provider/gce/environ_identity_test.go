// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gce_test

import (
	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/internal/testing"
)

func (s *environSuite) TestSupportsInstanceRole(c *tc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)
	c.Assert(env.SupportsInstanceRoles(c.Context()), tc.IsTrue)
}

func (s *environSuite) TestCreateAutoInstanceRole(c *tc.C) {
	ctrl := s.SetupMocks(c)
	defer ctrl.Finish()

	env := s.SetupEnv(c, s.MockService)

	s.MockService.EXPECT().DefaultServiceAccount(gomock.Any()).Return("fred@googledev.com", nil)

	p := environs.BootstrapParams{
		ControllerConfig: map[string]interface{}{
			controller.ControllerUUIDKey: testing.ControllerTag.Id(),
		},
	}
	res, err := env.CreateAutoInstanceRole(c.Context(), p)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(res, tc.Equals, "fred@googledev.com")
}
