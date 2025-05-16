// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure_test

import (
	"context"
	"net/http"
	"path"
	stdtesting "testing"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute/v2"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/network/armnetwork"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources"
	"github.com/juju/tc"

	"github.com/juju/juju/core/instance"
	corenetwork "github.com/juju/juju/core/network"
	"github.com/juju/juju/core/network/firewall"
	"github.com/juju/juju/core/status"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/instances"
	"github.com/juju/juju/internal/provider/azure"
	"github.com/juju/juju/internal/provider/azure/internal/azuretesting"
	"github.com/juju/juju/internal/testing"
)

type instanceSuite struct {
	testing.BaseSuite

	provider          environs.EnvironProvider
	requests          []*http.Request
	sender            azuretesting.Senders
	env               environs.Environ
	deployments       []*armresources.DeploymentExtended
	vms               []*armcompute.VirtualMachine
	networkInterfaces []*armnetwork.Interface
	publicIPAddresses []*armnetwork.PublicIPAddress

	credentialInvalidator environs.CredentialInvalidator
	invalidatedCredential bool
}

func TestInstanceSuite(t *stdtesting.T) { tc.Run(t, &instanceSuite{}) }
func (s *instanceSuite) SetUpTest(c *tc.C) {
	s.BaseSuite.SetUpTest(c)
	s.provider = newProvider(c, azure.ProviderConfig{
		Sender:           &s.sender,
		RequestInspector: &azuretesting.RequestRecorderPolicy{Requests: &s.requests},
		CreateTokenCredential: func(appId, appPassword, tenantID string, opts azcore.ClientOptions) (azcore.TokenCredential, error) {
			return &azuretesting.FakeCredential{}, nil
		},
	})
	s.env = openEnviron(c, s.provider, s.credentialInvalidator, &s.sender)
	s.sender = nil
	s.requests = nil
	s.networkInterfaces = []*armnetwork.Interface{
		makeNetworkInterface("nic-0", "machine-0"),
	}
	s.publicIPAddresses = nil
	s.deployments = []*armresources.DeploymentExtended{
		makeDeployment("machine-0", armresources.ProvisioningStateSucceeded),
		makeDeployment("machine-1", armresources.ProvisioningStateCreating),
	}
	s.vms = []*armcompute.VirtualMachine{{
		Name: to.Ptr("machine-0"),
		Tags: map[string]*string{
			"juju-controller-uuid": to.Ptr(testing.ControllerTag.Id()),
			"juju-model-uuid":      to.Ptr(testing.ModelTag.Id()),
			"juju-is-controller":   to.Ptr("true"),
		},
		Properties: &armcompute.VirtualMachineProperties{
			ProvisioningState: to.Ptr("Succeeded")},
	}}
	s.credentialInvalidator = azure.CredentialInvalidator(func(context.Context, environs.CredentialInvalidReason) error {
		s.invalidatedCredential = true
		return nil
	})
}

func makeDeployment(name string, provisioningState armresources.ProvisioningState) *armresources.DeploymentExtended {
	dependsOn := []*armresources.BasicDependency{{
		ResourceType: to.Ptr("Microsoft.Compute/availabilitySets"),
		ResourceName: to.Ptr("mysql"),
	}}
	dependencies := []*armresources.Dependency{{
		ResourceType: to.Ptr("Microsoft.Compute/virtualMachines"),
		DependsOn:    dependsOn,
	}}
	return &armresources.DeploymentExtended{
		Name: to.Ptr(name),
		Properties: &armresources.DeploymentPropertiesExtended{
			ProvisioningState: to.Ptr(provisioningState),
			Dependencies:      dependencies,
		},
		Tags: map[string]*string{
			"juju-model-uuid": to.Ptr(testing.ModelTag.Id()),
		},
	}
}

func makeNetworkInterface(nicName, vmName string, ipConfigurations ...*armnetwork.InterfaceIPConfiguration) *armnetwork.Interface {
	tags := map[string]*string{"juju-machine-name": &vmName}
	return &armnetwork.Interface{
		Name: to.Ptr(nicName),
		Tags: tags,
		Properties: &armnetwork.InterfacePropertiesFormat{
			IPConfigurations: ipConfigurations,
			Primary:          to.Ptr(true),
		},
	}
}

