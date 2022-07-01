// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources_test

import (
	"github.com/golang/mock/gomock"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/v3/core/resources"
	"github.com/juju/juju/v3/rpc/params"
)

var _ = gc.Suite(&ListResourcesSuite{})

type ListResourcesSuite struct {
	BaseSuite
}

func (s *ListResourcesSuite) TestListResources(c *gc.C) {
	defer s.setUpMocks(c).Finish()

	args := &params.ListResourcesArgs{[]params.Entity{{
		Tag: "application-a-application",
	}, {
		Tag: "application-other-application",
	}}}
	expected1, apiResult1 := newResourceResult(c, "spam")
	expected2, apiResult2 := newResourceResult(c, "eggs", "ham")
	resultParams := params.ResourcesResults{
		Results: []params.ResourcesResult{apiResult1, apiResult2},
	}
	s.facade.EXPECT().FacadeCall("ListResources", args, gomock.Any()).SetArg(2, resultParams).Return(nil)

	results, err := s.client.ListResources([]string{"a-application", "other-application"})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(results, jc.DeepEquals, []resources.ApplicationResources{
		{Resources: expected1},
		{Resources: expected2},
	})
}

func (s *ListResourcesSuite) TestBadApplication(c *gc.C) {
	defer s.setUpMocks(c).Finish()

	_, err := s.client.ListResources([]string{"???"})
	c.Check(err, gc.ErrorMatches, `.*invalid application.*`)
}

func (s *ListResourcesSuite) TestEmptyResources(c *gc.C) {
	defer s.setUpMocks(c).Finish()

	args := &params.ListResourcesArgs{[]params.Entity{{
		Tag: "application-a-application",
	}, {
		Tag: "application-other-application",
	}}}
	resultParams := params.ResourcesResults{
		Results: []params.ResourcesResult{{}, {}},
	}
	s.facade.EXPECT().FacadeCall("ListResources", args, gomock.Any()).SetArg(2, resultParams).Return(nil)

	results, err := s.client.ListResources([]string{"a-application", "other-application"})
	c.Assert(err, jc.ErrorIsNil)
	c.Check(results, jc.DeepEquals, []resources.ApplicationResources{{}, {}})
}

func (s *ListResourcesSuite) TestServerError(c *gc.C) {
	defer s.setUpMocks(c).Finish()

	args := &params.ListResourcesArgs{[]params.Entity{{
		Tag: "application-a-application",
	}}}
	resultParams := params.ResourcesResults{
		Results: []params.ResourcesResult{{}},
	}
	s.facade.EXPECT().FacadeCall("ListResources", args, gomock.Any()).SetArg(2, resultParams).Return(errors.New("boom"))

	_, err := s.client.ListResources([]string{"a-application"})
	c.Assert(err, gc.ErrorMatches, "boom")
}

func (s *ListResourcesSuite) TestArity(c *gc.C) {
	defer s.setUpMocks(c).Finish()

	args := &params.ListResourcesArgs{[]params.Entity{{
		Tag: "application-a-application",
	}, {
		Tag: "application-other-application",
	}}}
	resultParams := params.ResourcesResults{
		Results: []params.ResourcesResult{{}},
	}
	s.facade.EXPECT().FacadeCall("ListResources", args, gomock.Any()).SetArg(2, resultParams).Return(nil)

	_, err := s.client.ListResources([]string{"a-application", "other-application"})
	c.Assert(err, gc.ErrorMatches, "expected 2 results, got 1")
}

func (s *ListResourcesSuite) TestConversionFailed(c *gc.C) {
	defer s.setUpMocks(c).Finish()

	args := &params.ListResourcesArgs{[]params.Entity{{
		Tag: "application-a-application",
	}}}
	resultParams := params.ResourcesResults{
		Results: []params.ResourcesResult{{
			ErrorResult: params.ErrorResult{Error: &params.Error{Message: "boom"}},
		}},
	}
	s.facade.EXPECT().FacadeCall("ListResources", args, gomock.Any()).SetArg(2, resultParams).Return(nil)

	_, err := s.client.ListResources([]string{"a-application"})
	c.Assert(err, gc.ErrorMatches, "boom")
}
