// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure_test

import (
	"net/http"
	"path"

	"github.com/Azure/azure-sdk-for-go/Godeps/_workspace/src/github.com/Azure/go-autorest/autorest/mocks"
	"github.com/Azure/azure-sdk-for-go/Godeps/_workspace/src/github.com/Azure/go-autorest/autorest/to"
	"github.com/Azure/azure-sdk-for-go/arm/compute"
	"github.com/Azure/azure-sdk-for-go/arm/network"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/instance"
	jujunetwork "github.com/juju/juju/network"
	"github.com/juju/juju/provider/azure"
	"github.com/juju/juju/provider/azure/internal/azuretesting"
	"github.com/juju/juju/testing"
)

type instanceSuite struct {
	testing.BaseSuite

	provider          environs.EnvironProvider
	requests          []*http.Request
	sender            azuretesting.Senders
	env               environs.Environ
	virtualMachines   []compute.VirtualMachine
	networkInterfaces []network.Interface
	publicIPAddresses []network.PublicIPAddress
}

var _ = gc.Suite(&instanceSuite{})

func (s *instanceSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.provider, _ = newProviders(c, azure.ProviderConfig{
		Sender:           &s.sender,
		RequestInspector: requestRecorder(&s.requests),
	})
	s.env = openEnviron(c, s.provider, &s.sender)
	s.sender = nil
	s.requests = nil
	s.networkInterfaces = []network.Interface{
		makeNetworkInterface("nic-0", "machine-0"),
	}
	s.publicIPAddresses = nil
	s.virtualMachines = []compute.VirtualMachine{
		makeVirtualMachine("machine-0"),
		makeVirtualMachine("machine-1"),
	}
}

func makeVirtualMachine(name string) compute.VirtualMachine {
	return compute.VirtualMachine{
		Name: to.StringPtr(name),
		Properties: &compute.VirtualMachineProperties{
			ProvisioningState: to.StringPtr("Successful"),
		},
	}
}

func makeNetworkInterface(nicName, vmName string, ipConfigurations ...network.InterfaceIPConfiguration) network.Interface {
	tags := map[string]*string{"juju-machine-name": &vmName}
	return network.Interface{
		Name: to.StringPtr(nicName),
		Tags: &tags,
		Properties: &network.InterfacePropertiesFormat{
			IPConfigurations: &ipConfigurations,
		},
	}
}

func makeIPConfiguration(privateIPAddress string) network.InterfaceIPConfiguration {
	ipConfiguration := network.InterfaceIPConfiguration{
		Properties: &network.InterfaceIPConfigurationPropertiesFormat{},
	}
	if privateIPAddress != "" {
		ipConfiguration.Properties.PrivateIPAddress = to.StringPtr(privateIPAddress)
	}
	return ipConfiguration
}

func makePublicIPAddress(pipName, vmName, ipAddress string) network.PublicIPAddress {
	tags := map[string]*string{"juju-machine-name": &vmName}
	pip := network.PublicIPAddress{
		Name:       to.StringPtr(pipName),
		Tags:       &tags,
		Properties: &network.PublicIPAddressPropertiesFormat{},
	}
	if ipAddress != "" {
		pip.Properties.IPAddress = to.StringPtr(ipAddress)
	}
	return pip
}

func makeSecurityGroup(rules ...network.SecurityRule) network.SecurityGroup {
	return network.SecurityGroup{
		Properties: &network.SecurityGroupPropertiesFormat{
			SecurityRules: &rules,
		},
	}
}

func makeSecurityRule(name, ipAddress, ports string) network.SecurityRule {
	return network.SecurityRule{
		Name: to.StringPtr(name),
		Properties: &network.SecurityRulePropertiesFormat{
			Protocol:                 network.SecurityRuleProtocolTCP,
			DestinationAddressPrefix: to.StringPtr(ipAddress),
			DestinationPortRange:     to.StringPtr(ports),
			Access:                   network.Allow,
			Priority:                 to.IntPtr(200),
			Direction:                network.Inbound,
		},
	}
}

func (s *instanceSuite) getInstance(c *gc.C) instance.Instance {
	instances := s.getInstances(c, "machine-0")
	c.Assert(instances, gc.HasLen, 1)
	return instances[0]
}