func makeIPConfiguration(privateIPAddress string) *armnetwork.InterfaceIPConfiguration {
	ipConfiguration := &armnetwork.InterfaceIPConfiguration{
		Properties: &armnetwork.InterfaceIPConfigurationPropertiesFormat{},
	}
	if privateIPAddress != "" {
		ipConfiguration.Properties.PrivateIPAddress = to.Ptr(privateIPAddress)
	}
	return ipConfiguration
}

func makePublicIPAddress(pipName, vmName, ipAddress string) *armnetwork.PublicIPAddress {
	tags := map[string]*string{"juju-machine-name": &vmName}
	pip := &armnetwork.PublicIPAddress{
		Name:       to.Ptr(pipName),
		Tags:       tags,
		Properties: &armnetwork.PublicIPAddressPropertiesFormat{},
	}
	if ipAddress != "" {
		pip.Properties.IPAddress = to.Ptr(ipAddress)
	}
	return pip
}

func makeSecurityGroup(rules ...*armnetwork.SecurityRule) armnetwork.SecurityGroup {
	return armnetwork.SecurityGroup{
		Name: to.Ptr("juju-internal-nsg"),
		ID:   to.Ptr(internalSecurityGroupPath),
		Properties: &armnetwork.SecurityGroupPropertiesFormat{
			SecurityRules: rules,
		},
	}
}

func makeSecurityRule(name, ipAddress, ports string) *armnetwork.SecurityRule {
	return &armnetwork.SecurityRule{
		Name: to.Ptr(name),
		Properties: &armnetwork.SecurityRulePropertiesFormat{
			Protocol:                 to.Ptr(armnetwork.SecurityRuleProtocolTCP),
			DestinationAddressPrefix: to.Ptr(ipAddress),
			DestinationPortRange:     to.Ptr(ports),
			Access:                   to.Ptr(armnetwork.SecurityRuleAccessAllow),
			Priority:                 to.Ptr(int32(200)),
			Direction:                to.Ptr(armnetwork.SecurityRuleDirectionInbound),
		},
	}
}

func (s *instanceSuite) getInstance(c *tc.C, instID instance.Id) instances.Instance {
	instances := s.getInstances(c, instID)
	c.Assert(instances, tc.HasLen, 1)
	return instances[0]
}

func (s *instanceSuite) getInstances(c *tc.C, ids ...instance.Id) []instances.Instance {
	s.sender = s.getInstancesSender()
	instances, err := s.env.Instances(c.Context(), ids)
	c.Assert(err, tc.ErrorIsNil)
	s.sender = azuretesting.Senders{}
	s.requests = nil
	return instances
}

func (s *instanceSuite) getInstancesSender() azuretesting.Senders {
	deploymentsSender := azuretesting.NewSenderWithValue(&armresources.DeploymentListResult{
		Value: s.deployments,
	})
	deploymentsSender.PathPattern = ".*/deployments"
	vmSender := azuretesting.NewSenderWithValue(&armcompute.VirtualMachineListResult{
		Value: s.vms,
	})
	vmSender.PathPattern = ".*/virtualMachines"
	nicsSender := azuretesting.NewSenderWithValue(&armnetwork.InterfaceListResult{
		Value: s.networkInterfaces,
	})
	nicsSender.PathPattern = ".*/networkInterfaces"
	pipsSender := azuretesting.NewSenderWithValue(&armnetwork.PublicIPAddressListResult{
		Value: s.publicIPAddresses,
	})
	pipsSender.PathPattern = ".*/publicIPAddresses"
	return azuretesting.Senders{deploymentsSender, vmSender, nicsSender, pipsSender}
}

func networkSecurityGroupSender(rules []*armnetwork.SecurityRule) *azuretesting.MockSender {
	nsgSender := azuretesting.NewSenderWithValue(&armnetwork.SecurityGroup{
		Properties: &armnetwork.SecurityGroupPropertiesFormat{
			SecurityRules: rules,
		},
	})
	nsgSender.PathPattern = ".*/networkSecurityGroups/juju-internal-nsg"
	return nsgSender
}

func (s *instanceSuite) TestInstanceStatus(c *tc.C) {
	inst := s.getInstance(c, "machine-0")
	assertInstanceStatus(c, inst.Status(c.Context()), status.Running, "")
}

