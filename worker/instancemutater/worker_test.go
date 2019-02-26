// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instancemutater_test

import (
	"github.com/golang/mock/gomock"
	"github.com/juju/testing"
	gc "gopkg.in/check.v1"
	names "gopkg.in/juju/names.v2"

	"github.com/juju/juju/worker/instancemutater"
	"github.com/juju/juju/worker/instancemutater/mocks"
)

type workerConfigSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&workerConfigSuite{})

func (s *workerConfigSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
}

func (s *workerConfigSuite) TestInvalidConfigValidate(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	testcases := []struct {
		description string
		config      instancemutater.Config
		err         string
	}{
		{
			description: "Test empty configuration",
			config:      instancemutater.Config{},
			err:         "nil Logger not valid",
		},
		{
			description: "Test no logger",
			config:      instancemutater.Config{},
			err:         "nil Logger not valid",
		},
		{
			description: "Test no broker",
			config: instancemutater.Config{
				Logger: mocks.NewMockLogger(ctrl),
			},
			err: "nil InstanceBroker not valid",
		},
		{
			description: "Test no tag",
			config: instancemutater.Config{
				Logger: mocks.NewMockLogger(ctrl),
				Broker: mocks.NewMockInstanceBroker(ctrl),
			},
			err: "nil tag not valid",
		},
		{
			description: "Test invalid machine tag",
			config: instancemutater.Config{
				Logger: mocks.NewMockLogger(ctrl),
				Broker: mocks.NewMockInstanceBroker(ctrl),
				Tag:    names.ApplicationTag{},
			},
			err: "tag not valid",
		},
	}
	for i, test := range testcases {
		c.Logf("%d %s", i, test.description)
		err := test.config.Validate()
		c.Assert(err, gc.ErrorMatches, test.err)
	}
}

func (s *workerConfigSuite) TestValidConfigValidate(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	config := instancemutater.Config{
		Logger: mocks.NewMockLogger(ctrl),
		Broker: mocks.NewMockInstanceBroker(ctrl),
		Tag:    names.MachineTag{},
	}
	err := config.Validate()
	c.Assert(err, gc.IsNil)
}
