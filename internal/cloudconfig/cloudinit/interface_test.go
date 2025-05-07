// Copyright 2011, 2013, 2015 Canonical Ltd.
// Copyright 2015 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

package cloudinit

import (
	"github.com/juju/tc"
	jc "github.com/juju/testing/checkers"
)

var _ CloudConfig = (*ubuntuCloudConfig)(nil)

type InterfaceSuite struct{}

var _ = tc.Suite(InterfaceSuite{})

func (HelperSuite) TestNewCloudConfigWithoutMACMatch(c *tc.C) {
	cfg, err := New("ubuntu", WithNetplanMACMatch(true))
	c.Assert(err, jc.ErrorIsNil)
	c.Check(cfg.(*ubuntuCloudConfig).useNetplanHWAddrMatch, jc.IsTrue)
}
