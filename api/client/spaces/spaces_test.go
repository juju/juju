// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package spaces_test

import (
	"context"
	"errors"
	"fmt"
	"math/rand"

	"github.com/juju/names/v6"
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/base/mocks"
	"github.com/juju/juju/api/client/spaces"
	"github.com/juju/juju/rpc/params"
)

// spacesSuite are using mocks instead of the apicaller stubs
type spacesSuite struct {
	fCaller *mocks.MockFacadeCaller
	API     *spaces.API
}

var _ = gc.Suite(&spacesSuite{})

func (s *spacesSuite) SetUpTest(c *gc.C) {
}

func (s *spacesSuite) TearDownTest(c *gc.C) {
	s.fCaller = nil
}

func (s *spacesSuite) setUpMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	caller := mocks.NewMockAPICallCloser(ctrl)

	s.fCaller = mocks.NewMockFacadeCaller(ctrl)
	s.fCaller.EXPECT().RawAPICaller().Return(caller).AnyTimes()
	s.API = spaces.NewAPIFromCaller(s.fCaller)
	return ctrl
}

func (s *spacesSuite) TestRemoveSpace(c *gc.C) {
	defer s.setUpMocks(c).Finish()
	name := "myspace"
	resultSource := params.RemoveSpaceResults{
		Results: []params.RemoveSpaceResult{},
	}
	args := getRemoveSpaceArgs(name, false, false)

	s.fCaller.EXPECT().FacadeCall(gomock.Any(), "RemoveSpace", args, gomock.Any()).SetArg(3, resultSource).Return(nil)

	bounds, err := s.API.RemoveSpace(context.Background(), name, false, false)
	c.Assert(err, gc.ErrorMatches, "0 results, expected 1")
	c.Assert(bounds, gc.DeepEquals, params.RemoveSpaceResult{})
}

func (s *spacesSuite) TestRemoveSpaceUnexpectedError(c *gc.C) {
	defer s.setUpMocks(c).Finish()
	name := "myspace"
	resultSource := params.RemoveSpaceResults{
		Results: []params.RemoveSpaceResult{{
			Constraints:        nil,
			Bindings:           nil,
			ControllerSettings: nil,
			Error: &params.Error{
				Message: "bam",
				Code:    "500",
			},
		}},
	}
	args := getRemoveSpaceArgs(name, false, false)

	s.fCaller.EXPECT().FacadeCall(gomock.Any(), "RemoveSpace", args, gomock.Any()).SetArg(3, resultSource).Return(nil)

	bounds, err := s.API.RemoveSpace(context.Background(), name, false, false)
	c.Assert(err, gc.ErrorMatches, "bam")
	c.Assert(bounds, gc.DeepEquals, params.RemoveSpaceResult{})
}

func (s *spacesSuite) TestRemoveSpaceUnexpectedErrorAPICall(c *gc.C) {
	defer s.setUpMocks(c).Finish()
	name := "myspace"
	resultSource := params.RemoveSpaceResults{
		Results: []params.RemoveSpaceResult{}}
	args := getRemoveSpaceArgs(name, false, false)

	bam := errors.New("bam")
	s.fCaller.EXPECT().FacadeCall(gomock.Any(), "RemoveSpace", args, gomock.Any()).SetArg(3, resultSource).Return(bam)

	bounds, err := s.API.RemoveSpace(context.Background(), name, false, false)
	c.Assert(err, gc.ErrorMatches, bam.Error())
	c.Assert(bounds, gc.DeepEquals, params.RemoveSpaceResult{})
}

func (s *spacesSuite) TestRemoveSpaceUnexpectedErrorAPICallNotSupported(c *gc.C) {
	defer s.setUpMocks(c).Finish()
	name := "myspace"
	resultSource := params.RemoveSpaceResults{
		Results: []params.RemoveSpaceResult{}}
	args := getRemoveSpaceArgs(name, false, false)

	bam := params.Error{
		Message: "not supported",
		Code:    params.CodeNotSupported,
		Info:    nil,
	}
	s.fCaller.EXPECT().FacadeCall(gomock.Any(), "RemoveSpace", args, gomock.Any()).SetArg(3, resultSource).Return(bam)

	bounds, err := s.API.RemoveSpace(context.Background(), name, false, false)
	c.Assert(err, gc.ErrorMatches, bam.Error())
	c.Assert(bounds, gc.DeepEquals, params.RemoveSpaceResult{})
}

