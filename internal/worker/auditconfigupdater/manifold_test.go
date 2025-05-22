// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package auditconfigupdater

import (
	stdtesting "testing"

	"github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"
	dt "github.com/juju/worker/v4/dependency/testing"
	"github.com/juju/worker/v4/workertest"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/core/auditlog"
	controllerconfigservice "github.com/juju/juju/domain/controllerconfig/service"
	"github.com/juju/juju/internal/services"
)

type manifoldSuite struct {
	baseSuite
}

func TestManifoldSuite(t *stdtesting.T) {
	tc.Run(t, &manifoldSuite{})
}

func (s *manifoldSuite) TestValidateConfig(c *tc.C) {
	defer s.setupMocks(c).Finish()

	cfg := s.getConfig()
	c.Check(cfg.Validate(), tc.ErrorIsNil)

	cfg.AgentName = ""
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg.DomainServicesName = ""
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)
}

var expectedInputs = []string{"agent", "domain-services"}

func (s *manifoldSuite) TestInputs(c *tc.C) {
	c.Assert(Manifold(s.getConfig()).Inputs, tc.SameContents, expectedInputs)
}

func (s *manifoldSuite) TestStart(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAgentConfig(c)
	s.expectControllerConfig()

	w, err := Manifold(s.getConfig()).Start(c.Context(), s.newGetter())
	c.Assert(err, tc.ErrorIsNil)
	workertest.CleanKill(c, w)
}

func (s *manifoldSuite) expectAgentConfig(c *tc.C) {
	s.agentConfig.EXPECT().LogDir().Return(c.MkDir())
	s.agent.EXPECT().CurrentConfig().Return(s.agentConfig)
}

func (s *manifoldSuite) getConfig() ManifoldConfig {
	return ManifoldConfig{
		AgentName:          "agent",
		DomainServicesName: "domain-services",
		GetControllerConfigService: func(getter dependency.Getter, name string) (ControllerConfigService, error) {
			return s.controllerConfigService, nil
		},
		NewWorker: func(ControllerConfigService, auditlog.Config, AuditLogFactory) (worker.Worker, error) {
			return newStubWorker(), nil
		},
	}
}

func (s *manifoldSuite) newGetter() dependency.Getter {
	resources := map[string]interface{}{
		"agent":           s.agent,
		"domain-services": &stubDomainServicesGetter{},
	}
	return dt.StubGetter(resources)
}

// Note: This replicates the ability to get a controller domain services and
// a model domain services from the domain services getter.
type stubDomainServicesGetter struct {
	services.DomainServices
}

func (s *stubDomainServicesGetter) ControllerConfig() *controllerconfigservice.Service {
	return nil
}

type stubWorker struct {
	tomb tomb.Tomb
}

func newStubWorker() *stubWorker {
	w := &stubWorker{}
	w.tomb.Go(func() error {
		<-w.tomb.Dying()
		return nil
	})
	return w
}

func (w *stubWorker) Kill() {
	w.tomb.Kill(nil)
}

func (w *stubWorker) Wait() error {
	return w.tomb.Wait()
}
