// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package leaseconsumer

import (
	"github.com/golang/mock/gomock"
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v2"
	"github.com/juju/worker/v2/dependency"
	dt "github.com/juju/worker/v2/dependency/testing"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/apiserverhttp"
)

var expectedInputs = []string{"agent", "auth", "mux"}

type ManifoldSuite struct {
	testing.IsolationSuite

	manifold dependency.Manifold
	context  dependency.Context
	agent    *MockAgent
	auth     *MockAuthenticator
	worker   *MockWorker
	mux      *apiserverhttp.Mux
}

var _ = gc.Suite(&ManifoldSuite{})

func (s *ManifoldSuite) TestInputs(c *gc.C) {
	defer s.setupMocks(c).Finish()

	c.Assert(s.manifold.Inputs, jc.SameContents, expectedInputs)
}

func (s *ManifoldSuite) TestMissingInputs(c *gc.C) {
	defer s.setupMocks(c).Finish()

	for _, input := range expectedInputs {
		context := s.newContext(map[string]interface{}{
			input: dependency.ErrMissing,
		})
		_, err := s.manifold.Start(context)
		c.Assert(errors.Cause(err), gc.Equals, dependency.ErrMissing)
	}
}

func (s *ManifoldSuite) TestStart(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.startWorkerClean(c)
}

func (s *ManifoldSuite) startWorkerClean(c *gc.C) worker.Worker {
	w, err := s.manifold.Start(s.context)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(w, gc.Equals, s.worker)
	return w
}

func (s *ManifoldSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.agent = NewMockAgent(ctrl)
	s.auth = NewMockAuthenticator(ctrl)
	s.worker = NewMockWorker(ctrl)

	s.mux = &apiserverhttp.Mux{}

	s.context = s.newContext(nil)
	s.manifold = Manifold(ManifoldConfig{
		AgentName:         "agent",
		AuthenticatorName: "auth",
		MuxName:           "mux",
		NewWorker:         s.newWorker(c),
		Path:              "leaseconsumer/path",
	})

	return ctrl
}

func (s *ManifoldSuite) newContext(overlay map[string]interface{}) dependency.Context {
	resources := map[string]interface{}{
		"agent": s.agent,
		"auth":  s.auth,
		"mux":   s.mux,
	}
	for k, v := range overlay {
		resources[k] = v
	}
	return dt.StubContext(nil, resources)
}

func (s *ManifoldSuite) newWorker(c *gc.C) func(Config) (worker.Worker, error) {
	return func(config Config) (worker.Worker, error) {
		c.Assert(config, gc.DeepEquals, Config{
			Authenticator: s.auth,
			Mux:           s.mux,
			Path:          "leaseconsumer/path",
		})
		return s.worker, nil
	}
}
