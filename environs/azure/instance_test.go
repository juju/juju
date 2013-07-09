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
	// An instance's DNS name is taken from its Hosted Service label.
	host := fmt.Sprintf("hostname.%s", AZURE_DOMAIN_NAME)
	testService := makeHostedServiceDescriptor("service-name", host)
	azInstance := azureInstance{*testService}
	dnsName, err := azInstance.DNSName()
	c.Assert(err, IsNil)
	c.Check(dnsName, Equals, host)
}

func (StorageSuite) TestDNSNameReturnsErrNoDNSNameIfNotDNSName(c *C) {
	// While a Hosted Service is waiting for the provider to register its
	// DNS name, it still has a provisional label.  DNSName recognizes
	// this as meaning "no DNS name yet."
	label := makeProvisionalServiceLabel("foo")
	testService := makeHostedServiceDescriptor("service-name", label)
	azInstance := azureInstance{*testService}
	dnsName, err := azInstance.DNSName()
	c.Check(err, Equals, instance.ErrNoDNSName)
	c.Check(dnsName, Equals, "")
}

func (StorageSuite) TestWaitDNSName(c *C) {
	host := fmt.Sprintf("hostname.%s", AZURE_DOMAIN_NAME)
	testService := makeHostedServiceDescriptor("service-name", host)
	azInstance := azureInstance{*testService}
	dnsName, err := azInstance.WaitDNSName()
	c.Assert(err, IsNil)
	c.Check(dnsName, Equals, host)
}
