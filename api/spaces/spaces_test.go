// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package spaces_test

import (
	"errors"
	"fmt"
	"math/rand"

	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
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
	apiCaller base.APICallCloser
	api       *spaces.API
}

var _ = gc.Suite(&SpacesSuite{})

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
				Public:     true,
			}}}

	expectResults := params.ErrorResults{
		Results: []params.ErrorResult{{}},
	}

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
	err := s.api.CreateSpace(name, subnets, true)
	c.Assert(s.called, gc.Equals, 1)
	c.Assert(err, jc.ErrorIsNil)
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

func (s *SpacesSuite) TestCreateSpaceEmptyResults(c *gc.C) {
	_, _, args := makeArgs("foo", nil)
	args.Results = params.ErrorResults{}
	s.init(c, &args, nil)
	err := s.api.CreateSpace("foo", nil, true)
	c.Check(s.called, gc.Equals, 1)
	c.Assert(err, gc.ErrorMatches, "expected 1 result, got 0")
}

func (s *SpacesSuite) TestCreateSpaceFails(c *gc.C) {
	name, subnets, args := makeArgs("foo", []string{"1.1.1.0/24"})
	s.init(c, &args, errors.New("bang"))
	err := s.api.CreateSpace(name, subnets, true)
	c.Check(s.called, gc.Equals, 1)
	c.Assert(err, gc.ErrorMatches, "bang")
}

func (s *SpacesSuite) testListSpaces(c *gc.C, results []params.Space, err error, expectErr string) {
	var expectResults params.ListSpacesResults
	if results != nil {
		expectResults = params.ListSpacesResults{
			Results: results,
		}
	}

	args := apitesting.CheckArgs{
		Facade:  "Spaces",
		Method:  "ListSpaces",
		Results: expectResults,
	}
	s.init(c, &args, err)
	gotResults, gotErr := s.api.ListSpaces()
	c.Assert(s.called, gc.Equals, 1)
	c.Assert(gotResults, jc.DeepEquals, results)
	if expectErr != "" {
		c.Assert(gotErr, gc.ErrorMatches, expectErr)
		return
	}
	if err != nil {
		c.Assert(gotErr, jc.DeepEquals, err)
	} else {
		c.Assert(gotErr, jc.ErrorIsNil)
	}
}

func (s *SpacesSuite) TestListSpacesEmptyResults(c *gc.C) {
	s.testListSpaces(c, []params.Space{}, nil, "")
}

func (s *SpacesSuite) TestListSpacesManyResults(c *gc.C) {
	spaces := []params.Space{{
		Name: "space1",
		Subnets: []params.Subnet{{
			CIDR: "foo",
		}, {
			CIDR: "bar",
		}},
	}, {
		Name: "space2",
	}, {
		Name:    "space3",
		Subnets: []params.Subnet{},
	}}
	s.testListSpaces(c, spaces, nil, "")
}

func (s *SpacesSuite) TestListSpacesServerError(c *gc.C) {
	s.testListSpaces(c, nil, errors.New("boom"), "boom")
}
