// Copyright 2019 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package charm

import (
	"strings"

	gc "gopkg.in/check.v1"
)

type JujuIgnoreSuite struct{}

var _ = gc.Suite(&JujuIgnoreSuite{})

func (s *JujuIgnoreSuite) TestBuildRules(c *gc.C) {
	type test struct {
		path     string
		isDir    bool
		expMatch bool
	}

	specs := []struct {
		descr string
		rules string
		tests []test
	}{
		{
			descr: `Match a directory named "target" at any depth`,
			rules: `target/`,
			tests: []test{
				{path: "target", isDir: true, expMatch: true},
				{path: "foo/target", isDir: true, expMatch: true},
				{path: "foo/1target", isDir: true, expMatch: false},
				{path: "foo/target", isDir: false, expMatch: false},
			},
		},
		{
			descr: `Match a directory OR a file named "target" at any depth`,
			rules: `target`,
			tests: []test{
				{path: "/target", isDir: true, expMatch: true},
				{path: "/foo/target", isDir: true, expMatch: true},
				{path: "/foo/1target", isDir: true, expMatch: false},
				{path: "/foo/target", isDir: false, expMatch: true},
			},
		},
		{
			descr: `Match a directory at the root only`,
			rules: `/target/`,
			tests: []test{
				{path: "/target", isDir: true, expMatch: true},
				{path: "/foo/target", isDir: true, expMatch: false},
				{path: "/target", isDir: false, expMatch: false},
			},
		},
		{
			descr: `Match a directory OR file at the root only`,
			rules: `/target`,
			tests: []test{
				{path: "/target", isDir: true, expMatch: true},
				{path: "/foo/target", isDir: true, expMatch: false},
				{path: "/target", isDir: false, expMatch: true},
			},
		},
		{
			descr: `Every file or dir ending with .go recursively`,
			rules: `*.go`,
			tests: []test{
				{path: "/target.go", isDir: true, expMatch: true},
				{path: "/target.go", isDir: false, expMatch: true},
				{path: "/foo/target.go", isDir: true, expMatch: true},
				{path: "/foo/target.go", isDir: false, expMatch: true},
				{path: "/target.goT", isDir: true, expMatch: false},
				{path: "/target.goT", isDir: false, expMatch: false},
			},
		},
		{
			descr: `every file or dir named "#comment"`,
			rules: `
# NOTE: leading hash must be escaped so as not to treat line as a comment
\#comment
`,
			tests: []test{
				{path: "/#comment", isDir: true, expMatch: true},
				{path: "/#comment", isDir: false, expMatch: true},
			},
		},
		{
			descr: `Every dir called "logs" under "apps"`,
			rules: `apps/logs/`,
			tests: []test{
				{path: "/apps/logs", isDir: true, expMatch: true},
				{path: "/apps/foo/logs", isDir: true, expMatch: false},
			},
		},
		{
			descr: `Every dir called "logs" two levels under "apps"`,
			rules: `apps/*/logs/`,
			tests: []test{
				{path: "/apps/foo/logs", isDir: true, expMatch: true},
				{path: "/apps/foo/bar/logs", isDir: true, expMatch: false},
				{path: "/apps/logs", isDir: true, expMatch: false},
			},
		},
		{
			descr: `Every dir called "logs" any number of levels under "apps"`,
			rules: `apps/**/logs/`,
			tests: []test{
				{path: "/apps/foo/logs", isDir: true, expMatch: true},
				{path: "/apps/foo/bar/logs", isDir: true, expMatch: true},
				{path: "/apps/logs", isDir: true, expMatch: true},
			},
		},
		{
			descr: `Ignore all under "foo" but not "foo" itself`,
			rules: `foo/**`,
			tests: []test{
				{path: "/foo", isDir: true, expMatch: false},
				{path: "/foo/something", isDir: true, expMatch: true},
				{path: "/foo/something.txt", isDir: false, expMatch: true},
			},
		},
		{
			descr: `Ignore all under "foo" except README.md`,
			rules: `
foo/**
!foo/README.md
`,
			tests: []test{
				{path: "/foo", isDir: true, expMatch: false},
				{path: "/foo/something", isDir: true, expMatch: true},
				{path: "/foo/something.txt", isDir: false, expMatch: true},
				{path: "/foo/README.md", isDir: false, expMatch: false},
			},
		},
		{
			descr: `Multiple double-star separators`,
			rules: `foo/**/bar/**/baz`,
			tests: []test{
				{path: "/foo/1/2/bar/baz", isDir: true, expMatch: true},
				{path: "/foo/1/2/bar/1/2/baz", isDir: true, expMatch: true},
				{path: "/foo/bar/1/baz", isDir: true, expMatch: true},
				{path: "/foo/1/bar/baz", isDir: true, expMatch: true},
				{path: "/foo/bar/baz", isDir: true, expMatch: true},
			},
		},
	}

	for specIndex, spec := range specs {
		c.Logf("[spec %d] %s", specIndex, spec.descr)

		rs, err := newIgnoreRuleset(strings.NewReader(spec.rules))
		if err != nil {
			c.Assert(err, gc.IsNil)
			continue
		}

		for testIndex, test := range spec.tests {
			c.Logf("  [test %d] match against path %q", testIndex, test.path)
			c.Assert(rs.Match(test.path, test.isDir), gc.DeepEquals, test.expMatch)
		}
	}
}

func (s *JujuIgnoreSuite) TestGenIgnorePatternPermutations(c *gc.C) {
	specs := []struct {
		in  string
		exp []string
	}{
		{
			in:  "foo/bar",
			exp: []string{"foo/bar"},
		},
		{
			in: "foo/**/bar",
			exp: []string{
				"foo/**/bar",
				"foo/bar",
			},
		},
		{
			in: "foo/**/bar/**/baz",
			exp: []string{
				"foo/**/bar/**/baz",
				"foo/bar/**/baz",
				"foo/**/bar/baz",
				"foo/bar/baz",
			},
		},
	}

	for specIndex, spec := range specs {
		c.Logf("  [spec %d] gen permutations for %q", specIndex, spec.in)
		got := genIgnorePatternPermutations(spec.in)
		c.Assert(got, gc.DeepEquals, spec.exp)
	}
}

func (s *JujuIgnoreSuite) TestUnescapeIgnorePattern(c *gc.C) {
	specs := []struct {
		desc string
		in   string
		exp  string
	}{
		{
			desc: "trailing unescaped spaces should be trimmed",
			in:   `lore\ m\   `,
			exp:  `lore m `,
		},
		{
			desc: "escaped hashes should be unescaped",
			in:   `\#this-is-not-a-comment`,
			exp:  `#this-is-not-a-comment`,
		},
		{
			desc: "escaped bangs should be unescaped",
			in:   `\!important`,
			exp:  `!important`,
		},
	}

	for specIndex, spec := range specs {
		c.Logf("[spec %d] %s", specIndex, spec.desc)
		c.Assert(unescapeIgnorePattern(spec.in), gc.Equals, spec.exp)
	}
}
