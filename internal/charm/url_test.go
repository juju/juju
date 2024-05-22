// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package charm_test

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/juju/mgo/v3/bson"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
	"gopkg.in/yaml.v2"

	"github.com/juju/juju/internal/charm"
)

type URLSuite struct{}

var _ = gc.Suite(&URLSuite{})

var urlTests = []struct {
	s, err string
	exact  string
	url    *charm.URL
}{{
	s:   "local:series/name-1",
	url: &charm.URL{Schema: "local", Name: "name", Revision: 1, Series: "series", Architecture: ""},
}, {
	s:   "local:series/name",
	url: &charm.URL{Schema: "local", Name: "name", Revision: -1, Series: "series", Architecture: ""},
}, {
	s:   "local:series/n0-0n-n0",
	url: &charm.URL{Schema: "local", Name: "n0-0n-n0", Revision: -1, Series: "series", Architecture: ""},
}, {
	s:   "local:name",
	url: &charm.URL{Schema: "local", Name: "name", Revision: -1, Series: "", Architecture: ""},
}, {
	s:   "bs:~user/series/name-1",
	err: `cannot parse URL $URL: schema "bs" not valid`,
}, {
	s:   ":foo",
	err: `cannot parse charm or bundle URL: $URL`,
}, {
	s:   "local:~user/series/name",
	err: `local charm or bundle URL with user name: $URL`,
}, {
	s:   "local:~user/name",
	err: `local charm or bundle URL with user name: $URL`,
}, {
	s:     "amd64/name",
	url:   &charm.URL{Schema: "ch", Name: "name", Revision: -1, Series: "", Architecture: "amd64"},
	exact: "ch:amd64/name",
}, {
	s:     "foo",
	url:   &charm.URL{Schema: "ch", Name: "foo", Revision: -1, Series: "", Architecture: ""},
	exact: "ch:foo",
}, {
	s:     "foo-1",
	exact: "ch:foo-1",
	url:   &charm.URL{Schema: "ch", Name: "foo", Revision: 1, Series: "", Architecture: ""},
}, {
	s:     "n0-n0-n0",
	exact: "ch:n0-n0-n0",
	url:   &charm.URL{Schema: "ch", Name: "n0-n0-n0", Revision: -1, Series: "", Architecture: ""},
}, {
	s:     "local:foo",
	exact: "local:foo",
	url:   &charm.URL{Schema: "local", Name: "foo", Revision: -1, Series: "", Architecture: ""},
}, {
	s:     "arm64/series/bar",
	url:   &charm.URL{Schema: "ch", Name: "bar", Revision: -1, Series: "series", Architecture: "arm64"},
	exact: "ch:arm64/series/bar",
}, {
	s:   "ch:name",
	url: &charm.URL{Schema: "ch", Name: "name", Revision: -1, Series: "", Architecture: ""},
}, {
	s:   "ch:name-suffix",
	url: &charm.URL{Schema: "ch", Name: "name-suffix", Revision: -1, Series: "", Architecture: ""},
}, {
	s:   "ch:name-1",
	url: &charm.URL{Schema: "ch", Name: "name", Revision: 1, Series: "", Architecture: ""},
}, {
	s:   "ch:focal/istio-gateway-74",
	url: &charm.URL{Schema: "ch", Name: "istio-gateway", Revision: 74, Series: "focal", Architecture: ""},
}, {
	s:   "ch:amd64/istio-gateway-74",
	url: &charm.URL{Schema: "ch", Name: "istio-gateway", Revision: 74, Series: "", Architecture: "amd64"},
}, {
	s:     "ch:arm64/name",
	url:   &charm.URL{Schema: "ch", Name: "name", Revision: -1, Series: "", Architecture: "arm64"},
	exact: "ch:arm64/name",
}, {
	s:   "ch:~user/name",
	err: `charmhub charm or bundle URL with user name: "ch:~user/name" not valid`,
}, {
	s:   "ch:purple/series/name-0",
	err: `in URL "ch:purple/series/name-0": architecture name "purple" not valid`,
}, {
	s:   "ch:nam-!e",
	err: `cannot parse name and/or revision in URL "ch:nam-!e": name "nam-!e" not valid`,
}, {
	s:   "cs:testme",
	err: `cannot parse URL "cs:testme": schema "cs" not valid`,
}}