func (s *instanceSuite) getInstances(c *gc.C, ids ...instance.Id) []instance.Instance {

	nicsSender := azuretesting.NewSenderWithValue(&network.InterfaceListResult{
		Value: &s.networkInterfaces,
	})
	nicsSender.PathPattern = ".*/networkInterfaces"

	vmsSender := azuretesting.NewSenderWithValue(&compute.VirtualMachineListResult{
		Value: &s.virtualMachines,
	})
	vmsSender.PathPattern = ".*/virtualMachines"

	pipsSender := azuretesting.NewSenderWithValue(&network.PublicIPAddressListResult{
		Value: &s.publicIPAddresses,
	})
	pipsSender.PathPattern = ".*/publicIPAddresses"

	s.sender = azuretesting.Senders{nicsSender, vmsSender, pipsSender}

	instances, err := s.env.Instances(ids)
	c.Assert(err, jc.ErrorIsNil)
	s.sender = azuretesting.Senders{}
	s.requests = nil
	return instances
}

func networkSecurityGroupSender(rules []network.SecurityRule) *azuretesting.MockSender {
	nsgSender := azuretesting.NewSenderWithValue(&network.SecurityGroup{
		Properties: &network.SecurityGroupPropertiesFormat{
			SecurityRules: &rules,
		},
	})
	nsgSender.PathPattern = ".*/networkSecurityGroups/juju-internal"
	return nsgSender
}

func (s *instanceSuite) TestInstanceStatus(c *gc.C) {
	inst := s.getInstance(c)
	c.Assert(inst.Status(), gc.Equals, "Successful")
}

func (s *instanceSuite) TestInstanceStatusNilProvisioningState(c *gc.C) {
	s.virtualMachines[0].Properties.ProvisioningState = nil
	inst := s.getInstance(c)
	c.Assert(inst.Status(), gc.Equals, "")
}

func (s *instanceSuite) TestInstanceStatusNoVM(c *gc.C) {
	// Instances will still return an instance if there's a NIC, which is
	// the last thing we delete. If there's no VM, we return the string
	// "Partially Deleted" from Instance.Status().
	s.virtualMachines = nil
	inst := s.getInstance(c)
	c.Assert(inst.Status(), gc.Equals, "Partially Deleted")
}

func (s *instanceSuite) TestInstanceAddressesEmpty(c *gc.C) {
	addresses, err := s.getInstance(c).Addresses()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(addresses, gc.HasLen, 0)
}

func (s *instanceSuite) TestInstanceAddresses(c *gc.C) {
	nic0IPConfigurations := []network.InterfaceIPConfiguration{
		makeIPConfiguration("10.0.0.4"),
		makeIPConfiguration("10.0.0.5"),
	}
	nic1IPConfigurations := []network.InterfaceIPConfiguration{
		makeIPConfiguration(""),
	}
	s.networkInterfaces = []network.Interface{
		makeNetworkInterface("nic-0", "machine-0", nic0IPConfigurations...),
		makeNetworkInterface("nic-1", "machine-0", nic1IPConfigurations...),
		makeNetworkInterface("nic-2", "machine-0"),
		// unrelated NIC
		makeNetworkInterface("nic-3", "machine-1"),
	}
	s.publicIPAddresses = []network.PublicIPAddress{
		makePublicIPAddress("pip-0", "machine-0", "1.2.3.4"),
		makePublicIPAddress("pip-1", "machine-0", "1.2.3.5"),
		// unrelated PIP
		makePublicIPAddress("pip-2", "machine-1", "1.2.3.6"),
	}
	addresses, err := s.getInstance(c).Addresses()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(addresses, jc.DeepEquals, jujunetwork.NewAddresses(
		"10.0.0.4", "10.0.0.5", "1.2.3.4", "1.2.3.5",
	))
}

