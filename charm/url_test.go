// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package charm_test

import (
	"encoding/json"
	"fmt"
	"strings"

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
	{"cs:~user/series/name", "", &charm.URL{charm.Reference{"cs", "user", "name", -1}, "series"}},
	{"cs:~user/series/name-0", "", &charm.URL{charm.Reference{"cs", "user", "name", 0}, "series"}},
	{"cs:series/name", "", &charm.URL{charm.Reference{"cs", "", "name", -1}, "series"}},
	{"cs:series/name-42", "", &charm.URL{charm.Reference{"cs", "", "name", 42}, "series"}},
	{"local:series/name-1", "", &charm.URL{charm.Reference{"local", "", "name", 1}, "series"}},
	{"local:series/name", "", &charm.URL{charm.Reference{"local", "", "name", -1}, "series"}},
	{"local:series/n0-0n-n0", "", &charm.URL{charm.Reference{"local", "", "n0-0n-n0", -1}, "series"}},
	{"cs:~user/name", "", &charm.URL{charm.Reference{"cs", "user", "name", -1}, ""}},
	{"cs:name", "", &charm.URL{charm.Reference{"cs", "", "name", -1}, ""}},
	{"local:name", "", &charm.URL{charm.Reference{"local", "", "name", -1}, ""}},

	{"bs:~user/series/name-1", "charm URL has invalid schema: .*", nil},
	{"cs:~1/series/name-1", "charm URL has invalid user name: .*", nil},
	{"cs:~user", "charm URL without charm name: .*", nil},
	{"cs:~user/1/name-1", "charm URL has invalid series: .*", nil},
	{"cs:~user/series/name-1-2", "charm URL has invalid charm name: .*", nil},
	{"cs:~user/series/name-1-name-2", "charm URL has invalid charm name: .*", nil},
	{"cs:~user/series/name--name-2", "charm URL has invalid charm name: .*", nil},
	{"cs:~user/series/huh/name-1", "charm URL has invalid form: .*", nil},
	{"cs:/name", "charm URL has invalid series: .*", nil},
	{"local:~user/series/name", "local charm URL with user name: .*", nil},
	{"local:~user/name", "local charm URL with user name: .*", nil},
}