func (s *URLSuite) TestParseURL(c *gc.C) {
	for i, t := range urlTests {
		c.Logf("test %d: %q", i, t.s)

		expectStr := t.s
		if t.exact != "" {
			expectStr = t.exact
		}
		url, uerr := charm.ParseURL(t.s)
		if t.err != "" {
			t.err = strings.Replace(t.err, "$URL", regexp.QuoteMeta(fmt.Sprintf("%q", t.s)), -1)
			c.Check(uerr, gc.ErrorMatches, t.err)
			c.Check(url, gc.IsNil)
			continue
		}
		c.Assert(uerr, gc.IsNil)
		c.Check(url, gc.DeepEquals, t.url)
		c.Check(url.String(), gc.Equals, expectStr)

		// URL strings are generated as expected.  Reversability is preserved
		// with v1 URLs.
		if t.exact != "" {
			c.Check(url.String(), gc.Equals, t.exact)
		} else {
			c.Check(url.String(), gc.Equals, t.s)
		}
	}
}

var ensureSchemaTests = []struct {
	input, expected, err string
}{
	{input: "foo", expected: "ch:foo"},
	{input: "foo-1", expected: "ch:foo-1"},
	{input: "~user/foo", expected: "ch:~user/foo"},
	{input: "series/foo", expected: "ch:series/foo"},
	{input: "local:foo", expected: "local:foo"},
	{
		input: "unknown:foo",
		err:   `schema "unknown" not valid`,
	},
}

func (s *URLSuite) TestInferURLNoDefaultSeries(c *gc.C) {
	for i, t := range ensureSchemaTests {
		c.Logf("%d: %s", i, t.input)
		inferred, err := charm.EnsureSchema(t.input, charm.CharmHub)
		if t.err != "" {
			c.Assert(err, gc.ErrorMatches, t.err)
			continue
		}

		c.Assert(err, gc.IsNil)
		c.Assert(inferred, gc.Equals, t.expected)
	}
}

var validTests = []struct {
	valid  func(string) bool
	string string
	expect bool
}{

	{valid: charm.IsValidName, string: "", expect: false},
	{valid: charm.IsValidName, string: "wordpress", expect: true},
	{valid: charm.IsValidName, string: "Wordpress", expect: false},
	{valid: charm.IsValidName, string: "word-press", expect: true},
	{valid: charm.IsValidName, string: "word press", expect: false},
	{valid: charm.IsValidName, string: "word^press", expect: false},
	{valid: charm.IsValidName, string: "-wordpress", expect: false},
	{valid: charm.IsValidName, string: "wordpress-", expect: false},
	{valid: charm.IsValidName, string: "wordpress2", expect: true},
	{valid: charm.IsValidName, string: "wordpress-2", expect: false},
	{valid: charm.IsValidName, string: "word2-press2", expect: true},

	{valid: charm.IsValidSeries, string: "", expect: false},
	{valid: charm.IsValidSeries, string: "precise", expect: true},
	{valid: charm.IsValidSeries, string: "Precise", expect: false},
	{valid: charm.IsValidSeries, string: "pre cise", expect: false},
	{valid: charm.IsValidSeries, string: "pre-cise", expect: false},
	{valid: charm.IsValidSeries, string: "pre^cise", expect: false},
	{valid: charm.IsValidSeries, string: "prec1se", expect: true},
	{valid: charm.IsValidSeries, string: "-precise", expect: false},
	{valid: charm.IsValidSeries, string: "precise-", expect: false},
	{valid: charm.IsValidSeries, string: "precise-1", expect: false},
	{valid: charm.IsValidSeries, string: "precise1", expect: true},
	{valid: charm.IsValidSeries, string: "pre-c1se", expect: false},

	{valid: charm.IsValidArchitecture, string: "amd64", expect: true},
	{valid: charm.IsValidArchitecture, string: "~amd64", expect: false},
	{valid: charm.IsValidArchitecture, string: "not-an-arch", expect: false},
}

