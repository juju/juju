// Copyright 2017 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.
// Created from yum_test.go

package commands_test

import (
	"github.com/juju/proxy"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/internal/packaging/commands"
)

var _ = gc.Suite(&ZypperSuite{})

type ZypperSuite struct {
	paccmder commands.PackageCommander
}

func (s *ZypperSuite) SetUpSuite(c *gc.C) {
	s.paccmder = commands.NewZypperPackageCommander()
}

func (s *ZypperSuite) TestProxyConfigContentsEmpty(c *gc.C) {
	out := s.paccmder.ProxyConfigContents(proxy.Settings{})
	c.Assert(out, gc.Equals, "")
}

func (s *ZypperSuite) TestProxyConfigContentsPartial(c *gc.C) {
	sets := proxy.Settings{
		Http: "dat-proxy.zone:8080",
	}

	output := s.paccmder.ProxyConfigContents(sets)
	c.Assert(output, gc.Equals, "HTTP_PROXY=dat-proxy.zone:8080")
}

func (s *ZypperSuite) TestProxyConfigContentsFull(c *gc.C) {
	sets := proxy.Settings{
		Http:  "dat-proxy.zone:8080",
		Https: "https://much-security.com",
		Ftp:   "gimme-files.zone",
	}
	expected := `HTTP_PROXY=dat-proxy.zone:8080
HTTPS_PROXY=https://much-security.com
FTP_PROXY=gimme-files.zone`

	output := s.paccmder.ProxyConfigContents(sets)
	c.Assert(output, gc.Equals, expected)
}
