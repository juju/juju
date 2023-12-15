// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasunitsmanager_test

import (
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/errors"
	"github.com/juju/loggo"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/internal/worker/caasunitsmanager"
	"github.com/juju/juju/internal/worker/caasunitsmanager/mocks"
)

type manifoldSuite struct {
	config caasunitsmanager.ManifoldConfig
}

var _ = gc.Suite(&manifoldSuite{})

func (s *manifoldSuite) SetUpTest(c *gc.C) {
	s.config = caasunitsmanager.ManifoldConfig{
		Clock:  testclock.NewClock(time.Now()),
		Logger: loggo.GetLogger("test"),
		Hub:    mocks.NewMockHub(gomock.NewController(c)),
	}
}

func (s *manifoldSuite) TestConfigValidation(c *gc.C) {
	err := s.config.Validate()
	c.Assert(err, jc.ErrorIsNil)
}

func (s *manifoldSuite) TestConfigValidationMissingClock(c *gc.C) {
	s.config.Clock = nil
	err := s.config.Validate()
	c.Check(err, jc.ErrorIs, errors.NotValid)
	c.Check(err, gc.ErrorMatches, "missing Clock not valid")
}

func (s *manifoldSuite) TestConfigValidationMissingLogger(c *gc.C) {
	s.config.Logger = nil
	err := s.config.Validate()
	c.Check(err, jc.ErrorIs, errors.NotValid)
	c.Check(err, gc.ErrorMatches, "missing Logger not valid")
}

func (s *manifoldSuite) TestConfigValidationMissingHub(c *gc.C) {
	s.config.Hub = nil
	err := s.config.Validate()
	c.Check(err, jc.ErrorIs, errors.NotValid)
	c.Check(err, gc.ErrorMatches, "missing Hub not valid")
}
