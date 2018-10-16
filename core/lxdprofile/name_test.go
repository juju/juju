// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxdprofile_test

import (
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/lxdprofile"
	"github.com/juju/testing"
)

type LXDProfileNameSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&LXDProfileNameSuite{})

func (*LXDProfileNameSuite) TestProfileNames(c *gc.C) {
	testCases := []struct {
		input  []string
		output []string
	}{
		{
			input:  []string{},
			output: []string{},
		},
		{
			input:  []string{"default"},
			output: []string{},
		},
		{
			input: []string{
				lxdprofile.Name("foo", "bar", 1),
			},
			output: []string{
				lxdprofile.Name("foo", "bar", 1),
			},
		},
		{
			input: []string{
				"default",
				lxdprofile.Name("foo", "bar", 1),
				lxdprofile.Name("foo", "bar", 1),
				lxdprofile.Name("aaa", "bbb", 100),
			},
			output: []string{
				lxdprofile.Name("foo", "bar", 1),
				lxdprofile.Name("aaa", "bbb", 100),
			},
		},
		{
			input: []string{
				"default",
				lxdprofile.Name("foo", "bar", 1),
				lxdprofile.Name("foo", "bar", 1),
				"some-other-profile",
				lxdprofile.Name("aaa", "bbb", 100),
			},
			output: []string{
				lxdprofile.Name("foo", "bar", 1),
				lxdprofile.Name("aaa", "bbb", 100),
			},
		},
	}
	for k, tc := range testCases {
		c.Logf("running test %d with input %s", k, tc.input)
		c.Assert(lxdprofile.LXDProfileNames(tc.input), gc.DeepEquals, tc.output)
	}
}
