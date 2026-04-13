// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package agentbinaryfetcher

import (
	"context"
	"testing"

	"github.com/juju/tc"
	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/dependency"
	dt "github.com/juju/worker/v5/dependency/testing"
	"go.uber.org/goleak"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/worker/gate"
)

type manifoldSuite struct {
	testhelpers.IsolationSuite

	agentBinaryService *MockAgentBinaryService
	modelAgentService  *MockModelAgentService
}

func TestManifoldSuite(t *testing.T) {
	defer goleak.VerifyNone(t)
	tc.Run(t, &manifoldSuite{})
}

func (s *manifoldSuite) TestValidateConfig(c *tc.C) {
	cfg := s.getConfig(c)
	c.Check(cfg.Validate(), tc.ErrorIsNil)

	cfg = s.getConfig(c)
	cfg.DomainServicesName = ""
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.getConfig(c)
	cfg.GetDomainServices = nil
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.getConfig(c)
	cfg.NewWorker = nil
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.getConfig(c)
	cfg.Logger = nil
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)
}

func (s *manifoldSuite) TestStart(c *tc.C) {
	defer s.setupMocks(c).Finish()

	cfg := s.getConfig(c)

	_, err := Manifold(cfg).Start(c.Context(), s.newGetter())
	c.Assert(err, tc.ErrorIsNil)
}

func (s *manifoldSuite) getConfig(c *tc.C) ManifoldConfig {
	return ManifoldConfig{
		DomainServicesName: "domain-services",
		GetDomainServices: func(context.Context, dependency.Getter, string) (domainServices, error) {
			return domainServices{
				modelAgent:  s.modelAgentService,
				agentBinary: s.agentBinaryService,
			}, nil
		},
		NewWorker: func(WorkerConfig) worker.Worker { return nil },
		Logger:    loggertesting.WrapCheckLog(c),
	}
}

func (s *manifoldSuite) newGetter() dependency.Getter {
	resources := map[string]any{
		"gate": gate.NewLock(),
	}
	return dt.StubGetter(resources)
}

func (s *manifoldSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.modelAgentService = NewMockModelAgentService(ctrl)
	s.agentBinaryService = NewMockAgentBinaryService(ctrl)

	c.Cleanup(func() {
		s.modelAgentService = nil
		s.agentBinaryService = nil
	})

	return ctrl
}
