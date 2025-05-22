// Copyright 2019 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package commands_test

import (
	stdtesting "testing"

	"github.com/juju/proxy"
	"github.com/juju/tc"

	"github.com/juju/juju/internal/packaging/commands"
)

func TestSnapSuite(t *stdtesting.T) {
	tc.Run(t, &SnapSuite{})
}

type SnapSuite struct {
	snapCommander commands.SnapPackageCommander
}

func (s *SnapSuite) SetUpSuite(c *tc.C) {
	s.snapCommander = commands.NewSnapPackageCommander()
}

func (s *SnapSuite) TestSetProxyCommandsEmpty(c *tc.C) {
	out, err := s.snapCommander.SetProxyCmds(proxy.Settings{})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(out, tc.HasLen, 0)
}

func (s *SnapSuite) TestSetProxyCommandsPartial(c *tc.C) {
	sets := proxy.Settings{
		Http: "dat-proxy.zone:8080",
	}

	output, err := s.snapCommander.SetProxyCmds(sets)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(output, tc.HasLen, 1)
	c.Assert(output[0], tc.Equals, `snap set system proxy.http="dat-proxy.zone:8080"`)
}

func (s *SnapSuite) TestProxyConfigContentsFull(c *tc.C) {
	sets := proxy.Settings{
		Http:  "dat-proxy.zone:8080",
		Https: "https://much-security.com",
	}

	output, err := s.snapCommander.SetProxyCmds(sets)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(output, tc.HasLen, 2)
	c.Assert(output[0], tc.Equals, `snap set system proxy.http="dat-proxy.zone:8080"`)
	c.Assert(output[1], tc.Equals, `snap set system proxy.https="https://much-security.com"`)
}

func (s *SnapSuite) TestSetProxyCommandsUnsupported(c *tc.C) {
	sets := proxy.Settings{
		Ftp: "gimme-files.zone",
	}
	_, err := s.snapCommander.SetProxyCmds(sets)
	c.Assert(err, tc.ErrorIs, commands.ErrProxySettingNotSupported)

	sets = proxy.Settings{
		NoProxy: "local1,local2",
	}
	_, err = s.snapCommander.SetProxyCmds(sets)
	c.Assert(err, tc.ErrorIs, commands.ErrProxySettingNotSupported)

	sets = proxy.Settings{
		AutoNoProxy: "local1,local2",
	}
	_, err = s.snapCommander.SetProxyCmds(sets)
	c.Assert(err, tc.ErrorIs, commands.ErrProxySettingNotSupported)
}
