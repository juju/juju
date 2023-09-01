// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bundle_test

import (
	jc "github.com/juju/testing/checkers"
	"go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"

	basemocks "github.com/juju/juju/api/base/mocks"
	"github.com/juju/juju/api/client/bundle"
	"github.com/juju/juju/rpc/params"
)

type bundleMockSuite struct{}

var _ = gc.Suite(&bundleMockSuite{})

func (s *bundleMockSuite) TestGetChangesMapArgs(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	bundleURL := "ch:bundle-url"
	bundleYAML := `applications:
	ubuntu:
		charm: ch:ubuntu
		series: jammy
		num_units: 1
		options:
			key: value
			series: focal`
	changes := []*params.BundleChangesMapArgs{
		{
			Id:     "addCharm-0",
			Method: "addCharm",
			Args: map[string]interface{}{
				"charm":  "ch:ubuntu",
				"series": "jammy",
			},
			Requires: []string{},
		},
		{
			Id:     "deploy-1",
			Method: "deploy",
			Args: map[string]interface{}{
				"charm":     "$addCharm-0",
				"series":    "jammy",
				"num_units": "1",
				"options": map[string]interface{}{
					"key":    "value",
					"series": "focal",
				},
			},
			Requires: []string{"$addCharm-0"},
		},
	}

	args := params.BundleChangesParams{
		BundleDataYAML: bundleYAML,
		BundleURL:      bundleURL,
	}
	res := new(params.BundleChangesMapArgsResults)
	results := params.BundleChangesMapArgsResults{
		Changes: changes,
	}
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall("GetChangesMapArgs", args, res).SetArg(2, results).Return(nil)
	client := bundle.NewClientFromCaller(mockFacadeCaller)
	result, err := client.GetChangesMapArgs(bundleURL, bundleYAML)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Errors, gc.DeepEquals, []string(nil))
	c.Assert(result.Changes, gc.DeepEquals, changes)
}

func (s *bundleMockSuite) TestGetChangesMapArgsReturnsErrors(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	bundleURL := "ch:bundle-url"
	bundleYAML := `applications:
	ubuntu:
		charm: ch:ubuntu
		series: jammy
		num_units: 1
		options:
			key: value
			series: focal
		relations:
			- []`

	args := params.BundleChangesParams{
		BundleDataYAML: bundleYAML,
		BundleURL:      bundleURL,
	}
	res := new(params.BundleChangesMapArgsResults)
	results := params.BundleChangesMapArgsResults{
		Errors: []string{
			"Error returned from request",
		},
	}
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall("GetChangesMapArgs", args, res).SetArg(2, results).Return(nil)
	client := bundle.NewClientFromCaller(mockFacadeCaller)
	result, err := client.GetChangesMapArgs(bundleURL, bundleYAML)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result.Errors, gc.DeepEquals, []string{"Error returned from request"})
	c.Assert(result.Changes, gc.DeepEquals, []*params.BundleChangesMapArgs(nil))
}

func (s *bundleMockSuite) TestExportBundleLatest(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	bundleStr := `applications:
	ubuntu:
		charm: ch:ubuntu
		base: ubuntu@22.04/stable
		num_units: 1
		to:
			- \"0\"
		options:
			key: value
			base: ubuntu@22.04/stable
		relations:
			- []`

	args := params.ExportBundleParams{
		IncludeCharmDefaults: true,
	}
	res := new(params.StringResult)
	results := params.StringResult{
		Result: bundleStr,
	}
	mockFacadeCaller := basemocks.NewMockFacadeCaller(ctrl)
	mockFacadeCaller.EXPECT().FacadeCall("ExportBundle", args, res).SetArg(2, results).Return(nil)
	client := bundle.NewClientFromCaller(mockFacadeCaller)
	result, err := client.ExportBundle(true, false)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, bundleStr)
}
