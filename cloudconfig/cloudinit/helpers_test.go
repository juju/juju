package cloudinit

import (
	gc "gopkg.in/check.v1"

	"github.com/juju/utils/proxy"
)

type HelperSuite struct{}

var _ = gc.Suite(HelperSuite{})

type fakeCfg struct {
	CloudConfig
	packageProxySettings proxy.Settings
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
func (f *fakeCfg) updateProxySettings(s proxy.Settings) {
	f.packageProxySettings = s
}

func (HelperSuite) TestAddPkgCmdsCommon(c *gc.C) {
	f := &fakeCfg{}

	pps := proxy.Settings{
		Http:    "http",
		Https:   "https",
		Ftp:     "ftp",
		NoProxy: "noproxy",
	}
	mirror := "mirror"
	upd, upg := true, true

	addPackageCommandsCommon(f, pps, mirror, upd, upg, "trusty")
	c.Assert(f.packageProxySettings, gc.Equals, pps)
	c.Assert(f.packageMirror, gc.Equals, mirror)
	c.Assert(f.addUpdateScripts, gc.Equals, upd)
	c.Assert(f.addUpgradeScripts, gc.Equals, upg)
	c.Assert(f.calledAddReq, gc.Equals, true)

	f = &fakeCfg{}
	upd, upg = false, false
	addPackageCommandsCommon(f, pps, mirror, upd, upg, "trusty")
	c.Assert(f.packageProxySettings, gc.Equals, pps)
	c.Assert(f.packageMirror, gc.Equals, mirror)
	c.Assert(f.addUpdateScripts, gc.Equals, upd)
	c.Assert(f.addUpgradeScripts, gc.Equals, upg)
	c.Assert(f.calledAddReq, gc.Equals, true)

	f = &fakeCfg{}
	upd, upg = false, false
	addPackageCommandsCommon(f, pps, mirror, upd, upg, "precise")
	c.Assert(f.packageProxySettings, gc.Equals, pps)
	c.Assert(f.packageMirror, gc.Equals, mirror)
	// for precise we need to override addUpdateScripts to always be true
	c.Assert(f.addUpdateScripts, gc.Equals, true)
	c.Assert(f.addUpgradeScripts, gc.Equals, upg)
	c.Assert(f.calledAddReq, gc.Equals, true)
}
