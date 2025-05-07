// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package deployer

import (
	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"

	"github.com/juju/juju/core/constraints"
	"github.com/juju/juju/internal/charm"
)

type bundleSuite struct {
}

var _ = tc.Suite(&bundleSuite{})

func (s *bundleSuite) TestCheckExplicitBase(c *tc.C) {
	explicitBaseErrorUbuntu := "base must be explicitly provided for \"ch:ubuntu\" when image-id constraint is used"
	explicitBaseError := "base must be explicitly provided for(.)*"

	testCases := []struct {
		title         string
		deployBundle  deployBundle
		bundleData    *charm.BundleData
		expectedError string
	}{
		{
			title: "two apps, no image-id, no base -> no error",
			bundleData: &charm.BundleData{
				Applications: map[string]*charm.ApplicationSpec{
					"prometheus2": {
						Charm:       "ch:prometheus2",
						Constraints: "cpu-cores=2",
					},
					"ubuntu": {
						Charm:       "ch:ubuntu",
						Constraints: "mem=2G",
					},
				},
			},
			deployBundle: deployBundle{},
		},
		{
			title: "two apps, one with image-id, no base -> error",
			bundleData: &charm.BundleData{
				Applications: map[string]*charm.ApplicationSpec{
					"prometheus2": {
						Charm:       "ch:prometheus2",
						Constraints: "image-id=ubuntu-bf2",
					},
					"ubuntu": {
						Charm:       "ch:ubuntu",
						Constraints: "mem=2G",
					},
				},
			},
			deployBundle:  deployBundle{},
			expectedError: explicitBaseError,
		},
		{
			title: "two apps, model with image-id, no base -> error",
			bundleData: &charm.BundleData{
				Applications: map[string]*charm.ApplicationSpec{
					"prometheus2": {
						Charm:       "ch:prometheus2",
						Constraints: "cpu-cores=2",
					},
					"ubuntu": {
						Charm:       "ch:ubuntu",
						Constraints: "mem=2G",
					},
				},
			},
			deployBundle: deployBundle{
				modelConstraints: constraints.Value{
					ImageID: strptr("ubuntu-bf2"),
				},
			},
			expectedError: explicitBaseError,
		},
		{
			title: "two apps, model and one app with image-id, no base -> error",
			bundleData: &charm.BundleData{
				Applications: map[string]*charm.ApplicationSpec{
					"prometheus2": {
						Charm:       "ch:prometheus2",
						Constraints: "image-id=ubuntu-bf2",
					},
					"ubuntu": {
						Charm:       "ch:ubuntu",
						Constraints: "mem=2G",
					},
				},
			},
			deployBundle: deployBundle{
				modelConstraints: constraints.Value{
					ImageID: strptr("ubuntu-bf2"),
				},
			},
			expectedError: explicitBaseError,
		},
		{
			title: "two apps, machine with image-id in (app).To, no base -> error",
			bundleData: &charm.BundleData{
				Applications: map[string]*charm.ApplicationSpec{
					"prometheus2": {
						Charm:       "ch:prometheus2",
						Constraints: "cpu-cores=2",
					},
					"ubuntu": {
						Charm:       "ch:ubuntu",
						Constraints: "mem=2G",
						To:          []string{"0"},
					},
				},
				Machines: map[string]*charm.MachineSpec{
					"0": {
						Constraints: "image-id=ubuntu-bf2",
					},
					"1": {
						Constraints: "mem=2G",
					},
				},
			},
			deployBundle:  deployBundle{},
			expectedError: explicitBaseErrorUbuntu,
		},
		{
			title: "two apps, machine with image-id not in (app).To, no base -> no error",
			bundleData: &charm.BundleData{
				Applications: map[string]*charm.ApplicationSpec{
					"prometheus2": {
						Charm:       "ch:prometheus2",
						Constraints: "cpu-cores=2",
					},
					"ubuntu": {
						Charm:       "ch:ubuntu",
						Constraints: "mem=2G",
						To:          []string{"1"},
					},
				},
				Machines: map[string]*charm.MachineSpec{
					"0": {
						Constraints: "image-id=ubuntu-bf2",
					},
					"1": {
						Constraints: "mem=2G",
					},
				},
			},
			deployBundle: deployBundle{},
		},
		{
			title: "two apps, one with image-id, base in same app -> no error",
			bundleData: &charm.BundleData{
				Applications: map[string]*charm.ApplicationSpec{
					"prometheus2": {
						Charm:       "ch:prometheus2",
						Base:        "ubuntu@20.04",
						Constraints: "image-id=ubuntu-bf2",
					},
					"ubuntu": {
						Charm:       "ch:ubuntu",
						Constraints: "mem=2G",
					},
				},
			},
			deployBundle: deployBundle{},
		},
		{
			title: "two apps, model with image-id, base in one app -> error",
			bundleData: &charm.BundleData{
				Applications: map[string]*charm.ApplicationSpec{
					"prometheus2": {
						Charm:       "ch:prometheus2",
						Base:        "ubuntu@20.04",
						Constraints: "cpu-cores=2",
					},
					"ubuntu": {
						Charm:       "ch:ubuntu",
						Constraints: "mem=2G",
					},
				},
			},
			deployBundle: deployBundle{
				modelConstraints: constraints.Value{
					ImageID: strptr("ubuntu-bf2"),
				},
			},
			expectedError: explicitBaseErrorUbuntu,
		},
		{
			title: "two apps, model with image-id, base in two apps -> no error",
			bundleData: &charm.BundleData{
				Applications: map[string]*charm.ApplicationSpec{
					"prometheus2": {
						Charm:       "ch:prometheus2",
						Base:        "ubuntu@20.04",
						Constraints: "cpu-cores=2",
					},
					"ubuntu": {
						Charm:       "ch:ubuntu",
						Base:        "ubuntu@20.04",
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
			title: "two apps, model and one app with image-id, base in one app -> error",

			bundleData: &charm.BundleData{
				Applications: map[string]*charm.ApplicationSpec{
					"prometheus2": {
						Charm:       "ch:prometheus2",
						Base:        "ubuntu@20.04",
						Constraints: "image-id=ubuntu-bf2",
					},
					"ubuntu": {
						Charm:       "ch:ubuntu",
						Constraints: "mem=2G",
					},
				},
			},
			deployBundle: deployBundle{
				modelConstraints: constraints.Value{
					ImageID: strptr("ubuntu-bf2"),
				},
			},
			expectedError: explicitBaseErrorUbuntu,
		},
		{
			title: "two apps, model and one app with image-id, base in two apps -> no error",

			bundleData: &charm.BundleData{
				Applications: map[string]*charm.ApplicationSpec{
					"prometheus2": {
						Charm:       "ch:prometheus2",
						Base:        "ubuntu@20.04",
						Constraints: "image-id=ubuntu-bf2",
					},
					"ubuntu": {
						Charm:       "ch:ubuntu",
						Base:        "ubuntu@20.04",
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
			title: "two apps, machine with image-id in (app).To, base in app -> no error",
			bundleData: &charm.BundleData{
				Applications: map[string]*charm.ApplicationSpec{
					"prometheus2": {
						Charm:       "ch:prometheus2",
						Constraints: "cpu-cores=2",
					},
					"ubuntu": {
						Charm:       "ch:ubuntu",
						Base:        "ubuntu@22.04",
						Constraints: "mem=2G",
						To:          []string{"0"},
					},
				},
				Machines: map[string]*charm.MachineSpec{
					"0": {
						Constraints: "image-id=ubuntu-bf2",
					},
					"1": {
						Constraints: "mem=2G",
					},
				},
			},
			deployBundle: deployBundle{},
		},
		{
			title: "two apps, one with image-id, base in bundle -> no error",
			bundleData: &charm.BundleData{
				DefaultBase: "ubuntu@20.04",
				Applications: map[string]*charm.ApplicationSpec{
					"prometheus2": {
						Charm:       "ch:prometheus2",
						Constraints: "image-id=ubuntu-bf2",
					},
					"ubuntu": {
						Charm:       "ch:ubuntu",
						Constraints: "mem=2G",
					},
				},
			},
			deployBundle: deployBundle{},
		},
		{
			title: "two apps, model with image-id, base in bundle -> no error",
			bundleData: &charm.BundleData{
				DefaultBase: "ubuntu@20.04",
				Applications: map[string]*charm.ApplicationSpec{
					"prometheus2": {
						Charm:       "ch:prometheus2",
						Constraints: "cpu-cores=2",
					},
					"ubuntu": {
						Charm:       "ch:ubuntu",
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
			title: "two apps, model with image-id, base in bundle and app -> no error",
			bundleData: &charm.BundleData{
				DefaultBase: "ubuntu@20.04",
				Applications: map[string]*charm.ApplicationSpec{
					"prometheus2": {
						Charm:       "ch:prometheus2",
						Constraints: "cpu-cores=2",
					},
					"ubuntu": {
						Charm:       "ch:ubuntu",
						Base:        "ubuntu@22.04",
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
			title: "two apps, machine with image-id in (app).To, base in bundle -> no error",
			bundleData: &charm.BundleData{
				DefaultBase: "ubuntu@20.04",
				Applications: map[string]*charm.ApplicationSpec{
					"prometheus2": {
						Charm:       "ch:prometheus2",
						Constraints: "cpu-cores=2",
					},
					"ubuntu": {
						Charm:       "ch:ubuntu",
						Constraints: "mem=2G",
						To:          []string{"0"},
					},
				},
				Machines: map[string]*charm.MachineSpec{
					"0": {
						Constraints: "image-id=ubuntu-bf2",
					},
					"1": {
						Constraints: "mem=2G",
					},
				},
			},
			deployBundle: deployBundle{},
		},
		{
			title: "application targeting new container, no base -> error",
			bundleData: &charm.BundleData{
				Applications: map[string]*charm.ApplicationSpec{
					"prometheus2": {
						Charm:       "ch:prometheus2",
						Constraints: "cpu-cores=2",
					},
					"ubuntu": {
						Charm:       "ch:ubuntu",
						Constraints: "mem=2G",
						To:          []string{"lxc:new"},
					},
				},
				Machines: map[string]*charm.MachineSpec{
					"0": {
						Constraints: "image-id=ubuntu-bf2",
					},
				},
			},
			deployBundle: deployBundle{},
		},
		{
			title: "application targeting new machine, no base -> error",
			bundleData: &charm.BundleData{
				Applications: map[string]*charm.ApplicationSpec{
					"prometheus2": {
						Charm:       "ch:prometheus2",
						Constraints: "cpu-cores=2",
					},
					"ubuntu": {
						Charm:       "ch:ubuntu",
						Constraints: "mem=2G",
						To:          []string{"new"},
					},
				},
				Machines: map[string]*charm.MachineSpec{
					"0": {
						Constraints: "image-id=ubuntu-bf2",
					},
				},
			},
			deployBundle: deployBundle{},
		},
		{
			title: "application targeting container in bundle, no base -> error",
			bundleData: &charm.BundleData{
				Applications: map[string]*charm.ApplicationSpec{
					"prometheus2": {
						Charm:       "ch:prometheus2",
						Constraints: "cpu-cores=2",
					},
					"ubuntu": {
						Charm:       "ch:ubuntu",
						Constraints: "mem=2G",
						To:          []string{"lxd:0"},
					},
				},
				Machines: map[string]*charm.MachineSpec{
					"0": {
						Constraints: "image-id=ubuntu-bf2",
					},
				},
			},
			deployBundle:  deployBundle{},
			expectedError: explicitBaseErrorUbuntu,
		},
		{
			title: "application targeting container in bundle, base in bundle -> no error",
			bundleData: &charm.BundleData{
				DefaultBase: "ubuntu@20.04",
				Applications: map[string]*charm.ApplicationSpec{
					"prometheus2": {
						Charm:       "ch:prometheus2",
						Constraints: "cpu-cores=2",
						To:          []string{"ubuntu:0"},
					},
					"ubuntu": {
						Charm:       "ch:ubuntu",
						Constraints: "mem=2G",
						To:          []string{"lxd:0"},
					},
				},
				Machines: map[string]*charm.MachineSpec{
					"0": {
						Constraints: "image-id=ubuntu-bf2",
					},
				},
			},
			deployBundle: deployBundle{},
		},
		{
			title: "ensure nil machine spec produces no error",
			bundleData: &charm.BundleData{
				DefaultBase: "ubuntu@20.04",
				Applications: map[string]*charm.ApplicationSpec{
					"prometheus2": {
						Charm:       "ch:prometheus2",
						Constraints: "cpu-cores=2",
						To:          []string{"ubuntu:0"},
					},
					"ubuntu": {
						Charm:       "ch:ubuntu",
						Constraints: "mem=2G",
						To:          []string{"lxd:0"},
					},
				},
				Machines: map[string]*charm.MachineSpec{
					"0": nil,
				},
			},
			deployBundle: deployBundle{},
		},
	}
	for i, test := range testCases {
		c.Logf("test %d [%s]", i, test.title)

		err := test.deployBundle.checkExplicitBase(test.bundleData)

		if test.expectedError != "" {
			c.Check(err, tc.ErrorMatches, test.expectedError)
		} else {
			c.Check(err, jc.ErrorIsNil)
		}
	}
}
