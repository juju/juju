// Copyright 2023 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package bootstrap

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gomock "go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"
)

type deployerIAASSuite struct {
	baseSuite

	machineGetter *MockMachineGetter
}

var _ = gc.Suite(&deployerIAASSuite{})

func (s *deployerIAASSuite) TestValidate(c *gc.C) {
	defer s.setupMocks(c).Finish()

	cfg := s.newConfig()
	err := cfg.Validate()
	c.Assert(err, gc.IsNil)

	cfg = s.newConfig()
	cfg.MachineGetter = nil
	err = cfg.Validate()
	c.Assert(err, jc.ErrorIs, errors.NotValid)
}

func (s *deployerIAASSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := s.baseSuite.setupMocks(c)

	s.machineGetter = NewMockMachineGetter(ctrl)

	return ctrl
}

func (s *deployerIAASSuite) newConfig() IAASDeployerConfig {
	return IAASDeployerConfig{
		BaseDeployerConfig: s.baseSuite.newConfig(),
		MachineGetter:      s.machineGetter,
	}
}
