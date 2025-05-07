// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxdprofile_test

import (
	"github.com/juju/tc"

	"github.com/juju/juju/core/lxdprofile"
	"github.com/juju/juju/internal/testhelpers"
)

type LXDProfileStatusSuite struct {
	testhelpers.IsolationSuite
}

var _ = tc.Suite(&LXDProfileStatusSuite{})

func (*LXDProfileStatusSuite) TestUpgradeStatusFinished(c *tc.C) {
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
		c.Assert(lxdprofile.UpgradeStatusFinished(tc.input), tc.Equals, tc.output)
	}
}

func (*LXDProfileStatusSuite) TestUpgradeStatusTerminal(c *tc.C) {
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
		c.Assert(lxdprofile.UpgradeStatusTerminal(tc.input), tc.Equals, tc.output)
	}
}
