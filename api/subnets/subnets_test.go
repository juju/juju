// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package subnets_test

import (
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/base"
	apitesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/subnets"
	coretesting "github.com/juju/juju/testing"
)

// SubnetsSuite tests the client side subnets API
type SubnetsSuite struct {
	coretesting.BaseSuite

	called    int
	apiCaller base.APICaller
	api       *subnets.API
}

var _ = gc.Suite(&SubnetsSuite{})

func (s *SubnetsSuite) init(c *gc.C, args *apitesting.CheckArgs, err error) {
	s.called = 0
	s.apiCaller = apitesting.CheckingAPICaller(c, args, &s.called, err)
	s.api = subnets.NewAPI(s.apiCaller)
	c.Check(s.api, gc.NotNil)
	c.Check(s.called, gc.Equals, 0)
}

// TestNewAPISuccess checks that a new subnets API is created when passed a non-nil caller
func (s *SubnetsSuite) TestNewAPISuccess(c *gc.C) {
	var called int
	apiCaller := apitesting.CheckingAPICaller(c, nil, &called, nil)
	api := subnets.NewAPI(apiCaller)
	c.Check(api, gc.NotNil)
	c.Check(called, gc.Equals, 0)
}

// TestNewAPIWithNilCaller checks that a new subnets API is not created when passed a nil caller
func (s *SubnetsSuite) TestNewAPIWithNilCaller(c *gc.C) {
	panicFunc := func() { subnets.NewAPI(nil) }
	c.Assert(panicFunc, gc.PanicMatches, "caller is nil")
}
