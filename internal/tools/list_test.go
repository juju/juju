// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package tools_test

import (
	"strings"

	"github.com/juju/tc"

	"github.com/juju/juju/core/semversion"
	"github.com/juju/juju/internal/tools"
)

type ListSuite struct{}

var _ = tc.Suite(&ListSuite{})

func mustParseTools(name string) *tools.Tools {
	return &tools.Tools{
		Version: semversion.MustParseBinary(name),
		URL:     "http://testing.invalid/" + name,
	}
}

func extend(lists ...tools.List) tools.List {
	var result tools.List
	for _, list := range lists {
		result = append(result, list...)
	}
	return result
}

var (
	t100ubuntu   = mustParseTools("1.0.0-ubuntu-amd64")
	t100ubuntu32 = mustParseTools("1.0.0-ubuntu-i386")
	t100centos   = mustParseTools("1.0.0-centos-amd64")
	t100all      = tools.List{
		t100ubuntu, t100ubuntu32, t100centos,
	}
	t190ubuntu   = mustParseTools("1.9.0-ubuntu-amd64")
	t190ubuntu32 = mustParseTools("1.9.0-ubuntu-i386")
	t190centos   = mustParseTools("1.9.0-centos-amd64")
	t190all      = tools.List{
		t190ubuntu, t190ubuntu32, t190centos,
	}
	t200ubuntu   = mustParseTools("2.0.0-ubuntu-amd64")
	t200centos32 = mustParseTools("2.0.0-centos-i386")
	t200all      = tools.List{
		t200ubuntu, t200centos32,
	}
	t2001ubuntu   = mustParseTools("2.0.0.1-ubuntu-amd64")
	tAllBefore210 = extend(t100all, t190all, append(t200all, t2001ubuntu))
	t210ubuntu    = mustParseTools("2.1.0-ubuntu-amd64")
	t211ubuntu    = mustParseTools("2.1.1-ubuntu-amd64")
	t215ubuntu    = mustParseTools("2.1.5-ubuntu-amd64")
	t2152ubuntu   = mustParseTools("2.1.5.2-ubuntu-amd64")
	t210all       = tools.List{t210ubuntu, t211ubuntu, t215ubuntu, t2152ubuntu}
)

type releaseTest struct {
	src    tools.List
	expect []string
}

var releaseTests = []releaseTest{{
	src:    tools.List{t100ubuntu},
	expect: []string{"ubuntu"},
}, {
	src:    tools.List{t100ubuntu, t100ubuntu32, t200ubuntu},
	expect: []string{"ubuntu"},
}, {
	src:    tAllBefore210,
	expect: []string{"centos", "ubuntu"},
}}

func (s *ListSuite) TestReleases(c *tc.C) {
	for i, test := range releaseTests {
		c.Logf("test %d", i)
		c.Check(test.src.AllReleases(), tc.DeepEquals, test.expect)
		if len(test.expect) == 1 {
			c.Check(test.src.OneRelease(), tc.Equals, test.expect[0])
		}
	}
}

type archTest struct {
	src    tools.List
	expect string
	err    string
}

var archesTests = []archTest{{
	src:    tools.List{t100ubuntu},
	expect: "amd64",
}, {
	src:    tools.List{t100ubuntu, t100centos, t200ubuntu},
	expect: "amd64",
}, {
	src: tAllBefore210,
	err: "more than one agent arch present: \\[amd64 i386\\]",
}, {
	src: tools.List{},
	err: "tools list is empty",
}}

func (s *ListSuite) TestOneArch(c *tc.C) {
	for i, test := range archesTests {
		c.Logf("test %d", i)
		arch, err := test.src.OneArch()
		if test.err != "" {
			c.Check(err, tc.ErrorMatches, test.err)
		} else {
			c.Assert(err, tc.ErrorIsNil)
			c.Check(arch, tc.Equals, test.expect)
		}
	}
}

