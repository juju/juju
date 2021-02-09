// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package logpruner_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/facade/facadetest"
	"github.com/juju/juju/apiserver/facades/controller/logpruner"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
)

var _ = gc.Suite(&LogPrunerSuite{})

type LogPrunerSuite struct {
	coretesting.BaseSuite

	context facadetest.Context
	api     *logpruner.API
}

func (s *LogPrunerSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.PatchValue(&logpruner.SystemState, func(_ facade.Context) *state.State {
		return nil
	})
	s.context.Auth_ = testing.FakeAuthorizer{Controller: true}

	var err error
	s.api, err = logpruner.NewAPI(s.context)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *LogPrunerSuite) TestPruneNonController(c *gc.C) {
	s.context.Auth_ = testing.FakeAuthorizer{}
	api, err := logpruner.NewAPI(s.context)
	c.Assert(err, jc.ErrorIsNil)
	err = api.Prune(params.LogPruneArgs{})
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *LogPrunerSuite) TestPrune(c *gc.C) {
	called := false
	s.PatchValue(&logpruner.Prune, func(_ <-chan struct{}, st *state.State, maxHistoryMB int) error {
		c.Assert(maxHistoryMB, gc.Equals, 666)
		called = true
		return nil
	})
	err := s.api.Prune(params.LogPruneArgs{
		MaxLogMB: 666,
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(called, jc.IsTrue)
}
