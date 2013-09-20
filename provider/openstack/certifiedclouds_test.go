// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package openstack_test

import (
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/provider/openstack"
	"launchpad.net/juju-core/testing/testbase"
	jc "launchpad.net/juju-core/testing/checkers"
)

type CertifiedCloudsSuite struct {
	testbase.LoggingSuite
}

var _ = gc.Suite(&CertifiedCloudsSuite{})

func (s *CertifiedCloudsSuite) TestUnknownCloud(c *gc.C) {
	_, ok := openstack.GetCertifiedToolsURL("https://some-url")
	c.Assert(ok, jc.IsFalse)
}

func (s *CertifiedCloudsSuite) TestHPCloud(c *gc.C) {
	expected := "https://region-a.geo-1.objects.hpcloudsvc.com:443/v1/60502529753910/juju-dist/tools"
	authURL := "https://region-a.geo-1.identity.hpcloudsvc.com:35357/v2.0"
	toolsURL, ok := openstack.GetCertifiedToolsURL(authURL)
	c.Assert(ok, jc.IsTrue)
	c.Assert(toolsURL, gc.Equals, expected)
	authURL = authURL + "/"
	toolsURL, ok = openstack.GetCertifiedToolsURL(authURL)
	c.Assert(ok, jc.IsTrue)
	c.Assert(toolsURL, gc.Equals, expected)
}
