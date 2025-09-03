// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package lxdprofile_test

import (
	"testing"

	"github.com/juju/tc"

	"github.com/juju/juju/core/lxdprofile"
	"github.com/juju/juju/internal/testhelpers"
)

type LXDProfileNameSuite struct {
	testhelpers.IsolationSuite
}

func TestLXDProfileNameSuite(t *testing.T) {
	tc.Run(t, &LXDProfileNameSuite{})
}

func (*LXDProfileNameSuite) TestProfileNames(c *tc.C) {
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
				lxdprofile.Name("foo", "shortid", "bar", 1),
			},
			output: []string{
				lxdprofile.Name("foo", "shortid", "bar", 1),
			},
		},
		{
			input: []string{
				"default",
				lxdprofile.Name("foo", "shortid", "bar", 1),
				lxdprofile.Name("foo", "shortid", "bar", 1),
				lxdprofile.Name("aaa", "shortid2", "bbb", 100),
			},
			output: []string{
				lxdprofile.Name("foo", "shortid", "bar", 1),
				lxdprofile.Name("aaa", "shortid2", "bbb", 100),
			},
		},
		{
			input: []string{
				"default",
				lxdprofile.Name("foo", "shortid", "bar", 1),
				lxdprofile.Name("foo", "shortid", "bar", 1),
				"some-other-profile",
				lxdprofile.Name("aaa", "shortid2", "bbb", 100),
			},
			output: []string{
				lxdprofile.Name("foo", "shortid", "bar", 1),
				lxdprofile.Name("aaa", "shortid2", "bbb", 100),
			},
		},
	}
	for k, testCase := range testCases {
		c.Logf("running test %d with input %q", k, testCase.input)
		c.Assert(lxdprofile.FilterLXDProfileNames(testCase.input), tc.DeepEquals, testCase.output)
	}
}

func (*LXDProfileNameSuite) TestIsValidName(c *tc.C) {
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
			input:  lxdprofile.Name("foo", "shortid", "bar", 1),
			output: true,
		},
		{
			input:  lxdprofile.Name("aaa-zzz", "shortid", "b312--?123!!bb-x__xx-012-y123yy", 100),
			output: true,
		},
	}
	for k, testCase := range testCases {
		c.Logf("running test %d with input %q", k, testCase.input)
		c.Assert(lxdprofile.IsValidName(testCase.input), tc.Equals, testCase.output)
	}
}

func (*LXDProfileNameSuite) TestProfileRevision(c *tc.C) {
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
			input:  lxdprofile.Name("foo", "shortid", "bar", 1),
			output: 1,
		},
		{
			input:  lxdprofile.Name("aaa-zzz", "shortid", "b312--?123!!bb-x__xx-012-y123yy", 100),
			output: 100,
		},
	}
	for k, testCase := range testCases {
		c.Logf("running test %d of %d with input %q", k, len(testCases), testCase.input)
		obtained, err := lxdprofile.ProfileRevision(testCase.input)
		if testCase.err != "" {
			c.Assert(err, tc.ErrorMatches, testCase.err)
			continue
		}
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(obtained, tc.Equals, testCase.output)
	}
}

func (*LXDProfileNameSuite) TestProfileReplaceRevision(c *tc.C) {
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
			input:    lxdprofile.Name("foo", "shortid", "bar", 1),
			inputRev: 4,
			output:   lxdprofile.Name("foo", "shortid", "bar", 4),
		},
		{
			input:    lxdprofile.Name("aaa-zzz", "shortid", "b312--?123!!bb-x__xx-012-y123yy", 123),
			inputRev: 312,
			output:   lxdprofile.Name("aaa-zzz", "shortid", "b312--?123!!bb-x__xx-012-y123yy", 312),
		},
	}
	for k, testCase := range testCases {
		c.Logf("running test %d of %d with input %q", k, len(testCases), testCase.input)
		obtained, err := lxdprofile.ProfileReplaceRevision(testCase.input, testCase.inputRev)
		if testCase.err != "" {
			c.Assert(err, tc.ErrorMatches, testCase.err)
			continue
		}
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(obtained, tc.Equals, testCase.output)
	}
}

func (*LXDProfileNameSuite) TestMatchProfileNameByAppName(c *tc.C) {
	testCases := []struct {
		input    []string
		inputApp string
		output   string
		err      string
	}{
		{
			input:    []string{},
			inputApp: "",
			err:      "no application name specified",
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
				lxdprofile.Name("foo", "shortid", "bar", 2),
			},
			inputApp: "bar",
			output:   lxdprofile.Name("foo", "shortid", "bar", 2),
		},
		{
			input: []string{
				"default",
				"juju-model",
				lxdprofile.Name("foo", "shortid", "nonebar", 2),
				lxdprofile.Name("foo", "shortid", "bar", 2),
			},
			inputApp: "bar",
			output:   lxdprofile.Name("foo", "shortid", "bar", 2),
		},
		{
			input: []string{
				"default",
				"juju-model",
				lxdprofile.Name("aaa-zzz", "shortid", "b312--?123!!bb-x__xx-012-y123yy", 123),
			},
			inputApp: "b312--?123!!bb-x__xx-012-y123yy",
			output:   lxdprofile.Name("aaa-zzz", "shortid", "b312--?123!!bb-x__xx-012-y123yy", 123),
		},
	}
	for k, testCase := range testCases {
		c.Logf("running test %d of %d with input %q", k, len(testCases), testCase.input)
		obtained, err := lxdprofile.MatchProfileNameByAppName(testCase.input, testCase.inputApp)
		if testCase.err != "" {
			c.Assert(err, tc.ErrorMatches, testCase.err)
			continue
		}
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(obtained, tc.Equals, testCase.output)
	}
}
