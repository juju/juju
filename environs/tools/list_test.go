package tools_test

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/environs/tools"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/version"
	"testing"
)

func TestPackage(t *testing.T) {
	TestingT(t)
}

type ListSuite struct{}

var _ = Suite(&ListSuite{})

func mustParseTools(name string) *state.Tools {
	return &state.Tools{Binary: version.MustParseBinary(name)}
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
	t2001precise = mustParseTools("2.0.0.1-precise-amd64")
	tAll         = extend(t100all, t190all, append(t200all, t2001precise))
)

type stringsTest struct {
	src    tools.List
	expect []string
}

func (s *ListSuite) TestSeries(c *C) {
	for i, test := range []stringsTest{{
		tools.List{t100precise},
		[]string{"precise"},
	}, {
		tools.List{t100precise, t100precise32, t200precise},
		[]string{"precise"},
	}, {
		tAll,
		[]string{"precise", "quantal"},
	}} {
		c.Logf("test %d", i)
		c.Check(test.src.Series(), DeepEquals, test.expect)
	}
}

func (s *ListSuite) TestArches(c *C) {
	for i, test := range []stringsTest{{
		tools.List{t100precise},
		[]string{"amd64"},
	}, {
		tools.List{t100precise, t100quantal, t200precise},
		[]string{"amd64"},
	}, {
		tAll,
		[]string{"amd64", "i386"},
	}} {
		c.Logf("test %d", i)
		c.Check(test.src.Arches(), DeepEquals, test.expect)
	}
}

func (s *ListSuite) TestNewest(c *C) {
	for i, test := range []struct {
		src    tools.List
		expect tools.List
	}{{
		tools.List{t100precise},
		tools.List{t100precise},
	}, {
		t100all,
		t100all,
	}, {
		extend(t100all, t190all, t200all),
		t200all,
	}, {
		tAll,
		tools.List{t2001precise},
	}} {
		c.Logf("test %d", i)
		c.Check(test.src.Newest(), DeepEquals, test.expect)
	}
}

func (s *ListSuite) TestDifference(c *C) {
	for i, test := range []struct {
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
		nil, tAll, nil,
	}, {
		tAll, nil, tAll,
	}, {
		tAll, tAll, nil,
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
	}} {
		c.Logf("test %d", i)
		c.Check(test.src.Difference(test.arg), DeepEquals, test.expect)
	}
}

func (s *ListSuite) TestFilter(c *C) {
	for i, test := range []struct {
		src    tools.List
		filter tools.Filter
		expect tools.List
	}{{
		tools.List{t100precise},
		tools.Filter{},
		tools.List{t100precise},
	}, {
		tAll,
		tools.Filter{},
		tAll,
	}, {
		tAll,
		tools.Filter{Released: true},
		t200all,
	}, {
		t100all,
		tools.Filter{Released: true},
		nil,
	}, {
		tAll,
		tools.Filter{Number: version.MustParse("1.9.0")},
		t190all,
	}, {
		tAll,
		tools.Filter{Number: version.MustParse("1.9.0.1")},
		nil,
	}, {
		tAll,
		tools.Filter{Series: "quantal"},
		tools.List{t100quantal, t100quantal32, t190quantal, t200quantal32},
	}, {
		tAll,
		tools.Filter{Series: "raring"},
		nil,
	}, {
		tAll,
		tools.Filter{Arch: "i386"},
		tools.List{t100precise32, t100quantal32, t190precise32, t200quantal32},
	}, {
		tAll,
		tools.Filter{Arch: "arm"},
		nil,
	}, {
		tAll,
		tools.Filter{
			Released: true,
			Number:   version.MustParse("2.0.0"),
			Series:   "quantal",
			Arch:     "i386",
		},
		tools.List{t200quantal32},
	}} {
		c.Logf("test %d", i)
		actual, err := test.src.Filter(test.filter)
		c.Check(actual, DeepEquals, test.expect)
		if len(test.expect) > 0 {
			c.Check(err, IsNil)
		} else {
			c.Check(err, Equals, tools.ErrNoMatches)
		}
	}
}
