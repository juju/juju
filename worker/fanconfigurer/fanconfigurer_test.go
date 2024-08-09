// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package fanconfigurer

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"
)

type fanConfigurerSuite struct {
	testing.IsolationSuite

	facade *MockFanConfigurerFacade
}

var _ = gc.Suite(&fanConfigurerSuite{})

func (s *fanConfigurerSuite) TestProcessNewConfigNotImplemented(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.facade.EXPECT().FanConfig().Return(nil, errors.NotImplemented)

	fc := &FanConfigurer{
		config: FanConfigurerConfig{
			Facade: s.facade,
		},
	}

	err := fc.processNewConfig()
	c.Assert(err, gc.IsNil)
}

func (s *fanConfigurerSuite) TestProcessLoopNotImplemented(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.facade.EXPECT().WatchForFanConfigChanges().Return(nil, errors.NotImplemented)

	fc := &FanConfigurer{
		config: FanConfigurerConfig{
			Facade: s.facade,
		},
	}

	err := fc.loop()
	c.Assert(err, gc.IsNil)
}

func (s *fanConfigurerSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.facade = NewMockFanConfigurerFacade(ctrl)

	return ctrl
}
