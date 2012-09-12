package state_test

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/charm"
	"net/http"
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
	dummy, err := s.State.Charm(s.curl)
	c.Assert(err, IsNil)
	c.Assert(dummy.URL().String(), Equals, s.curl.String())
	c.Assert(dummy.Revision(), Equals, 1)
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
	resp, err := http.Get(dummy.BundleURL().String())
	c.Assert(err, IsNil)
	defer resp.Body.Close()
	c.Assert(resp.StatusCode, Equals, http.StatusOK)
	hash := sha256.New()
	_, err = io.Copy(hash, resp.Body)
	c.Assert(err, IsNil)
	c.Assert(dummy.BundleSha256(), Equals, hex.EncodeToString(hash.Sum(nil)))
}
