// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charm_test

import (
	"encoding/json"
	"fmt"

	"labix.org/v2/mgo/bson"
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/charm"
)

type URLSuite struct{}

var _ = gc.Suite(&URLSuite{})

var urlTests = []struct {
	s, err string
	url    *charm.URL
}{
	{"cs:~user/series/name", "", &charm.URL{"cs", "user", "series", "name", -1}},
	{"cs:~user/series/name-0", "", &charm.URL{"cs", "user", "series", "name", 0}},
	{"cs:series/name", "", &charm.URL{"cs", "", "series", "name", -1}},
	{"cs:series/name-42", "", &charm.URL{"cs", "", "series", "name", 42}},
	{"local:series/name-1", "", &charm.URL{"local", "", "series", "name", 1}},
	{"local:series/name", "", &charm.URL{"local", "", "series", "name", -1}},
	{"local:series/n0-0n-n0", "", &charm.URL{"local", "", "series", "n0-0n-n0", -1}},

	{"bs:~user/series/name-1", "charm URL has invalid schema: .*", nil},
	{"cs:~1/series/name-1", "charm URL has invalid user name: .*", nil},
	{"cs:~user/1/name-1", "charm URL has invalid series: .*", nil},
	{"cs:~user/series/name-1-2", "charm URL has invalid charm name: .*", nil},
	{"cs:~user/series/name-1-name-2", "charm URL has invalid charm name: .*", nil},
	{"cs:~user/series/name--name-2", "charm URL has invalid charm name: .*", nil},
	{"cs:~user/series/huh/name-1", "charm URL has invalid form: .*", nil},
	{"cs:~user/name", "charm URL without series: .*", nil},
	{"cs:name", "charm URL without series: .*", nil},
	{"local:~user/series/name", "local charm URL with user name: .*", nil},
	{"local:~user/name", "local charm URL with user name: .*", nil},
	{"local:name", "charm URL without series: .*", nil},
}

func (s *URLSuite) TestParseURL(c *gc.C) {
	for i, t := range urlTests {
		c.Logf("test %d", i)
		url, err := charm.ParseURL(t.s)
		comment := gc.Commentf("ParseURL(%q)", t.s)
		if t.err != "" {
			c.Check(err.Error(), gc.Matches, t.err, comment)
		} else {
			c.Check(url, gc.DeepEquals, t.url, comment)
			c.Check(t.url.String(), gc.Equals, t.s)
		}
	}
}

var inferTests = []struct {
	vague, exact string
}{
	{"foo", "cs:defseries/foo"},
	{"foo-1", "cs:defseries/foo-1"},
	{"n0-n0-n0", "cs:defseries/n0-n0-n0"},
	{"cs:foo", "cs:defseries/foo"},
	{"local:foo", "local:defseries/foo"},
	{"series/foo", "cs:series/foo"},
	{"cs:series/foo", "cs:series/foo"},
	{"local:series/foo", "local:series/foo"},
	{"cs:~user/foo", "cs:~user/defseries/foo"},
	{"cs:~user/series/foo", "cs:~user/series/foo"},
	{"local:~user/series/foo", "local:~user/series/foo"},
	{"bs:foo", "bs:defseries/foo"},
	{"cs:~1/foo", "cs:~1/defseries/foo"},
	{"cs:foo-1-2", "cs:defseries/foo-1-2"},
}

func (s *URLSuite) TestInferURL(c *gc.C) {
	for i, t := range inferTests {
		c.Logf("test %d", i)
		comment := gc.Commentf("InferURL(%q, %q)", t.vague, "defseries")
		inferred, ierr := charm.InferURL(t.vague, "defseries")
		parsed, perr := charm.ParseURL(t.exact)
		if parsed != nil {
			c.Check(inferred, gc.DeepEquals, parsed, comment)
		} else {
			expect := perr.Error()
			if t.vague != t.exact {
				expect = fmt.Sprintf("%s (URL inferred from %q)", expect, t.vague)
			}
			c.Check(ierr.Error(), gc.Equals, expect, comment)
		}
	}
	u, err := charm.InferURL("~blah", "defseries")
	c.Assert(u, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "cannot infer charm URL with user but no schema: .*")
}

var inferNoDefaultSeriesTests = []struct {
	vague, exact string
}{
	{"foo", ""},
	{"foo-1", ""},
	{"cs:foo", ""},
	{"cs:~user/foo", ""},
	{"series/foo", "cs:series/foo"},
	{"cs:series/foo", "cs:series/foo"},
	{"cs:~user/series/foo", "cs:~user/series/foo"},
}

func (s *URLSuite) TestInferURLNoDefaultSeries(c *gc.C) {
	for _, t := range inferNoDefaultSeriesTests {
		inferred, err := charm.InferURL(t.vague, "")
		if t.exact == "" {
			c.Assert(err, gc.ErrorMatches, fmt.Sprintf("cannot infer charm URL for %q: no series provided", t.vague))
		} else {
			parsed, err := charm.ParseURL(t.exact)
			c.Assert(err, gc.IsNil)
			c.Assert(inferred, gc.DeepEquals, parsed, gc.Commentf(`InferURL(%q, "")`, t.vague))
		}
	}
}

