// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resources_test

import (
	"context"
	"reflect"
	"testing"

	"github.com/canonical/gomock/gomock"
	"github.com/juju/errors"
	"github.com/juju/tc"

	"github.com/juju/juju/api/base/mocks"
	"github.com/juju/juju/api/client/resources"
	coreresource "github.com/juju/juju/core/resource"
	"github.com/juju/juju/rpc/params"
)

func TestListResourcesSuite(t *testing.T) {
	tc.Run(t, &ListResourcesSuite{})
}

type ListResourcesSuite struct{}

func (s *ListResourcesSuite) TestListResources(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	args := &params.ListResourcesArgs{[]params.Entity{{
		Tag: "application-a-application",
	}, {
		Tag: "application-other-application",
	}}}
	expected1, apiResult1 := newResourceResult(c, "spam")
	expected2, apiResult2 := newResourceResult(c, "eggs", "ham")
	result := new(params.ResourcesResults)
	results := params.ResourcesResults{
		Results: []params.ResourcesResult{apiResult1, apiResult2},
	}

	mockFacadeCaller := mocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(
		gomock.Any(), "ListResources", args, result,
	).DoAndReturn(func(_ context.Context, _ string, _ interface{}, resPtr interface{}) error {
		reflect.ValueOf(resPtr).Elem().Set(reflect.ValueOf(results))
		return nil
	})
	client := resources.NewClientFromCaller(mockFacadeCaller)

	res, err := client.ListResources(c.Context(), []string{"a-application", "other-application"})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(res, tc.DeepEquals, []coreresource.ApplicationResources{
		{Resources: expected1},
		{Resources: expected2},
	})
}

func (s *ListResourcesSuite) TestBadApplication(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	mockFacadeCaller := mocks.NewMockFacadeCaller(ctrl)
	client := resources.NewClientFromCaller(mockFacadeCaller)
	_, err := client.ListResources(c.Context(), []string{"???"})
	c.Check(err, tc.ErrorMatches, `.*invalid application.*`)
}

func (s *ListResourcesSuite) TestEmptyResources(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	args := &params.ListResourcesArgs{[]params.Entity{{
		Tag: "application-a-application",
	}, {
		Tag: "application-other-application",
	}}}
	result := new(params.ResourcesResults)
	results := params.ResourcesResults{
		Results: []params.ResourcesResult{{}, {}},
	}
	mockFacadeCaller := mocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(
		gomock.Any(), "ListResources", args, result,
	).DoAndReturn(func(_ context.Context, _ string, _ interface{}, resPtr interface{}) error {
		reflect.ValueOf(resPtr).Elem().Set(reflect.ValueOf(results))
		return nil
	})
	client := resources.NewClientFromCaller(mockFacadeCaller)

	res, err := client.ListResources(c.Context(), []string{"a-application", "other-application"})
	c.Assert(err, tc.ErrorIsNil)
	c.Check(res, tc.DeepEquals, []coreresource.ApplicationResources{{}, {}})
}

func (s *ListResourcesSuite) TestServerError(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	args := &params.ListResourcesArgs{[]params.Entity{{
		Tag: "application-a-application",
	}}}
	result := new(params.ResourcesResults)
	results := params.ResourcesResults{
		Results: []params.ResourcesResult{{}},
	}
	mockFacadeCaller := mocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(
		gomock.Any(), "ListResources", args, result,
	).DoAndReturn(func(_ context.Context, _ string, _ interface{}, resPtr interface{}) error {
		reflect.ValueOf(resPtr).Elem().Set(reflect.ValueOf(results))
		return errors.New("boom")
	})
	client := resources.NewClientFromCaller(mockFacadeCaller)

	_, err := client.ListResources(c.Context(), []string{"a-application"})
	c.Assert(err, tc.ErrorMatches, "boom")
}

func (s *ListResourcesSuite) TestArity(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	args := &params.ListResourcesArgs{[]params.Entity{{
		Tag: "application-a-application",
	}, {
		Tag: "application-other-application",
	}}}
	result := new(params.ResourcesResults)
	results := params.ResourcesResults{
		Results: []params.ResourcesResult{{}},
	}
	mockFacadeCaller := mocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(
		gomock.Any(), "ListResources", args, result,
	).DoAndReturn(func(_ context.Context, _ string, _ interface{}, resPtr interface{}) error {
		reflect.ValueOf(resPtr).Elem().Set(reflect.ValueOf(results))
		return nil
	})
	client := resources.NewClientFromCaller(mockFacadeCaller)

	_, err := client.ListResources(c.Context(), []string{"a-application", "other-application"})
	c.Assert(err, tc.ErrorMatches, "expected 2 results, got 1")
}

func (s *ListResourcesSuite) TestConversionFailed(c *tc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	args := &params.ListResourcesArgs{[]params.Entity{{
		Tag: "application-a-application",
	}}}
	result := new(params.ResourcesResults)
	results := params.ResourcesResults{
		Results: []params.ResourcesResult{{
			ErrorResult: params.ErrorResult{Error: &params.Error{Message: "boom"}},
		}},
	}
	mockFacadeCaller := mocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall(
		gomock.Any(), "ListResources", args, result,
	).DoAndReturn(func(_ context.Context, _ string, _ interface{}, resPtr interface{}) error {
		reflect.ValueOf(resPtr).Elem().Set(reflect.ValueOf(results))
		return nil
	})
	client := resources.NewClientFromCaller(mockFacadeCaller)

	_, err := client.ListResources(c.Context(), []string{"a-application"})
	c.Assert(err, tc.ErrorMatches, "boom")
}
