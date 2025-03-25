// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxdprofile_test

import (
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/lxdprofile"
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
				"default",
				"juju-model",
			},
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
		c.Logf("running test %d with input %q", k, tc.input)
		c.Assert(lxdprofile.FilterLXDProfileNames(tc.input), gc.DeepEquals, tc.output)
	}
}

func (*LXDProfileNameSuite) TestIsValidName(c *gc.C) {
	testCases := []struct {
		input  string
		output bool
	}{
		{
			input:  "",
			output: false,
		},
		{
			input:  "default",
			output: false,
		},
		{
			input:  "juju-model",
			output: false,
		},
		{
			input:  lxdprofile.Name("foo", "bar", 1),
			output: true,
		},
		{
			input:  lxdprofile.Name("aaa-zzz", "b312--?123!!bb-x__xx-012-y123yy", 100),
			output: true,
		},
	}
	for k, tc := range testCases {
		c.Logf("running test %d with input %q", k, tc.input)
		c.Assert(lxdprofile.IsValidName(tc.input), gc.Equals, tc.output)
	}
}

func (*LXDProfileNameSuite) TestProfileRevision(c *gc.C) {
	testCases := []struct {
		input  string
		output int
		err    string
	}{
		{
			input: "",
			err:   "not a juju profile name: \"\"",
		},
		{
			input: "default",
			err:   "not a juju profile name: \"default\"",
		},
		{
			input: "juju-model",
			err:   "not a juju profile name: \"juju-model\"",
		},
		{
			input:  lxdprofile.Name("foo", "bar", 1),
			output: 1,
		},
		{
			input:  lxdprofile.Name("aaa-zzz", "b312--?123!!bb-x__xx-012-y123yy", 100),
			output: 100,
		},
	}
	for k, tc := range testCases {
		c.Logf("running test %d of %d with input %q", k, len(testCases), tc.input)
		obtained, err := lxdprofile.ProfileRevision(tc.input)
		if tc.err != "" {
			c.Assert(err, gc.ErrorMatches, tc.err)
			continue
		}
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(obtained, gc.Equals, tc.output)
	}
}

func (*LXDProfileNameSuite) TestProfileReplaceRevision(c *gc.C) {
	testCases := []struct {
		input    string
		inputRev int
		output   string
		err      string
	}{
		{
			input: "",
			err:   "not a juju profile name: \"\"",
		},
		{
			input: "default",
			err:   "not a juju profile name: \"default\"",
		},
		{
			input: "juju-model",
			err:   "not a juju profile name: \"juju-model\"",
		},
		{
			input:    lxdprofile.Name("foo", "bar", 1),
			inputRev: 4,
			output:   lxdprofile.Name("foo", "bar", 4),
		},
		{
			input:    lxdprofile.Name("aaa-zzz", "b312--?123!!bb-x__xx-012-y123yy", 123),
			inputRev: 312,
			output:   lxdprofile.Name("aaa-zzz", "b312--?123!!bb-x__xx-012-y123yy", 312),
		},
	}
	for k, tc := range testCases {
		c.Logf("running test %d of %d with input %q", k, len(testCases), tc.input)
		obtained, err := lxdprofile.ProfileReplaceRevision(tc.input, tc.inputRev)
		if tc.err != "" {
			c.Assert(err, gc.ErrorMatches, tc.err)
			continue
		}
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(obtained, gc.Equals, tc.output)
	}
}

func (*LXDProfileNameSuite) TestMatchProfileNameByAppName(c *gc.C) {
	testCases := []struct {
		input    []string
		inputApp string
		output   string
		err      string
	}{
		{
			input:    []string{},
			inputApp: "",
			err:      "no application name specified bad request",
		},
		{
			input:    []string{"default"},
			inputApp: "one",
			output:   "",
		},
		{
			input:    []string{"default", "juju-model"},
			inputApp: "one",
			output:   "",
		},
		{
			input: []string{
				"default",
				"juju-model",
				lxdprofile.Name("foo", "bar", 2),
			},
			inputApp: "bar",
			output:   lxdprofile.Name("foo", "bar", 2),
		},
		{
			input: []string{
				"default",
				"juju-model",
				lxdprofile.Name("foo", "nonebar", 2),
				lxdprofile.Name("foo", "bar", 2),
			},
			inputApp: "bar",
			output:   lxdprofile.Name("foo", "bar", 2),
		},
		{
			input: []string{
				"default",
				"juju-model",
				lxdprofile.Name("aaa-zzz", "b312--?123!!bb-x__xx-012-y123yy", 123),
			},
			inputApp: "b312--?123!!bb-x__xx-012-y123yy",
			output:   lxdprofile.Name("aaa-zzz", "b312--?123!!bb-x__xx-012-y123yy", 123),
		},
	}
	for k, tc := range testCases {
		c.Logf("running test %d of %d with input %q", k, len(testCases), tc.input)
		obtained, err := lxdprofile.MatchProfileNameByAppName(tc.input, tc.inputApp)
		if tc.err != "" {
			c.Assert(err, gc.ErrorMatches, tc.err)
			continue
		}
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(obtained, gc.Equals, tc.output)
	}
}
