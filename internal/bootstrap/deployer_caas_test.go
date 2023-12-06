// Copyright 2023 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package bootstrap

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gomock "go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"
)

type deployerCAASSuite struct {
	baseSuite

	cloudServiceGetter *MockCloudServiceGetter
	operationApplier   *MockOperationApplier
}

var _ = gc.Suite(&deployerCAASSuite{})

func (s *deployerCAASSuite) TestValidate(c *gc.C) {
	defer s.setupMocks(c).Finish()

	cfg := s.newConfig()
	err := cfg.Validate()
	c.Assert(err, gc.IsNil)

	cfg = s.newConfig()
	cfg.CloudServiceGetter = nil
	err = cfg.Validate()
	c.Assert(err, jc.ErrorIs, errors.NotValid)

	cfg = s.newConfig()
	cfg.OperationApplier = nil
	err = cfg.Validate()
	c.Assert(err, jc.ErrorIs, errors.NotValid)
}

func (s *deployerCAASSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := s.baseSuite.setupMocks(c)

	s.cloudServiceGetter = NewMockCloudServiceGetter(ctrl)
	s.operationApplier = NewMockOperationApplier(ctrl)

	return ctrl
}

func (s *deployerCAASSuite) newConfig() CAASDeployerConfig {
	return CAASDeployerConfig{
		BaseDeployerConfig: s.baseSuite.newConfig(),
		CloudServiceGetter: s.cloudServiceGetter,
		OperationApplier:   s.operationApplier,
	}
}
