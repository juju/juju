// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controllerlogger_test

import (
	"context"
	"testing"

	"github.com/juju/errors"
	"github.com/juju/loggo/v3"
	"github.com/juju/names/v6"
	"github.com/juju/tc"
	dt "github.com/juju/worker/v5/dependency/testing"

	internallogger "github.com/juju/juju/internal/logger"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/worker/controllerlogger"
)

type ManifoldSuite struct {
	testhelpers.IsolationSuite

	config controllerlogger.ManifoldConfig
}

func TestManifoldSuite(t *testing.T) {
	tc.Run(t, &ManifoldSuite{})
}

func (s *ManifoldSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.config = validManifoldConfig(c)
}

func validManifoldConfig(c *tc.C) controllerlogger.ManifoldConfig {
	return controllerlogger.ManifoldConfig{
		DomainServicesName: "domain-services",
		LoggerContext:      internallogger.WrapLoggoContext(loggo.NewContext(loggo.DEBUG)),
		Logger:             loggertesting.WrapCheckLog(c),
		Tag:                names.NewControllerAgentTag("0"),
		LoggingOverride:    "",
		UpdateAgentFunc:    func(string) error { return nil },
	}
}

func (s *ManifoldSuite) TestValidateEmptyDomainServicesName(c *tc.C) {
	s.config.DomainServicesName = ""
	err := s.config.Validate()
	c.Assert(err, tc.ErrorIs, errors.NotValid)
}

func (s *ManifoldSuite) TestValidateNilLoggerContext(c *tc.C) {
	s.config.LoggerContext = nil
	err := s.config.Validate()
	c.Assert(err, tc.ErrorIs, errors.NotValid)
}

func (s *ManifoldSuite) TestValidateNilLogger(c *tc.C) {
	s.config.Logger = nil
	err := s.config.Validate()
	c.Assert(err, tc.ErrorIs, errors.NotValid)
}

func (s *ManifoldSuite) TestValidateNilTag(c *tc.C) {
	s.config.Tag = nil
	err := s.config.Validate()
	c.Assert(err, tc.ErrorIs, errors.NotValid)
}

func (s *ManifoldSuite) TestValidateSuccess(c *tc.C) {
	err := s.config.Validate()
	c.Assert(err, tc.ErrorIsNil)
}

func (s *ManifoldSuite) TestInputs(c *tc.C) {
	manifold := controllerlogger.Manifold(s.config)
	c.Assert(manifold.Inputs, tc.DeepEquals, []string{"domain-services"})
}

func (s *ManifoldSuite) TestStartMissingDomainServicesDependency(c *tc.C) {
	manifold := controllerlogger.Manifold(s.config)
	getter := dt.StubGetter(map[string]any{})
	_, err := manifold.Start(context.Background(), getter)
	c.Assert(err, tc.ErrorMatches, "unexpected resource name: domain-services")
}
