// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package spaces_test

import (
	"errors"
	"fmt"
	"math/rand"

	"github.com/golang/mock/gomock"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v3"

	"github.com/juju/juju/api/base/mocks"
	apitesting "github.com/juju/juju/api/base/testing"
	"github.com/juju/juju/api/spaces"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/core/network"
	coretesting "github.com/juju/juju/testing"
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
	caller.EXPECT().BestFacadeVersion(gomock.Any()).Return(0).AnyTimes()

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
	args := params.Entities{Entities: []params.Entity{{
		Tag: names.NewSpaceTag(name).String(),
	}}}
	s.fCaller.EXPECT().FacadeCall("RemoveSpace", args, gomock.Any()).SetArg(2, resultSource).Return(nil)

	err := s.API.RemoveSpace(name)
	c.Assert(err, gc.IsNil)
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
	args := params.Entities{Entities: []params.Entity{{
		Tag: names.NewSpaceTag(name).String(),
	}}}
	s.fCaller.EXPECT().FacadeCall("RemoveSpace", args, gomock.Any()).SetArg(2, resultSource).Return(nil)

	err := s.API.RemoveSpace(name)
	c.Assert(err, gc.ErrorMatches, "bam")
}

func (s *spacesSuite) TestRemoveSpaceUnexpectedErrorAPICall(c *gc.C) {
	defer s.setUpMocks(c).Finish()
	name := "myspace"
	resultSource := params.RemoveSpaceResults{
		Results: []params.RemoveSpaceResult{}}
	args := params.Entities{Entities: []params.Entity{{
		Tag: names.NewSpaceTag(name).String(),
	}}}
	bam := errors.New("bam")
	s.fCaller.EXPECT().FacadeCall("RemoveSpace", args, gomock.Any()).SetArg(2, resultSource).Return(bam)

	err := s.API.RemoveSpace(name)
	c.Assert(err, gc.ErrorMatches, bam.Error())
}

func (s *spacesSuite) TestRemoveSpaceUnexpectedErrorAPICallNotSupported(c *gc.C) {
	defer s.setUpMocks(c).Finish()
	name := "myspace"
	resultSource := params.RemoveSpaceResults{
		Results: []params.RemoveSpaceResult{}}
	args := params.Entities{Entities: []params.Entity{{
		Tag: names.NewSpaceTag(name).String(),
	}}}
	bam := params.Error{
		Message: "not supported",
		Code:    params.CodeNotSupported,
		Info:    nil,
	}
	s.fCaller.EXPECT().FacadeCall("RemoveSpace", args, gomock.Any()).SetArg(2, resultSource).Return(bam)

	err := s.API.RemoveSpace(name)
	c.Assert(err, gc.ErrorMatches, bam.Error())
}

func (s *spacesSuite) TestRemoveSpaceConstraintsBindings(c *gc.C) {
	defer s.setUpMocks(c).Finish()
	name := "myspace"
	resultSource := params.RemoveSpaceResults{
		Results: []params.RemoveSpaceResult{{
			Constraints: []params.Entity{{
				Tag: "model-42c4f770-86ed-4fcc-8e39-697063d082bc:e",
			}},
			Bindings: []params.Entity{
				{
					Tag: "application-mysql",
				},
				{
					Tag: "application-mediawiki",
				},
			},
			ControllerSettings: []string{"jujuhaspace", "juuuu-space"},
			Error:              nil,
		}}}
	args := params.Entities{Entities: []params.Entity{{
		Tag: names.NewSpaceTag(name).String(),
	}}}
	s.fCaller.EXPECT().FacadeCall("RemoveSpace", args, gomock.Any()).SetArg(2, resultSource).Return(nil)

	err := s.API.RemoveSpace(name)
	expectedErrMsg := "" +
		"\n" +
		"- Found the following existing constraints: model constraint\n" +
		"- Found the following existing bindings: mysql, mediawiki\n" +
		"- Found the following existing controller settings: jujuhaspace, juuuu-space"
	c.Assert(err, gc.Not(gc.IsNil))
	c.Assert(err.Error(), jc.DeepEquals, expectedErrMsg)
}
func (s *spacesSuite) TestRemoveSpaceConstraints(c *gc.C) {
	defer s.setUpMocks(c).Finish()
	name := "myspace"
	resultSource := params.RemoveSpaceResults{
		Results: []params.RemoveSpaceResult{{
			Constraints: []params.Entity{{
				Tag: "model-42c4f770-86ed-4fcc-8e39-697063d082bc:e",
			}},
			Error: nil,
		}}}
	args := params.Entities{Entities: []params.Entity{{
		Tag: names.NewSpaceTag(name).String(),
	}}}
	s.fCaller.EXPECT().FacadeCall("RemoveSpace", args, gomock.Any()).SetArg(2, resultSource).Return(nil)

	err := s.API.RemoveSpace(name)
	expectedErrMsg :=
		"\n- Found the following existing constraints: model constraint"
	c.Assert(err, gc.Not(gc.IsNil))
	c.Assert(err.Error(), jc.DeepEquals, expectedErrMsg)
}

func (s *spacesSuite) TestRenameSpace(c *gc.C) {
	defer s.setUpMocks(c).Finish()
	from, to := "from", "to"
	resultSource := params.ErrorResults{}
	args := params.RenameSpacesParams{SpacesRenames: []params.RenameSpaceParams{{
		FromSpaceTag: names.NewSpaceTag(from).String(),
		ToSpaceTag:   names.NewSpaceTag(to).String(),
	}}}
	s.fCaller.EXPECT().FacadeCall("RenameSpace", args, gomock.Any()).SetArg(2, resultSource).Return(nil)

	err := s.API.RenameSpace(from, to)
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
	args := params.RenameSpacesParams{SpacesRenames: []params.RenameSpaceParams{{
		FromSpaceTag: names.NewSpaceTag(from).String(),
		ToSpaceTag:   names.NewSpaceTag(to).String(),
	}}}
	s.fCaller.EXPECT().FacadeCall("RenameSpace", args, gomock.Any()).SetArg(2, resultSource).Return(nil)

	err := s.API.RenameSpace(from, to)
	c.Assert(err, gc.ErrorMatches, "bam")
}

