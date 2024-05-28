// Copyright 2019 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package commands_test

import (
	"github.com/juju/proxy"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/internal/packaging/commands"
)

var _ = gc.Suite(&SnapSuite{})

type SnapSuite struct {
	paccmder commands.PackageCommander
}

func (s *SnapSuite) SetUpSuite(c *gc.C) {
	s.paccmder = commands.NewSnapPackageCommander()
}

func (s *SnapSuite) TestProxyConfigContentsEmpty(c *gc.C) {
	out := s.paccmder.ProxyConfigContents(proxy.Settings{})
	c.Assert(out, gc.Equals, "")
}

func (s *SnapSuite) TestProxyConfigContentsPartial(c *gc.C) {
	sets := proxy.Settings{
		Http: "dat-proxy.zone:8080",
	}

	output := s.paccmder.ProxyConfigContents(sets)
	c.Assert(output, gc.Equals, `proxy.http="dat-proxy.zone:8080"`)
}

func (s *SnapSuite) TestProxyConfigContentsFull(c *gc.C) {
	sets := proxy.Settings{
		Http:  "dat-proxy.zone:8080",
		Https: "https://much-security.com",
		Ftp:   "gimme-files.zone",
	}
	expected := `proxy.http="dat-proxy.zone:8080"
proxy.https="https://much-security.com"
proxy.ftp="gimme-files.zone"`

	output := s.paccmder.ProxyConfigContents(sets)
	c.Assert(output, gc.Equals, expected)
}

func (s *SnapSuite) TestSetProxyCommands(c *gc.C) {
	sets := proxy.Settings{
		Http:  "dat-proxy.zone:8080",
		Https: "https://much-security.com",
		Ftp:   "gimme-files.zone",
	}
	expected := []string{
		`snap set system proxy.http="dat-proxy.zone:8080"`,
		`snap set system proxy.https="https://much-security.com"`,
		`snap set system proxy.ftp="gimme-files.zone"`,
	}

	output := s.paccmder.SetProxyCmds(sets)
	c.Assert(output, gc.DeepEquals, expected)
}