func (s *spacesSuite) TestRemoveSpaceConstraintsBindings(c *gc.C) {
	defer s.setUpMocks(c).Finish()
	name := "myspace"
	resultSource := params.RemoveSpaceResults{
		Results: []params.RemoveSpaceResult{{
			Constraints: []params.Entity{
				{Tag: "model-42c4f770-86ed-4fcc-8e39-697063d082bc:e"},
				{Tag: "application-mysql"},
			},
			Bindings: []params.Entity{
				{Tag: "application-mysql"},
				{Tag: "application-mediawiki"},
			},
			ControllerSettings: []string{"jujuhaspace", "juuuu-space"},
			Error:              nil,
		}}}
	args := getRemoveSpaceArgs(name, false, false)

	s.fCaller.EXPECT().FacadeCall(gomock.Any(), "RemoveSpace", args, gomock.Any()).SetArg(3, resultSource).Return(nil)

	bounds, err := s.API.RemoveSpace(context.Background(), name, false, false)

	expectedBounds := params.RemoveSpaceResult{
		Constraints: []params.Entity{
			{Tag: "model-42c4f770-86ed-4fcc-8e39-697063d082bc:e"},
			{Tag: "application-mysql"},
		},
		Bindings: []params.Entity{
			{Tag: "application-mysql"},
			{Tag: "application-mediawiki"},
		},
		ControllerSettings: []string{"jujuhaspace", "juuuu-space"},
	}
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(bounds, jc.DeepEquals, expectedBounds)
}
func (s *spacesSuite) TestRemoveSpaceConstraints(c *gc.C) {
	defer s.setUpMocks(c).Finish()
	name := "myspace"
	resultSource := params.RemoveSpaceResults{
		Results: []params.RemoveSpaceResult{{
			Constraints: []params.Entity{
				{Tag: "model-42c4f770-86ed-4fcc-8e39-697063d082bc:e"},
				{Tag: "application-mysql"},
			},
			Error: nil,
		}}}
	args := getRemoveSpaceArgs(name, false, false)
	s.fCaller.EXPECT().FacadeCall(gomock.Any(), "RemoveSpace", args, gomock.Any()).SetArg(3, resultSource).Return(nil)

	bounds, err := s.API.RemoveSpace(context.Background(), name, false, false)
	expectedBounds := params.RemoveSpaceResult{
		Constraints: []params.Entity{
			{Tag: "model-42c4f770-86ed-4fcc-8e39-697063d082bc:e"},
			{Tag: "application-mysql"},
		},
	}
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(bounds, jc.DeepEquals, expectedBounds)
}

func (s *spacesSuite) TestRemoveSpaceForce(c *gc.C) {
	defer s.setUpMocks(c).Finish()
	name := "myspace"
	resultSource := params.RemoveSpaceResults{
		Results: []params.RemoveSpaceResult{{}}}
	args := getRemoveSpaceArgs(name, true, false)
	s.fCaller.EXPECT().FacadeCall(gomock.Any(), "RemoveSpace", args, gomock.Any()).SetArg(3, resultSource).Return(nil)

	bounds, err := s.API.RemoveSpace(context.Background(), name, true, false)

	c.Assert(err, gc.IsNil)
	c.Assert(bounds, gc.DeepEquals, params.RemoveSpaceResult{})
}

func getRemoveSpaceArgs(spaceName string, force, dryRun bool) params.RemoveSpaceParams {
	return params.RemoveSpaceParams{SpaceParams: []params.RemoveSpaceParam{
		{
			Space:  params.Entity{Tag: names.NewSpaceTag(spaceName).String()},
			Force:  force,
			DryRun: dryRun,
		},
	}}
}

func (s *spacesSuite) TestRenameSpace(c *gc.C) {
	defer s.setUpMocks(c).Finish()
	from, to := "from", "to"
	resultSource := params.ErrorResults{}
	args := params.RenameSpacesParams{
		Changes: []params.RenameSpaceParams{{
			FromSpaceTag: names.NewSpaceTag(from).String(),
			ToSpaceTag:   names.NewSpaceTag(to).String(),
		}},
	}
	s.fCaller.EXPECT().FacadeCall(gomock.Any(), "RenameSpace", args, gomock.Any()).SetArg(3, resultSource).Return(nil)

	err := s.API.RenameSpace(context.Background(), from, to)
	c.Assert(err, gc.IsNil)
}

func (s *spacesSuite) TestRenameSpaceError(c *gc.C) {
	defer s.setUpMocks(c).Finish()
	from, to := "from", "to"
	resultSource := params.ErrorResults{Results: []params.ErrorResult{{
		Error: &params.Error{
			Message: "bam",
			Code:    "500",
		},
	}}}
	args := params.RenameSpacesParams{
		Changes: []params.RenameSpaceParams{{
			FromSpaceTag: names.NewSpaceTag(from).String(),
			ToSpaceTag:   names.NewSpaceTag(to).String(),
		}},
	}
	s.fCaller.EXPECT().FacadeCall(gomock.Any(), "RenameSpace", args, gomock.Any()).SetArg(3, resultSource).Return(nil)

	err := s.API.RenameSpace(context.Background(), from, to)
	c.Assert(err, gc.ErrorMatches, "bam")
}

