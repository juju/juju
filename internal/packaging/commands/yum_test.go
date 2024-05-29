// Copyright 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the LGPLv3, see LICENCE file for details.

package commands_test

import (
	"github.com/juju/proxy"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/internal/packaging/commands"
)

var _ = gc.Suite(&YumSuite{})

type YumSuite struct {
	paccmder commands.PackageCommander
}

func (s *YumSuite) SetUpSuite(c *gc.C) {
	s.paccmder = commands.NewYumPackageCommander()
}

func (s *YumSuite) TestProxyConfigContentsEmpty(c *gc.C) {
	out := s.paccmder.ProxyConfigContents(proxy.Settings{})
	c.Assert(out, gc.Equals, "")
}

func (s *YumSuite) TestProxyConfigContentsPartial(c *gc.C) {
	sets := proxy.Settings{
		Http: "dat-proxy.zone:8080",
	}

	output := s.paccmder.ProxyConfigContents(sets)
	c.Assert(output, gc.Equals, "http_proxy=dat-proxy.zone:8080")
}

func (s *YumSuite) TestProxyConfigContentsFull(c *gc.C) {
	sets := proxy.Settings{
		Http:  "dat-proxy.zone:8080",
		Https: "https://much-security.com",
		Ftp:   "gimme-files.zone",
	}
	expected := `http_proxy=dat-proxy.zone:8080
https_proxy=https://much-security.com
ftp_proxy=gimme-files.zone`

	output := s.paccmder.ProxyConfigContents(sets)
	c.Assert(output, gc.Equals, expected)
}
