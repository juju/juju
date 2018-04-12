// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package spaces_test

import (
	"errors"
	"fmt"
	"math/rand"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"

	apitesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/spaces"
	"github.com/juju/juju/apiserver/params"
	coretesting "github.com/juju/juju/testing"
)

type SpacesSuite struct {
	coretesting.BaseSuite

	apiCaller *apitesting.CallChecker
	api       *spaces.API
}

var _ = gc.Suite(&SpacesSuite{})

func (s *SpacesSuite) init(c *gc.C, args apitesting.APICall) {
	s.apiCaller = apitesting.APICallChecker(c, args)
	s.api = spaces.NewAPI(s.apiCaller)
	c.Check(s.api, gc.NotNil)
	c.Check(s.apiCaller.CallCount, gc.Equals, 0)
}

func (s *SpacesSuite) TestNewAPISuccess(c *gc.C) {
	apiCaller := apitesting.APICallChecker(c)
	api := spaces.NewAPI(apiCaller)
	c.Check(api, gc.NotNil)
	c.Check(apiCaller.CallCount, gc.Equals, 0)
}

func (s *SpacesSuite) TestNewAPIWithNilCaller(c *gc.C) {
	panicFunc := func() { spaces.NewAPI(nil) }
	c.Assert(panicFunc, gc.PanicMatches, "caller is nil")
}

func makeArgs(name string, subnets []string) (string, []string, apitesting.APICall) {
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

	args := apitesting.APICall{
		Facade:  "Spaces",
		Method:  "CreateSpaces",
		Args:    expectArgs,
		Results: expectResults,
	}
	return name, subnets, args
}

func (s *SpacesSuite) testCreateSpace(c *gc.C, name string, subnets []string) {
	_, _, args := makeArgs(name, subnets)
	s.init(c, args)
	err := s.api.CreateSpace(name, subnets, true)
	c.Assert(s.apiCaller.CallCount, gc.Equals, 1)
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
	s.init(c, args)
	err := s.api.CreateSpace("foo", nil, true)
	c.Assert(s.apiCaller.CallCount, gc.Equals, 1)
	c.Assert(err, gc.ErrorMatches, "expected 1 result, got 0")
}

func (s *SpacesSuite) TestCreateSpaceFails(c *gc.C) {
	name, subnets, args := makeArgs("foo", []string{"1.1.1.0/24"})
	args.Error = errors.New("bang")
	s.init(c, args)
	err := s.api.CreateSpace(name, subnets, true)
	c.Check(s.apiCaller.CallCount, gc.Equals, 1)
	c.Assert(err, gc.ErrorMatches, "bang")
}

func (s *SpacesSuite) testListSpaces(c *gc.C, results []params.Space, err error, expectErr string) {
	var expectResults params.ListSpacesResults
	if results != nil {
		expectResults = params.ListSpacesResults{
			Results: results,
		}
	}

	s.init(c, apitesting.APICall{
		Facade:  "Spaces",
		Method:  "ListSpaces",
		Results: expectResults,
		Error:   err,
	})
	gotResults, gotErr := s.api.ListSpaces()
	c.Assert(s.apiCaller.CallCount, gc.Equals, 1)
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
