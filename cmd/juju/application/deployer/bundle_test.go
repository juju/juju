// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package deployer

import (
	"github.com/juju/charm/v10"
	"github.com/juju/juju/core/constraints"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

type bundleSuite struct {
}

var _ = gc.Suite(&bundleSuite{})

func (s *bundleSuite) TestCheckExplicitBase(c *gc.C) {
	explicitSeriesError := "series must be explicitly provided when image-id constraint is used"

	testCases := []struct {
		title         string
		deployBundle  deployBundle
		bundleData    *charm.BundleData
		expectedError string
	}{
		// NO IMAGE-ID
		{
			title: "two apps, no image-id, no series -> no error",
			bundleData: &charm.BundleData{
				Applications: map[string]*charm.ApplicationSpec{
					"prometheus2": {
						Constraints: "cpu-cores=2",
					},
					"ubuntu": {
						Constraints: "mem=2G",
					},
				},
			},
			deployBundle: deployBundle{},
		},
		// NO SERIES
		{
			title: "two apps, one with image-id, no series -> error",
			bundleData: &charm.BundleData{
				Applications: map[string]*charm.ApplicationSpec{
					"prometheus2": {
						Constraints: "image-id=ubuntu-bf2",
					},
					"ubuntu": {
						Constraints: "mem=2G",
					},
				},
			},
			deployBundle:  deployBundle{},
			expectedError: explicitSeriesError,
		},
		{
			title: "two apps, model with image-id, no series -> error",
			bundleData: &charm.BundleData{
				Applications: map[string]*charm.ApplicationSpec{
					"prometheus2": {
						Constraints: "cpu-cores=2",
					},
					"ubuntu": {
						Constraints: "mem=2G",
					},
				},
			},
			deployBundle: deployBundle{
				modelConstraints: constraints.Value{
					ImageID: strptr("ubuntu-bf2"),
				},
			},
			expectedError: explicitSeriesError,
		},
		{
			title: "two apps, model and one app with image-id, no series -> error",
			bundleData: &charm.BundleData{
				Applications: map[string]*charm.ApplicationSpec{
					"prometheus2": {
						Constraints: "image-id=ubuntu-bf2",
					},
					"ubuntu": {
						Constraints: "mem=2G",
					},
				},
			},
			deployBundle: deployBundle{
				modelConstraints: constraints.Value{
					ImageID: strptr("ubuntu-bf2"),
				},
			},
			expectedError: explicitSeriesError,
		},
		// SERIES IN APPS
		{
			title: "two apps, one with image-id, series in same app -> no error",
			bundleData: &charm.BundleData{
				Applications: map[string]*charm.ApplicationSpec{
					"prometheus2": {
						Series:      "focal",
						Constraints: "image-id=ubuntu-bf2",
					},
					"ubuntu": {
						Constraints: "mem=2G",
					},
				},
			},
			deployBundle: deployBundle{},
		},
		{
			title: "two apps, model with image-id, series in one app -> error",
			bundleData: &charm.BundleData{
				Applications: map[string]*charm.ApplicationSpec{
					"prometheus2": {
						Series:      "focal",
						Constraints: "cpu-cores=2",
					},
					"ubuntu": {
						Constraints: "mem=2G",
					},
				},
			},
			deployBundle: deployBundle{
				modelConstraints: constraints.Value{
					ImageID: strptr("ubuntu-bf2"),
				},
			},
			expectedError: explicitSeriesError,
		},
		{
			title: "two apps, model with image-id, series in two apps -> no error",
			bundleData: &charm.BundleData{
				Applications: map[string]*charm.ApplicationSpec{
					"prometheus2": {
						Series:      "focal",
						Constraints: "cpu-cores=2",
					},
					"ubuntu": {
						Series:      "focal",
						Constraints: "mem=2G",
					},
				},
			},
			deployBundle: deployBundle{
				modelConstraints: constraints.Value{
					ImageID: strptr("ubuntu-bf2"),
				},
			},
		},
		{
			title: "two apps, model and one app with image-id, series in one app -> error",

			bundleData: &charm.BundleData{
				Applications: map[string]*charm.ApplicationSpec{
					"prometheus2": {
						Series:      "focal",
						Constraints: "image-id=ubuntu-bf2",
					},
					"ubuntu": {
						Constraints: "mem=2G",
					},
				},
			},
			deployBundle: deployBundle{
				modelConstraints: constraints.Value{
					ImageID: strptr("ubuntu-bf2"),
				},
			},
			expectedError: explicitSeriesError,
		},
		{
			title: "two apps, model and one app with image-id, series in two apps -> no error",

			bundleData: &charm.BundleData{
				Applications: map[string]*charm.ApplicationSpec{
					"prometheus2": {
						Series:      "focal",
						Constraints: "image-id=ubuntu-bf2",
					},
					"ubuntu": {
						Series:      "focal",
						Constraints: "mem=2G",
					},
				},
			},
			deployBundle: deployBundle{
				modelConstraints: constraints.Value{
					ImageID: strptr("ubuntu-bf2"),
				},
			},
		},
		// SERIES IN BUNDLE
		{
			title: "two apps, one with image-id, series in bundle -> no error",
			bundleData: &charm.BundleData{
				Series: "focal",
				Applications: map[string]*charm.ApplicationSpec{
					"prometheus2": {
						Constraints: "image-id=ubuntu-bf2",
					},
					"ubuntu": {
						Constraints: "mem=2G",
					},
				},
			},
			deployBundle: deployBundle{},
		},
		{
			title: "two apps, model with image-id, series in bundle -> no error",
			bundleData: &charm.BundleData{
				Series: "focal",
				Applications: map[string]*charm.ApplicationSpec{
					"prometheus2": {
						Constraints: "cpu-cores=2",
					},
					"ubuntu": {
						Constraints: "mem=2G",
					},
				},
			},
			deployBundle: deployBundle{
				modelConstraints: constraints.Value{
					ImageID: strptr("ubuntu-bf2"),
				},
			},
		},
		{
			title: "two apps, model with image-id, series in bundle and app -> no error",
			bundleData: &charm.BundleData{
				Series: "focal",
				Applications: map[string]*charm.ApplicationSpec{
					"prometheus2": {
						Constraints: "cpu-cores=2",
					},
					"ubuntu": {
						Series:      "jammy",
						Constraints: "mem=2G",
					},
				},
			},
			deployBundle: deployBundle{
				modelConstraints: constraints.Value{
					ImageID: strptr("ubuntu-bf2"),
				},
			},
		},
	}
	for i, test := range testCases {
		c.Logf("test %d [%s]", i, test.title)

		err := test.deployBundle.checkExplicitSeries(test.bundleData)

		if test.expectedError != "" {
			c.Check(err, gc.ErrorMatches, test.expectedError)
		} else {
			c.Check(err, jc.ErrorIsNil)
		}
	}
}
