// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package externalcontrollerupdater_test

import (
	"context"
	"testing"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/tc"

	"github.com/juju/juju/api"
	"github.com/juju/juju/internal/testhelpers"
	"github.com/juju/juju/internal/worker/externalcontrollerupdater"
)

type ManifoldConfigSuite struct {
	testhelpers.IsolationSuite
	config externalcontrollerupdater.ManifoldConfig
}

func TestManifoldConfigSuite(t *testing.T) {
	tc.Run(t, &ManifoldConfigSuite{})
}

func (s *ManifoldConfigSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.config = s.validConfig()
}

func (s *ManifoldConfigSuite) validConfig() externalcontrollerupdater.ManifoldConfig {
	return externalcontrollerupdater.ManifoldConfig{
		DomainServicesName: "domain-services",
		Clock:              clock.WallClock,
		NewExternalControllerWatcherClient: func(context.Context, *api.Info) (externalcontrollerupdater.ExternalControllerWatcherClientCloser, string, error) {
			panic("should not be called")
		},
	}
}

func (s *ManifoldConfigSuite) TestValid(c *tc.C) {
	c.Check(s.config.Validate(), tc.ErrorIsNil)
}

func (s *ManifoldConfigSuite) TestMissingDomainServicesName(c *tc.C) {
	s.config.DomainServicesName = ""
	s.checkNotValid(c, "empty DomainServicesName not valid")
}

func (s *ManifoldConfigSuite) TestMissingNewExternalControllerWatcherClient(c *tc.C) {
	s.config.NewExternalControllerWatcherClient = nil
	s.checkNotValid(c, "nil NewExternalControllerWatcherClient not valid")
}

func (s *ManifoldConfigSuite) TestNilClock(c *tc.C) {
	s.config.Clock = nil
	s.checkNotValid(c, "nil Clock not valid")
}

func (s *ManifoldConfigSuite) checkNotValid(c *tc.C, expect string) {
	err := s.config.Validate()
	c.Check(err, tc.ErrorMatches, expect)
	c.Check(err, tc.ErrorIs, errors.NotValid)
}