func (s *instanceSuite) TestInstanceStatusDeploying(c *tc.C) {
	s.deployments[1].Properties.ProvisioningState = to.Ptr(armresources.ProvisioningStateCreating)
	inst := s.getInstance(c, "machine-1")
	assertInstanceStatus(c, inst.Status(c.Context()), status.Provisioning, "")
}

func (s *instanceSuite) TestInstanceStatusDeploymentFailed(c *tc.C) {
	s.deployments[1].Properties.ProvisioningState = to.Ptr(armresources.ProvisioningStateFailed)
	s.deployments[1].Properties.Error = &armresources.ErrorResponse{
		Details: []*armresources.ErrorResponse{{
			Message: to.Ptr("boom"),
		}},
	}
	inst := s.getInstance(c, "machine-1")
	assertInstanceStatus(c, inst.Status(c.Context()), status.ProvisioningError, "boom")
}

func (s *instanceSuite) TestInstanceStatusDeploymentCanceled(c *tc.C) {
	s.deployments[1].Properties.ProvisioningState = to.Ptr(armresources.ProvisioningStateCanceled)
	inst := s.getInstance(c, "machine-1")
	assertInstanceStatus(c, inst.Status(c.Context()), status.ProvisioningError, "Canceled")
}

func (s *instanceSuite) TestInstanceStatusUnsetProvisioningState(c *tc.C) {
	s.deployments[1].Properties.ProvisioningState = to.Ptr(armresources.ProvisioningStateNotSpecified)
	inst := s.getInstance(c, "machine-1")
	assertInstanceStatus(c, inst.Status(c.Context()), status.Allocating, "")
}

func assertInstanceStatus(c *tc.C, actual instance.Status, status status.Status, message string) {
	c.Assert(actual, tc.DeepEquals, instance.Status{
		Status:  status,
		Message: message,
	})
}

