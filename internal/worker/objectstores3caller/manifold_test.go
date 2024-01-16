// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package objectstores3caller

import (
	context "context"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/worker/v4/dependency"
	dependencytesting "github.com/juju/worker/v4/dependency/testing"
	"github.com/juju/worker/v4/workertest"
	gomock "go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	controller "github.com/juju/juju/controller"
	"github.com/juju/juju/core/objectstore"
	servicefactorytesting "github.com/juju/juju/domain/servicefactory/testing"
	"github.com/juju/juju/internal/s3client"
	"github.com/juju/juju/internal/servicefactory"
	"github.com/juju/juju/testing"
)

type manifoldSuite struct {
	baseSuite
}

var _ = gc.Suite(&manifoldSuite{})

func (s *manifoldSuite) TestValidateConfig(c *gc.C) {
	defer s.setupMocks(c).Finish()

	cfg := s.getConfig()
	c.Check(cfg.Validate(), jc.ErrorIsNil)

	cfg = s.getConfig()
	cfg.ServiceFactoryName = ""
	c.Check(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	cfg = s.getConfig()
	cfg.HTTPClientName = ""
	c.Check(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	cfg = s.getConfig()
	cfg.NewClient = nil
	c.Check(cfg.Validate(), jc.ErrorIs, errors.NotValid)

	cfg = s.getConfig()
	cfg.Logger = nil
	c.Check(cfg.Validate(), jc.ErrorIs, errors.NotValid)
}

func (s *manifoldSuite) newGetter() dependency.Getter {
	resources := map[string]any{
		"http-client":     s.httpClient,
		"service-factory": servicefactorytesting.NewTestingServiceFactory(),
	}
	return dependencytesting.StubGetter(resources)
}

func (s *manifoldSuite) getConfig() ManifoldConfig {
	return ManifoldConfig{
		HTTPClientName:     "http-client",
		ServiceFactoryName: "service-factory",
		NewClient: func(string, s3client.HTTPClient, s3client.Credentials, s3client.Logger) (objectstore.Session, error) {
			return s.session, nil
		},
		Logger: s.logger,
		Clock:  s.clock,
		GetService: func(string, dependency.Getter, func(servicefactory.ControllerServiceFactory) ControllerService) (ControllerService, error) {
			return s.controllerService, nil
		},
	}
}

var expectedInputs = []string{"http-client", "service-factory"}

func (s *manifoldSuite) TestInputs(c *gc.C) {
	c.Assert(Manifold(s.getConfig()).Inputs, jc.SameContents, expectedInputs)
}

func (s *manifoldSuite) TestStart(c *gc.C) {
	defer s.setupMocks(c).Finish()

	s.expectControllerConfig(c, testing.FakeControllerConfig())

	_, err := Manifold(s.getConfig()).Start(context.Background(), s.newGetter())
	c.Assert(err, jc.ErrorIs, dependency.ErrUninstall)
}

func (s *manifoldSuite) TestStartS3Backend(c *gc.C) {
	defer s.setupMocks(c).Finish()

	config := testing.FakeControllerConfig()
	config[controller.ObjectStoreType] = objectstore.S3Backend

	s.expectControllerConfig(c, config)

	w, err := Manifold(s.getConfig()).Start(context.Background(), s.newGetter())
	c.Assert(err, jc.ErrorIsNil)

	workertest.CleanKill(c, w)
}

func (s *manifoldSuite) TestOutput(c *gc.C) {
	defer s.setupMocks(c).Finish()

	manifold := Manifold(s.getConfig())
	w, err := manifold.Start(context.Background(), s.newGetter())
	c.Assert(err, jc.ErrorIsNil)
	defer workertest.DirtyKill(c, w)

	var client objectstore.Client
	err = manifold.Output(w, &client)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(client, gc.Equals, s.session)

	workertest.CleanKill(c, w)
}

func (s *manifoldSuite) expectControllerConfig(c *gc.C, config controller.Config) {
	s.controllerService.EXPECT().ControllerConfig(gomock.Any()).Return(config, nil)
}
