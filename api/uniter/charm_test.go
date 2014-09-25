// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package uniter_test

import (
	"fmt"
	"net/url"

	"github.com/juju/names"
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/api/uniter"
	"github.com/juju/juju/testing"
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

type charmsURLSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&charmsURLSuite{})

func (s *charmsURLSuite) TestCharmsURL(c *gc.C) {
	testCharmsURL(c, "", "", "https:///charms")
	testCharmsURL(c, "abc.com", "", "https://abc.com/charms")
	testCharmsURL(c, "abc.com:123", "", "https://abc.com:123/charms")
	testCharmsURL(c, "abc.com:123", "invalid-uuid", "https://abc.com:123/charms")
	testCharmsURL(c, "abc.com:123", "environment-f47ac10b-58cc-4372-a567-0e02b2c3d479", "https://abc.com:123/environment/f47ac10b-58cc-4372-a567-0e02b2c3d479/charms")
}

func testCharmsURL(c *gc.C, addr, envTag, expected string) {
	tag, err := names.ParseEnvironTag(envTag)
	if err != nil {
		// If it's invalid, pretend it's not set at all.
		tag = names.NewEnvironTag("")
	}
	url := uniter.CharmsURL(addr, tag)
	if !c.Check(url, gc.NotNil) {
		return
	}
	c.Check(url.String(), gc.Equals, expected)
}
