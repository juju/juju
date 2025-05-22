// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package objectstores3caller

import (
	"context"
	stdtesting "testing"

	"github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"
	dependencytesting "github.com/juju/worker/v4/dependency/testing"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/goleak"

	"github.com/juju/juju/controller"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/objectstore"
	"github.com/juju/juju/internal/s3client"
	"github.com/juju/juju/internal/testing"
)

type manifoldSuite struct {
	baseSuite
}

func TestManifoldSuite(t *stdtesting.T) {
	defer goleak.VerifyNone(t)
	tc.Run(t, &manifoldSuite{})
}

func (s *manifoldSuite) TestValidateConfig(c *tc.C) {
	defer s.setupMocks(c).Finish()

	cfg := s.getConfig()
	c.Check(cfg.Validate(), tc.ErrorIsNil)

	cfg = s.getConfig()
	cfg.ObjectStoreServicesName = ""
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.getConfig()
	cfg.HTTPClientName = ""
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.getConfig()
	cfg.NewClient = nil
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.getConfig()
	cfg.Logger = nil
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)
}

func (s *manifoldSuite) newGetter() dependency.Getter {
	resources := map[string]any{
		"http-client":           s.httpClientGetter,
		"object-store-services": s.domainServices,
	}
	return dependencytesting.StubGetter(resources)
}

func (s *manifoldSuite) getConfig() ManifoldConfig {
	return ManifoldConfig{
		HTTPClientName:          "http-client",
		ObjectStoreServicesName: "object-store-services",
		NewClient: func(string, s3client.HTTPClient, s3client.Credentials, logger.Logger) (objectstore.Session, error) {
			return s.session, nil
		},
		Logger: s.logger,
		Clock:  s.clock,
		GetControllerConfigService: func(getter dependency.Getter, name string) (ControllerConfigService, error) {
			return s.controllerConfigService, nil
		},
		NewWorker: func(cfg workerConfig) (worker.Worker, error) {
			return newWorker(cfg, s.states)
		},
	}
}

var expectedInputs = []string{"http-client", "object-store-services"}

func (s *manifoldSuite) TestInputs(c *tc.C) {
	c.Assert(Manifold(s.getConfig()).Inputs, tc.SameContents, expectedInputs)
}

func (s *manifoldSuite) TestStart(c *tc.C) {
	defer s.setupMocks(c).Finish()

	s.expectControllerConfig(c, testing.FakeControllerConfig())

	w, err := Manifold(s.getConfig()).Start(c.Context(), s.newGetter())
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.DirtyKill(c, w)

	// This should still be a worker, just a useless one.
	nw, ok := w.(*noopWorker)
	c.Assert(ok, tc.IsTrue)
	err = nw.Session(c.Context(), func(context.Context, objectstore.Session) error {
		c.Fatalf("unexpected call to Session")
		return nil
	})
	c.Assert(err, tc.ErrorIs, errors.NotSupported)
}

func (s *manifoldSuite) TestStartS3Backend(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Expect that the controller config has started with the S3 backend. and
	// that the manifold has started the worker.

	// The controller config will be called twice, once to get the initial
	// config in the manifold and once more when creating the initial session.

	config := testing.FakeControllerConfig()
	config[controller.ObjectStoreType] = string(objectstore.S3Backend)

	s.expectControllerConfig(c, config)
	s.expectControllerConfig(c, config)
	s.expectControllerConfigWatch(c)
	s.expectHTTPClient(c)

	w, err := Manifold(s.getConfig()).Start(c.Context(), s.newGetter())
	c.Assert(err, tc.ErrorIsNil)

	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c)

	workertest.CleanKill(c, w)
}

func (s *manifoldSuite) TestOutput(c *tc.C) {
	defer s.setupMocks(c).Finish()

	config := testing.FakeControllerConfig()
	config[controller.ObjectStoreType] = string(objectstore.S3Backend)

	s.expectClock()
	s.expectControllerConfig(c, config)
	s.expectControllerConfig(c, config)
	s.expectControllerConfigWatch(c)
	s.expectHTTPClient(c)

	manifold := Manifold(s.getConfig())
	w, err := manifold.Start(c.Context(), s.newGetter())
	c.Assert(err, tc.ErrorIsNil)
	defer workertest.DirtyKill(c, w)

	s.ensureStartup(c)

	var client objectstore.Client
	err = manifold.Output(w, &client)
	c.Assert(err, tc.ErrorIsNil)

	c.Check(client, tc.NotNil)

	var session objectstore.Session
	err = client.Session(c.Context(), func(ctx context.Context, s objectstore.Session) error {
		session = s
		return nil
	})
	c.Assert(err, tc.ErrorIsNil)

	c.Check(session, tc.Equals, s.session)

	workertest.CleanKill(c, w)
}