func (s *instanceSuite) TestInstanceAddressesEmpty(c *tc.C) {
	addresses, err := s.getInstance(c, "machine-0").Addresses(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(addresses, tc.HasLen, 0)
}

func (s *instanceSuite) TestInstanceAddresses(c *tc.C) {
	nic0IPConfigurations := []*armnetwork.InterfaceIPConfiguration{
		makeIPConfiguration("10.0.0.4"),
		makeIPConfiguration("10.0.0.5"),
	}
	nic1IPConfigurations := []*armnetwork.InterfaceIPConfiguration{
		makeIPConfiguration(""),
	}
	s.networkInterfaces = []*armnetwork.Interface{
		makeNetworkInterface("nic-0", "machine-0", nic0IPConfigurations...),
		makeNetworkInterface("nic-1", "machine-0", nic1IPConfigurations...),
		makeNetworkInterface("nic-2", "machine-0"),
		// unrelated NIC
		makeNetworkInterface("nic-3", "machine-1"),
	}
	s.publicIPAddresses = []*armnetwork.PublicIPAddress{
		makePublicIPAddress("pip-0", "machine-0", "1.2.3.4"),
		makePublicIPAddress("pip-1", "machine-0", "1.2.3.5"),
		// unrelated PIP
		makePublicIPAddress("pip-2", "machine-1", "1.2.3.6"),
	}
	addresses, err := s.getInstance(c, "machine-0").Addresses(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(addresses, tc.DeepEquals, corenetwork.NewMachineAddresses([]string{
		"10.0.0.4", "10.0.0.5", "1.2.3.4", "1.2.3.5",
	}).AsProviderAddresses())
}

func (s *instanceSuite) TestMultipleInstanceAddresses(c *tc.C) {
	nic0IPConfiguration := makeIPConfiguration("10.0.0.4")
	nic1IPConfiguration := makeIPConfiguration("10.0.0.5")
	s.networkInterfaces = []*armnetwork.Interface{
		makeNetworkInterface("nic-0", "machine-0", nic0IPConfiguration),
		makeNetworkInterface("nic-1", "machine-1", nic1IPConfiguration),
	}
	s.publicIPAddresses = []*armnetwork.PublicIPAddress{
		makePublicIPAddress("pip-0", "machine-0", "1.2.3.4"),
		makePublicIPAddress("pip-1", "machine-1", "1.2.3.5"),
	}
	instances := s.getInstances(c, "machine-0", "machine-1")
	c.Assert(instances, tc.HasLen, 2)

	inst0Addresses, err := instances[0].Addresses(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(inst0Addresses, tc.DeepEquals, corenetwork.NewMachineAddresses([]string{
		"10.0.0.4", "1.2.3.4",
	}).AsProviderAddresses())

	inst1Addresses, err := instances[1].Addresses(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(inst1Addresses, tc.DeepEquals, corenetwork.NewMachineAddresses([]string{
		"10.0.0.5", "1.2.3.5",
	}).AsProviderAddresses())
}

func (s *instanceSuite) TestIngressRulesEmpty(c *tc.C) {
	inst := s.getInstance(c, "machine-0")
	fwInst, ok := inst.(instances.InstanceFirewaller)
	c.Assert(ok, tc.Equals, true)
	nsgSender := networkSecurityGroupSender(nil)
	s.sender = azuretesting.Senders{nsgSender}
	rules, err := fwInst.IngressRules(c.Context(), "0")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(rules, tc.HasLen, 0)
}

func (s *instanceSuite) setupSecurityGroupRules(nsgRules ...*armnetwork.SecurityRule) *azuretesting.Senders {
	nsg := &armnetwork.SecurityGroup{
		ID:   &internalSecurityGroupPath,
		Name: to.Ptr("juju-internal-nsg"),
		Properties: &armnetwork.SecurityGroupPropertiesFormat{
			SecurityRules: nsgRules,
		},
	}
	nic0IPConfigurations := []*armnetwork.InterfaceIPConfiguration{
		makeIPConfiguration("10.0.0.4"),
	}
	nic0IPConfigurations[0].Properties.Primary = to.Ptr(true)
	nic0IPConfigurations[0].Properties.Subnet = &armnetwork.Subnet{
		ID: &internalSubnetPath,
		Properties: &armnetwork.SubnetPropertiesFormat{
			NetworkSecurityGroup: nsg,
		},
	}
	s.networkInterfaces = []*armnetwork.Interface{
		makeNetworkInterface("nic-0", "machine-0", nic0IPConfigurations...),
		makeNetworkInterface("nic-2", "machine-0"),
		// unrelated NIC
		makeNetworkInterface("nic-3", "machine-1"),
	}
	return &azuretesting.Senders{
		makeSender(internalSubnetPath, nic0IPConfigurations[0].Properties.Subnet), // GET: subnets to get security group
		networkSecurityGroupSender(nsgRules),
	}
}

func (s *instanceSuite) TestIngressRules(c *tc.C) {
	nsgRules := []*armnetwork.SecurityRule{{
		Name: to.Ptr("machine-0-xyzzy"),
		Properties: &armnetwork.SecurityRulePropertiesFormat{
			Protocol:             to.Ptr(armnetwork.SecurityRuleProtocolUDP),
			DestinationPortRange: to.Ptr("*"),
			Access:               to.Ptr(armnetwork.SecurityRuleAccessAllow),
			Priority:             to.Ptr(int32(200)),
			Direction:            to.Ptr(armnetwork.SecurityRuleDirectionInbound),
		},
	}, {
		Name: to.Ptr("machine-0-tcpcp-1"),
		Properties: &armnetwork.SecurityRulePropertiesFormat{
			Protocol:             to.Ptr(armnetwork.SecurityRuleProtocolTCP),
			DestinationPortRange: to.Ptr("1000-2000"),
			SourceAddressPrefix:  to.Ptr("*"),
			Access:               to.Ptr(armnetwork.SecurityRuleAccessAllow),
			Priority:             to.Ptr(int32(201)),
			Direction:            to.Ptr(armnetwork.SecurityRuleDirectionInbound),
		},
	}, {
		Name: to.Ptr("machine-0-tcpcp-2"),
		Properties: &armnetwork.SecurityRulePropertiesFormat{
			Protocol:             to.Ptr(armnetwork.SecurityRuleProtocolTCP),
			DestinationPortRange: to.Ptr("1000-2000"),
			SourceAddressPrefix:  to.Ptr("192.168.1.0/24"),
			Access:               to.Ptr(armnetwork.SecurityRuleAccessAllow),
			Priority:             to.Ptr(int32(201)),
			Direction:            to.Ptr(armnetwork.SecurityRuleDirectionInbound),
		},
	}, {
		Name: to.Ptr("machine-0-tcpcp-3"),
		Properties: &armnetwork.SecurityRulePropertiesFormat{
			Protocol:             to.Ptr(armnetwork.SecurityRuleProtocolTCP),
			DestinationPortRange: to.Ptr("1000-2000"),
			SourceAddressPrefix:  to.Ptr("10.0.0.0/24"),
			Access:               to.Ptr(armnetwork.SecurityRuleAccessAllow),
			Priority:             to.Ptr(int32(201)),
			Direction:            to.Ptr(armnetwork.SecurityRuleDirectionInbound),
		},
	}, {
		Name: to.Ptr("machine-0-http"),
		Properties: &armnetwork.SecurityRulePropertiesFormat{
			Protocol:             to.Ptr(armnetwork.SecurityRuleProtocolAsterisk),
			DestinationPortRange: to.Ptr("80"),
			Access:               to.Ptr(armnetwork.SecurityRuleAccessAllow),
			Priority:             to.Ptr(int32(202)),
			Direction:            to.Ptr(armnetwork.SecurityRuleDirectionInbound),
		},
	}, {
		Name: to.Ptr("machine-00-ignored"),
		Properties: &armnetwork.SecurityRulePropertiesFormat{
			Protocol:             to.Ptr(armnetwork.SecurityRuleProtocolTCP),
			DestinationPortRange: to.Ptr("80"),
			Access:               to.Ptr(armnetwork.SecurityRuleAccessAllow),
			Priority:             to.Ptr(int32(202)),
			Direction:            to.Ptr(armnetwork.SecurityRuleDirectionInbound),
		},
	}, {
		Name: to.Ptr("machine-0-ignored"),
		Properties: &armnetwork.SecurityRulePropertiesFormat{
			Protocol:             to.Ptr(armnetwork.SecurityRuleProtocolTCP),
			DestinationPortRange: to.Ptr("80"),
			Access:               to.Ptr(armnetwork.SecurityRuleAccessDeny),
			Priority:             to.Ptr(int32(202)),
			Direction:            to.Ptr(armnetwork.SecurityRuleDirectionInbound),
		},
	}, {
		Name: to.Ptr("machine-0-ignored"),
		Properties: &armnetwork.SecurityRulePropertiesFormat{
			Protocol:             to.Ptr(armnetwork.SecurityRuleProtocolTCP),
			DestinationPortRange: to.Ptr("80"),
			Access:               to.Ptr(armnetwork.SecurityRuleAccessAllow),
			Priority:             to.Ptr(int32(202)),
			Direction:            to.Ptr(armnetwork.SecurityRuleDirectionOutbound),
		},
	}, {
		Name: to.Ptr("machine-0-ignored"),
		Properties: &armnetwork.SecurityRulePropertiesFormat{
			Protocol:             to.Ptr(armnetwork.SecurityRuleProtocolTCP),
			DestinationPortRange: to.Ptr("80"),
			Access:               to.Ptr(armnetwork.SecurityRuleAccessAllow),
			Priority:             to.Ptr(int32(199)), // internal range
			Direction:            to.Ptr(armnetwork.SecurityRuleDirectionInbound),
		},
	}}
	nsgSender := s.setupSecurityGroupRules(nsgRules...)
	inst := s.getInstance(c, "machine-0")
	s.sender = *nsgSender

	fwInst, ok := inst.(instances.InstanceFirewaller)
	c.Assert(ok, tc.Equals, true)

	rules, err := fwInst.IngressRules(c.Context(), "0")
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(rules, tc.DeepEquals, firewall.IngressRules{
		firewall.NewIngressRule(corenetwork.MustParsePortRange("80/tcp"), firewall.AllNetworksIPV4CIDR),
		firewall.NewIngressRule(corenetwork.MustParsePortRange("1000-2000/tcp"), firewall.AllNetworksIPV4CIDR, "192.168.1.0/24", "10.0.0.0/24"),
		firewall.NewIngressRule(corenetwork.MustParsePortRange("1-65535/udp"), firewall.AllNetworksIPV4CIDR),
		firewall.NewIngressRule(corenetwork.MustParsePortRange("80/udp"), firewall.AllNetworksIPV4CIDR),
	})
}

func (s *instanceSuite) TestInstanceClosePorts(c *tc.C) {
	nsgSender := s.setupSecurityGroupRules()
	inst := s.getInstance(c, "machine-0")
	fwInst, ok := inst.(instances.InstanceFirewaller)
	c.Assert(ok, tc.Equals, true)

	sender := &azuretesting.MockSender{}
	notFoundSender := &azuretesting.MockSender{}
	notFoundSender.AppendAndRepeatResponse(azuretesting.NewResponseWithStatus(
		"rule not found", http.StatusNotFound,
	), 2)
	s.sender = azuretesting.Senders{nsgSender, sender, notFoundSender, notFoundSender, notFoundSender}

	err := fwInst.ClosePorts(c.Context(), "0", firewall.IngressRules{
		firewall.NewIngressRule(corenetwork.MustParsePortRange("1000/tcp")),
		firewall.NewIngressRule(corenetwork.MustParsePortRange("1000-2000/udp")),
		firewall.NewIngressRule(corenetwork.MustParsePortRange("1000-2000/udp"), "192.168.1.0/24", "10.0.0.0/24"),
	})
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(s.requests, tc.HasLen, 5)
	c.Assert(s.requests[0].Method, tc.Equals, "GET")
	c.Assert(s.requests[0].URL.Path, tc.Equals, internalSubnetPath)
	c.Assert(s.requests[1].Method, tc.Equals, "DELETE")
	c.Assert(s.requests[1].URL.Path, tc.Equals, securityRulePath("machine-0-tcp-1000"))
	c.Assert(s.requests[2].Method, tc.Equals, "DELETE")
	c.Assert(s.requests[2].URL.Path, tc.Equals, securityRulePath("machine-0-udp-1000-2000"))
	c.Assert(s.requests[3].Method, tc.Equals, "DELETE")
	c.Assert(s.requests[3].URL.Path, tc.Equals, securityRulePath("machine-0-udp-1000-2000-cidr-10-0-0-0-24"))
	c.Assert(s.requests[4].Method, tc.Equals, "DELETE")
	c.Assert(s.requests[4].URL.Path, tc.Equals, securityRulePath("machine-0-udp-1000-2000-cidr-192-168-1-0-24"))
}

func (s *instanceSuite) TestInstanceOpenPorts(c *tc.C) {
	nsgSender := s.setupSecurityGroupRules()
	inst := s.getInstance(c, "machine-0")
	fwInst, ok := inst.(instances.InstanceFirewaller)
	c.Assert(ok, tc.Equals, true)

	okSender := &azuretesting.MockSender{}
	okSender.AppendResponse(azuretesting.NewResponseWithContent("{}"))
	s.sender = azuretesting.Senders{nsgSender, okSender, okSender, okSender, okSender}

	err := fwInst.OpenPorts(c.Context(), "0", firewall.IngressRules{
		firewall.NewIngressRule(corenetwork.MustParsePortRange("1000/tcp")),
		firewall.NewIngressRule(corenetwork.MustParsePortRange("1000-2000/udp")),
		firewall.NewIngressRule(corenetwork.MustParsePortRange("1000-2000/tcp"), "192.168.1.0/24", "10.0.0.0/24"),
	})
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(s.requests, tc.HasLen, 5)
	c.Assert(s.requests[0].Method, tc.Equals, "GET")
	c.Assert(s.requests[0].URL.Path, tc.Equals, internalSubnetPath)
	c.Assert(s.requests[1].Method, tc.Equals, "PUT")
	c.Assert(s.requests[1].URL.Path, tc.Equals, securityRulePath("machine-0-tcp-1000"))
	assertRequestBody(c, s.requests[1], &armnetwork.SecurityRule{
		Properties: &armnetwork.SecurityRulePropertiesFormat{
			Description:              to.Ptr("1000/tcp from *"),
			Protocol:                 to.Ptr(armnetwork.SecurityRuleProtocolTCP),
			SourcePortRange:          to.Ptr("*"),
			SourceAddressPrefix:      to.Ptr("*"),
			DestinationPortRange:     to.Ptr("1000"),
			DestinationAddressPrefix: to.Ptr("10.0.0.4"),
			Access:                   to.Ptr(armnetwork.SecurityRuleAccessAllow),
			Priority:                 to.Ptr(int32(200)),
			Direction:                to.Ptr(armnetwork.SecurityRuleDirectionInbound),
		},
	})
	c.Assert(s.requests[2].Method, tc.Equals, "PUT")
	c.Assert(s.requests[2].URL.Path, tc.Equals, securityRulePath("machine-0-udp-1000-2000"))
	assertRequestBody(c, s.requests[2], &armnetwork.SecurityRule{
		Properties: &armnetwork.SecurityRulePropertiesFormat{
			Description:              to.Ptr("1000-2000/udp from *"),
			Protocol:                 to.Ptr(armnetwork.SecurityRuleProtocolUDP),
			SourcePortRange:          to.Ptr("*"),
			SourceAddressPrefix:      to.Ptr("*"),
			DestinationPortRange:     to.Ptr("1000-2000"),
			DestinationAddressPrefix: to.Ptr("10.0.0.4"),
			Access:                   to.Ptr(armnetwork.SecurityRuleAccessAllow),
			Priority:                 to.Ptr(int32(201)),
			Direction:                to.Ptr(armnetwork.SecurityRuleDirectionInbound),
		},
	})
	c.Assert(s.requests[3].Method, tc.Equals, "PUT")
	c.Assert(s.requests[3].URL.Path, tc.Equals, securityRulePath("machine-0-tcp-1000-2000-cidr-10-0-0-0-24"))
	assertRequestBody(c, s.requests[3], &armnetwork.SecurityRule{
		Properties: &armnetwork.SecurityRulePropertiesFormat{
			Description:              to.Ptr("1000-2000/tcp from 10.0.0.0/24"),
			Protocol:                 to.Ptr(armnetwork.SecurityRuleProtocolTCP),
			SourcePortRange:          to.Ptr("*"),
			SourceAddressPrefix:      to.Ptr("10.0.0.0/24"),
			DestinationPortRange:     to.Ptr("1000-2000"),
			DestinationAddressPrefix: to.Ptr("10.0.0.4"),
			Access:                   to.Ptr(armnetwork.SecurityRuleAccessAllow),
			Priority:                 to.Ptr(int32(202)),
			Direction:                to.Ptr(armnetwork.SecurityRuleDirectionInbound),
		},
	})
	c.Assert(s.requests[4].Method, tc.Equals, "PUT")
	c.Assert(s.requests[4].URL.Path, tc.Equals, securityRulePath("machine-0-tcp-1000-2000-cidr-192-168-1-0-24"))
	assertRequestBody(c, s.requests[4], &armnetwork.SecurityRule{
		Properties: &armnetwork.SecurityRulePropertiesFormat{
			Description:              to.Ptr("1000-2000/tcp from 192.168.1.0/24"),
			Protocol:                 to.Ptr(armnetwork.SecurityRuleProtocolTCP),
			SourcePortRange:          to.Ptr("*"),
			SourceAddressPrefix:      to.Ptr("192.168.1.0/24"),
			DestinationPortRange:     to.Ptr("1000-2000"),
			DestinationAddressPrefix: to.Ptr("10.0.0.4"),
			Access:                   to.Ptr(armnetwork.SecurityRuleAccessAllow),
			Priority:                 to.Ptr(int32(203)),
			Direction:                to.Ptr(armnetwork.SecurityRuleDirectionInbound),
		},
	})
}

func (s *instanceSuite) TestInstanceOpenPortsAlreadyOpen(c *tc.C) {
	nsgRule := &armnetwork.SecurityRule{
		Name: to.Ptr("machine-0-tcp-1000"),
		Properties: &armnetwork.SecurityRulePropertiesFormat{
			Protocol:             to.Ptr(armnetwork.SecurityRuleProtocolAsterisk),
			DestinationPortRange: to.Ptr("1000"),
			Access:               to.Ptr(armnetwork.SecurityRuleAccessAllow),
			Priority:             to.Ptr(int32(202)),
			Direction:            to.Ptr(armnetwork.SecurityRuleDirectionInbound),
		},
	}
	nsgSender := s.setupSecurityGroupRules(nsgRule)
	inst := s.getInstance(c, "machine-0")
	fwInst, ok := inst.(instances.InstanceFirewaller)
	c.Assert(ok, tc.Equals, true)

	okSender := &azuretesting.MockSender{}
	okSender.AppendResponse(azuretesting.NewResponseWithContent("{}"))
	s.sender = azuretesting.Senders{nsgSender, okSender, okSender}

	err := fwInst.OpenPorts(c.Context(), "0", firewall.IngressRules{
		firewall.NewIngressRule(corenetwork.MustParsePortRange("1000/tcp")),
		firewall.NewIngressRule(corenetwork.MustParsePortRange("1000-2000/udp")),
	})
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(s.requests, tc.HasLen, 2)
	c.Assert(s.requests[0].Method, tc.Equals, "GET")
	c.Assert(s.requests[0].URL.Path, tc.Equals, internalSubnetPath)
	c.Assert(s.requests[1].Method, tc.Equals, "PUT")
	c.Assert(s.requests[1].URL.Path, tc.Equals, securityRulePath("machine-0-udp-1000-2000"))
	assertRequestBody(c, s.requests[1], &armnetwork.SecurityRule{
		Properties: &armnetwork.SecurityRulePropertiesFormat{
			Description:              to.Ptr("1000-2000/udp from *"),
			Protocol:                 to.Ptr(armnetwork.SecurityRuleProtocolUDP),
			SourcePortRange:          to.Ptr("*"),
			SourceAddressPrefix:      to.Ptr("*"),
			DestinationPortRange:     to.Ptr("1000-2000"),
			DestinationAddressPrefix: to.Ptr("10.0.0.4"),
			Access:                   to.Ptr(armnetwork.SecurityRuleAccessAllow),
			Priority:                 to.Ptr(int32(200)),
			Direction:                to.Ptr(armnetwork.SecurityRuleDirectionInbound),
		},
	})
}

func (s *instanceSuite) TestInstanceOpenPortsNoInternalAddress(c *tc.C) {
	s.networkInterfaces = []*armnetwork.Interface{
		makeNetworkInterface("nic-0", "machine-0"),
	}
	inst := s.getInstance(c, "machine-0")
	fwInst, ok := inst.(instances.InstanceFirewaller)
	c.Assert(ok, tc.Equals, true)
	err := fwInst.OpenPorts(c.Context(), "0", nil)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(s.requests, tc.HasLen, 0)
}

func (s *instanceSuite) TestAllInstances(c *tc.C) {
	s.sender = s.getInstancesSender()
	instances, err := s.env.AllInstances(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(instances, tc.HasLen, 2)
	c.Assert(instances[0].Id(), tc.Equals, instance.Id("machine-0"))
	c.Assert(instances[1].Id(), tc.Equals, instance.Id("machine-1"))
}

func (s *instanceSuite) TestAllRunningInstances(c *tc.C) {
	s.sender = s.getInstancesSender()
	instances, err := s.env.AllRunningInstances(c.Context())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(instances, tc.HasLen, 2)
	c.Assert(instances[0].Id(), tc.Equals, instance.Id("machine-0"))
	c.Assert(instances[1].Id(), tc.Equals, instance.Id("machine-1"))
}

func (s *instanceSuite) TestControllerInstancesSomePending(c *tc.C) {
	*((s.deployments[1].Properties.Dependencies)[0].DependsOn)[0].ResourceName = "juju-controller"
	s.sender = s.getInstancesSender()
	ids, err := s.env.ControllerInstances(c.Context(), testing.ControllerTag.Id())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(ids, tc.HasLen, 2)
	c.Assert(ids[0], tc.Equals, instance.Id("machine-0"))
	c.Assert(ids[1], tc.Equals, instance.Id("machine-1"))
}

func (s *instanceSuite) TestControllerInstances(c *tc.C) {
	s.sender = s.getInstancesSender()
	ids, err := s.env.ControllerInstances(c.Context(), testing.ControllerTag.Id())
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(ids, tc.HasLen, 1)
	c.Assert(ids[0], tc.Equals, instance.Id("machine-0"))
}

var internalSecurityGroupPath = path.Join(
	"/subscriptions", fakeManagedSubscriptionId,
	"resourceGroups", "juju-testmodel-"+testing.ModelTag.Id()[:8],
	"providers/Microsoft.Network/networkSecurityGroups/juju-internal-nsg",
)

var internalSubnetPath = path.Join(
	"/subscriptions", fakeManagedSubscriptionId,
	"resourceGroups/juju-testmodel-model-deadbeef-0bad-400d-8000-4b1d0d06f00d",
	"providers/Microsoft.Network/virtualNetworks/juju-internal-network/subnets/juju-internal-subnet",
)

func securityRulePath(ruleName string) string {
	return path.Join(internalSecurityGroupPath, "securityRules", ruleName)
}