type SpacesSuite struct {
	coretesting.BaseSuite

	apiCaller *apitesting.CallChecker
	api       *spaces.API
}

var _ = gc.Suite(&SpacesSuite{})

func (s *SpacesSuite) init(c *gc.C, args apitesting.APICall) {
	s.apiCaller = apitesting.APICallChecker(c, args)
	best := &apitesting.BestVersionCaller{
		BestVersion:   6,
		APICallerFunc: s.apiCaller.APICallerFunc,
	}
	s.api = spaces.NewAPI(best)
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

func makeArgs(name string, cidrs []string) (string, []string, apitesting.APICall) {
	spaceTag := names.NewSpaceTag(name).String()

	expectArgs := params.CreateSpacesParams{
		Spaces: []params.CreateSpaceParams{
			{
				SpaceTag: spaceTag,
				CIDRs:    cidrs,
				Public:   true,
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
	return name, cidrs, args
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

func (s *SpacesSuite) TestCreateSpaceV4(c *gc.C) {
	var called bool
	apicaller := &apitesting.BestVersionCaller{
		APICallerFunc: apitesting.APICallerFunc(
			func(objType string,
				version int,
				id, request string,
				a, result interface{},
			) error {
				c.Check(objType, gc.Equals, "Spaces")
				c.Check(id, gc.Equals, "")
				c.Check(request, gc.Equals, "CreateSpaces")
				c.Assert(result, gc.FitsTypeOf, &params.ErrorResults{})
				c.Assert(a, jc.DeepEquals, params.CreateSpacesParamsV4{
					Spaces: []params.CreateSpaceParamsV4{{
						SpaceTag:   names.NewSpaceTag("testv4").String(),
						SubnetTags: []string{"subnet-1.1.1.0/24"},
						Public:     false,
					}}})
				*result.(*params.ErrorResults) = params.ErrorResults{
					Results: []params.ErrorResult{{}},
				}
				called = true
				return nil
			},
		),
		BestVersion: 4,
	}
	apiv4 := spaces.NewAPI(apicaller)
	err := apiv4.CreateSpace("testv4", []string{"1.1.1.0/24"}, false)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(called, jc.IsTrue)
}

func (s *SpacesSuite) testShowSpaces(c *gc.C, spaceName string, results []params.ShowSpaceResult, err error, expectErr string) {
	var expectResults params.ShowSpaceResults
	if results != nil {
		expectResults = params.ShowSpaceResults{
			Results: results,
		}
	}

	s.init(c, apitesting.APICall{
		Facade:  "Spaces",
		Method:  "ShowSpace",
		Results: expectResults,
		Error:   err,
	})
	gotResults, gotErr := s.api.ShowSpace(spaceName)
	c.Assert(s.apiCaller.CallCount, gc.Equals, 1)
	if expectErr != "" {
		c.Assert(gotErr, gc.ErrorMatches, expectErr)
		c.Assert(gotResults, jc.DeepEquals, network.ShowSpace{})
		return
	} else {
		c.Assert(results, gc.NotNil)
		c.Assert(len(results), gc.Equals, 1)
		converted := spaces.ShowSpaceFromResult(results[0])
		c.Assert(gotResults, jc.DeepEquals, converted)
	}
	if err != nil {
		c.Assert(gotErr, jc.DeepEquals, err)
	} else {
		c.Assert(gotErr, jc.ErrorIsNil)
	}
}

func (s *SpacesSuite) TestShowSpaceTooManyResults(c *gc.C) {
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

func (s *SpacesSuite) TestShowSpaceNoResultsResults(c *gc.C) {
	s.testShowSpaces(c, "empty", nil, nil, "expected 1 result, got 0")
}

func (s *SpacesSuite) TestShowSpaceResult(c *gc.C) {
	result := []params.ShowSpaceResult{{
		Space:        params.Space{Id: "1", Name: "default"},
		Applications: []string{},
		MachineCount: 0,
	}}
	s.testShowSpaces(c, "default", result, nil, "")
}

func (s *SpacesSuite) TestShowSpaceServerError(c *gc.C) {
	s.testShowSpaces(c, "nil", nil, errors.New("boom"), "boom")
}

func (s *SpacesSuite) TestShowSpaceError(c *gc.C) {
	arg := "space"
	var called bool
	apicaller := &apitesting.BestVersionCaller{
		APICallerFunc: apitesting.APICallerFunc(
			func(objType string, version int, id, request string, a, result interface{}) error {
				c.Check(objType, gc.Equals, "Spaces")
				c.Check(id, gc.Equals, "")
				c.Check(request, gc.Equals, "ShowSpace")
				c.Assert(result, gc.FitsTypeOf, &params.ShowSpaceResults{})
				c.Assert(a, jc.DeepEquals, params.Entities{
					Entities: []params.Entity{{Tag: names.NewSpaceTag(arg).String()}},
				})
				called = true
				return nil
			},
		),
	}
	api := spaces.NewAPI(apicaller)
	_, err := api.ShowSpace(arg)
	c.Assert(err, gc.ErrorMatches, "expected 1 result, got 0")
	c.Assert(called, jc.IsTrue)
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
