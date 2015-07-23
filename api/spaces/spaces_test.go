// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package spaces_test

import (
	"errors"
	"fmt"
	"math/rand"

	"github.com/juju/names"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/base"
	apitesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/spaces"
	"github.com/juju/juju/apiserver/params"
	coretesting "github.com/juju/juju/testing"
)

type SpacesSuite struct {
	coretesting.BaseSuite

	called    int
	apiCaller base.APICaller
	api       *spaces.API
}

var _ = gc.Suite(&SpacesSuite{})

func (s *SpacesSuite) SetUpSuite(c *gc.C) {
	s.BaseSuite.SetUpSuite(c)
}

func (s *SpacesSuite) TearDownSuite(c *gc.C) {
	s.BaseSuite.TearDownSuite(c)
}

func (s *SpacesSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
}

func (s *SpacesSuite) TearDownTest(c *gc.C) {
	s.BaseSuite.TearDownTest(c)
}

func (s *SpacesSuite) init(c *gc.C, args *apitesting.CheckArgs, err error) {
	s.called = 0
	s.apiCaller = apitesting.CheckingAPICaller(c, args, &s.called, err)
	s.api = spaces.NewAPI(s.apiCaller)
	c.Check(s.api, gc.NotNil)
	c.Check(s.called, gc.Equals, 0)
}

func (s *SpacesSuite) TestNewAPISuccess(c *gc.C) {
	var called int
	apiCaller := apitesting.CheckingAPICaller(c, nil, &called, nil)
	api := spaces.NewAPI(apiCaller)
	c.Check(api, gc.NotNil)
	c.Check(called, gc.Equals, 0)
}

func (s *SpacesSuite) TestNewAPIWithNilCaller(c *gc.C) {
	panicFunc := func() { spaces.NewAPI(nil) }
	c.Assert(panicFunc, gc.PanicMatches, "caller is nil")
}

func makeArgs(name string, subnets []string) (string, []string, apitesting.CheckArgs) {
	spaceTag := names.NewSpaceTag(name).String()
	subnetTags := []string{}

	for _, s := range subnets {
		subnetTags = append(subnetTags, names.NewSubnetTag(s).String())
	}

	expectArgs := params.CreateSpacesParams{
		Spaces: []params.CreateSpaceParams{
			params.CreateSpaceParams{
				SpaceTag:   spaceTag,
				SubnetTags: subnetTags,
			}}}

	expectResults := params.ErrorResults{}

	args := apitesting.CheckArgs{
		Facade:  "Spaces",
		Method:  "CreateSpaces",
		Args:    expectArgs,
		Results: expectResults,
	}

	return name, subnets, args
}

func (s *SpacesSuite) testCreateSpace(c *gc.C, name string, subnets []string) {
	_, _, args := makeArgs(name, subnets)
	s.init(c, &args, nil)
	results, err := s.api.CreateSpace(name, subnets, false)
	c.Assert(s.called, gc.Equals, 1)
	c.Assert(err, gc.IsNil)
	c.Assert(results, gc.DeepEquals, args.Results)
}

func (s *SpacesSuite) TestCreateSpace(c *gc.C) {
	name := "foo"
	subnets := []string{}
	r := rand.New(rand.NewSource(0xdeadbeef))
	for i := 0; i < 100; i++ {
		for j := 0; j < 10; j++ {
			n := r.Uint32()
			newSubnet := fmt.Sprintf("%d.%d.%d.0/24", uint8(n>>16), uint8(n>>8), uint8(n))
			subnets = append(subnets, newSubnet)
		}
		s.testCreateSpace(c, name, subnets)
	}
}

func (s *SpacesSuite) TestCreateSpaceFails(c *gc.C) {
	name, subnets, args := makeArgs("foo", []string{"1.1.1.0/24"})
	s.init(c, &args, errors.New("bang"))
	results, err := s.api.CreateSpace(name, subnets, false)
	c.Check(s.called, gc.Equals, 1)
	c.Assert(err, gc.ErrorMatches, "bang")
	c.Assert(results, gc.DeepEquals, args.Results)
}
