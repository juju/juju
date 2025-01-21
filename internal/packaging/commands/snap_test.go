// Copyright 2019 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package commands_test

import (
	"github.com/juju/proxy"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/internal/packaging/commands"
)

var _ = gc.Suite(&SnapSuite{})

type SnapSuite struct {
	snapCommander commands.SnapPackageCommander
}

func (s *SnapSuite) SetUpSuite(c *gc.C) {
	s.snapCommander = commands.NewSnapPackageCommander()
}

func (s *SnapSuite) TestSetProxyCommandsEmpty(c *gc.C) {
	out, err := s.snapCommander.SetProxyCmds(proxy.Settings{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(out, gc.HasLen, 0)
}

func (s *SnapSuite) TestSetProxyCommandsPartial(c *gc.C) {
	sets := proxy.Settings{
		Http: "dat-proxy.zone:8080",
	}

	output, err := s.snapCommander.SetProxyCmds(sets)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(output, gc.HasLen, 1)
	c.Assert(output[0], gc.Equals, `snap set system proxy.http="dat-proxy.zone:8080"`)
}

func (s *SnapSuite) TestProxyConfigContentsFull(c *gc.C) {
	sets := proxy.Settings{
		Http:  "dat-proxy.zone:8080",
		Https: "https://much-security.com",
	}

	output, err := s.snapCommander.SetProxyCmds(sets)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(output, gc.HasLen, 2)
	c.Assert(output[0], gc.Equals, `snap set system proxy.http="dat-proxy.zone:8080"`)
	c.Assert(output[1], gc.Equals, `snap set system proxy.https="https://much-security.com"`)
}

func (s *SnapSuite) TestSetProxyCommandsUnsupported(c *gc.C) {
	sets := proxy.Settings{
		Ftp: "gimme-files.zone",
	}
	_, err := s.snapCommander.SetProxyCmds(sets)
	c.Assert(err, jc.ErrorIs, commands.ErrProxySettingNotSupported)

	sets = proxy.Settings{
		NoProxy: "local1,local2",
	}
	_, err = s.snapCommander.SetProxyCmds(sets)
	c.Assert(err, jc.ErrorIs, commands.ErrProxySettingNotSupported)

	sets = proxy.Settings{
		AutoNoProxy: "local1,local2",
	}
	_, err = s.snapCommander.SetProxyCmds(sets)
	c.Assert(err, jc.ErrorIs, commands.ErrProxySettingNotSupported)
}
