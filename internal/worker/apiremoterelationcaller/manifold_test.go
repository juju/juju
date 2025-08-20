// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiremoterelationcaller

import (
	context "context"
	"testing"

	"github.com/juju/clock"
	names "github.com/juju/names/v6"
	"github.com/juju/tc"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"
	dependencytesting "github.com/juju/worker/v4/dependency/testing"
	"github.com/juju/worker/v4/workertest"
	"go.uber.org/goleak"
	gomock "go.uber.org/mock/gomock"

	"github.com/juju/juju/api"
	"github.com/juju/juju/core/crossmodel"
	"github.com/juju/juju/core/errors"
	"github.com/juju/juju/core/logger"
	"github.com/juju/juju/core/model"
	"github.com/juju/juju/core/network"
)

type manifoldSuite struct {
	baseSuite
}

func TestManifoldSuite(t *testing.T) {
	defer goleak.VerifyNone(t)
	tc.Run(t, &manifoldSuite{})
}

func (s *manifoldSuite) TestValidateConfig(c *tc.C) {
	defer s.setupMocks(c).Finish()

	cfg := s.getConfig(c)
	c.Check(cfg.Validate(), tc.ErrorIsNil)

	cfg = s.getConfig(c)
	cfg.DomainServicesName = ""
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.getConfig(c)
	cfg.NewWorker = nil
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)

	cfg = s.getConfig(c)
	cfg.Logger = nil
	c.Check(cfg.Validate(), tc.ErrorIs, errors.NotValid)
}

func (s *manifoldSuite) getConfig(c *tc.C) ManifoldConfig {
	return ManifoldConfig{
		DomainServicesName: "domain-services",
		NewAPIInfoGetter: func(DomainServicesGetter) APIInfoGetter {
			return nil
		},
		NewConnectionGetter: func(DomainServicesGetter, logger.Logger) ConnectionGetter {
			return nil
		},
		GetDomainServicesGetterFunc: func(getter dependency.Getter, name string) (DomainServicesGetter, error) {
			return nil, nil
		},
		NewWorker: func(config Config) (worker.Worker, error) {
			return workertest.NewErrorWorker(nil), nil
		},
		Logger: s.logger,
		Clock:  clock.WallClock,
	}
}

func (s *manifoldSuite) newGetter() dependency.Getter {
	resources := map[string]any{
		"domain-services": struct{}{},
	}
	return dependencytesting.StubGetter(resources)
}

var expectedInputs = []string{"domain-services"}

func (s *manifoldSuite) TestInputs(c *tc.C) {
	c.Assert(Manifold(s.getConfig(c)).Inputs, tc.SameContents, expectedInputs)
}

func (s *manifoldSuite) TestStart(c *tc.C) {
	defer s.setupMocks(c).Finish()

	w, err := Manifold(s.getConfig(c)).Start(c.Context(), s.newGetter())
	c.Assert(err, tc.ErrorIsNil)
	workertest.CleanKill(c, w)
}

type connectionSuite struct {
	baseSuite
}

func TestConnectionSuite(t *testing.T) {
	defer goleak.VerifyNone(t)
	tc.Run(t, &connectionSuite{})
}

