// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	mgotesting "github.com/juju/mgo/v3/testing"
	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs/config"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/testing/factory"
	"github.com/juju/juju/state"
)

var _ = gc.Suite(&StateWithWallClockSuite{})

// StateWithWallClockSuite provides setup and teardown for tests that require a
// state.State. This should be deprecated in favour of StateSuite, and tests
// updated to use the testing clock StateSuite provides.
type StateWithWallClockSuite struct {
	mgotesting.MgoSuite
	coretesting.BaseSuite
	NewPolicy                 state.NewPolicyFunc
	Controller                *state.Controller
	StatePool                 *state.StatePool
	State                     *state.State
	Model                     *state.Model
	Owner                     names.UserTag
	Factory                   *factory.Factory
	InitialConfig             *config.Config
	ControllerInheritedConfig map[string]interface{}
	RegionConfig              cloud.RegionConfig
}

func (s *StateWithWallClockSuite) SetUpSuite(c *gc.C) {
	s.MgoSuite.SetUpSuite(c)
	s.BaseSuite.SetUpSuite(c)
}

func (s *StateWithWallClockSuite) TearDownSuite(c *gc.C) {
	s.BaseSuite.TearDownSuite(c)
	s.MgoSuite.TearDownSuite(c)
}

func (s *StateWithWallClockSuite) SetUpTest(c *gc.C) {
	s.MgoSuite.SetUpTest(c)
	s.BaseSuite.SetUpTest(c)

	s.Owner = names.NewLocalUserTag("test-admin")
	s.Controller = Initialize(c, s.Owner, s.InitialConfig, s.ControllerInheritedConfig, s.RegionConfig, s.NewPolicy)
	s.AddCleanup(func(*gc.C) {
		s.Controller.Close()
	})
	s.StatePool = s.Controller.StatePool()
	var err error
	s.State, err = s.StatePool.SystemState()
	c.Assert(err, jc.ErrorIsNil)
	model, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)
	s.Model = model

	s.Factory = factory.NewFactory(s.State, s.StatePool, s.ControllerInheritedConfig).
		WithModelConfigService(&stubModelConfigService{cfg: coretesting.ModelConfig(c)})
}

func (s *StateWithWallClockSuite) TearDownTest(c *gc.C) {
	s.BaseSuite.TearDownTest(c)
	s.MgoSuite.TearDownTest(c)
}
