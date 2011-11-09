package charm_test

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju/go/charm"
)

var urlTests = []struct {
	s, err string
	url    *charm.URL
}{
	{"cs:~user/series/name", "", &charm.URL{"name", -1, charm.Collection{"cs", "user", "series"}}},
	{"cs:~user/series/name-0", "", &charm.URL{"name", 0, charm.Collection{"cs", "user", "series"}}},
	{"cs:series/name", "", &charm.URL{"name", -1, charm.Collection{"cs", "", "series"}}},
	{"cs:series/name-42", "", &charm.URL{"name", 42, charm.Collection{"cs", "", "series"}}},
	{"local:series/name-1", "", &charm.URL{"name", 1, charm.Collection{"local", "", "series"}}},
	{"local:series/name", "", &charm.URL{"name", -1, charm.Collection{"local", "", "series"}}},
	{"local:series/n0-0n-n0", "", &charm.URL{"n0-0n-n0", -1, charm.Collection{"local", "", "series"}}},

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

func (s *S) TestNewURL(c *C) {
	for _, t := range urlTests {
		url, err := charm.NewURL(t.s)
		bug := Bug("NewURL(%q)", t.s)
		if t.err != "" {
			c.Check(err.Error(), Matches, t.err, bug)
		} else {
			c.Check(url, Equals, t.url, bug)
			c.Check(t.url.String(), Equals, t.s)
		}
	}
}
