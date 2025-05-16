// Copyright 2011, 2012, 2013 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package charm_test

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	stdtesting "testing"

	"github.com/juju/tc"
	"gopkg.in/yaml.v2"

	"github.com/juju/juju/internal/charm"
)

type URLSuite struct{}

func TestURLSuite(t *stdtesting.T) { tc.Run(t, &URLSuite{}) }

var urlTests = []struct {
	s, err string
	exact  string
	url    *charm.URL
}{{
	s:   "local:name-1",
	url: &charm.URL{Schema: "local", Name: "name", Revision: 1, Architecture: ""},
}, {
	s:   "local:name",
	url: &charm.URL{Schema: "local", Name: "name", Revision: -1, Architecture: ""},
}, {
	s:   "local:n0-0n-n0",
	url: &charm.URL{Schema: "local", Name: "n0-0n-n0", Revision: -1, Architecture: ""},
}, {
	s:   "local:name",
	url: &charm.URL{Schema: "local", Name: "name", Revision: -1, Architecture: ""},
}, {
	s:   "bs:~user/name-1",
	err: `cannot parse URL $URL: schema "bs" not valid`,
}, {
	s:   ":foo",
	err: `cannot parse charm or bundle URL: $URL`,
}, {
	s:   "local:~user/name",
	err: `local charm or bundle URL with user name: $URL`,
}, {
	s:   "local:~user/name",
	err: `local charm or bundle URL with user name: $URL`,
}, {
	s:     "amd64/name",
	url:   &charm.URL{Schema: "ch", Name: "name", Revision: -1, Architecture: "amd64"},
	exact: "ch:amd64/name",
}, {
	s:     "foo",
	url:   &charm.URL{Schema: "ch", Name: "foo", Revision: -1, Architecture: ""},
	exact: "ch:foo",
}, {
	s:     "foo-1",
	exact: "ch:foo-1",
	url:   &charm.URL{Schema: "ch", Name: "foo", Revision: 1, Architecture: ""},
}, {
	s:     "n0-n0-n0",
	exact: "ch:n0-n0-n0",
	url:   &charm.URL{Schema: "ch", Name: "n0-n0-n0", Revision: -1, Architecture: ""},
}, {
	s:     "local:foo",
	exact: "local:foo",
	url:   &charm.URL{Schema: "local", Name: "foo", Revision: -1, Architecture: ""},
}, {
	s:     "arm64/bar",
	url:   &charm.URL{Schema: "ch", Name: "bar", Revision: -1, Architecture: "arm64"},
	exact: "ch:arm64/bar",
}, {
	s:   "ch:name",
	url: &charm.URL{Schema: "ch", Name: "name", Revision: -1, Architecture: ""},
}, {
	s:   "ch:name-suffix",
	url: &charm.URL{Schema: "ch", Name: "name-suffix", Revision: -1, Architecture: ""},
}, {
	s:   "ch:name-1",
	url: &charm.URL{Schema: "ch", Name: "name", Revision: 1, Architecture: ""},
}, {
	s:   "ch:istio-gateway-74",
	url: &charm.URL{Schema: "ch", Name: "istio-gateway", Revision: 74, Architecture: ""},
}, {
	s:   "ch:amd64/istio-gateway-74",
	url: &charm.URL{Schema: "ch", Name: "istio-gateway", Revision: 74, Architecture: "amd64"},
}, {
	s:     "ch:arm64/name",
	url:   &charm.URL{Schema: "ch", Name: "name", Revision: -1, Architecture: "arm64"},
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

func (s *URLSuite) TestParseURL(c *tc.C) {
	for i, t := range urlTests {
		c.Logf("test %d: %q", i, t.s)

		expectStr := t.s
		if t.exact != "" {
			expectStr = t.exact
		}
		url, uerr := charm.ParseURL(t.s)
		if t.err != "" {
			t.err = strings.Replace(t.err, "$URL", regexp.QuoteMeta(fmt.Sprintf("%q", t.s)), -1)
			c.Check(uerr, tc.ErrorMatches, t.err)
			c.Check(url, tc.IsNil)
			continue
		}
		c.Assert(uerr, tc.IsNil)
		c.Check(url, tc.DeepEquals, t.url)
		c.Check(url.String(), tc.Equals, expectStr)

		// URL strings are generated as expected.  Reversability is preserved
		// with v1 URLs.
		if t.exact != "" {
			c.Check(url.String(), tc.Equals, t.exact)
		} else {
			c.Check(url.String(), tc.Equals, t.s)
		}
	}
}

var ensureSchemaTests = []struct {
	input, expected, err string
}{
	{input: "foo", expected: "ch:foo"},
	{input: "foo-1", expected: "ch:foo-1"},
	{input: "~user/foo", expected: "ch:~user/foo"},
	{input: "foo", expected: "ch:foo"},
	{input: "local:foo", expected: "local:foo"},
	{
		input: "unknown:foo",
		err:   `schema "unknown" not valid`,
	},
}

func (s *URLSuite) TestInferURL(c *tc.C) {
	for i, t := range ensureSchemaTests {
		c.Logf("%d: %s", i, t.input)
		inferred, err := charm.EnsureSchema(t.input, charm.CharmHub)
		if t.err != "" {
			c.Assert(err, tc.ErrorMatches, t.err)
			continue
		}

		c.Assert(err, tc.IsNil)
		c.Assert(inferred, tc.Equals, t.expected)
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

	{valid: charm.IsValidArchitecture, string: "amd64", expect: true},
	{valid: charm.IsValidArchitecture, string: "~amd64", expect: false},
	{valid: charm.IsValidArchitecture, string: "not-an-arch", expect: false},
}

func (s *URLSuite) TestValidCheckers(c *tc.C) {
	for i, t := range validTests {
		c.Logf("test %d: %s", i, t.string)
		c.Assert(t.valid(t.string), tc.Equals, t.expect, tc.Commentf("%s", t.string))
	}
}

func (s *URLSuite) TestMustParseURL(c *tc.C) {
	url := charm.MustParseURL("ch:name")
	c.Assert(url, tc.DeepEquals, &charm.URL{Schema: "ch", Name: "name", Revision: -1, Architecture: ""})
}

func (s *URLSuite) TestWithRevision(c *tc.C) {
	url := charm.MustParseURL("ch:name")
	other := url.WithRevision(1)
	c.Assert(url, tc.DeepEquals, &charm.URL{Schema: "ch", Name: "name", Revision: -1, Architecture: ""})
	c.Assert(other, tc.DeepEquals, &charm.URL{Schema: "ch", Name: "name", Revision: 1, Architecture: ""})

	// Should always copy. The opposite behavior is error prone.
	c.Assert(other.WithRevision(1), tc.Not(tc.Equals), other)
	c.Assert(other.WithRevision(1), tc.DeepEquals, other)
}

var codecs = []struct {
	Name      string
	Marshal   func(interface{}) ([]byte, error)
	Unmarshal func([]byte, interface{}) error
}{{
	Name:      "json",
	Marshal:   json.Marshal,
	Unmarshal: json.Unmarshal,
}, {
	Name:      "yaml",
	Marshal:   yaml.Marshal,
	Unmarshal: yaml.Unmarshal,
}}

func (s *URLSuite) TestURLCodecs(c *tc.C) {
	for i, codec := range codecs {
		c.Logf("codec %d: %v", i, codec.Name)
		type doc struct {
			URL *charm.URL `json:",omitempty" yaml:",omitempty"`
		}
		url := charm.MustParseURL("ch:name")
		v0 := doc{URL: url}
		data, err := codec.Marshal(v0)
		c.Assert(err, tc.ErrorIsNil)
		var v doc
		err = codec.Unmarshal(data, &v)
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(v, tc.DeepEquals, v0)

		// Check that the underlying representation
		// is a string.
		type strDoc struct {
			URL string
		}
		var vs strDoc
		err = codec.Unmarshal(data, &vs)
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(vs.URL, tc.Equals, "ch:name")

		data, err = codec.Marshal(doc{})
		c.Assert(err, tc.ErrorIsNil)
		v = doc{}
		err = codec.Unmarshal(data, &v)
		c.Assert(err, tc.ErrorIsNil)
		c.Assert(v.URL, tc.IsNil, tc.Commentf("data: %q", data))
	}
}

func (s *URLSuite) TestJSONGarbage(c *tc.C) {
	// unmarshalling json gibberish
	for _, value := range []string{":{", `"ch:{}+<"`, `"ch:~_~/f00^^&^/baaaar$%-?"`} {
		err := json.Unmarshal([]byte(value), new(struct{ URL *charm.URL }))
		c.Check(err, tc.NotNil)
	}
}

type QuoteSuite struct{}

func TestQuoteSuite(t *stdtesting.T) { tc.Run(t, &QuoteSuite{}) }
func (s *QuoteSuite) TestUnmodified(c *tc.C) {
	// Check that a string containing only valid
	// chars stays unmodified.
	in := "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789.-"
	out := charm.Quote(in)
	c.Assert(out, tc.Equals, in)
}

func (s *QuoteSuite) TestQuote(c *tc.C) {
	// Check that invalid chars are translated correctly.
	in := "hello_there/how'are~you-today.sir"
	out := charm.Quote(in)
	c.Assert(out, tc.Equals, "hello_5f_there_2f_how_27_are_7e_you-today.sir")
}
