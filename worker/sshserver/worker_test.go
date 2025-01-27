// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshserver_test

import (
	"github.com/juju/juju/controller"
	"github.com/juju/juju/worker/sshserver"
	"github.com/juju/juju/worker/sshserver/mocks"
	"github.com/juju/worker/v3"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"
)

type workerSuite struct {
}

var _ = gc.Suite(&workerSuite{})

func newServerWrapperWorkerConfig(
	l *mocks.MockLogger,
	s *mocks.MockSystemStateGetter,
	modifier func(*sshserver.ServerWrapperWorkerConfig),
) *sshserver.ServerWrapperWorkerConfig {
	cfg := &sshserver.ServerWrapperWorkerConfig{
		NewServerWorker: func() (worker.Worker, error) { return nil, nil },
		Logger:          l,
		StatePool:       s,
		StateInfo: controller.StateServingInfo{
			APIPort: 1234, // Fill in so it acts as non-zero value.
		},
	}

	modifier(cfg)

	return cfg
}

func (s *workerSuite) TestValidate(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockLogger := mocks.NewMockLogger(ctrl)
	mockStateGetter := mocks.NewMockSystemStateGetter(ctrl)

	cfg := newServerWrapperWorkerConfig(mockLogger, mockStateGetter, func(cfg *sshserver.ServerWrapperWorkerConfig) {})
	c.Assert(cfg.Validate(), gc.IsNil)

	// Test no Logger.
	cfg = newServerWrapperWorkerConfig(
		mockLogger,
		mockStateGetter,
		func(cfg *sshserver.ServerWrapperWorkerConfig) {
			cfg.Logger = nil
		},
	)
	c.Assert(cfg.Validate(), gc.ErrorMatches, ".*is required.*")

	// Test no StatePool.
	cfg = newServerWrapperWorkerConfig(
		mockLogger,
		mockStateGetter,
		func(cfg *sshserver.ServerWrapperWorkerConfig) {
			cfg.StatePool = nil
		},
	)
	c.Assert(cfg.Validate(), gc.ErrorMatches, ".*is required.*")

	// Test no StateServingInfo.
	cfg = newServerWrapperWorkerConfig(
		mockLogger,
		mockStateGetter,
		func(cfg *sshserver.ServerWrapperWorkerConfig) {
			cfg.StateInfo = controller.StateServingInfo{}
		},
	)
	c.Assert(cfg.Validate(), gc.ErrorMatches, ".*is required.*")

	// Test no NewServerWorker.
	cfg = newServerWrapperWorkerConfig(
		mockLogger,
		mockStateGetter,
		func(cfg *sshserver.ServerWrapperWorkerConfig) {
			cfg.NewServerWorker = nil
		},
	)
	c.Assert(cfg.Validate(), gc.ErrorMatches, ".*is required.*")
}
