// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure_test

import (
	"net/http"

	"github.com/Azure/azure-sdk-for-go/arm/compute"
	"github.com/Azure/azure-sdk-for-go/arm/network"
	"github.com/Azure/azure-sdk-for-go/arm/resources/resources"
	"github.com/Azure/azure-sdk-for-go/arm/storage"
	"github.com/Azure/go-autorest/autorest/to"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/provider/azure"
	"github.com/juju/juju/provider/azure/internal/armtemplates"
	"github.com/juju/juju/provider/azure/internal/azuretesting"
	"github.com/juju/juju/testing"
)

type environUpgradeSuite struct {
	testing.BaseSuite

	requests []*http.Request
	sender   azuretesting.Senders
	provider environs.EnvironProvider
	env      environs.Environ
}

var _ = gc.Suite(&environUpgradeSuite{})

func (s *environUpgradeSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.sender = nil
	s.requests = nil

	s.provider = newProvider(c, azure.ProviderConfig{
		Sender:                     azuretesting.NewSerialSender(&s.sender),
		RequestInspector:           azuretesting.RequestRecorder(&s.requests),
		RandomWindowsAdminPassword: func() string { return "sorandom" },
	})
	s.env = openEnviron(c, s.provider, &s.sender)
}

func (s *environUpgradeSuite) TestEnvironImplementsUpgrader(c *gc.C) {
	c.Assert(s.env, gc.Implements, new(environs.Upgrader))
}

func (s *environUpgradeSuite) TestEnvironUpgradeOperations(c *gc.C) {
	upgrader := s.env.(environs.Upgrader)
	ops := upgrader.UpgradeOperations()
	c.Assert(ops, gc.HasLen, 1)
	c.Assert(ops[0].TargetVersion, gc.Equals, version.MustParse("2.2-alpha1"))
	c.Assert(ops[0].Steps, gc.HasLen, 1)
	c.Assert(ops[0].Steps[0].Description(), gc.Equals, "Create common resource deployment")
}

