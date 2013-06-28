// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

import (
	. "launchpad.net/gocheck"
	"launchpad.net/gwacl"
	"launchpad.net/juju-core/instance"
)

type InstanceSuite struct{}

var _ = Suite(new(InstanceSuite))

var deploymentName = "deploymentName"

var deploymentFQDN = deploymentName + ".cloudapp.net"

var testDeployment = gwacl.Deployment{
	Name: deploymentName,
	URL:  "http://" + deploymentFQDN,
}

func (StorageSuite) TestId(c *C) {
	azInstance := azureInstance{deployment: testDeployment}
	c.Check(azInstance.Id(), Equals, instance.Id(deploymentName))
}

func (StorageSuite) TestDNSName(c *C) {
	azInstance := azureInstance{deployment: testDeployment}
	dnsName, err := azInstance.DNSName()
	c.Assert(err, IsNil)
	c.Check(dnsName, Equals, deploymentFQDN)
}

func (StorageSuite) TestWaitDNSName(c *C) {
	azInstance := azureInstance{deployment: testDeployment}
	dnsName, err := azInstance.WaitDNSName()
	c.Assert(err, IsNil)
	c.Check(dnsName, Equals, deploymentFQDN)
}
