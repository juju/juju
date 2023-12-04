// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bootstrap

import (
	"context"
	"os"
	"path/filepath"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v3/dependency"
	dependencytesting "github.com/juju/worker/v3/dependency/testing"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/agent"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/domain/servicefactory/testing"
	"github.com/juju/juju/internal/cloudconfig"
)

type manifoldSuite struct {
	baseSuite
}

var _ = gc.Suite(&manifoldSuite{})

func (s *manifoldSuite) TestValidateConfig(c *gc.C) {
	defer s.setupMocks(c).Finish()

	cfg := s.getConfig()
	c.Check(cfg.Validate(), jc.ErrorIsNil)

	cfg.AgentName = ""
	c.Check(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	cfg.StateName = ""
	c.Check(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	cfg.ObjectStoreName = ""
	c.Check(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	cfg.ServiceFactoryName = ""
	c.Check(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	cfg.BootstrapGateName = ""
	c.Check(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	cfg = s.getConfig()
	cfg.Logger = nil
	c.Check(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	cfg = s.getConfig()
	cfg.BootstrapParamsFileExists = nil
	c.Check(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	cfg = s.getConfig()
	cfg.RemoveBootstrapParamsFile = nil
	c.Check(cfg.Validate(), jc.ErrorIs, errors.NotValid)
}

func (s *manifoldSuite) getConfig() ManifoldConfig {
	return ManifoldConfig{
		AgentName:          "agent",
		ObjectStoreName:    "object-store",
		StateName:          "state",
		BootstrapGateName:  "bootstrap-gate",
		ServiceFactoryName: "service-factory",
		Logger:             s.logger,
		AgentBinaryUploader: func(context.Context, string, BinaryAgentStorageService, objectstore.ObjectStore, Logger) error {
			return nil
		},
		BootstrapParamsFileExists: func(agent.Config) (bool, error) { return false, nil },
		RemoveBootstrapParamsFile: func(agent.Config) error { return nil },
	}
}

func (s *manifoldSuite) getContext() dependency.Context {
	resources := map[string]any{
		"agent":           s.agent,
		"state":           s.stateTracker,
		"object-store":    s.objectStore,
		"bootstrap-gate":  s.bootstrapUnlocker,
		"service-factory": testing.NewTestingServiceFactory(),
	}
	return dependencytesting.StubContext(nil, resources)
}

var expectedInputs = []string{"agent", "state", "object-store", "bootstrap-gate", "service-factory"}

func (s *manifoldSuite) TestInputs(c *gc.C) {
	c.Assert(Manifold(s.getConfig()).Inputs, jc.SameContents, expectedInputs)
}

func (s *manifoldSuite) TestStartAlreadyBootstrapped(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectGateUnlock()
	s.expectAgentConfig(c)

	_, err := Manifold(s.getConfig()).Start(s.getContext())
	c.Assert(err, jc.ErrorIs, dependency.ErrUninstall)
}

func (s *manifoldSuite) TestBootstrapParamsFileExists(c *gc.C) {
	defer s.setupMocks(c).Finish()

	tests := []struct {
		name  string
		ok    bool
		err   error
		setup func(*gc.C, string)
	}{{
		name: "file exists",
		ok:   true,
		err:  nil,
		setup: func(c *gc.C, dir string) {
			path := filepath.Join(dir, cloudconfig.FileNameBootstrapParams)
			err := os.WriteFile(path, []byte("test"), 0644)
			c.Assert(err, jc.ErrorIsNil)
		},
	}, {
		name:  "file does not exist",
		ok:    false,
		err:   nil,
		setup: func(c *gc.C, dir string) {},
	}}

	for _, test := range tests {
		c.Logf("test %q", test.name)

		dir := c.MkDir()
		s.agentConfig.EXPECT().DataDir().Return(dir)

		test.setup(c, dir)

		ok, err := BootstrapParamsFileExists(s.agentConfig)
		if test.err != nil {
			c.Assert(err, gc.ErrorMatches, test.err.Error())
			continue
		}

		c.Assert(err, jc.ErrorIsNil)
		c.Check(ok, gc.Equals, test.ok)
	}
}
