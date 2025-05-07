// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package externalcontrollerupdater_test

import (
	"context"

	"github.com/juju/errors"
	"github.com/juju/tc"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"

	"github.com/juju/juju/api"
	"github.com/juju/juju/internal/worker/externalcontrollerupdater"
)

type ManifoldConfigSuite struct {
	testing.IsolationSuite
	config externalcontrollerupdater.ManifoldConfig
}

var _ = tc.Suite(&ManifoldConfigSuite{})

func (s *ManifoldConfigSuite) SetUpTest(c *tc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.config = s.validConfig()
}

func (s *ManifoldConfigSuite) validConfig() externalcontrollerupdater.ManifoldConfig {
	return externalcontrollerupdater.ManifoldConfig{
		APICallerName: "api-caller",
		NewExternalControllerWatcherClient: func(context.Context, *api.Info) (externalcontrollerupdater.ExternalControllerWatcherClientCloser, error) {
			panic("should not be called")
		},
	}
}

func (s *ManifoldConfigSuite) TestValid(c *tc.C) {
	c.Check(s.config.Validate(), jc.ErrorIsNil)
}

func (s *ManifoldConfigSuite) TestMissingAPICallerName(c *tc.C) {
	s.config.APICallerName = ""
	s.checkNotValid(c, "empty APICallerName not valid")
}

func (s *ManifoldConfigSuite) TestMissingNewExternalControllerWatcherClient(c *tc.C) {
	s.config.NewExternalControllerWatcherClient = nil
	s.checkNotValid(c, "nil NewExternalControllerWatcherClient not valid")
}

func (s *ManifoldConfigSuite) checkNotValid(c *tc.C, expect string) {
	err := s.config.Validate()
	c.Check(err, tc.ErrorMatches, expect)
	c.Check(err, jc.ErrorIs, errors.NotValid)
}