func (s *ListSuite) TestURLs(c *tc.C) {
	empty := tools.List{}
	c.Check(empty.URLs(), tc.DeepEquals, map[semversion.Binary][]string{})

	alt := *t100centos
	alt.URL = strings.Replace(alt.URL, "testing.invalid", "testing.invalid2", 1)
	full := tools.List{
		t100ubuntu,
		t190centos,
		t100centos,
		&alt,
		t2001ubuntu,
	}
	c.Check(full.URLs(), tc.DeepEquals, map[semversion.Binary][]string{
		t100ubuntu.Version:  {t100ubuntu.URL},
		t100centos.Version:  {t100centos.URL, alt.URL},
		t190centos.Version:  {t190centos.URL},
		t2001ubuntu.Version: {t2001ubuntu.URL},
	})
}

var newestTests = []struct {
	src    tools.List
	expect tools.List
	number semversion.Number
}{{
	src:    nil,
	expect: nil,
	number: semversion.Zero,
}, {
	src:    tools.List{t100ubuntu},
	expect: tools.List{t100ubuntu},
	number: semversion.MustParse("1.0.0"),
}, {
	src:    t100all,
	expect: t100all,
	number: semversion.MustParse("1.0.0"),
}, {
	src:    extend(t100all, t190all, t200all),
	expect: t200all,
	number: semversion.MustParse("2.0.0"),
}, {
	src:    tAllBefore210,
	expect: tools.List{t2001ubuntu},
	number: semversion.MustParse("2.0.0.1"),
}}

func (s *ListSuite) TestNewest(c *tc.C) {
	for i, test := range newestTests {
		c.Logf("test %d", i)
		number, actual := test.src.Newest()
		c.Check(number, tc.DeepEquals, test.number)
		c.Check(actual, tc.DeepEquals, test.expect)
	}
}

func (s *ListSuite) TestNewestVersions(c *tc.C) {
	for i, test := range newestTests {
		c.Logf("test %d", i)
		versions := make(tools.Versions, len(test.src))
		for i, v := range test.src {
			versions[i] = v
		}
		number, actual := versions.Newest()
		c.Check(number, tc.DeepEquals, test.number)

		var expectVersions tools.Versions
		for _, v := range test.expect {
			expectVersions = append(expectVersions, v)
		}
		c.Check(actual, tc.DeepEquals, expectVersions)
	}
}

var newestCompatibleTests = []struct {
	src            tools.List
	base           semversion.Number
	expect         semversion.Number
	allowDevBuilds bool
	found          bool
}{{
	src:    nil,
	base:   semversion.Zero,
	expect: semversion.Zero,
	found:  false,
}, {
	src:    tools.List{t100ubuntu},
	base:   semversion.Zero,
	expect: semversion.Zero,
	found:  false,
}, {
	src:    t100all,
	base:   semversion.MustParse("1.0.0"),
	expect: semversion.MustParse("1.0.0"),
	found:  true,
}, {
	src:            tAllBefore210,
	base:           semversion.MustParse("2.0.0"),
	expect:         semversion.MustParse("2.0.0.1"),
	allowDevBuilds: true,
	found:          true,
}, {
	src:    tAllBefore210,
	base:   semversion.MustParse("1.9.0"),
	expect: semversion.MustParse("1.9.0"),
	found:  true,
}, {
	src:            t210all,
	base:           semversion.MustParse("2.1.1"),
	expect:         semversion.MustParse("2.1.5.2"),
	allowDevBuilds: true,
	found:          true,
}, {
	src:    t210all,
	base:   semversion.MustParse("2.1.1"),
	expect: semversion.MustParse("2.1.5"),
	found:  true,
}, {
	src:    t210all,
	base:   semversion.MustParse("2.0.0"),
	expect: semversion.MustParse("2.0.0"),
	found:  false,
}}