func (s *instanceSuite) TestMultipleInstanceAddresses(c *gc.C) {
	nic0IPConfiguration := makeIPConfiguration("10.0.0.4")
	nic1IPConfiguration := makeIPConfiguration("10.0.0.5")
	s.networkInterfaces = []network.Interface{
		makeNetworkInterface("nic-0", "machine-0", nic0IPConfiguration),
		makeNetworkInterface("nic-1", "machine-1", nic1IPConfiguration),
	}
	s.publicIPAddresses = []network.PublicIPAddress{
		makePublicIPAddress("pip-0", "machine-0", "1.2.3.4"),
		makePublicIPAddress("pip-1", "machine-1", "1.2.3.5"),
	}
	instances := s.getInstances(c, "machine-0", "machine-1")
	c.Assert(instances, gc.HasLen, 2)

	inst0Addresses, err := instances[0].Addresses()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(inst0Addresses, jc.DeepEquals, jujunetwork.NewAddresses(
		"10.0.0.4", "1.2.3.4",
	))

	inst1Addresses, err := instances[1].Addresses()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(inst1Addresses, jc.DeepEquals, jujunetwork.NewAddresses(
		"10.0.0.5", "1.2.3.5",
	))
}

func (s *instanceSuite) TestInstancePortsEmpty(c *gc.C) {
	inst := s.getInstance(c)
	nsgSender := networkSecurityGroupSender(nil)
	s.sender = azuretesting.Senders{nsgSender}
	ports, err := inst.Ports("0")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ports, gc.HasLen, 0)
}

func (s *instanceSuite) TestInstancePorts(c *gc.C) {
	inst := s.getInstance(c)
	nsgSender := networkSecurityGroupSender([]network.SecurityRule{{
		Name: to.StringPtr("machine-0-xyzzy"),
		Properties: &network.SecurityRulePropertiesFormat{
			Protocol:             network.SecurityRuleProtocolUDP,
			DestinationPortRange: to.StringPtr("*"),
			Access:               network.Allow,
			Priority:             to.IntPtr(200),
			Direction:            network.Inbound,
		},
	}, {
		Name: to.StringPtr("machine-0-tcpcp"),
		Properties: &network.SecurityRulePropertiesFormat{
			Protocol:             network.SecurityRuleProtocolTCP,
			DestinationPortRange: to.StringPtr("1000-2000"),
			Access:               network.Allow,
			Priority:             to.IntPtr(201),
			Direction:            network.Inbound,
		},
	}, {
		Name: to.StringPtr("machine-0-http"),
		Properties: &network.SecurityRulePropertiesFormat{
			Protocol:             network.SecurityRuleProtocolAsterisk,
			DestinationPortRange: to.StringPtr("80"),
			Access:               network.Allow,
			Priority:             to.IntPtr(202),
			Direction:            network.Inbound,
		},
	}, {
		Name: to.StringPtr("machine-00-ignored"),
		Properties: &network.SecurityRulePropertiesFormat{
			Protocol:             network.SecurityRuleProtocolTCP,
			DestinationPortRange: to.StringPtr("80"),
			Access:               network.Allow,
			Priority:             to.IntPtr(202),
			Direction:            network.Inbound,
		},
	}, {
		Name: to.StringPtr("machine-0-ignored"),
		Properties: &network.SecurityRulePropertiesFormat{
			Protocol:             network.SecurityRuleProtocolTCP,
			DestinationPortRange: to.StringPtr("80"),
			Access:               network.Deny,
			Priority:             to.IntPtr(202),
			Direction:            network.Inbound,
		},
	}, {
		Name: to.StringPtr("machine-0-ignored"),
		Properties: &network.SecurityRulePropertiesFormat{
			Protocol:             network.SecurityRuleProtocolTCP,
			DestinationPortRange: to.StringPtr("80"),
			Access:               network.Allow,
			Priority:             to.IntPtr(202),
			Direction:            network.Outbound,
		},
	}, {
		Name: to.StringPtr("machine-0-ignored"),
		Properties: &network.SecurityRulePropertiesFormat{
			Protocol:             network.SecurityRuleProtocolTCP,
			DestinationPortRange: to.StringPtr("80"),
			Access:               network.Allow,
			Priority:             to.IntPtr(199), // internal range
			Direction:            network.Inbound,
		},
	}})
	s.sender = azuretesting.Senders{nsgSender}

	ports, err := inst.Ports("0")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(ports, jc.DeepEquals, []jujunetwork.PortRange{{
		FromPort: 0,
		ToPort:   65535,
		Protocol: "udp",
	}, {
		FromPort: 1000,
		ToPort:   2000,
		Protocol: "tcp",
	}, {
		FromPort: 80,
		ToPort:   80,
		Protocol: "tcp",
	}, {
		FromPort: 80,
		ToPort:   80,
		Protocol: "udp",
	}})
}

