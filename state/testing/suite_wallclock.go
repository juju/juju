// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	"github.com/juju/juju/cloud"
	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/feature"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
)

var _ = gc.Suite(&StateWithWallClockSuite{})

// StateWithWallClockSuite provides setup and teardown for tests that require a
// state.State. This should be deprecated in favour of StateSuite, and tests
// updated to use the testing clock StateSuite provides.
type StateWithWallClockSuite struct {
	testing.MgoSuite
	coretesting.BaseSuite
	NewPolicy                 state.NewPolicyFunc
	Controller                *state.Controller
	State                     *state.State
	Model                     *state.Model
	IAASModel                 *state.IAASModel
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
	s.SetInitialFeatureFlags(feature.CrossModelRelations)
	s.MgoSuite.SetUpTest(c)
	s.BaseSuite.SetUpTest(c)

	s.Owner = names.NewLocalUserTag("test-admin")
	s.Controller, s.State = Initialize(c, s.Owner, s.InitialConfig, s.ControllerInheritedConfig, s.RegionConfig, s.NewPolicy)
	s.AddCleanup(func(*gc.C) {
		s.State.Close()
		s.Controller.Close()
	})

	model, err := s.State.Model()
	c.Assert(err, jc.ErrorIsNil)
	s.Model = model

	im, err := s.State.IAASModel()
	c.Assert(err, jc.ErrorIsNil)
	s.IAASModel = im

	s.Factory = factory.NewFactory(s.State)
}

func (s *StateWithWallClockSuite) TearDownTest(c *gc.C) {
	s.BaseSuite.TearDownTest(c)
	s.MgoSuite.TearDownTest(c)
}
