// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package statushistory_test

import (
	"context"
	"time"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/facade/facadetest"
	"github.com/juju/juju/apiserver/facades/controller/statushistory"
	"github.com/juju/juju/apiserver/testing"
	coretesting "github.com/juju/juju/internal/testing"
	"github.com/juju/juju/rpc/params"
	"github.com/juju/juju/state"
)

var _ = gc.Suite(&StatusHistoryPrunerSuite{})

type StatusHistoryPrunerSuite struct {
	coretesting.BaseSuite

	context facadetest.ModelContext
	api     *statushistory.API
}

func (s *StatusHistoryPrunerSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.context.Auth_ = testing.FakeAuthorizer{Controller: true}

	var err error
	s.api, err = statushistory.NewAPI(s.context)
	c.Assert(err, jc.ErrorIsNil)
}

func (s *StatusHistoryPrunerSuite) TestPruneNonController(c *gc.C) {
	s.context.Auth_ = testing.FakeAuthorizer{}
	api, err := statushistory.NewAPI(s.context)
	c.Assert(err, jc.ErrorIsNil)
	err = api.Prune(context.Background(), params.StatusHistoryPruneArgs{})
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *StatusHistoryPrunerSuite) TestPrune(c *gc.C) {
	called := false
	s.PatchValue(&statushistory.Prune, func(_ <-chan struct{}, st *state.State, maxHistoryTime time.Duration, maxHistoryMB int) error {
		c.Assert(maxHistoryTime, gc.Equals, time.Hour)
		c.Assert(maxHistoryMB, gc.Equals, 666)
		called = true
		return nil
	})
	err := s.api.Prune(context.Background(), params.StatusHistoryPruneArgs{
		MaxHistoryTime: time.Hour,
		MaxHistoryMB:   666,
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(called, jc.IsTrue)
}