func (s *instanceSuite) TestInstanceClosePorts(c *gc.C) {
	inst := s.getInstance(c)
	sender := mocks.NewSender()
	notFoundSender := mocks.NewSender()
	notFoundSender.EmitStatus("rule not found", http.StatusNotFound)
	s.sender = azuretesting.Senders{sender, notFoundSender}

	err := inst.ClosePorts("0", []jujunetwork.PortRange{{
		Protocol: "tcp",
		FromPort: 1000,
		ToPort:   1000,
	}, {
		Protocol: "udp",
		FromPort: 1000,
		ToPort:   2000,
	}})
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(s.requests, gc.HasLen, 2)
	c.Assert(s.requests[0].Method, gc.Equals, "DELETE")
	c.Assert(s.requests[0].URL.Path, gc.Equals, securityRulePath("machine-0-tcp-1000"))
	c.Assert(s.requests[1].Method, gc.Equals, "DELETE")
	c.Assert(s.requests[1].URL.Path, gc.Equals, securityRulePath("machine-0-udp-1000-2000"))
}

func (s *instanceSuite) TestInstanceOpenPorts(c *gc.C) {
	internalSubnetId := path.Join(
		"/subscriptions", fakeSubscriptionId,
		"resourceGroups/arbitrary/providers/Microsoft.Network/virtualnetworks/juju-internal/subnets",
		"juju-testenv-model-"+testing.ModelTag.Id(),
	)
	ipConfiguration := network.InterfaceIPConfiguration{
		Properties: &network.InterfaceIPConfigurationPropertiesFormat{
			PrivateIPAddress: to.StringPtr("10.0.0.4"),
			Subnet: &network.SubResource{
				ID: to.StringPtr(internalSubnetId),
			},
		},
	}
	s.networkInterfaces = []network.Interface{
		makeNetworkInterface("nic-0", "machine-0", ipConfiguration),
	}

	inst := s.getInstance(c)
	okSender := mocks.NewSender()
	okSender.EmitContent("{}")
	nsgSender := networkSecurityGroupSender(nil)
	s.sender = azuretesting.Senders{nsgSender, okSender, okSender}

	err := inst.OpenPorts("0", []jujunetwork.PortRange{{
		Protocol: "tcp",
		FromPort: 1000,
		ToPort:   1000,
	}, {
		Protocol: "udp",
		FromPort: 1000,
		ToPort:   2000,
	}})
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(s.requests, gc.HasLen, 3)
	c.Assert(s.requests[0].Method, gc.Equals, "GET")
	c.Assert(s.requests[0].URL.Path, gc.Equals, internalSecurityGroupPath)
	c.Assert(s.requests[1].Method, gc.Equals, "PUT")
	c.Assert(s.requests[1].URL.Path, gc.Equals, securityRulePath("machine-0-tcp-1000"))
	assertRequestBody(c, s.requests[1], &network.SecurityRule{
		Properties: &network.SecurityRulePropertiesFormat{
			Description:              to.StringPtr("1000/tcp"),
			Protocol:                 network.SecurityRuleProtocolTCP,
			SourcePortRange:          to.StringPtr("*"),
			SourceAddressPrefix:      to.StringPtr("*"),
			DestinationPortRange:     to.StringPtr("1000"),
			DestinationAddressPrefix: to.StringPtr("10.0.0.4"),
			Access:    network.Allow,
			Priority:  to.IntPtr(200),
			Direction: network.Inbound,
		},
	})
	c.Assert(s.requests[2].Method, gc.Equals, "PUT")
	c.Assert(s.requests[2].URL.Path, gc.Equals, securityRulePath("machine-0-udp-1000-2000"))
	assertRequestBody(c, s.requests[2], &network.SecurityRule{
		Properties: &network.SecurityRulePropertiesFormat{
			Description:              to.StringPtr("1000-2000/udp"),
			Protocol:                 network.SecurityRuleProtocolUDP,
			SourcePortRange:          to.StringPtr("*"),
			SourceAddressPrefix:      to.StringPtr("*"),
			DestinationPortRange:     to.StringPtr("1000-2000"),
			DestinationAddressPrefix: to.StringPtr("10.0.0.4"),
			Access:    network.Allow,
			Priority:  to.IntPtr(201),
			Direction: network.Inbound,
		},
	})
}

