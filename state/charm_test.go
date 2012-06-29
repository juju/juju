package state_test

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/charm"
	"net/url"
)

type CharmSuite struct {
	ConnSuite
	curl *charm.URL
}

var _ = Suite(&CharmSuite{})

func (s *CharmSuite) SetUpTest(c *C) {
	s.ConnSuite.SetUpTest(c)
	added := s.AddTestingCharm(c, "dummy")
	s.curl = added.URL()
}

func (s *CharmSuite) TestCharm(c *C) {
	dummy, err := s.St.Charm(s.curl)
	c.Assert(err, IsNil)
	c.Assert(dummy.URL().String(), Equals, s.curl.String())
	c.Assert(dummy.Revision(), Equals, 1)
	bundleURL, err := url.Parse("http://bundles.example.com/dummy-1")
	c.Assert(err, IsNil)
	c.Assert(dummy.BundleURL(), DeepEquals, bundleURL)
	c.Assert(dummy.BundleSha256(), Equals, "dummy-1-sha256")
	meta := dummy.Meta()
	c.Assert(meta.Name, Equals, "dummy")
	config := dummy.Config()
	c.Assert(config.Options["title"], Equals,
		charm.Option{
			Default:     "My Title",
			Description: "A descriptive title used for the service.",
			Type:        "string",
		},
	)
}