func (s *connectionSuite) TestGetConnectionForModel(c *tc.C) {
	defer s.setupMocks(c).Finish()

	getter := s.newConnectionGetter(c, func(apiInfo *api.Info) (api.Connection, error) {
		c.Assert(apiInfo.Tag, tc.Equals, connectionTag)
		return s.connection, nil
	})
	conn, err := getter.GetConnectionForModel(c.Context(), model.UUID("test-model-uuid"), api.Info{
		Tag: names.NewUserTag("test-tag"),
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(conn == s.connection, tc.IsTrue)
}

func (s *connectionSuite) TestGetConnectionForModelWithError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	getter := s.newConnectionGetter(c, func(apiInfo *api.Info) (api.Connection, error) {
		return nil, errors.NotFound
	})
	_, err := getter.GetConnectionForModel(c.Context(), model.UUID("test-model-uuid"), api.Info{
		Tag: names.NewUserTag("test-tag"),
	})
	c.Assert(err, tc.ErrorIs, errors.NotFound)
}

func (s *connectionSuite) TestGetConnectionForModelWithRedirectError(c *tc.C) {
	defer s.setupMocks(c).Finish()

	modelUUID := model.UUID("test-model-uuid")

	s.domainServicesGetter.EXPECT().ServicesForModel(gomock.Any(), modelUUID).Return(s.domainServices, nil)
	s.domainServices.EXPECT().ExternalController().Return(s.externalController)
	s.externalController.EXPECT().UpdateExternalController(gomock.Any(), crossmodel.ControllerInfo{
		Alias:          "test-controller-alias",
		Addrs:          []string{"7.7.7.7:1234"},
		CACert:         "test-ca-cert",
		ControllerUUID: "f47ac10b-58cc-4372-a567-0e02b2c3d479",
	})

	var called uint64
	getter := s.newConnectionGetter(c, func(apiInfo *api.Info) (api.Connection, error) {
		defer func() { called++ }()

		c.Assert(apiInfo.Tag, tc.Equals, connectionTag)

		if called == 0 {
			return nil, &api.RedirectError{
				Servers: []network.MachineHostPorts{
					network.NewMachineHostPorts(1234, "7.7.7.7"),
				},
				ControllerTag:   names.NewControllerTag("f47ac10b-58cc-4372-a567-0e02b2c3d479"),
				ControllerAlias: "test-controller-alias",
				CACert:          "test-ca-cert",
			}
		}

		// Ensure we followed the redirect and created a new connection.
		c.Assert(apiInfo.Addrs, tc.DeepEquals, []string{"7.7.7.7:1234"})

		return s.connection, nil
	})
	conn, err := getter.GetConnectionForModel(c.Context(), modelUUID, api.Info{
		Tag: names.NewUserTag("test-tag"),
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(conn == s.connection, tc.IsTrue)
}

func (s *connectionSuite) TestGetConnectionForModelWithRedirectErrorFailsToFindModel(c *tc.C) {
	defer s.setupMocks(c).Finish()

	modelUUID := model.UUID("test-model-uuid")

	s.domainServicesGetter.EXPECT().ServicesForModel(gomock.Any(), modelUUID).Return(s.domainServices, errors.NotFound)

	var called uint64
	getter := s.newConnectionGetter(c, func(apiInfo *api.Info) (api.Connection, error) {
		defer func() { called++ }()

		c.Assert(apiInfo.Tag, tc.Equals, connectionTag)

		if called == 0 {
			return nil, &api.RedirectError{
				Servers: []network.MachineHostPorts{
					network.NewMachineHostPorts(1234, "7.7.7.7"),
				},
				ControllerTag:   names.NewControllerTag("f47ac10b-58cc-4372-a567-0e02b2c3d479"),
				ControllerAlias: "test-controller-alias",
				CACert:          "test-ca-cert",
			}
		}

		// Ensure we followed the redirect and created a new connection.
		c.Assert(apiInfo.Addrs, tc.DeepEquals, []string{"7.7.7.7:1234"})

		return s.connection, nil
	})
	conn, err := getter.GetConnectionForModel(c.Context(), modelUUID, api.Info{
		Tag: names.NewUserTag("test-tag"),
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(conn == s.connection, tc.IsTrue)
}

func (s *connectionSuite) TestGetConnectionForModelWithRedirectErrorFailsUpdate(c *tc.C) {
	defer s.setupMocks(c).Finish()

	modelUUID := model.UUID("test-model-uuid")

	s.domainServicesGetter.EXPECT().ServicesForModel(gomock.Any(), modelUUID).Return(s.domainServices, nil)
	s.domainServices.EXPECT().ExternalController().Return(s.externalController)
	s.externalController.EXPECT().UpdateExternalController(gomock.Any(), crossmodel.ControllerInfo{
		Alias:          "test-controller-alias",
		Addrs:          []string{"7.7.7.7:1234"},
		CACert:         "test-ca-cert",
		ControllerUUID: "f47ac10b-58cc-4372-a567-0e02b2c3d479",
	}).Return(errors.NotFound)

	var called uint64
	getter := s.newConnectionGetter(c, func(apiInfo *api.Info) (api.Connection, error) {
		defer func() { called++ }()

		c.Assert(apiInfo.Tag, tc.Equals, connectionTag)

		if called == 0 {
			return nil, &api.RedirectError{
				Servers: []network.MachineHostPorts{
					network.NewMachineHostPorts(1234, "7.7.7.7"),
				},
				ControllerTag:   names.NewControllerTag("f47ac10b-58cc-4372-a567-0e02b2c3d479"),
				ControllerAlias: "test-controller-alias",
				CACert:          "test-ca-cert",
			}
		}

		// Ensure we followed the redirect and created a new connection.
		c.Assert(apiInfo.Addrs, tc.DeepEquals, []string{"7.7.7.7:1234"})

		return s.connection, nil
	})
	conn, err := getter.GetConnectionForModel(c.Context(), modelUUID, api.Info{
		Tag: names.NewUserTag("test-tag"),
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(conn == s.connection, tc.IsTrue)
}

func (s *connectionSuite) newConnectionGetter(c *tc.C, fn func(*api.Info) (api.Connection, error)) *connectionGetter {
	return &connectionGetter{
		domainServicesGetter: s.domainServicesGetter,
		newConnection: func(ctx context.Context, apiInfo *api.Info) (api.Connection, error) {
			return fn(apiInfo)
		},
		logger: s.logger,
	}
}
