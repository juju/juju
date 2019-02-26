// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instancemutater_test

import (
	"github.com/golang/mock/gomock"
	"github.com/juju/juju/worker/instancemutater"
	"github.com/juju/juju/worker/instancemutater/mocks"
	"github.com/juju/testing"
	gc "gopkg.in/check.v1"
	worker "gopkg.in/juju/worker.v1"
)

type manifoldConfigSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&manifoldConfigSuite{})

func (s *manifoldConfigSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
}

func (s *manifoldConfigSuite) TestInvalidConfigValidate(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	testcases := []struct {
		description string
		config      instancemutater.ManifoldConfig
		err         string
	}{
		{
			description: "Test empty configuration",
			config:      instancemutater.ManifoldConfig{},
			err:         "nil Logger not valid",
		},
		{
			description: "Test no logger",
			config:      instancemutater.ManifoldConfig{},
			err:         "nil Logger not valid",
		},
		{
			description: "Test no new worker constructor",
			config: instancemutater.ManifoldConfig{
				Logger: mocks.NewMockLogger(ctrl),
			},
			err: "nil NewWorker not valid",
		},
	}
	for i, test := range testcases {
		c.Logf("%d %s", i, test.description)
		err := test.config.Validate()
		c.Assert(err, gc.ErrorMatches, test.err)
	}
}

func (s *manifoldConfigSuite) TestValidConfigValidate(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	config := instancemutater.ManifoldConfig{
		Logger: mocks.NewMockLogger(ctrl),
		NewWorker: func(cfg instancemutater.Config) (worker.Worker, error) {
			return mocks.NewMockWorker(ctrl), nil
		},
	}
	err := config.Validate()
	c.Assert(err, gc.IsNil)
}
