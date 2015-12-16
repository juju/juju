// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package discoverspaces_test

import (
	"errors"

	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/base"
	apitesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/discoverspaces"
	coretesting "github.com/juju/juju/testing"
)

type DiscoverSpacesSuite struct {
	coretesting.BaseSuite
}

var _ = gc.Suite(&DiscoverSpacesSuite{})

func (s *DiscoverSpacesSuite) TestNewAPI(c *gc.C) {
	var called int
	apiCaller := clientErrorAPICaller(c, "CleanupIPAddresses", nil, &called)
	api := discoverspaces.NewAPI(apiCaller)
	c.Check(api, gc.NotNil)
	c.Check(called, gc.Equals, 0)

	// Make a call so that an error will be returned.
	err, _ := api.ListSpaces()
	c.Assert(err, gc.ErrorMatches, "client error!")
	c.Assert(called, gc.Equals, 1)
}

func clientErrorAPICaller(c *gc.C, method string, expectArgs interface{}, numCalls *int) base.APICaller {
	args := &apitesting.CheckArgs{
		Facade:        "Addresser",
		VersionIsZero: true,
		IdIsEmpty:     true,
		Method:        method,
		Args:          expectArgs,
	}
	return apitesting.CheckingAPICaller(c, args, numCalls, errors.New("client error!"))
}