func (s *instanceSuite) TestInstanceOpenPortsAlreadyOpen(c *gc.C) {
	internalSubnetId := path.Join(
		"/subscriptions", fakeSubscriptionId,
		"resourceGroups/arbitrary/providers/Microsoft.Network/virtualnetworks/juju-internal/subnets",
		"juju-testenv-model-"+testing.ModelTag.Id(),
	)
	ipConfiguration := network.InterfaceIPConfiguration{
		Properties: &network.InterfaceIPConfigurationPropertiesFormat{
			PrivateIPAddress: to.StringPtr("10.0.0.4"),
			Subnet: &network.SubResource{
				ID: to.StringPtr(internalSubnetId),
			},
		},
	}
	s.networkInterfaces = []network.Interface{
		makeNetworkInterface("nic-0", "machine-0", ipConfiguration),
	}

	inst := s.getInstance(c)
	okSender := mocks.NewSender()
	okSender.EmitContent("{}")
	nsgSender := networkSecurityGroupSender([]network.SecurityRule{{
		Name: to.StringPtr("machine-0-tcp-1000"),
		Properties: &network.SecurityRulePropertiesFormat{
			Protocol:             network.SecurityRuleProtocolAsterisk,
			DestinationPortRange: to.StringPtr("1000"),
			Access:               network.Allow,
			Priority:             to.IntPtr(202),
			Direction:            network.Inbound,
		},
	}})
	s.sender = azuretesting.Senders{nsgSender, okSender, okSender}

	err := inst.OpenPorts("0", []jujunetwork.PortRange{{
		Protocol: "tcp",
		FromPort: 1000,
		ToPort:   1000,
	}, {
		Protocol: "udp",
		FromPort: 1000,
		ToPort:   2000,
	}})
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(s.requests, gc.HasLen, 2)
	c.Assert(s.requests[0].Method, gc.Equals, "GET")
	c.Assert(s.requests[0].URL.Path, gc.Equals, internalSecurityGroupPath)
	c.Assert(s.requests[1].Method, gc.Equals, "PUT")
	c.Assert(s.requests[1].URL.Path, gc.Equals, securityRulePath("machine-0-udp-1000-2000"))
	assertRequestBody(c, s.requests[1], &network.SecurityRule{
		Properties: &network.SecurityRulePropertiesFormat{
			Description:              to.StringPtr("1000-2000/udp"),
			Protocol:                 network.SecurityRuleProtocolUDP,
			SourcePortRange:          to.StringPtr("*"),
			SourceAddressPrefix:      to.StringPtr("*"),
			DestinationPortRange:     to.StringPtr("1000-2000"),
			DestinationAddressPrefix: to.StringPtr("10.0.0.4"),
			Access:    network.Allow,
			Priority:  to.IntPtr(200),
			Direction: network.Inbound,
		},
	})
}

func (s *instanceSuite) TestInstanceOpenPortsNoInternalAddress(c *gc.C) {
	err := s.getInstance(c).OpenPorts("0", nil)
	c.Assert(err, gc.ErrorMatches, "internal network address not found")
}

var internalSecurityGroupPath = path.Join(
	"/subscriptions", fakeSubscriptionId,
	"resourceGroups", "juju-testenv-model-"+testing.ModelTag.Id(),
	"providers/Microsoft.Network/networkSecurityGroups/juju-internal",
)

func securityRulePath(ruleName string) string {
	return path.Join(internalSecurityGroupPath, "securityRules", ruleName)
}
