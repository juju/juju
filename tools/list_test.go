// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package tools_test

import (
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/tools"
	"launchpad.net/juju-core/version"
)

type ListSuite struct{}

var _ = gc.Suite(&ListSuite{})

func mustParseTools(name string) *tools.Tools {
	return &tools.Tools{
		Version: version.MustParseBinary(name),
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
	t100precise   = mustParseTools("1.0.0-precise-amd64")
	t100precise32 = mustParseTools("1.0.0-precise-i386")
	t100quantal   = mustParseTools("1.0.0-quantal-amd64")
	t100quantal32 = mustParseTools("1.0.0-quantal-i386")
	t100all       = tools.List{
		t100precise, t100precise32, t100quantal, t100quantal32,
	}
	t190precise   = mustParseTools("1.9.0-precise-amd64")
	t190precise32 = mustParseTools("1.9.0-precise-i386")
	t190quantal   = mustParseTools("1.9.0-quantal-amd64")
	t190all       = tools.List{
		t190precise, t190precise32, t190quantal,
	}
	t200precise   = mustParseTools("2.0.0-precise-amd64")
	t200quantal32 = mustParseTools("2.0.0-quantal-i386")
	t200all       = tools.List{
		t200precise, t200quantal32,
	}
	t2001precise  = mustParseTools("2.0.0.1-precise-amd64")
	tAllBefore210 = extend(t100all, t190all, append(t200all, t2001precise))
	t210precise   = mustParseTools("2.1.0-precise-amd64")
	t211precise   = mustParseTools("2.1.1-precise-amd64")
	t215precise   = mustParseTools("2.1.5-precise-amd64")
	t2152precise  = mustParseTools("2.1.5.2-precise-amd64")
	t210all       = tools.List{t210precise, t211precise, t215precise, t2152precise}
)

type stringsTest struct {
	src    tools.List
	expect []string
}

var seriesTests = []stringsTest{{
	src:    tools.List{t100precise},
	expect: []string{"precise"},
}, {
	src:    tools.List{t100precise, t100precise32, t200precise},
	expect: []string{"precise"},
}, {
	src:    tAllBefore210,
	expect: []string{"precise", "quantal"},
}}

func (s *ListSuite) TestSeries(c *gc.C) {
	for i, test := range seriesTests {
		c.Logf("test %d", i)
		c.Check(test.src.AllSeries(), gc.DeepEquals, test.expect)
		if len(test.expect) == 1 {
			c.Check(test.src.OneSeries(), gc.Equals, test.expect[0])
		}
	}
}

var archesTests = []stringsTest{{
	src:    tools.List{t100precise},
	expect: []string{"amd64"},
}, {
	src:    tools.List{t100precise, t100quantal, t200precise},
	expect: []string{"amd64"},
}, {
	src:    tAllBefore210,
	expect: []string{"amd64", "i386"},
}}

func (s *ListSuite) TestArches(c *gc.C) {
	for i, test := range archesTests {
		c.Logf("test %d", i)
		c.Check(test.src.Arches(), gc.DeepEquals, test.expect)
	}
}

func (s *ListSuite) TestURLs(c *gc.C) {
	empty := tools.List{}
	c.Check(empty.URLs(), gc.DeepEquals, map[version.Binary]string{})

	full := tools.List{t100precise, t190quantal, t2001precise}
	c.Check(full.URLs(), gc.DeepEquals, map[version.Binary]string{
		t100precise.Version:  t100precise.URL,
		t190quantal.Version:  t190quantal.URL,
		t2001precise.Version: t2001precise.URL,
	})
}

var newestTests = []struct {
	src    tools.List
	expect tools.List
	number version.Number
}{{
	src:    nil,
	expect: nil,
	number: version.Zero,
}, {
	src:    tools.List{t100precise},
	expect: tools.List{t100precise},
	number: version.MustParse("1.0.0"),
}, {
	src:    t100all,
	expect: t100all,
	number: version.MustParse("1.0.0"),
}, {
	src:    extend(t100all, t190all, t200all),
	expect: t200all,
	number: version.MustParse("2.0.0"),
}, {
	src:    tAllBefore210,
	expect: tools.List{t2001precise},
	number: version.MustParse("2.0.0.1"),
}}

func (s *ListSuite) TestNewest(c *gc.C) {
	for i, test := range newestTests {
		c.Logf("test %d", i)
		number, actual := test.src.Newest()
		c.Check(number, gc.DeepEquals, test.number)
		c.Check(actual, gc.DeepEquals, test.expect)
	}
}

var newestCompatibleTests = []struct {
	src    tools.List
	base   version.Number
	expect version.Number
	found  bool
}{{
	src:    nil,
	base:   version.Zero,
	expect: version.Zero,
	found:  false,
}, {
	src:    tools.List{t100precise},
	base:   version.Zero,
	expect: version.Zero,
	found:  false,
}, {
	src:    t100all,
	base:   version.MustParse("1.0.0"),
	expect: version.MustParse("1.0.0"),
	found:  true,
}, {
	src:    tAllBefore210,
	base:   version.MustParse("2.0.0"),
	expect: version.MustParse("2.0.0.1"),
	found:  true,
}, {
	src:    tAllBefore210,
	base:   version.MustParse("1.9.0"),
	expect: version.MustParse("1.9.0"),
	found:  true,
}, {
	src:    t210all,
	base:   version.MustParse("2.1.1"),
	expect: version.MustParse("2.1.5.2"),
	found:  true,
}}

func (s *ListSuite) TestNewestCompatible(c *gc.C) {
	for i, test := range newestCompatibleTests {
		c.Logf("test %d", i)
		actual, found := test.src.NewestCompatible(test.base)
		c.Check(actual, gc.DeepEquals, test.expect)
		c.Check(found, gc.Equals, test.found)
	}
}

var excludeTests = []struct {
	src    tools.List
	arg    tools.List
	expect tools.List
}{{
	nil, tools.List{t100precise}, nil,
}, {
	tools.List{t100precise}, nil, tools.List{t100precise},
}, {
	tools.List{t100precise}, tools.List{t100precise}, nil,
}, {
	nil, tAllBefore210, nil,
}, {
	tAllBefore210, nil, tAllBefore210,
}, {
	tAllBefore210, tAllBefore210, nil,
}, {
	t100all,
	tools.List{t100precise},
	tools.List{t100precise32, t100quantal, t100quantal32},
}, {
	t100all,
	tools.List{t100precise32, t100quantal, t100quantal32},
	tools.List{t100precise},
}, {
	t100all, t190all, t100all,
}, {
	t190all, t100all, t190all,
}, {
	extend(t100all, t190all),
	t190all,
	t100all,
}}

func (s *ListSuite) TestExclude(c *gc.C) {
	for i, test := range excludeTests {
		c.Logf("test %d", i)
		c.Check(test.src.Exclude(test.arg), gc.DeepEquals, test.expect)
	}
}

var matchTests = []struct {
	src    tools.List
	filter tools.Filter
	expect tools.List
}{{
	tools.List{t100precise},
	tools.Filter{},
	tools.List{t100precise},
}, {
	tAllBefore210,
	tools.Filter{},
	tAllBefore210,
}, {
	tAllBefore210,
	tools.Filter{Released: true},
	extend(t100all, t200all),
}, {
	t190all,
	tools.Filter{Released: true},
	nil,
}, {
	tAllBefore210,
	tools.Filter{Number: version.MustParse("1.9.0")},
	t190all,
}, {
	tAllBefore210,
	tools.Filter{Number: version.MustParse("1.9.0.1")},
	nil,
}, {
	tAllBefore210,
	tools.Filter{Series: "quantal"},
	tools.List{t100quantal, t100quantal32, t190quantal, t200quantal32},
}, {
	tAllBefore210,
	tools.Filter{Series: "raring"},
	nil,
}, {
	tAllBefore210,
	tools.Filter{Arch: "i386"},
	tools.List{t100precise32, t100quantal32, t190precise32, t200quantal32},
}, {
	tAllBefore210,
	tools.Filter{Arch: "arm"},
	nil,
}, {
	tAllBefore210,
	tools.Filter{
		Released: true,
		Number:   version.MustParse("2.0.0"),
		Series:   "quantal",
		Arch:     "i386",
	},
	tools.List{t200quantal32},
}}

func (s *ListSuite) TestMatch(c *gc.C) {
	for i, test := range matchTests {
		c.Logf("test %d", i)
		actual, err := test.src.Match(test.filter)
		c.Check(actual, gc.DeepEquals, test.expect)
		if len(test.expect) > 0 {
			c.Check(err, gc.IsNil)
		} else {
			c.Check(err, gc.Equals, tools.ErrNoMatches)
		}
	}
}
