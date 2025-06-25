// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package certupdater

import (
	"testing"

	"github.com/juju/tc"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"
	dt "github.com/juju/worker/v4/dependency/testing"
	"go.uber.org/goleak"
	"go.uber.org/mock/gomock"

	"github.com/juju/juju/core/errors"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/pki"
	pkitest "github.com/juju/juju/internal/pki/test"
	"github.com/juju/juju/internal/testhelpers"
)

type manifoldConfigSuite struct {
	testhelpers.IsolationSuite

	config ManifoldConfig
}

func TestManifoldConfigSuite(t *testing.T) {
	defer goleak.VerifyNone(t)
	tc.Run(t, &manifoldConfigSuite{})
}

func (s *manifoldConfigSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)

	s.config = validConfig(c)
}

func (s *manifoldConfigSuite) TestMissingDomainServicesName(c *tc.C) {
	s.config.DomainServicesName = ""
	s.checkNotValid(c, "empty DomainServicesName not valid")
}

func (s *manifoldConfigSuite) TestMissingAuthorityName(c *tc.C) {
	s.config.AuthorityName = ""
	s.checkNotValid(c, "empty AuthorityName not valid")
}

func (s *manifoldConfigSuite) TestMissingGetControllerDomainServices(c *tc.C) {
	s.config.GetControllerDomainServices = nil
	s.checkNotValid(c, "nil GetControllerDomainServices not valid")
}

func (s *manifoldConfigSuite) TestMissingNewWorker(c *tc.C) {
	s.config.NewWorker = nil
	s.checkNotValid(c, "nil NewWorker not valid")
}

func (s *manifoldConfigSuite) TestMissingLogger(c *tc.C) {
	s.config.Logger = nil
	s.checkNotValid(c, "nil Logger not valid")
}

func (s *manifoldConfigSuite) checkNotValid(c *tc.C, expect string) {
	err := s.config.Validate()
	c.Check(err, tc.ErrorMatches, expect)
	c.Check(err, tc.ErrorIs, errors.NotValid)
}

func validConfig(c *tc.C) ManifoldConfig {
	return ManifoldConfig{
		AuthorityName:               "authority-name",
		DomainServicesName:          "domain-services",
		GetControllerDomainServices: GetControllerDomainServices,
		NewWorker:                   func(Config) (worker.Worker, error) { return noWorker{}, nil },
		Logger:                      loggertesting.WrapCheckLog(c),
	}
}

type manifoldSuite struct {
	testhelpers.IsolationSuite

	authority      pki.Authority
	getter         dependency.Getter
	domainServices *MockControllerDomainServices
	controllerNode *MockControllerNodeService
}

func TestManifoldSuite(t *testing.T) {
	defer goleak.VerifyNone(t)
	tc.Run(t, &manifoldSuite{})
}

func (s *manifoldSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.controllerNode = NewMockControllerNodeService(ctrl)
	s.domainServices = NewMockControllerDomainServices(ctrl)

	c.Cleanup(func() {
		s.controllerNode = nil
		s.domainServices = nil
	})

	return ctrl
}

func (s *manifoldSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)

	authority, err := pkitest.NewTestAuthority()
	c.Assert(err, tc.ErrorIsNil)
	s.authority = authority

	s.getter = s.newGetter(nil)

	c.Cleanup(func() {
		s.getter = nil
		s.authority = nil
	})
}

func (s *manifoldSuite) newGetter(overlay map[string]any) dependency.Getter {
	resources := map[string]any{
		"authority": s.authority,
	}
	for k, v := range overlay {
		resources[k] = v
	}
	return dt.StubGetter(resources)
}

func (s *manifoldSuite) newManifold(c *tc.C) dependency.Manifold {
	cfg := ManifoldConfig{
		AuthorityName:      "authority",
		DomainServicesName: "domain-services",
		GetControllerDomainServices: func(getter dependency.Getter, name string) (ControllerDomainServices, error) {
			return s.domainServices, nil
		},
		NewWorker: func(cfg Config) (worker.Worker, error) {
			if err := cfg.Validate(); err != nil {
				return nil, err
			}
			return noWorker{}, nil
		},
		Logger: loggertesting.WrapCheckLog(c),
	}
	return Manifold(cfg)
}

var expectedInputs = []string{"authority", "domain-services"}

func (s *manifoldSuite) TestInputs(c *tc.C) {
	c.Assert(s.newManifold(c).Inputs, tc.SameContents, expectedInputs)
}

func (s *manifoldSuite) TestStart(c *tc.C) {
	defer s.setupMocks(c).Finish()

	// Arrange
	s.domainServices.EXPECT().ControllerNode().Return(s.controllerNode)
	manifold := s.newManifold(c)

	// Act
	w, err := manifold.Start(c.Context(), s.getter)

	// Assert
	c.Assert(err, tc.ErrorIsNil)
	c.Check(w, tc.NotNil)
}

type noWorker struct {
	worker.Worker
}
