// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package migrationmaster_test

import (
	"testing"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/juju/worker/v5"

	"github.com/juju/juju/internal/services"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/worker/migrationmaster"
)

type ManifoldConfigSuite struct {
	testhelpers.IsolationSuite
	config migrationmaster.ManifoldConfig
}

func TestManifoldConfigSuite(t *testing.T) {
	tc.Run(t, &ManifoldConfigSuite{})
}

func (s *ManifoldConfigSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.config = s.validConfig()
}

func (s *ManifoldConfigSuite) validConfig() migrationmaster.ManifoldConfig {
	return migrationmaster.ManifoldConfig{
		DomainServicesName:   "domain-services",
		DomainServicesGetter: struct{ services.DomainServicesGetter }{},
		FortressName:         "fortress",
		ModelUUID:            "model-uuid",
		LogDir:               "/tmp/logs",
		Clock:                struct{ clock.Clock }{},
		NewWorker:            func(migrationmaster.Config) (worker.Worker, error) { return nil, nil },
	}
}

func (s *ManifoldConfigSuite) TestValid(c *tc.C) {
	c.Check(s.config.Validate(), tc.ErrorIsNil)
}

func (s *ManifoldConfigSuite) TestMissingModelUUID(c *tc.C) {
	s.config.ModelUUID = ""
	s.checkNotValid(c, "empty ModelUUID not valid")
}

func (s *ManifoldConfigSuite) TestMissingDomainServicesName(c *tc.C) {
	s.config.DomainServicesName = ""
	s.checkNotValid(c, "empty DomainServicesName not valid")
}

func (s *ManifoldConfigSuite) TestMissingDomainServicesGetter(c *tc.C) {
	s.config.DomainServicesGetter = nil
	s.checkNotValid(c, "nil DomainServicesGetter not valid")
}

func (s *ManifoldConfigSuite) TestMissingFortressName(c *tc.C) {
	s.config.FortressName = ""
	s.checkNotValid(c, "empty FortressName not valid")
}

func (s *ManifoldConfigSuite) TestMissingLogDir(c *tc.C) {
	s.config.LogDir = ""
	s.checkNotValid(c, "empty LogDir not valid")
}

func (s *ManifoldConfigSuite) TestMissingClock(c *tc.C) {
	s.config.Clock = nil
	s.checkNotValid(c, "nil Clock not valid")
}

func (s *ManifoldConfigSuite) TestMissingNewWorker(c *tc.C) {
	s.config.NewWorker = nil
	s.checkNotValid(c, "nil NewWorker not valid")
}

func (s *ManifoldConfigSuite) checkNotValid(c *tc.C, expect string) {
	err := s.config.Validate()
	c.Check(err, tc.ErrorMatches, expect)
	c.Check(err, tc.ErrorIs, errors.NotValid)
}
