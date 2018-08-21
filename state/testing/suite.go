// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"github.com/juju/clock/testclock"
	jujutesting "github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
)

var _ = gc.Suite(&StateSuite{})

// StateSuite provides setup and teardown for tests that require a
// state.State.
type StateSuite struct {
	jujutesting.MgoSuite
	testing.BaseSuite
	NewPolicy                 state.NewPolicyFunc
	Controller                *state.Controller
	StatePool                 *state.StatePool
	State                     *state.State
	Model                     *state.Model
	Owner                     names.UserTag
	Factory                   *factory.Factory
	InitialConfig             *config.Config
	ControllerConfig          map[string]interface{}
	ControllerInheritedConfig map[string]interface{}
	RegionConfig              cloud.RegionConfig
	Clock                     *testclock.Clock
}

func (s *StateSuite) SetUpSuite(c *gc.C) {
	s.MgoSuite.SetUpSuite(c)
	s.BaseSuite.SetUpSuite(c)
}

func (s *StateSuite) TearDownSuite(c *gc.C) {
	s.BaseSuite.TearDownSuite(c)
	s.MgoSuite.TearDownSuite(c)
}

func (s *StateSuite) SetUpTest(c *gc.C) {
	s.MgoSuite.SetUpTest(c)
	s.BaseSuite.SetUpTest(c)

	s.Owner = names.NewLocalUserTag("test-admin")
	s.Clock = testclock.NewClock(testing.NonZeroTime())
	s.Controller, s.State = InitializeWithArgs(c, InitializeArgs{
		Owner:                     s.Owner,
		InitialConfig:             s.InitialConfig,
		ControllerConfig:          s.ControllerConfig,
		ControllerInheritedConfig: s.ControllerInheritedConfig,
		RegionConfig:              s.RegionConfig,
		NewPolicy:                 s.NewPolicy,
		Clock:                     s.Clock,
	})
	s.AddCleanup(func(*gc.C) {
		s.State.Close()
		s.Controller.Close()
	})

	s.StatePool = state.NewStatePool(s.State)
	s.AddCleanup(func(*gc.C) { s.StatePool.Close() })

	model, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)
	s.Model = model

	s.Factory = factory.NewFactory(s.State)
}

func (s *StateSuite) TearDownTest(c *gc.C) {
	s.BaseSuite.TearDownTest(c)
	s.MgoSuite.TearDownTest(c)
}
