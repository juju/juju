// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package raftleaseservice

import (
	"time"

	gc "gopkg.in/check.v1"

	gomock "github.com/golang/mock/gomock"
	"github.com/juju/errors"
	api "github.com/juju/juju/api"
	"github.com/juju/juju/apiserver/apiserverhttp"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v2/workertest"
)

type WorkerSuite struct {
	testing.IsolationSuite

	worker  *Worker
	config  Config
	apiInfo *api.Info

	auth        *MockAuthenticator
	target      *MockNotifyTarget
	raft        *MockRaftApplier
	applyFuture *MockApplyFuture
	fsmResponse *MockFSMResponse
	logger      *MockLogger
	clock       *MockClock
	registerer  *MockRegisterer
	mux         *apiserverhttp.Mux
}

var _ = gc.Suite(&WorkerSuite{})

func (s *WorkerSuite) TestWorkerNotify(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.fsmResponse.EXPECT().Notify(s.target)
	s.fsmResponse.EXPECT().Error().Return(nil)

	s.applyFuture.EXPECT().Error().Return(nil)
	s.applyFuture.EXPECT().Response().Return(s.fsmResponse)

	s.raft.EXPECT().Apply([]byte("claim"), applyTimeout).Return(s.applyFuture)

	done := make(chan struct{})
	s.worker.operations <- operation{
		Commands: []string{"claim"},
		Callback: func(err error) {
			c.Assert(err, jc.ErrorIsNil)
			done <- struct{}{}
		},
	}

	select {
	case <-done:
	case <-time.After(coretesting.LongWait):
		c.Fatalf("no first message")
	}

	// Stopping the worker should cause the state to
	// eventually be released.
	workertest.CleanKill(c, s.worker)
}

func (s *WorkerSuite) TestWorkerNotifyError(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.applyFuture.EXPECT().Error().Return(errors.New("boom"))
	s.raft.EXPECT().Apply([]byte("claim"), applyTimeout).Return(s.applyFuture)

	done := make(chan struct{})
	s.worker.operations <- operation{
		Commands: []string{"claim"},
		Callback: func(err error) {
			c.Assert(err, gc.ErrorMatches, `boom`)
			done <- struct{}{}
		},
	}

	select {
	case <-done:
	case <-time.After(coretesting.LongWait):
		c.Fatalf("no first message")
	}

	// Stopping the worker should cause the state to
	// eventually be released.
	workertest.CleanKill(c, s.worker)
}

func (s *WorkerSuite) TestValidateErrors(c *gc.C) {
	defer s.setupMocks(c).Finish()

	type test struct {
		f      func(*Config)
		expect string
	}
	tests := []test{{
		func(cfg *Config) { cfg.APIInfo = nil },
		"nil APIInfo not valid",
	}, {
		func(cfg *Config) { cfg.Raft = nil },
		"nil Raft not valid",
	}, {
		func(cfg *Config) { cfg.Mux = nil },
		"nil Mux not valid",
	}, {
		func(cfg *Config) { cfg.Path = "" },
		"empty Path not valid",
	}, {
		func(cfg *Config) { cfg.Target = nil },
		"nil Target not valid",
	}, {
		func(cfg *Config) { cfg.Logger = nil },
		"nil Logger not valid",
	}, {
		func(cfg *Config) { cfg.PrometheusRegisterer = nil },
		"nil PrometheusRegisterer not valid",
	}, {
		func(cfg *Config) { cfg.Clock = nil },
		"nil Clock not valid",
	}}
	for i, test := range tests {
		c.Logf("test #%d (%s)", i, test.expect)
		s.testValidateError(c, test.f, test.expect)
	}
}

func (s *WorkerSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.apiInfo = &api.Info{}

	s.auth = NewMockAuthenticator(ctrl)
	s.target = NewMockNotifyTarget(ctrl)
	s.logger = NewMockLogger(ctrl)
	s.clock = NewMockClock(ctrl)
	s.registerer = NewMockRegisterer(ctrl)
	s.raft = NewMockRaftApplier(ctrl)
	s.applyFuture = NewMockApplyFuture(ctrl)
	s.fsmResponse = NewMockFSMResponse(ctrl)

	s.mux = apiserverhttp.NewMux()

	s.clock.EXPECT().Now().Return(time.Now()).AnyTimes()
	s.registerer.EXPECT().Register(gomock.Any()).AnyTimes()
	s.registerer.EXPECT().Unregister(gomock.Any()).AnyTimes()

	s.config = Config{
		APIInfo:              s.apiInfo,
		Authenticator:        s.auth,
		Mux:                  s.mux,
		Path:                 "lease",
		Raft:                 s.raft,
		Target:               s.target,
		PrometheusRegisterer: s.registerer,
		Clock:                s.clock,
		Logger:               s.logger,
	}

	worker, err := NewWorker(s.config)
	c.Assert(err, jc.ErrorIsNil)

	s.worker = worker.(*Worker)

	return ctrl
}

func (s *WorkerSuite) testValidateError(c *gc.C, f func(*Config), expect string) {
	config := s.config
	f(&config)
	w, err := NewWorker(config)
	if !c.Check(err, gc.NotNil) {
		workertest.DirtyKill(c, w)
		return
	}
	c.Check(w, gc.IsNil)
	c.Check(err, gc.ErrorMatches, expect)
}
