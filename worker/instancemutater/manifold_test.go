// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instancemutater_test

import (
	"github.com/golang/mock/gomock"
	"github.com/juju/errors"
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/environs"
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

type environAPIManifoldSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&environAPIManifoldSuite{})

func (s *environAPIManifoldSuite) TestStartReturnsWorker(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockEnviron := mocks.NewMockEnviron(ctrl)
	mockAPICaller := mocks.NewMockAPICaller(ctrl)

	mockContext := mocks.NewMockContext(ctrl)
	mockContext.EXPECT().Get("foobar", gomock.Any()).SetArg(1, mockEnviron).Return(nil)
	mockContext.EXPECT().Get("baz", gomock.Any()).SetArg(1, mockAPICaller).Return(nil)
	mockWorker := mocks.NewMockWorker(ctrl)

	config := instancemutater.EnvironAPIConfig{
		EnvironName:   "foobar",
		APICallerName: "baz",
	}
	manifold := instancemutater.EnvironAPIManifold(config, func(environ environs.Environ, apiCaller base.APICaller) (worker.Worker, error) {
		c.Assert(environ, gc.Equals, mockEnviron)
		c.Assert(apiCaller, gc.Equals, mockAPICaller)

		return mockWorker, nil
	})
	result, err := manifold.Start(mockContext)
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.Equals, mockWorker)
}

func (s *environAPIManifoldSuite) TestMissingEnvironFromContext(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockContext := mocks.NewMockContext(ctrl)
	mockContext.EXPECT().Get("foobar", gomock.Any()).Return(errors.New("missing"))

	config := instancemutater.EnvironAPIConfig{
		EnvironName:   "foobar",
		APICallerName: "baz",
	}
	manifold := instancemutater.EnvironAPIManifold(config, func(environs.Environ, base.APICaller) (worker.Worker, error) {
		c.Fail()
		return nil, nil
	})
	_, err := manifold.Start(mockContext)
	c.Assert(err, gc.ErrorMatches, "missing")
}

func (s *environAPIManifoldSuite) TestMissingAPICallerFromContext(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockEnviron := mocks.NewMockEnviron(ctrl)

	mockContext := mocks.NewMockContext(ctrl)
	mockContext.EXPECT().Get("foobar", gomock.Any()).SetArg(1, mockEnviron).Return(nil)
	mockContext.EXPECT().Get("baz", gomock.Any()).Return(errors.New("missing"))

	config := instancemutater.EnvironAPIConfig{
		EnvironName:   "foobar",
		APICallerName: "baz",
	}
	manifold := instancemutater.EnvironAPIManifold(config, func(environs.Environ, base.APICaller) (worker.Worker, error) {
		c.Fail()
		return nil, nil
	})
	_, err := manifold.Start(mockContext)
	c.Assert(err, gc.ErrorMatches, "missing")
}

type manifoldSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&manifoldSuite{})

func (s *manifoldSuite) TestNewWorkerIsCalled(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockEnviron := mocks.NewMockEnviron(ctrl)
	mockAPICaller := mocks.NewMockAPICaller(ctrl)

	mockContext := mocks.NewMockContext(ctrl)
	mockContext.EXPECT().Get("foobar", gomock.Any()).SetArg(1, mockEnviron).Return(nil)
	mockContext.EXPECT().Get("baz", gomock.Any()).SetArg(1, mockAPICaller).Return(nil)
	mockWorker := mocks.NewMockWorker(ctrl)

	config := instancemutater.ManifoldConfig{
		EnvironName:   "foobar",
		APICallerName: "baz",
		Logger:        mocks.NewMockLogger(ctrl),
		NewWorker: func(cfg instancemutater.Config) (worker.Worker, error) {
			return mockWorker, nil
		},
	}
	manifold := instancemutater.Manifold(config)
	result, err := manifold.Start(mockContext)
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.Equals, mockWorker)
}

func (s *manifoldSuite) TestNewWorkerReturnsError(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockEnviron := mocks.NewMockEnviron(ctrl)
	mockAPICaller := mocks.NewMockAPICaller(ctrl)

	mockContext := mocks.NewMockContext(ctrl)
	mockContext.EXPECT().Get("foobar", gomock.Any()).SetArg(1, mockEnviron).Return(nil)
	mockContext.EXPECT().Get("baz", gomock.Any()).SetArg(1, mockAPICaller).Return(nil)

	config := instancemutater.ManifoldConfig{
		EnvironName:   "foobar",
		APICallerName: "baz",
		Logger:        mocks.NewMockLogger(ctrl),
		NewWorker: func(cfg instancemutater.Config) (worker.Worker, error) {
			return nil, errors.New("errored")
		},
	}
	manifold := instancemutater.Manifold(config)
	_, err := manifold.Start(mockContext)
	c.Assert(err, gc.ErrorMatches, "cannot start machine upgrade series worker: errored")
}

func (s *manifoldSuite) TestConfigValidates(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockEnviron := mocks.NewMockEnviron(ctrl)
	mockAPICaller := mocks.NewMockAPICaller(ctrl)

	mockContext := mocks.NewMockContext(ctrl)
	mockContext.EXPECT().Get("foobar", gomock.Any()).SetArg(1, mockEnviron).Return(nil)
	mockContext.EXPECT().Get("baz", gomock.Any()).SetArg(1, mockAPICaller).Return(nil)

	config := instancemutater.ManifoldConfig{
		EnvironName:   "foobar",
		APICallerName: "baz",
		Logger:        mocks.NewMockLogger(ctrl),
	}
	manifold := instancemutater.Manifold(config)
	_, err := manifold.Start(mockContext)
	c.Assert(err, gc.ErrorMatches, "nil NewWorker not valid")
}
