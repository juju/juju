// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	"fmt"
	"net/url"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

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
	c.Assert(err, jc.ErrorIsNil)
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

func (s *charmSuite) TestArchiveURLs(c *gc.C) {
	apiInfo := s.APIInfo(c)
	url, err := url.Parse(fmt.Sprintf(
		"https://0.1.2.3:1234/environment/%s/charms?file=%s&url=%s",
		apiInfo.EnvironTag.Id(),
		url.QueryEscape("*"),
		url.QueryEscape(s.apiCharm.URL().String()),
	))
	c.Assert(err, jc.ErrorIsNil)
	archiveURLs, err := s.apiCharm.ArchiveURLs()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(archiveURLs, gc.HasLen, 1)
	c.Assert(archiveURLs[0], gc.DeepEquals, url)
}

func (s *charmSuite) TestArchiveSha256(c *gc.C) {
	archiveSha256, err := s.apiCharm.ArchiveSha256()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(archiveSha256, gc.Equals, s.wordpressCharm.BundleSha256())
}
