// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package deployer

import (
	"context"
	"testing"

	"github.com/juju/clock"
	"github.com/juju/tc"
	"github.com/juju/utils/v4/voyeur"

	internallogger "github.com/juju/juju/internal/logger"
)

func TestManifold(t *testing.T) {
	tc.Run(t, &manifoldSuite{})
}

type manifoldSuite struct{}

func (s *manifoldSuite) TestValidateAcceptsValidConfig(c *tc.C) {
	cfg := ManifoldConfig{
		AgentName:          "agent",
		APICallerName:      "api-caller",
		HTTPClientName:     "http-client",
		AgentConfigChanged: voyeur.NewValue(false),
		Clock:              clock.WallClock,
		Logger:             internallogger.GetLogger("juju.worker.deployer.test"),
		NewDeployContext:   func(ContextConfig) (Context, error) { return nil, nil },
	}

	c.Check(cfg.Validate(), tc.ErrorIsNil)
}

func (s *manifoldSuite) TestValidateRejectsEmptyAgentName(c *tc.C) {
	cfg := ManifoldConfig{
		APICallerName:      "api-caller",
		HTTPClientName:     "http-client",
		AgentConfigChanged: voyeur.NewValue(false),
		Clock:              clock.WallClock,
		Logger:             internallogger.GetLogger("juju.worker.deployer.test"),
		NewDeployContext:   func(ContextConfig) (Context, error) { return nil, nil },
	}

	err := cfg.Validate()
	c.Check(err, tc.NotNil)
	c.Check(err.Error(), tc.Equals, `empty AgentName not valid`)
}

func (s *manifoldSuite) TestValidateRejectsEmptyAPICallerName(c *tc.C) {
	cfg := ManifoldConfig{
		AgentName:          "agent",
		HTTPClientName:     "http-client",
		AgentConfigChanged: voyeur.NewValue(false),
		Clock:              clock.WallClock,
		Logger:             internallogger.GetLogger("juju.worker.deployer.test"),
		NewDeployContext:   func(ContextConfig) (Context, error) { return nil, nil },
	}

	err := cfg.Validate()
	c.Check(err, tc.NotNil)
	c.Check(err.Error(), tc.Equals, `empty APICallerName not valid`)
}

func (s *manifoldSuite) TestValidateRejectsEmptyHTTPClientName(c *tc.C) {
	cfg := ManifoldConfig{
		AgentName:          "agent",
		APICallerName:      "api-caller",
		AgentConfigChanged: voyeur.NewValue(false),
		Clock:              clock.WallClock,
		Logger:             internallogger.GetLogger("juju.worker.deployer.test"),
		NewDeployContext:   func(ContextConfig) (Context, error) { return nil, nil },
	}

	err := cfg.Validate()
	c.Check(err, tc.NotNil)
	c.Check(err.Error(), tc.Equals, `empty HTTPClientName not valid`)
}

func (s *manifoldSuite) TestValidateRejectsNilClock(c *tc.C) {
	cfg := ManifoldConfig{
		AgentName:          "agent",
		APICallerName:      "api-caller",
		HTTPClientName:     "http-client",
		AgentConfigChanged: voyeur.NewValue(false),
		Logger:             internallogger.GetLogger("juju.worker.deployer.test"),
		NewDeployContext:   func(ContextConfig) (Context, error) { return nil, nil },
	}

	err := cfg.Validate()
	c.Check(err, tc.NotNil)
	c.Check(err.Error(), tc.Equals, `nil Clock not valid`)
}

func (s *manifoldSuite) TestValidateRejectsNilLogger(c *tc.C) {
	cfg := ManifoldConfig{
		AgentName:          "agent",
		APICallerName:      "api-caller",
		HTTPClientName:     "http-client",
		AgentConfigChanged: voyeur.NewValue(false),
		Clock:              clock.WallClock,
		NewDeployContext:   func(ContextConfig) (Context, error) { return nil, nil },
	}

	err := cfg.Validate()
	c.Check(err, tc.NotNil)
	c.Check(err.Error(), tc.Equals, `nil Logger not valid`)
}

func (s *manifoldSuite) TestValidateRejectsNilNewDeployContext(c *tc.C) {
	cfg := ManifoldConfig{
		AgentName:          "agent",
		APICallerName:      "api-caller",
		HTTPClientName:     "http-client",
		AgentConfigChanged: voyeur.NewValue(false),
		Clock:              clock.WallClock,
		Logger:             internallogger.GetLogger("juju.worker.deployer.test"),
	}

	err := cfg.Validate()
	c.Check(err, tc.NotNil)
	c.Check(err.Error(), tc.Equals, `nil NewDeployContext not valid`)
}

func (s *manifoldSuite) TestValidateRejectsNilAgentConfigChanged(c *tc.C) {
	cfg := ManifoldConfig{
		AgentName:        "agent",
		APICallerName:    "api-caller",
		HTTPClientName:   "http-client",
		Clock:            clock.WallClock,
		Logger:           internallogger.GetLogger("juju.worker.deployer.test"),
		NewDeployContext: func(ContextConfig) (Context, error) { return nil, nil },
	}

	err := cfg.Validate()
	c.Check(err, tc.NotNil)
	c.Check(err.Error(), tc.Equals, `nil AgentConfigChanged not valid`)
}

func (s *manifoldSuite) TestManifoldHasCorrectInputs(c *tc.C) {
	manifold := Manifold(ManifoldConfig{
		AgentName:          "agent",
		APICallerName:      "api-caller",
		HTTPClientName:     "http-client",
		AgentConfigChanged: voyeur.NewValue(false),
	})

	c.Check(manifold.Inputs, tc.DeepEquals, []string{"agent", "api-caller", "http-client"})
}

func (s *manifoldSuite) TestStartValidatesBeforeGettingDependencies(c *tc.C) {
	var getterCalled bool
	manifold := Manifold(ManifoldConfig{})

	w, err := manifold.Start(context.Background(), &mockGetter{
		getter: func(name string, out any) error {
			getterCalled = true
			return nil
		},
	})
	c.Check(w, tc.IsNil)
	c.Check(err, tc.NotNil)
	c.Check(getterCalled, tc.IsFalse)
}

type mockGetter struct {
	getter func(name string, out any) error
}

func (g *mockGetter) Get(_ string, out any) error {
	if g.getter != nil {
		return g.getter("", out)
	}
	return nil
}
