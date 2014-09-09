// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	"fmt"
	"net/url"

	gc "launchpad.net/gocheck"

	"github.com/juju/juju/api/uniter"
)

type charmSuite struct {
	uniterSuite

	apiCharm *uniter.Charm
}

var _ = gc.Suite(&charmSuite{})

func (s *charmSuite) SetUpTest(c *gc.C) {
	s.uniterSuite.SetUpTest(c)

	var err error
	s.apiCharm, err = s.uniter.Charm(s.wordpressCharm.URL())
	c.Assert(err, gc.IsNil)
	c.Assert(s.apiCharm, gc.NotNil)
}

func (s *charmSuite) TearDownTest(c *gc.C) {
	s.uniterSuite.TearDownTest(c)
}

func (s *charmSuite) TestCharmWithNilFails(c *gc.C) {
	_, err := s.uniter.Charm(nil)
	c.Assert(err, gc.ErrorMatches, "charm url cannot be nil")
}

func (s *charmSuite) TestString(c *gc.C) {
	c.Assert(s.apiCharm.String(), gc.Equals, s.wordpressCharm.String())
}

func (s *charmSuite) TestURL(c *gc.C) {
	c.Assert(s.apiCharm.URL(), gc.DeepEquals, s.wordpressCharm.URL())
}

func (s *charmSuite) TestArchiveURL(c *gc.C) {
	apiInfo := s.APIInfo(c)
	url, err := url.Parse(fmt.Sprintf(
		"https://%s/environment/%s/charms?file=%s&url=%s",
		apiInfo.Addrs[0],
		apiInfo.EnvironTag.Id(),
		url.QueryEscape("*"),
		url.QueryEscape(s.apiCharm.URL().String()),
	))
	c.Assert(err, gc.IsNil)
	archiveURL := s.apiCharm.ArchiveURL()
	c.Assert(archiveURL, gc.DeepEquals, url)
}

func (s *charmSuite) TestArchiveSha256(c *gc.C) {
	archiveSha256, err := s.apiCharm.ArchiveSha256()
	c.Assert(err, gc.IsNil)
	c.Assert(archiveSha256, gc.Equals, s.wordpressCharm.BundleSha256())
}