func (s *spacesSuite) TestCreateSpace(c *gc.C) {
	defer s.setUpMocks(c).Finish()
	name := "foo"
	subnets := []string{}
	r := rand.New(rand.NewSource(0xdeadbeef))
	for i := 0; i < 100; i++ {
		for j := 0; j < 10; j++ {
			n := r.Uint32()
			newSubnet := fmt.Sprintf("%d.%d.%d.0/24", uint8(n>>16), uint8(n>>8), uint8(n))
			subnets = append(subnets, newSubnet)
		}
		args := params.CreateSpacesParams{
			Spaces: []params.CreateSpaceParams{
				{
					SpaceTag: names.NewSpaceTag(name).String(),
					CIDRs:    subnets,
					Public:   true,
				},
			},
		}
		res := new(params.ErrorResults)
		ress := params.ErrorResults{
			Results: []params.ErrorResult{{}},
		}
		s.fCaller.EXPECT().FacadeCall(gomock.Any(), "CreateSpaces", args, res).SetArg(3, ress).Return(nil)
		err := s.API.CreateSpace(context.Background(), name, subnets, true)
		c.Assert(err, jc.ErrorIsNil)
	}
}

func (s *spacesSuite) TestCreateSpaceEmptyResults(c *gc.C) {
	defer s.setUpMocks(c).Finish()
	args := params.CreateSpacesParams{
		Spaces: []params.CreateSpaceParams{
			{
				SpaceTag: names.NewSpaceTag("foo").String(),
				CIDRs:    nil,
				Public:   true,
			},
		},
	}
	res := new(params.ErrorResults)
	ress := params.ErrorResults{
		Results: []params.ErrorResult{{
			Error: &params.Error{Message: "expected 1 result, got 0"},
		}},
	}

	s.fCaller.EXPECT().FacadeCall(gomock.Any(), "CreateSpaces", args, res).SetArg(3, ress).Return(nil)
	err := s.API.CreateSpace(context.Background(), "foo", nil, true)
	c.Assert(err, gc.ErrorMatches, "expected 1 result, got 0")
}

func (s *spacesSuite) TestCreateSpaceFails(c *gc.C) {
	defer s.setUpMocks(c).Finish()
	args := params.CreateSpacesParams{
		Spaces: []params.CreateSpaceParams{
			{
				SpaceTag: names.NewSpaceTag("foo").String(),
				CIDRs:    []string{"1.1.1.0/24"},
				Public:   true,
			},
		},
	}
	res := new(params.ErrorResults)
	ress := params.ErrorResults{
		Results: []params.ErrorResult{{
			Error: &params.Error{Message: "bang"},
		}},
	}

	s.fCaller.EXPECT().FacadeCall(gomock.Any(), "CreateSpaces", args, res).SetArg(3, ress).Return(nil)
	err := s.API.CreateSpace(context.Background(), "foo", []string{"1.1.1.0/24"}, true)
	c.Assert(err, gc.ErrorMatches, "bang")
}

func (s *spacesSuite) testShowSpaces(c *gc.C, spaceName string, results []params.ShowSpaceResult, err error, expectErr string) {
	defer s.setUpMocks(c).Finish()

	var expectResults params.ShowSpaceResults
	if results != nil {
		expectResults = params.ShowSpaceResults{
			Results: results,
		}
	}

	args := params.Entities{
		Entities: []params.Entity{{Tag: names.NewSpaceTag(spaceName).String()}},
	}
	res := new(params.ShowSpaceResults)

	s.fCaller.EXPECT().FacadeCall(gomock.Any(), "ShowSpace", args, res).SetArg(3, expectResults).Return(err)
	gotResults, gotErr := s.API.ShowSpace(context.Background(), spaceName)
	if expectErr != "" {
		c.Assert(gotErr, gc.ErrorMatches, expectErr)
		return
	} else {
		c.Assert(results, gc.NotNil)
		c.Assert(len(results), gc.Equals, 1)
		c.Assert(gotResults, jc.DeepEquals, results[0])
	}
	if err != nil {
		c.Assert(gotErr, jc.DeepEquals, err)
	} else {
		c.Assert(gotErr, jc.ErrorIsNil)
	}
}

func (s *spacesSuite) TestShowSpaceTooManyResults(c *gc.C) {
	s.testShowSpaces(c, "empty",
		[]params.ShowSpaceResult{
			{
				Space: params.Space{},
			},
			{
				Space: params.Space{},
			},
		}, nil, "expected 1 result, got 2")
}

func (s *spacesSuite) TestShowSpaceNoResultsResults(c *gc.C) {
	s.testShowSpaces(c, "empty", nil, nil, "expected 1 result, got 0")
}

