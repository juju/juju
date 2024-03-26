// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package auditconfigupdater

import (
	"context"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"
	dt "github.com/juju/worker/v4/dependency/testing"
	"github.com/juju/worker/v4/workertest"
	gc "gopkg.in/check.v1"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/core/auditlog"
	controllerconfigservice "github.com/juju/juju/domain/controllerconfig/service"
	"github.com/juju/juju/internal/servicefactory"
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

	cfg.ServiceFactoryName = ""
	c.Check(cfg.Validate(), jc.ErrorIs, errors.NotValid)
}

var expectedInputs = []string{"agent", "service-factory"}

func (s *manifoldSuite) TestInputs(c *gc.C) {
	c.Assert(Manifold(s.getConfig()).Inputs, jc.SameContents, expectedInputs)
}

func (s *manifoldSuite) TestStart(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectAgentConfig(c)
	s.expectControllerConfig()

	w, err := Manifold(s.getConfig()).Start(context.Background(), s.newGetter())
	c.Assert(err, jc.ErrorIsNil)
	workertest.CleanKill(c, w)
}

func (s *manifoldSuite) expectAgentConfig(c *gc.C) {
	s.agentConfig.EXPECT().LogDir().Return(c.MkDir())
	s.agent.EXPECT().CurrentConfig().Return(s.agentConfig)
}

func (s *manifoldSuite) getConfig() ManifoldConfig {
	return ManifoldConfig{
		AgentName:          "agent",
		ServiceFactoryName: "service-factory",
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
		"service-factory": &stubServiceFactoryGetter{},
	}
	return dt.StubGetter(resources)
}

// Note: This replicates the ability to get a controller service factory and
// a model service factory from the service factory getter.
type stubServiceFactoryGetter struct {
	servicefactory.ServiceFactory
}

func (s *stubServiceFactoryGetter) ControllerConfig() *controllerconfigservice.Service {
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