func (s *ListSuite) TestNewestCompatible(c *tc.C) {
	for i, test := range newestCompatibleTests {
		c.Logf("test %d", i)
		versions := make(tools.Versions, len(test.src))
		for i, v := range test.src {
			versions[i] = v
		}
		actual, found := versions.NewestCompatible(test.base, test.allowDevBuilds)
		c.Check(actual, tc.DeepEquals, test.expect)
		c.Check(found, tc.Equals, test.found)
	}
}

var excludeTests = []struct {
	src    tools.List
	arg    tools.List
	expect tools.List
}{{
	nil, tools.List{t100ubuntu}, nil,
}, {
	tools.List{t100ubuntu}, nil, tools.List{t100ubuntu},
}, {
	tools.List{t100ubuntu}, tools.List{t100ubuntu}, nil,
}, {
	nil, tAllBefore210, nil,
}, {
	tAllBefore210, nil, tAllBefore210,
}, {
	tAllBefore210, tAllBefore210, nil,
}, {
	t100all,
	tools.List{t100ubuntu},
	tools.List{t100ubuntu32, t100centos},
}, {
	t100all,
	tools.List{t100ubuntu32, t100centos},
	tools.List{t100ubuntu},
}, {
	t100all, t190all, t100all,
}, {
	t190all, t100all, t190all,
}, {
	extend(t100all, t190all),
	t190all,
	t100all,
}}

func (s *ListSuite) TestExclude(c *tc.C) {
	for i, test := range excludeTests {
		c.Logf("test %d", i)
		c.Check(test.src.Exclude(test.arg), tc.DeepEquals, test.expect)
	}
}

var matchTests = []struct {
	src    tools.List
	filter tools.Filter
	expect tools.List
}{{
	tools.List{t100ubuntu},
	tools.Filter{},
	tools.List{t100ubuntu},
}, {
	tAllBefore210,
	tools.Filter{},
	tAllBefore210,
}, {
	tAllBefore210,
	tools.Filter{Number: semversion.MustParse("1.9.0")},
	t190all,
}, {
	tAllBefore210,
	tools.Filter{Number: semversion.MustParse("1.9.0.1")},
	nil,
}, {
	tAllBefore210,
	tools.Filter{OSType: "centos"},
	tools.List{t100centos, t190centos, t200centos32},
}, {
	tAllBefore210,
	tools.Filter{OSType: "opensuse"},
	nil,
}, {
	tAllBefore210,
	tools.Filter{Arch: "i386"},
	tools.List{t100ubuntu32, t190ubuntu32, t200centos32},
}, {
	tAllBefore210,
	tools.Filter{Arch: "arm"},
	nil,
}, {
	tAllBefore210,
	tools.Filter{
		Number: semversion.MustParse("2.0.0"),
		OSType: "centos",
		Arch:   "i386",
	},
	tools.List{t200centos32},
}}

func (s *ListSuite) TestMatch(c *tc.C) {
	for i, test := range matchTests {
		c.Logf("test %d", i)
		actual, err := test.src.Match(test.filter)
		c.Check(actual, tc.DeepEquals, test.expect)
		if len(test.expect) > 0 {
			c.Check(err, tc.ErrorIsNil)
		} else {
			c.Check(err, tc.Equals, tools.ErrNoMatches)
		}
	}
}

func (s *ListSuite) TestMatchVersions(c *tc.C) {
	for i, test := range matchTests {
		c.Logf("test %d", i)
		versions := make(tools.Versions, len(test.src))
		for i, v := range test.src {
			versions[i] = v
		}
		actual, err := versions.Match(test.filter)
		if len(test.expect) > 0 {
			c.Check(err, tc.ErrorIsNil)
		} else {
			c.Check(err, tc.Equals, tools.ErrNoMatches)
		}

		var expectVersions tools.Versions
		for _, v := range test.expect {
			expectVersions = append(expectVersions, v)
		}
		c.Check(actual, tc.DeepEquals, expectVersions)
	}
}
