// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloudinit

import (
	"testing"

	"github.com/juju/proxy"
	"github.com/juju/tc"
)

type HelperSuite struct{}

func TestHelperSuite(t *testing.T) {
	tc.Run(t, &HelperSuite{})
}

type fakeCfg struct {
	CloudConfig
	packageProxySettings proxy.Settings
	snapProxySettings    proxy.Settings
	packageMirror        string
	addUpdateScripts     bool
	addUpgradeScripts    bool
	calledAddReq         bool
}

func (f *fakeCfg) SetPackageMirror(m string) {
	f.packageMirror = m
}

func (f *fakeCfg) SetSystemUpdate(b bool) {
	f.addUpdateScripts = b
}

func (f *fakeCfg) SetSystemUpgrade(b bool) {
	f.addUpgradeScripts = b
}

func (f *fakeCfg) addRequiredPackages() {
	f.calledAddReq = true
}
func (f *fakeCfg) updateProxySettings(s PackageManagerProxyConfig) error {
	f.packageProxySettings = s.AptProxy()
	f.snapProxySettings = s.SnapProxy()
	return nil
}

func (HelperSuite) TestAddPkgCmdsCommon(c *tc.C) {
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

	upd, upg := true, true

	err := addPackageCommandsCommon(f, proxyCfg, upd, upg)
	c.Assert(err, tc.IsNil)
	c.Assert(f.packageProxySettings, tc.Equals, pps)
	c.Assert(f.snapProxySettings, tc.Equals, sps)
	c.Assert(f.packageMirror, tc.Equals, proxyCfg.aptMirror)
	c.Assert(f.addUpdateScripts, tc.Equals, upd)
	c.Assert(f.addUpgradeScripts, tc.Equals, upg)
	c.Assert(f.calledAddReq, tc.Equals, true)

	f = &fakeCfg{}
	upd, upg = false, false
	err = addPackageCommandsCommon(f, proxyCfg, upd, upg)
	c.Assert(err, tc.IsNil)
	c.Assert(f.packageProxySettings, tc.Equals, pps)
	c.Assert(f.snapProxySettings, tc.Equals, sps)
	c.Assert(f.packageMirror, tc.Equals, proxyCfg.aptMirror)
	c.Assert(f.addUpdateScripts, tc.Equals, upd)
	c.Assert(f.addUpgradeScripts, tc.Equals, upg)
	c.Assert(f.calledAddReq, tc.Equals, true)

	f = &fakeCfg{}
	upd, upg = false, false
	err = addPackageCommandsCommon(f, proxyCfg, upd, upg)
	c.Assert(err, tc.IsNil)
	c.Assert(f.packageProxySettings, tc.Equals, pps)
	c.Assert(f.snapProxySettings, tc.Equals, sps)
	c.Assert(f.packageMirror, tc.Equals, proxyCfg.aptMirror)
	c.Assert(f.addUpdateScripts, tc.Equals, upd)
	c.Assert(f.addUpgradeScripts, tc.Equals, upg)
	c.Assert(f.calledAddReq, tc.Equals, true)
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
