package charm_test

import (
	"fmt"
	. "launchpad.net/gocheck"
	"launchpad.net/juju/go/charm"
)

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

func (s *S) TestParseURL(c *C) {
	for _, t := range urlTests {
		url, err := charm.ParseURL(t.s)
		comment := Commentf("ParseURL(%q)", t.s)
		if t.err != "" {
			c.Check(err.Error(), Matches, t.err, comment)
		} else {
			c.Check(url, DeepEquals, t.url, comment)
			c.Check(t.url.String(), Equals, t.s)
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

func (s *S) TestInferURL(c *C) {
	for _, t := range inferTests {
		comment := Commentf("InferURL(%q, %q)", t.vague, "defseries")
		inferred, ierr := charm.InferURL(t.vague, "defseries")
		parsed, perr := charm.ParseURL(t.exact)
		if parsed != nil {
			c.Check(inferred, DeepEquals, parsed, comment)
		} else {
			expect := perr.Error()
			if t.vague != t.exact {
				expect = fmt.Sprintf("%s (URL inferred from %q)", expect, t.vague)
			}
			c.Check(ierr.Error(), Equals, expect, comment)
		}
	}
	u, err := charm.InferURL("~blah", "defseries")
	c.Assert(u, IsNil)
	c.Assert(err, ErrorMatches, "cannot infer charm URL with user but no schema: .*")
}

func (s *S) TestMustParseURL(c *C) {
	url := charm.MustParseURL("cs:series/name")
	c.Assert(url, DeepEquals, &charm.URL{"cs", "", "series", "name", -1})
	f := func() { charm.MustParseURL("local:name") }
	c.Assert(f, PanicMatches, "charm URL without series: .*")
}

func (s *S) TestWithRevision(c *C) {
	url := charm.MustParseURL("cs:series/name")
	other := url.WithRevision(1)
	c.Assert(url, DeepEquals, &charm.URL{"cs", "", "series", "name", -1})
	c.Assert(other, DeepEquals, &charm.URL{"cs", "", "series", "name", 1})

	// Should always copy. The opposite behavior is error prone.
	c.Assert(other.WithRevision(1), Not(Equals), other)
	c.Assert(other.WithRevision(1), DeepEquals, other)
}

type QuoteSuite struct{}

var _ = Suite(&QuoteSuite{})

func (s *QuoteSuite) TestUnmodified(c *C) {
	// Check that a string containing only valid
	// chars stays unmodified.
	in := "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789.-"
	out := charm.Quote(in)
	c.Assert(out, Equals, in)
}

func (s *QuoteSuite) TestQuote(c *C) {
	// Check that invalid chars are translated correctly.
	in := "hello_there/how'are~you-today.sir"
	out := charm.Quote(in)
	c.Assert(out, Equals, "hello_5f_there_2f_how_27_are_7e_you-today.sir")
}
