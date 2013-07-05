// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

import (
	"encoding/base64"
	"fmt"
	. "launchpad.net/gocheck"
	"launchpad.net/gwacl"
	"launchpad.net/juju-core/instance"
)

type InstanceSuite struct{}

var _ = Suite(new(InstanceSuite))

func makeHostedServiceDescriptor(name, label string) *gwacl.HostedServiceDescriptor {
	labelBase64 := base64.StdEncoding.EncodeToString([]byte(label))
	return &gwacl.HostedServiceDescriptor{ServiceName: name, Label: labelBase64}
}

func (StorageSuite) TestId(c *C) {
	serviceName := "test-name"
	testService := makeHostedServiceDescriptor(serviceName, "label")
	azInstance := azureInstance{*testService}
	c.Check(azInstance.Id(), Equals, instance.Id(serviceName))
}

func (StorageSuite) TestDNSName(c *C) {
	label := "hostname"
	expectedDNSName := fmt.Sprintf("%s.%s", label, AZURE_DOMAIN_NAME)
	testService := makeHostedServiceDescriptor("service-name", label)
	azInstance := azureInstance{*testService}
	dnsName, err := azInstance.DNSName()
	c.Assert(err, IsNil)
	c.Check(dnsName, Equals, expectedDNSName)
}

func (StorageSuite) TestWaitDNSName(c *C) {
	label := "hostname"
	expectedDNSName := fmt.Sprintf("%s.%s", label, AZURE_DOMAIN_NAME)
	testService := makeHostedServiceDescriptor("service-name", label)
	azInstance := azureInstance{*testService}
	dnsName, err := azInstance.WaitDNSName()
	c.Assert(err, IsNil)
	c.Check(dnsName, Equals, expectedDNSName)
}
