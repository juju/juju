// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package externalcontrollerupdater_test

import (
	"github.com/juju/errors"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api"
	"github.com/juju/juju/internal/worker/externalcontrollerupdater"
)

type ManifoldConfigSuite struct {
	testing.IsolationSuite
	config externalcontrollerupdater.ManifoldConfig
}

var _ = gc.Suite(&ManifoldConfigSuite{})

func (s *ManifoldConfigSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
	s.config = s.validConfig()
}

func (s *ManifoldConfigSuite) validConfig() externalcontrollerupdater.ManifoldConfig {
	return externalcontrollerupdater.ManifoldConfig{
		APICallerName: "api-caller",
		NewExternalControllerWatcherClient: func(*api.Info) (externalcontrollerupdater.ExternalControllerWatcherClientCloser, string, error) {
			panic("should not be called")
		},
	}
}

func (s *ManifoldConfigSuite) TestValid(c *gc.C) {
	c.Check(s.config.Validate(), jc.ErrorIsNil)
}

func (s *ManifoldConfigSuite) TestMissingAPICallerName(c *gc.C) {
	s.config.APICallerName = ""
	s.checkNotValid(c, "empty APICallerName not valid")
}

func (s *ManifoldConfigSuite) TestMissingNewExternalControllerWatcherClient(c *gc.C) {
	s.config.NewExternalControllerWatcherClient = nil
	s.checkNotValid(c, "nil NewExternalControllerWatcherClient not valid")
}

func (s *ManifoldConfigSuite) checkNotValid(c *gc.C, expect string) {
	err := s.config.Validate()
	c.Check(err, gc.ErrorMatches, expect)
	c.Check(err, jc.Satisfies, errors.IsNotValid)
}
