// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package applicationscaler_test

import (
	"context"

	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facades/controller/applicationscaler"
	"github.com/juju/juju/rpc/params"
)

type FacadeSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&FacadeSuite{})

func (s *FacadeSuite) TestModelManager(c *gc.C) {
	facade, err := applicationscaler.NewFacade(nil, nil, auth(true))
	c.Check(err, jc.ErrorIsNil)
	c.Check(facade, gc.NotNil)
}

func (s *FacadeSuite) TestNotModelManager(c *gc.C) {
	facade, err := applicationscaler.NewFacade(nil, nil, auth(false))
	c.Check(err, gc.Equals, apiservererrors.ErrPerm)
	c.Check(facade, gc.IsNil)
}

func (s *FacadeSuite) TestWatchError(c *gc.C) {
	fix := newWatchFixture(c, false)
	result, err := fix.Facade.Watch(context.Background())
	c.Check(err, gc.ErrorMatches, "blammo")
	c.Check(result, gc.DeepEquals, params.StringsWatchResult{})
	c.Check(fix.Resources.Count(), gc.Equals, 0)
}

func (s *FacadeSuite) TestWatchSuccess(c *gc.C) {
	fix := newWatchFixture(c, true)
	result, err := fix.Facade.Watch(context.Background())
	c.Check(err, jc.ErrorIsNil)
	c.Check(result.Changes, jc.DeepEquals, []string{"pow", "zap", "kerblooie"})
	c.Check(fix.Resources.Count(), gc.Equals, 1)
	resource := fix.Resources.Get(result.StringsWatcherId)
	c.Check(resource, gc.NotNil)
}

func (s *FacadeSuite) TestRescaleNonsense(c *gc.C) {
	fix := newRescaleFixture(c)
	result := fix.Facade.Rescale(context.Background(), entities("burble plink"))
	c.Assert(result.Results, gc.HasLen, 1)
	err := result.Results[0].Error
	c.Check(err, gc.ErrorMatches, `"burble plink" is not a valid tag`)
}

func (s *FacadeSuite) TestRescaleUnauthorized(c *gc.C) {
	fix := newRescaleFixture(c)
	result := fix.Facade.Rescale(context.Background(), entities("unit-foo-27"))
	c.Assert(result.Results, gc.HasLen, 1)
	err := result.Results[0].Error
	c.Check(err, gc.ErrorMatches, "permission denied")
	c.Check(err, jc.Satisfies, params.IsCodeUnauthorized)
}

func (s *FacadeSuite) TestRescaleNotFound(c *gc.C) {
	fix := newRescaleFixture(c)
	result := fix.Facade.Rescale(context.Background(), entities("application-missing"))
	c.Assert(result.Results, gc.HasLen, 1)
	err := result.Results[0].Error
	c.Check(err, gc.ErrorMatches, "application not found")
	c.Check(err, jc.Satisfies, params.IsCodeNotFound)
}

func (s *FacadeSuite) TestRescaleError(c *gc.C) {
	fix := newRescaleFixture(c)
	result := fix.Facade.Rescale(context.Background(), entities("application-error"))
	c.Assert(result.Results, gc.HasLen, 1)
	err := result.Results[0].Error
	c.Check(err, gc.ErrorMatches, "blammo")
}

func (s *FacadeSuite) TestRescaleSuccess(c *gc.C) {
	fix := newRescaleFixture(c)
	result := fix.Facade.Rescale(context.Background(), entities("application-expected"))
	c.Assert(result.Results, gc.HasLen, 1)
	err := result.Results[0].Error
	c.Check(err, gc.IsNil)
}

func (s *FacadeSuite) TestRescaleMultiple(c *gc.C) {
	fix := newRescaleFixture(c)
	result := fix.Facade.Rescale(context.Background(), entities("application-error", "application-expected"))
	c.Assert(result.Results, gc.HasLen, 2)
	err0 := result.Results[0].Error
	c.Check(err0, gc.ErrorMatches, "blammo")
	err1 := result.Results[1].Error
	c.Check(err1, gc.IsNil)
}
