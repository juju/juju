// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package bundle_test

import (
	"github.com/golang/mock/gomock"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	basemocks "github.com/juju/juju/api/base/mocks"
	"github.com/juju/juju/api/client/bundle"
	"github.com/juju/juju/rpc/params"
)

type bundleMockSuite struct{}

var _ = gc.Suite(&bundleMockSuite{})

func (s *bundleMockSuite) TestExportBundleLatest(c *gc.C) {
	ctrl := gomock.NewController(c)
	defer ctrl.Finish()

	bundleStr := `applications:
	ubuntu:
		charm: ch:ubuntu
		series: jammy
		num_units: 1
		to:
			- \"0\"
		options:
			key: value
			series: focal
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
	result, err := client.ExportBundle(true)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, jc.DeepEquals, bundleStr)
}
