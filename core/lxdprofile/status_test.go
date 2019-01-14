// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxdprofile_test

import (
	"github.com/juju/testing"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/lxdprofile"
)

type LXDProfileStatusSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&LXDProfileStatusSuite{})

func (*LXDProfileStatusSuite) TestUpgradeStatusFinished(c *gc.C) {
	testCases := []struct {
		input  string
		output bool
	}{
		{
			input:  "",
			output: false,
		},
		{
			input:  lxdprofile.SuccessStatus,
			output: true,
		},
		{
			input:  lxdprofile.NotRequiredStatus,
			output: true,
		},
		{
			input:  lxdprofile.NotSupportedStatus,
			output: true,
		},
		{
			input:  lxdprofile.NotKnownStatus,
			output: false,
		},
		{
			input:  lxdprofile.ErrorStatus,
			output: false,
		},
	}
	for k, tc := range testCases {
		c.Logf("running test %d with input %q", k, tc.input)
		c.Assert(lxdprofile.UpgradeStatusFinished(tc.input), gc.Equals, tc.output)
	}
}

func (*LXDProfileStatusSuite) TestUpgradeStatusTerminal(c *gc.C) {
	testCases := []struct {
		input  string
		output bool
	}{
		{
			input:  "",
			output: false,
		},
		{
			input:  lxdprofile.SuccessStatus,
			output: true,
		},
		{
			input:  lxdprofile.NotRequiredStatus,
			output: true,
		},
		{
			input:  lxdprofile.NotSupportedStatus,
			output: true,
		},
		{
			input:  lxdprofile.NotKnownStatus,
			output: false,
		},
		{
			input:  lxdprofile.ErrorStatus,
			output: true,
		},
	}
	for k, tc := range testCases {
		c.Logf("running test %d with input %q", k, tc.input)
		c.Assert(lxdprofile.UpgradeStatusTerminal(tc.input), gc.Equals, tc.output)
	}
}
