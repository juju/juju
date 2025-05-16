// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bundle_test

import (
	stdtesting "testing"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	basemocks "github.com/juju/juju/api/base/mocks"
	"github.com/juju/juju/api/client/bundle"
	"github.com/juju/juju/rpc/params"
)

type bundleMockSuite struct{}

func TestBundleMockSuite(t *stdtesting.T) { tc.Run(t, &bundleMockSuite{}) }
func (s *bundleMockSuite) TestGetChangesMapArgs(c *tc.C) {
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
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "GetChangesMapArgs", args, res).SetArg(3, results).Return(nil)
	client := bundle.NewClientFromCaller(mockFacadeCaller)
	result, err := client.GetChangesMapArgs(c.Context(), bundleURL, bundleYAML)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Errors, tc.DeepEquals, []string(nil))
	c.Assert(result.Changes, tc.DeepEquals, changes)
}

func (s *bundleMockSuite) TestGetChangesMapArgsReturnsErrors(c *tc.C) {
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
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "GetChangesMapArgs", args, res).SetArg(3, results).Return(nil)
	client := bundle.NewClientFromCaller(mockFacadeCaller)
	result, err := client.GetChangesMapArgs(c.Context(), bundleURL, bundleYAML)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result.Errors, tc.DeepEquals, []string{"Error returned from request"})
	c.Assert(result.Changes, tc.DeepEquals, []*params.BundleChangesMapArgs(nil))
}

func (s *bundleMockSuite) TestExportBundleLatest(c *tc.C) {
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
	mockFacadeCaller.EXPECT().FacadeCall(gomock.Any(), "ExportBundle", args, res).SetArg(3, results).Return(nil)
	client := bundle.NewClientFromCaller(mockFacadeCaller)
	result, err := client.ExportBundle(c.Context(), true)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(result, tc.DeepEquals, bundleStr)
}
