// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloudinit

import (
	"github.com/juju/proxy"
	gc "gopkg.in/check.v1"
)

type HelperSuite struct{}

var _ = gc.Suite(HelperSuite{})

type fakeCfg struct {
	CloudConfig
	packageProxySettings proxy.Settings
	snapProxySettings    proxy.Settings
	packageMirror        string
	calledAddReq         bool
}

func (f *fakeCfg) SetPackageMirror(m string) {
	f.packageMirror = m
}

func (f *fakeCfg) updateProxySettings(s PackageManagerProxyConfig) error {
	f.packageProxySettings = s.AptProxy()
	f.snapProxySettings = s.SnapProxy()
	return nil
}

func (HelperSuite) TestAddPkgCmdsCommon(c *gc.C) {
	f := &fakeCfg{}

	pps := proxy.Settings{
		Http:    "http",
		Https:   "https",
		Ftp:     "ftp",
		NoProxy: "noproxy",
	}
	sps := proxy.Settings{
		Http: "snap-http",
	}
	proxyCfg := packageManagerProxySettings{
		aptProxy:  pps,
		aptMirror: "mirror",
		snapProxy: sps,
	}

	err := addPackageCommandsCommon(f, proxyCfg)
	c.Assert(err, gc.IsNil)
	c.Assert(f.packageProxySettings, gc.Equals, pps)
	c.Assert(f.snapProxySettings, gc.Equals, sps)
	c.Assert(f.packageMirror, gc.Equals, proxyCfg.aptMirror)
	c.Assert(f.calledAddReq, gc.Equals, true)
}

// packageManagerProxySettings implements cloudinit.PackageManagerConfig.
type packageManagerProxySettings struct {
	aptProxy            proxy.Settings
	aptMirror           string
	snapProxy           proxy.Settings
	snapStoreAssertions string
	snapStoreProxyID    string
	snapStoreProxyURL   string
}

func (p packageManagerProxySettings) AptProxy() proxy.Settings    { return p.aptProxy }
func (p packageManagerProxySettings) AptMirror() string           { return p.aptMirror }
func (p packageManagerProxySettings) SnapProxy() proxy.Settings   { return p.snapProxy }
func (p packageManagerProxySettings) SnapStoreAssertions() string { return p.snapStoreAssertions }
func (p packageManagerProxySettings) SnapStoreProxyID() string    { return p.snapStoreProxyID }
func (p packageManagerProxySettings) SnapStoreProxyURL() string   { return p.snapStoreProxyURL }
