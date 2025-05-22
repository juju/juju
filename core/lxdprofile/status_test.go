// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxdprofile_test

import (
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/core/lxdprofile"
	"github.com/juju/juju/internal/testhelpers"
)

type LXDProfileStatusSuite struct {
	testhelpers.IsolationSuite
}

func TestLXDProfileStatusSuite(t *testing.T) {
	tc.Run(t, &LXDProfileStatusSuite{})
}

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
	for k, testCase := range testCases {
		c.Logf("running test %d with input %q", k, testCase.input)
		c.Assert(lxdprofile.UpgradeStatusFinished(testCase.input), tc.Equals, testCase.output)
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
	for k, testCase := range testCases {
		c.Logf("running test %d with input %q", k, testCase.input)
		c.Assert(lxdprofile.UpgradeStatusTerminal(testCase.input), tc.Equals, testCase.output)
	}
}
