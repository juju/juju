// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package auditconfigupdater

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/juju/worker/v5"
	"github.com/juju/worker/v5/dependency"
	dt "github.com/juju/worker/v5/dependency/testing"
	"github.com/juju/worker/v5/workertest"
	"gopkg.in/tomb.v2"

	"github.com/juju/juju/core/auditlog"
	controllerconfigservice "github.com/juju/juju/domain/controllerconfig/service"
	"github.com/juju/juju/internal/services"
)

type manifoldSuite struct {
	baseSuite
}

func TestManifoldSuite(t *testing.T) {
	tc.Run(t, &manifoldSuite{})
}

func (s *manifoldSuite) TestValidateConfig(c *tc.C) {
	defer s.setupMocks(c).Finish()

	cfg := s.getConfig()
	c.Check(cfg.Validate(), tc.ErrorIsNil)

	cfg.LogDir = ""
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg.DomainServicesName = ""
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)
}

var expectedInputs = []string{"domain-services"}

func (s *manifoldSuite) TestInputs(c *tc.C) {
	c.Assert(Manifold(s.getConfig()).Inputs, tc.SameContents, expectedInputs)
}

func (s *manifoldSuite) TestStart(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectControllerConfig()

	w, err := Manifold(s.getConfig()).Start(c.Context(), s.newGetter())
	c.Assert(err, tc.ErrorIsNil)
	workertest.CleanKill(c, w)
}

func (s *manifoldSuite) TestStartConfig(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectControllerConfig()

	logDir := c.MkDir()
	cfg := s.getConfig()
	cfg.LogDir = logDir
	cfg.NewWorker = func(_ ControllerConfigService, _ auditlog.Config, logFactory AuditLogFactory) (worker.Worker, error) {
		auditLog := logFactory(auditlog.Config{})
		c.Assert(auditLog.Close(), tc.ErrorIsNil)
		info, err := os.Stat(filepath.Join(logDir, "audit.log"))
		c.Assert(err, tc.ErrorIsNil)
		c.Check(info.IsDir(), tc.IsFalse)
		return newStubWorker(), nil
	}

	w, err := Manifold(cfg).Start(c.Context(), s.newGetter())
	c.Assert(err, tc.ErrorIsNil)
	workertest.CleanKill(c, w)
}

func (s *manifoldSuite) getConfig() ManifoldConfig {
	return ManifoldConfig{
		LogDir:             "log-dir",
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
	resources := map[string]any{
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