func (s *URLSuite) TestParseURL(c *gc.C) {
	for i, t := range urlTests {
		c.Logf("test %d", i)
		url, uerr := charm.ParseURL(t.s)
		ref, series, rerr := charm.ParseReference(t.s)
		comment := gc.Commentf("ParseURL(%q)", t.s)
		if t.url != nil && t.url.Series == "" {
			if t.err != "" {
				// Expected error should match
				c.Assert(rerr, gc.NotNil, comment)
				c.Check(rerr.Error(), gc.Matches, t.err, comment)
			} else {
				// Expected charm reference should match
				c.Check(ref, gc.DeepEquals, t.url.Reference, comment)
				c.Check(t.url.Reference.String(), gc.Equals, t.s)
			}
			if rerr != nil {
				// If ParseReference has an error, ParseURL should share it
				c.Check(uerr.Error(), gc.Equals, rerr.Error(), comment)
			} else {
				// Otherwise, ParseURL with an empty series should error unresolved.
				c.Check(uerr.Error(), gc.Equals, charm.ErrUnresolvedUrl.Error(), comment)
			}
		} else {
			if t.err != "" {
				c.Assert(uerr, gc.NotNil, comment)
				c.Check(uerr.Error(), gc.Matches, t.err, comment)
				c.Check(uerr.Error(), gc.Equals, rerr.Error(), comment)
			} else {
				c.Check(url.Series, gc.Equals, series, comment)
				c.Check(url, gc.DeepEquals, t.url, comment)
				c.Check(t.url.String(), gc.Equals, t.s)
			}
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
		if perr == nil {
			c.Check(inferred, gc.DeepEquals, parsed, comment)
			c.Check(ierr, gc.IsNil)
		} else {
			expect := perr.Error()
			if t.vague != t.exact {
				if colIdx := strings.Index(expect, ":"); colIdx > 0 {
					expect = expect[:colIdx]
				}
			}
			c.Check(ierr.Error(), gc.Matches, expect+".*", comment)
		}
	}
	u, err := charm.InferURL("~blah", "defseries")
	c.Assert(u, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "charm URL without charm name: .*")
}

var inferNoDefaultSeriesTests = []struct {
	vague, exact string
	resolved     bool
}{
	{"foo", "", false},
	{"foo-1", "", false},
	{"cs:foo", "", false},
	{"cs:~user/foo", "", false},
	{"series/foo", "cs:series/foo", true},
	{"cs:series/foo", "cs:series/foo", true},
	{"cs:~user/series/foo", "cs:~user/series/foo", true},
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

func (s *URLSuite) TestParseUnresolved(c *gc.C) {
	for _, t := range inferNoDefaultSeriesTests {
		if t.resolved {
			url, err := charm.ParseURL(t.vague)
			c.Assert(err, gc.IsNil)
			c.Assert(url.Series, gc.Not(gc.Equals), "")
		} else {
			_, series, err := charm.ParseReference(t.vague)
			c.Assert(err, gc.IsNil)
			c.Assert(series, gc.Equals, "")
			_, err = charm.ParseURL(t.vague)
			c.Assert(err, gc.NotNil)
			c.Assert(err, gc.Equals, charm.ErrUnresolvedUrl)
		}
	}
}

var validTests = []struct {
	valid  func(string) bool
	string string
	expect bool
}{

	{charm.IsValidUser, "", false},
	{charm.IsValidUser, "b^b", false},
	{charm.IsValidUser, "jim.bob99-1", true},

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
	{charm.IsValidSeries, "pre-cise", false},
	{charm.IsValidSeries, "pre^cise", false},
	{charm.IsValidSeries, "prec1se", true},
	{charm.IsValidSeries, "-precise", false},
	{charm.IsValidSeries, "precise-", false},
	{charm.IsValidSeries, "precise-1", false},
	{charm.IsValidSeries, "precise1", true},
	{charm.IsValidSeries, "pre-c1se", false},
}

func (s *URLSuite) TestValidCheckers(c *gc.C) {
	for i, t := range validTests {
		c.Logf("test %d: %s", i, t.string)
		c.Assert(t.valid(t.string), gc.Equals, t.expect, gc.Commentf("%s", t.string))
	}
}

func (s *URLSuite) TestMustParseURL(c *gc.C) {
	url := charm.MustParseURL("cs:series/name")
	c.Assert(url, gc.DeepEquals,
		&charm.URL{Reference: charm.Reference{"cs", "", "name", -1}, Series: "series"})
	f := func() { charm.MustParseURL("local:@@/name") }
	c.Assert(f, gc.PanicMatches, "charm URL has invalid series: .*")
	f = func() { charm.MustParseURL("cs:~user") }
	c.Assert(f, gc.PanicMatches, "charm URL without charm name: .*")
	f = func() { charm.MustParseURL("cs:~user") }
	c.Assert(f, gc.PanicMatches, "charm URL without charm name: .*")
	f = func() { charm.MustParseURL("cs:name") }
	c.Assert(f, gc.PanicMatches, "charm url series is not resolved")
}

func (s *URLSuite) TestWithRevision(c *gc.C) {
	url := charm.MustParseURL("cs:series/name")
	other := url.WithRevision(1)
	c.Assert(url, gc.DeepEquals, &charm.URL{charm.Reference{"cs", "", "name", -1}, "series"})
	c.Assert(other, gc.DeepEquals, &charm.URL{charm.Reference{"cs", "", "name", 1}, "series"})

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

func (s *URLSuite) TestURLCodecs(c *gc.C) {
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

func (s *URLSuite) TestReferenceJSON(c *gc.C) {
	ref, _, err := charm.ParseReference("cs:series/name")
	c.Assert(err, gc.IsNil)
	data, err := json.Marshal(&ref)
	c.Assert(err, gc.IsNil)
	c.Check(string(data), gc.Equals, `"cs:name"`)

	var parsed charm.Reference
	err = json.Unmarshal(data, &parsed)
	c.Assert(err, gc.IsNil)
	c.Check(parsed, gc.DeepEquals, ref)

	// unmarshalling json gibberish and invalid charm reference strings
	for _, value := range []string{":{", `"cs:{}+<"`, `"cs:~_~/f00^^&^/baaaar$%-?"`} {
		err = json.Unmarshal([]byte(value), &parsed)
		c.Check(err, gc.NotNil)
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