var validTests = []struct {
	valid  func(string) bool
	string string
	expect bool
}{
	{charm.IsValidUser, "", false},
	{charm.IsValidUser, "bob", true},
	{charm.IsValidUser, "Bob", false},
	{charm.IsValidUser, "bOB", true},
	{charm.IsValidUser, "b^b", false},
	{charm.IsValidUser, "bob1", true},
	{charm.IsValidUser, "bob-1", true},
	{charm.IsValidUser, "bob+1", true},
	{charm.IsValidUser, "bob.1", true},
	{charm.IsValidUser, "1bob", true},
	{charm.IsValidUser, "1-bob", true},
	{charm.IsValidUser, "1+bob", true},
	{charm.IsValidUser, "1.bob", true},
	{charm.IsValidUser, "jim.bob+99-1.", true},

	{charm.IsValidName, "", false},
	{charm.IsValidName, "wordpress", true},
	{charm.IsValidName, "Wordpress", false},
	{charm.IsValidName, "word-press", true},
	{charm.IsValidName, "word press", false},
	{charm.IsValidName, "word^press", false},
	{charm.IsValidName, "-wordpress", false},
	{charm.IsValidName, "wordpress-", false},
	{charm.IsValidName, "wordpress2", true},
	{charm.IsValidName, "wordpress-2", false},
	{charm.IsValidName, "word2-press2", true},

	{charm.IsValidSeries, "", false},
	{charm.IsValidSeries, "precise", true},
	{charm.IsValidSeries, "Precise", false},
	{charm.IsValidSeries, "pre cise", false},
	{charm.IsValidSeries, "pre-cise", true},
	{charm.IsValidSeries, "pre^cise", false},
	{charm.IsValidSeries, "prec1se", false},
	{charm.IsValidSeries, "-precise", false},
	{charm.IsValidSeries, "precise-", false},
	{charm.IsValidSeries, "pre-c1se", false},
}

func (s *URLSuite) TestValidCheckers(c *gc.C) {
	for i, t := range validTests {
		c.Logf("test %d: %s", i, t.string)
		c.Assert(t.valid(t.string), gc.Equals, t.expect)
	}
}

func (s *URLSuite) TestMustParseURL(c *gc.C) {
	url := charm.MustParseURL("cs:series/name")
	c.Assert(url, gc.DeepEquals, &charm.URL{"cs", "", "series", "name", -1})
	f := func() { charm.MustParseURL("local:name") }
	c.Assert(f, gc.PanicMatches, "charm URL without series: .*")
}

func (s *URLSuite) TestWithRevision(c *gc.C) {
	url := charm.MustParseURL("cs:series/name")
	other := url.WithRevision(1)
	c.Assert(url, gc.DeepEquals, &charm.URL{"cs", "", "series", "name", -1})
	c.Assert(other, gc.DeepEquals, &charm.URL{"cs", "", "series", "name", 1})

	// Should always copy. The opposite behavior is error prone.
	c.Assert(other.WithRevision(1), gc.Not(gc.Equals), other)
	c.Assert(other.WithRevision(1), gc.DeepEquals, other)
}

var codecs = []struct {
	Marshal   func(interface{}) ([]byte, error)
	Unmarshal func([]byte, interface{}) error
}{{
	Marshal:   bson.Marshal,
	Unmarshal: bson.Unmarshal,
}, {
	Marshal:   json.Marshal,
	Unmarshal: json.Unmarshal,
}}

func (s *URLSuite) TestCodecs(c *gc.C) {
	for i, codec := range codecs {
		c.Logf("codec %d", i)
		type doc struct {
			URL *charm.URL
		}
		url := charm.MustParseURL("cs:series/name")
		data, err := codec.Marshal(doc{url})
		c.Assert(err, gc.IsNil)
		var v doc
		err = codec.Unmarshal(data, &v)
		c.Assert(v.URL, gc.DeepEquals, url)

		data, err = codec.Marshal(doc{})
		c.Assert(err, gc.IsNil)
		err = codec.Unmarshal(data, &v)
		c.Assert(err, gc.IsNil)
		c.Assert(v.URL, gc.IsNil)
	}
}

type QuoteSuite struct{}

var _ = gc.Suite(&QuoteSuite{})

func (s *QuoteSuite) TestUnmodified(c *gc.C) {
	// Check that a string containing only valid
	// chars stays unmodified.
	in := "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789.-"
	out := charm.Quote(in)
	c.Assert(out, gc.Equals, in)
}

func (s *QuoteSuite) TestQuote(c *gc.C) {
	// Check that invalid chars are translated correctly.
	in := "hello_there/how'are~you-today.sir"
	out := charm.Quote(in)
	c.Assert(out, gc.Equals, "hello_5f_there_2f_how_27_are_7e_you-today.sir")
}
