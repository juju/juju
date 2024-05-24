// Copyright 2011, 2013, 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package cloudinit

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

var _ CloudConfig = (*ubuntuCloudConfig)(nil)
var _ CloudConfig = (*centOSCloudConfig)(nil)

type InterfaceSuite struct{}

var _ = gc.Suite(InterfaceSuite{})

func (HelperSuite) TestNewCloudConfigWithoutMACMatch(c *gc.C) {
	cfg, err := New("ubuntu", WithNetplanMACMatch(true))
	c.Assert(err, jc.ErrorIsNil)
	c.Check(cfg.(*ubuntuCloudConfig).useNetplanHWAddrMatch, jc.IsTrue)
}
