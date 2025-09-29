// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package remoterelationconsumer

import (
	"testing"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/juju/worker/v4"
	"github.com/juju/worker/v4/dependency"

	modeltesting "github.com/juju/juju/core/model/testing"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/worker/apiremoterelationcaller"
)

type ManifoldConfigSuite struct {
	testhelpers.IsolationSuite
	config ManifoldConfig
}

func TestManifoldConfigSuite(t *testing.T) {
	tc.Run(t, &ManifoldConfigSuite{})
}

func (s *ManifoldConfigSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.config = s.validConfig(c)
}

func (s *ManifoldConfigSuite) validConfig(c *tc.C) ManifoldConfig {
	return ManifoldConfig{
		ModelUUID:                   modeltesting.GenModelUUID(c),
		APICallerName:               "api-caller",
		APIRemoteRelationCallerName: "api-remote-relation-caller",
		DomainServicesName:          "domain-services",
		GetCrossModelServices: func(getter dependency.Getter, domainServicesName string) (CrossModelService, error) {
			return nil, nil
		},
		NewRemoteRelationClientGetter: func(acg apiremoterelationcaller.APIRemoteCallerGetter) RemoteRelationClientGetter {
			return nil
		},
		NewWorker: func(Config) (worker.Worker, error) {
			return nil, nil
		},
		NewRemoteApplicationWorker: func(rac RemoteApplicationConfig) (ReportableWorker, error) {
			return nil, nil
		},
		Clock:  clock.WallClock,
		Logger: loggertesting.WrapCheckLog(c),
	}
}

func (s *ManifoldConfigSuite) TestValid(c *tc.C) {
	c.Check(s.config.Validate(), tc.ErrorIsNil)
}

func (s *ManifoldConfigSuite) TestMissingModelUUID(c *tc.C) {
	s.config.ModelUUID = ""
	s.checkNotValid(c, "empty ModelUUID not valid")
}

func (s *ManifoldConfigSuite) TestMissingAPICallerName(c *tc.C) {
	s.config.APICallerName = ""
	s.checkNotValid(c, "empty APICallerName not valid")
}

func (s *ManifoldConfigSuite) TestMissingAPIRemoteRelationCallerName(c *tc.C) {
	s.config.APIRemoteRelationCallerName = ""
	s.checkNotValid(c, "empty APIRemoteRelationCallerName not valid")
}

func (s *ManifoldConfigSuite) TestMissingDomainServicesName(c *tc.C) {
	s.config.DomainServicesName = ""
	s.checkNotValid(c, "empty DomainServicesName not valid")
}

func (s *ManifoldConfigSuite) TestMissingNewRemoteRelationsFacade(c *tc.C) {
	s.config.NewRemoteRelationClientGetter = nil
	s.checkNotValid(c, "nil NewRemoteRelationClientGetter not valid")
}

func (s *ManifoldConfigSuite) TestMissingNewWorker(c *tc.C) {
	s.config.NewWorker = nil
	s.checkNotValid(c, "nil NewWorker not valid")
}

func (s *ManifoldConfigSuite) TestMissingNewRemoteRelationClientGetter(c *tc.C) {
	s.config.NewRemoteRelationClientGetter = nil
	s.checkNotValid(c, "nil NewRemoteRelationClientGetter not valid")
}

func (s *ManifoldConfigSuite) TestMissingGetCrossModelServices(c *tc.C) {
	s.config.GetCrossModelServices = nil
	s.checkNotValid(c, "nil GetCrossModelServices not valid")
}

func (s *ManifoldConfigSuite) TestMissingClock(c *tc.C) {
	s.config.Clock = nil
	s.checkNotValid(c, "nil Clock not valid")
}

func (s *ManifoldConfigSuite) TestMissingLogger(c *tc.C) {
	s.config.Logger = nil
	s.checkNotValid(c, "nil Logger not valid")
}

func (s *ManifoldConfigSuite) checkNotValid(c *tc.C, expect string) {
	err := s.config.Validate()
	c.Check(err, tc.ErrorMatches, expect)
	c.Check(err, tc.ErrorIs, errors.NotValid)
}