func (s *URLSuite) TestValidCheckers(c *gc.C) {
	for i, t := range validTests {
		c.Logf("test %d: %s", i, t.string)
		c.Assert(t.valid(t.string), gc.Equals, t.expect, gc.Commentf("%s", t.string))
	}
}

func (s *URLSuite) TestMustParseURL(c *gc.C) {
	url := charm.MustParseURL("ch:series/name")
	c.Assert(url, gc.DeepEquals, &charm.URL{Schema: "ch", Name: "name", Revision: -1, Series: "series", Architecture: ""})
	f := func() { charm.MustParseURL("local:@@/name") }
	c.Assert(f, gc.PanicMatches, "cannot parse URL \"local:@@/name\": series name \"@@\" not valid")
}

func (s *URLSuite) TestWithRevision(c *gc.C) {
	url := charm.MustParseURL("ch:series/name")
	other := url.WithRevision(1)
	c.Assert(url, gc.DeepEquals, &charm.URL{Schema: "ch", Name: "name", Revision: -1, Series: "series", Architecture: ""})
	c.Assert(other, gc.DeepEquals, &charm.URL{Schema: "ch", Name: "name", Revision: 1, Series: "series", Architecture: ""})

	// Should always copy. The opposite behavior is error prone.
	c.Assert(other.WithRevision(1), gc.Not(gc.Equals), other)
	c.Assert(other.WithRevision(1), gc.DeepEquals, other)
}

var codecs = []struct {
	Name      string
	Marshal   func(interface{}) ([]byte, error)
	Unmarshal func([]byte, interface{}) error
}{{
	Name:      "bson",
	Marshal:   bson.Marshal,
	Unmarshal: bson.Unmarshal,
}, {
	Name:      "json",
	Marshal:   json.Marshal,
	Unmarshal: json.Unmarshal,
}, {
	Name:      "yaml",
	Marshal:   yaml.Marshal,
	Unmarshal: yaml.Unmarshal,
}}

func (s *URLSuite) TestURLCodecs(c *gc.C) {
	for i, codec := range codecs {
		c.Logf("codec %d: %v", i, codec.Name)
		type doc struct {
			URL *charm.URL `json:",omitempty" bson:",omitempty" yaml:",omitempty"`
		}
		url := charm.MustParseURL("ch:name")
		v0 := doc{URL: url}
		data, err := codec.Marshal(v0)
		c.Assert(err, jc.ErrorIsNil)
		var v doc
		err = codec.Unmarshal(data, &v)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(v, gc.DeepEquals, v0)

		// Check that the underlying representation
		// is a string.
		type strDoc struct {
			URL string
		}
		var vs strDoc
		err = codec.Unmarshal(data, &vs)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(vs.URL, gc.Equals, "ch:name")

		data, err = codec.Marshal(doc{})
		c.Assert(err, jc.ErrorIsNil)
		v = doc{}
		err = codec.Unmarshal(data, &v)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(v.URL, gc.IsNil, gc.Commentf("data: %q", data))
	}
}

func (s *URLSuite) TestJSONGarbage(c *gc.C) {
	// unmarshalling json gibberish
	for _, value := range []string{":{", `"ch:{}+<"`, `"ch:~_~/f00^^&^/baaaar$%-?"`} {
		err := json.Unmarshal([]byte(value), new(struct{ URL *charm.URL }))
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
