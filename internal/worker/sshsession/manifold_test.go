// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshsession_test

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/names/v5"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v3"
	dt "github.com/juju/worker/v3/dependency/testing"
	"go.uber.org/mock/gomock"

	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/base"
	"github.com/juju/juju/internal/worker/sshsession"
)

type manifoldSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&manifoldSuite{})

func newManifoldConfig(l loggo.Logger, modifier func(cfg *sshsession.ManifoldConfig)) *sshsession.ManifoldConfig {
	cfg := &sshsession.ManifoldConfig{
		APICallerName: "api-caller",
		AgentName:     "agent",
		Logger:        l,
		NewWorker:     func(cfg sshsession.WorkerConfig) (worker.Worker, error) { return nil, nil },
	}

	modifier(cfg)

	return cfg
}

func (s *manifoldSuite) TestConfigValidate(c *gc.C) {
	l := loggo.GetLogger("test")
	// Check config as expected.

	cfg := newManifoldConfig(l, func(cfg *sshsession.ManifoldConfig) {})
	c.Assert(cfg.Validate(), jc.ErrorIsNil)

	// Test empty APICallerName.
	cfg = newManifoldConfig(l, func(cfg *sshsession.ManifoldConfig) {
		cfg.APICallerName = ""
	})
	c.Check(errors.Is(cfg.Validate(), errors.NotValid), jc.IsTrue)

	// Test empty AgentName.
	cfg = newManifoldConfig(l, func(cfg *sshsession.ManifoldConfig) {
		cfg.AgentName = ""
	})
	c.Check(errors.Is(cfg.Validate(), errors.NotValid), jc.IsTrue)

	// Test Nil Logger.
	cfg = newManifoldConfig(l, func(cfg *sshsession.ManifoldConfig) {
		cfg.Logger = nil
	})
	c.Check(errors.Is(cfg.Validate(), errors.NotValid), jc.IsTrue)

	// Test Nil NewWorker.
	cfg = newManifoldConfig(l, func(cfg *sshsession.ManifoldConfig) {
		cfg.NewWorker = nil
	})
	c.Check(errors.Is(cfg.Validate(), errors.NotValid), jc.IsTrue)
}

func (s *manifoldSuite) TestManifoldStart(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockAgent := NewMockAgent(ctrl)
	mockAgentConfig := NewMockConfig(ctrl)

	mockAgentConfig.EXPECT().Tag().Return(names.NewMachineTag("0"))
	mockAgent.EXPECT().CurrentConfig().Return(mockAgentConfig)

	// Setup the manifold
	manifold := sshsession.Manifold(sshsession.ManifoldConfig{
		APICallerName: "api-caller",
		AgentName:     "agent",
		Logger:        loggo.GetLogger("test"),
		NewWorker:     func(cfg sshsession.WorkerConfig) (worker.Worker, error) { return nil, nil },
	})

	// Check the inputs are as expected
	c.Assert(manifold.Inputs, gc.DeepEquals, []string{
		"api-caller",
		"agent",
	})

	// Start the worker
	w, err := manifold.Start(
		dt.StubContext(nil, map[string]interface{}{
			"api-caller": mockAPICaller{},
			"agent":      mockAgent,
		}),
	)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(w, gc.IsNil)
}

type mockAPICaller struct {
	base.APICaller
}

func (a mockAPICaller) BestFacadeVersion(facade string) int {
	return 0
}