func (s *environUpgradeSuite) TestEnvironUpgradeOperationCreateCommonDeployment(c *gc.C) {
	upgrader := s.env.(environs.Upgrader)
	op0 := upgrader.UpgradeOperations()[0]

	// The existing NSG has two rules: one for Juju API traffic,
	// and an application-specific rule. Only the latter should
	// be preserved; we will recreate the "builtin" SSH rule,
	// and the API rule is not needed for non-controller models.
	customRule := network.SecurityRule{
		Name: to.StringPtr("machine-0-tcp-1234"),
		Properties: &network.SecurityRulePropertiesFormat{
			Description:              to.StringPtr("custom rule"),
			Protocol:                 network.TCP,
			SourceAddressPrefix:      to.StringPtr("*"),
			SourcePortRange:          to.StringPtr("*"),
			DestinationAddressPrefix: to.StringPtr("*"),
			DestinationPortRange:     to.StringPtr("1234"),
			Access:                   network.Allow,
			Priority:                 to.Int32Ptr(102),
			Direction:                network.Inbound,
		},
	}
	securityRules := []network.SecurityRule{{
		Name: to.StringPtr("JujuAPIInbound"),
		Properties: &network.SecurityRulePropertiesFormat{
			Description:              to.StringPtr("Allow API connections to controller machines"),
			Protocol:                 network.TCP,
			SourceAddressPrefix:      to.StringPtr("*"),
			SourcePortRange:          to.StringPtr("*"),
			DestinationAddressPrefix: to.StringPtr("192.168.16.0/20"),
			DestinationPortRange:     to.StringPtr("17777"),
			Access:                   network.Allow,
			Priority:                 to.Int32Ptr(101),
			Direction:                network.Inbound,
		},
	}, customRule}
	nsg := network.SecurityGroup{
		Properties: &network.SecurityGroupPropertiesFormat{
			SecurityRules: &securityRules,
		},
	}

	vmListSender := azuretesting.NewSenderWithValue(&compute.VirtualMachineListResult{})
	vmListSender.PathPattern = ".*/virtualMachines"
	nsgSender := azuretesting.NewSenderWithValue(&nsg)
	nsgSender.PathPattern = ".*/networkSecurityGroups/juju-internal-nsg"
	deploymentSender := azuretesting.NewSenderWithValue(&resources.Deployment{})
	deploymentSender.PathPattern = ".*/deployments/common"
	s.sender = append(s.sender, vmListSender, nsgSender, deploymentSender)
	c.Assert(op0.Steps[0].Run(), jc.ErrorIsNil)
	c.Assert(s.requests, gc.HasLen, 3)

	expectedSecurityRules := []network.SecurityRule{{
		Name: to.StringPtr("SSHInbound"),
		Properties: &network.SecurityRulePropertiesFormat{
			Description:              to.StringPtr("Allow SSH access to all machines"),
			Protocol:                 network.TCP,
			SourceAddressPrefix:      to.StringPtr("*"),
			SourcePortRange:          to.StringPtr("*"),
			DestinationAddressPrefix: to.StringPtr("*"),
			DestinationPortRange:     to.StringPtr("22"),
			Access:                   network.Allow,
			Priority:                 to.Int32Ptr(100),
			Direction:                network.Inbound,
		},
	}, customRule}
	nsgId := `[resourceId('Microsoft.Network/networkSecurityGroups', 'juju-internal-nsg')]`
	subnets := []network.Subnet{{
		Name: to.StringPtr("juju-internal-subnet"),
		Properties: &network.SubnetPropertiesFormat{
			AddressPrefix: to.StringPtr("192.168.0.0/20"),
			NetworkSecurityGroup: &network.SecurityGroup{
				ID: to.StringPtr(nsgId),
			},
		},
	}, {
		Name: to.StringPtr("juju-controller-subnet"),
		Properties: &network.SubnetPropertiesFormat{
			AddressPrefix: to.StringPtr("192.168.16.0/20"),
			NetworkSecurityGroup: &network.SecurityGroup{
				ID: to.StringPtr(nsgId),
			},
		},
	}}
	addressPrefixes := []string{"192.168.0.0/20", "192.168.16.0/20"}
	templateResources := []armtemplates.Resource{{
		APIVersion: network.APIVersion,
		Type:       "Microsoft.Network/networkSecurityGroups",
		Name:       "juju-internal-nsg",
		Location:   "westus",
		Properties: &network.SecurityGroupPropertiesFormat{
			SecurityRules: &expectedSecurityRules,
		},
	}, {
		APIVersion: network.APIVersion,
		Type:       "Microsoft.Network/virtualNetworks",
		Name:       "juju-internal-network",
		Location:   "westus",
		Properties: &network.VirtualNetworkPropertiesFormat{
			AddressSpace: &network.AddressSpace{&addressPrefixes},
			Subnets:      &subnets,
		},
		DependsOn: []string{nsgId},
	}, {
		APIVersion: storage.APIVersion,
		Type:       "Microsoft.Storage/storageAccounts",
		Name:       storageAccountName,
		Location:   "westus",
		StorageSku: &storage.Sku{
			Name: storage.SkuName("Standard_LRS"),
		},
	}}

	var actual resources.Deployment
	unmarshalRequestBody(c, s.requests[2], &actual)
	c.Assert(actual.Properties, gc.NotNil)
	c.Assert(actual.Properties.Template, gc.NotNil)
	resources := (*actual.Properties.Template)["resources"].([]interface{})
	c.Assert(resources, gc.HasLen, len(templateResources))
}

func (s *environUpgradeSuite) TestEnvironUpgradeOperationCreateCommonDeploymentControllerModel(c *gc.C) {
	s.sender = nil
	s.requests = nil
	env := openEnviron(c, s.provider, &s.sender, testing.Attrs{"name": "controller"})
	upgrader := env.(environs.Upgrader)

	controllerTags := make(map[string]*string)
	trueString := "true"
	controllerTags["juju-is-controller"] = &trueString
	vms := []compute.VirtualMachine{{
		Tags: nil,
	}, {
		Tags: &controllerTags,
	}}
	vmListSender := azuretesting.NewSenderWithValue(&compute.VirtualMachineListResult{
		Value: &vms,
	})
	vmListSender.PathPattern = ".*/virtualMachines"
	s.sender = append(s.sender, vmListSender)

	op0 := upgrader.UpgradeOperations()[0]
	c.Assert(op0.Steps[0].Run(), jc.ErrorIsNil)
}