func (s *spacesSuite) TestShowSpaceResult(c *gc.C) {
	result := []params.ShowSpaceResult{{
		Space:        params.Space{Id: "1", Name: "default"},
		Applications: []string{},
		MachineCount: 0,
	}}
	s.testShowSpaces(c, "default", result, nil, "")
}

func (s *spacesSuite) TestShowSpaceServerError(c *gc.C) {
	s.testShowSpaces(c, "nil", nil, errors.New("boom"), "boom")
}

func (s *spacesSuite) TestShowSpaceError(c *gc.C) {
	defer s.setUpMocks(c).Finish()

	arg := "space"
	args := params.Entities{
		Entities: []params.Entity{{Tag: names.NewSpaceTag(arg).String()}},
	}
	res := new(params.ShowSpaceResults)
	ress := params.ShowSpaceResults{
		Results: []params.ShowSpaceResult{},
	}

	s.fCaller.EXPECT().FacadeCall(gomock.Any(), "ShowSpace", args, res).SetArg(3, ress).Return(nil)

	_, err := s.API.ShowSpace(context.Background(), arg)
	c.Assert(err, gc.ErrorMatches, "expected 1 result, got 0")
}

func (s *spacesSuite) testListSpaces(c *gc.C, results []params.Space, err error, expectErr string) {
	defer s.setUpMocks(c).Finish()

	var expectResults params.ListSpacesResults
	if results != nil {
		expectResults = params.ListSpacesResults{
			Results: results,
		}
	}

	res := new(params.ListSpacesResults)

	s.fCaller.EXPECT().FacadeCall(gomock.Any(), "ListSpaces", nil, res).SetArg(3, expectResults).Return(err)
	gotResults, gotErr := s.API.ListSpaces(context.Background())
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

func (s *spacesSuite) TestListSpacesEmptyResults(c *gc.C) {
	s.testListSpaces(c, []params.Space{}, nil, "")
}

func (s *spacesSuite) TestListSpacesManyResults(c *gc.C) {
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

func (s *spacesSuite) TestListSpacesServerError(c *gc.C) {
	s.testListSpaces(c, nil, errors.New("boom"), "boom")
}

func (s *spacesSuite) testMoveSubnets(c *gc.C,
	space names.SpaceTag,
	subnets []names.SubnetTag,
	results []params.MoveSubnetsResult,
	err error, expectErr string,
) {
	defer s.setUpMocks(c).Finish()

	var expectedResults params.MoveSubnetsResults
	if results != nil {
		expectedResults.Results = results
	}

	subnetTags := make([]string, len(subnets))
	for k, subnet := range subnets {
		subnetTags[k] = subnet.String()
	}
	args := params.MoveSubnetsParams{
		Args: []params.MoveSubnetsParam{{
			SubnetTags: subnetTags,
			SpaceTag:   space.String(),
			Force:      false,
		}},
	}
	res := new(params.MoveSubnetsResults)

	s.fCaller.EXPECT().FacadeCall(gomock.Any(), "MoveSubnets", args, res).SetArg(3, expectedResults).Return(err)
	gotResult, gotErr := s.API.MoveSubnets(context.Background(), space, subnets, false)
	if len(results) > 0 {
		c.Assert(gotResult, jc.DeepEquals, results[0])
	} else {
		c.Assert(gotResult, jc.DeepEquals, params.MoveSubnetsResult{})
	}

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

func (s *spacesSuite) TestMoveSubnetsEmptyResults(c *gc.C) {
	space := names.NewSpaceTag("aaabbb")
	subnets := []names.SubnetTag{names.NewSubnetTag("0195847b-95bb-7ca1-a7ee-2211d802d5b3")}

	s.testMoveSubnets(c, space, subnets, []params.MoveSubnetsResult{}, nil, "expected 1 result, got 0")
}

func (s *spacesSuite) TestMoveSubnets(c *gc.C) {
	space := names.NewSpaceTag("aaabbb")
	subnets := []names.SubnetTag{names.NewSubnetTag("0195847b-95bb-7ca1-a7ee-2211d802d5b3")}

	s.testMoveSubnets(c, space, subnets, []params.MoveSubnetsResult{{
		MovedSubnets: []params.MovedSubnet{{
			SubnetTag:   "2",
			OldSpaceTag: "aaabbb",
		}},
		NewSpaceTag: "xxxyyy",
	}}, nil, "")
}

func (s *spacesSuite) TestMoveSubnetsServerError(c *gc.C) {
	space := names.NewSpaceTag("aaabbb")
	subnets := []names.SubnetTag{names.NewSubnetTag("0195847b-95bb-7ca1-a7ee-2211d802d5b3")}

	s.testMoveSubnets(c, space, subnets, nil, errors.New("boom"), "boom")
}
