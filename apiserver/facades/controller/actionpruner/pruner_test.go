// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package actionpruner_test

import (
	"context"
	"time"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/apiserver/facade/facadetest"
	"github.com/juju/juju/apiserver/facades/controller/actionpruner"
	"github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
	coretesting "github.com/juju/juju/testing"
)

var _ = gc.Suite(&ActionPrunerSuite{})

type ActionPrunerSuite struct {
	coretesting.BaseSuite

	context facadetest.Context
	api     *actionpruner.API
}

func (s *ActionPrunerSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.PatchValue(&actionpruner.Model, func(_ facade.Context) (state.ModelAccessor, error) {
		return nil, nil
	})
	s.context.Auth_ = testing.FakeAuthorizer{Controller: true}

	var err error
	s.api, err = actionpruner.NewAPI(s.context)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *ActionPrunerSuite) TestPruneNonController(c *gc.C) {
	s.context.Auth_ = testing.FakeAuthorizer{}
	api, err := actionpruner.NewAPI(s.context)
	c.Assert(err, jc.ErrorIsNil)
	err = api.Prune(context.Background(), params.ActionPruneArgs{})
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *ActionPrunerSuite) TestPrune(c *gc.C) {
	called := false
	s.PatchValue(&actionpruner.Prune, func(_ <-chan struct{}, st *state.State, maxHistoryTime time.Duration, maxHistoryMB int) error {
		c.Assert(maxHistoryTime, gc.Equals, time.Hour)
		c.Assert(maxHistoryMB, gc.Equals, 666)
		called = true
		return nil
	})
	err := s.api.Prune(context.Background(), params.ActionPruneArgs{
		MaxHistoryTime: time.Hour,
		MaxHistoryMB:   666,
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(called, jc.IsTrue)
}
