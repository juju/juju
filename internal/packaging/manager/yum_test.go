// Copyright 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the LGPLv3, see LICENCE file for details.

package manager_test

import (
	"strings"

	"github.com/juju/proxy"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/internal/packaging/commands"
	"github.com/juju/juju/internal/packaging/manager"
)

var _ = gc.Suite(&YumSuite{})

type YumSuite struct {
	testing.IsolationSuite
	paccmder commands.PackageCommander
	pacman   manager.PackageManager
}

func (s *YumSuite) SetUpSuite(c *gc.C) {
	s.IsolationSuite.SetUpSuite(c)
	s.paccmder = commands.NewYumPackageCommander()
	s.pacman = manager.NewYumPackageManager()
}

func (s *YumSuite) SetUpTest(c *gc.C) {
	s.IsolationSuite.SetUpTest(c)
}

func (s *YumSuite) TearDownTest(c *gc.C) {
	s.IsolationSuite.TearDownTest(c)
}

func (s *YumSuite) TearDownSuite(c *gc.C) {
	s.IsolationSuite.TearDownSuite(c)
}

func (s *YumSuite) TestGetProxySettingsEmpty(c *gc.C) {
	cmdChan := s.HookCommandOutput(&manager.CommandOutput, []byte{}, nil)

	out, err := s.pacman.GetProxySettings()
	c.Assert(err, jc.ErrorIsNil)

	cmd := <-cmdChan
	c.Assert(cmd.Args, gc.DeepEquals, strings.Fields(s.paccmder.GetProxyCmd()))
	c.Assert(out, gc.Equals, proxy.Settings{})
}

func (s *YumSuite) TestGetProxySettingsConfigured(c *gc.C) {
	const expected = `# some random comment
http_proxy = 10.0.3.1:3142
https_proxy = false
magic_proxy = narnia
ftp_proxy = none`
	cmdChan := s.HookCommandOutput(&manager.CommandOutput, []byte(expected), nil)

	out, err := s.pacman.GetProxySettings()
	c.Assert(err, gc.IsNil)

	cmd := <-cmdChan
	c.Assert(cmd.Args, gc.DeepEquals, strings.Fields(s.paccmder.GetProxyCmd()))

	c.Assert(out, gc.Equals, proxy.Settings{
		Http:  "10.0.3.1:3142",
		Https: "false",
		Ftp:   "none",
	})
}

func (s *YumSuite) TestProxySettingsRoundTrip(c *gc.C) {
	initial := proxy.Settings{
		Http:  "some-proxy.local:8080",
		Https: "some-secure-proxy.local:9696",
		Ftp:   "some-ftp-proxy.local:1212",
	}

	expected := s.paccmder.ProxyConfigContents(initial)
	cmdChan := s.HookCommandOutput(&manager.CommandOutput, []byte(expected), nil)

	result, err := s.pacman.GetProxySettings()
	c.Assert(err, gc.IsNil)

	cmd := <-cmdChan
	c.Assert(cmd.Args, gc.DeepEquals, strings.Fields(s.paccmder.GetProxyCmd()))

	c.Assert(result, gc.Equals, initial)
}
