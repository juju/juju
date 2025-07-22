// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"time"

	"github.com/juju/clock/testclock"
	"github.com/juju/names/v6"
	"github.com/juju/tc"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/testing/factory"
	"github.com/juju/juju/state"
)

// StateSuite provides setup and teardown for tests that require a
// state.State.
type StateSuite struct {
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
	ControllerModelType       state.ModelType
	RegionConfig              cloud.RegionConfig
	Clock                     testclock.AdvanceableClock
}

func (s *StateSuite) SetUpSuite(c *tc.C) {
	s.BaseSuite.SetUpSuite(c)
}

func (s *StateSuite) TearDownSuite(c *tc.C) {
	s.BaseSuite.TearDownSuite(c)
}

func (s *StateSuite) SetUpTest(c *tc.C) {
	s.BaseSuite.SetUpTest(c)

	s.Owner = names.NewLocalUserTag("test-admin")

	if s.Clock == nil {
		s.Clock = testclock.NewDilatedWallClock(100 * time.Millisecond)
	}

	s.Controller = InitializeWithArgs(c, InitializeArgs{
		Owner:                     s.Owner,
		InitialConfig:             s.InitialConfig,
		ControllerConfig:          s.ControllerConfig,
		ControllerInheritedConfig: s.ControllerInheritedConfig,
		ControllerModelType:       s.ControllerModelType,
		RegionConfig:              s.RegionConfig,
		NewPolicy:                 s.NewPolicy,
		Clock:                     s.Clock,
	})
	s.AddCleanup(func(*tc.C) {
		_ = s.Controller.Close()
	})
	s.StatePool = s.Controller.StatePool()
	var err error
	s.State, err = s.StatePool.SystemState()
	c.Assert(err, tc.ErrorIsNil)
	model, err := s.State.Model()
	c.Assert(err, tc.ErrorIsNil)
	s.Model = model

	s.Factory = factory.NewFactory(s.State, s.StatePool, s.ControllerConfig)
}

func (s *StateSuite) TearDownTest(c *tc.C) {
	s.BaseSuite.TearDownTest(c)
}
