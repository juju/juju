// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"github.com/juju/names"
	jujutesting "github.com/juju/testing"
	gc "gopkg.in/check.v1"

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
	Policy  state.Policy
	State   *state.State
	Owner   names.UserTag
	Factory *factory.Factory
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
	s.State = Initialize(c, s.Owner, nil, s.Policy)
	s.AddCleanup(func(*gc.C) { s.State.Close() })
	s.Factory = factory.NewFactory(s.State)
}

func (s *StateSuite) TearDownTest(c *gc.C) {
	s.BaseSuite.TearDownTest(c)
	s.MgoSuite.TearDownTest(c)
}
