// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure_test

import (
	"net/http"

	"github.com/Azure/azure-sdk-for-go/services/compute/mgmt/2019-07-01/compute"
	"github.com/Azure/azure-sdk-for-go/services/network/mgmt/2018-08-01/network"
	"github.com/Azure/azure-sdk-for-go/services/resources/mgmt/2020-06-01/resources"
	"github.com/Azure/go-autorest/autorest/mocks"
	"github.com/Azure/go-autorest/autorest/to"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/context"
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

	callCtx           *context.CloudCallContext
	invalidCredential bool
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
	s.requests = nil
	s.invalidCredential = false
	s.callCtx = &context.CloudCallContext{
		InvalidateCredentialFunc: func(string) error {
			s.invalidCredential = true
			return nil
		},
	}
}

func (s *environUpgradeSuite) TestEnvironImplementsUpgrader(c *gc.C) {
	c.Assert(s.env, gc.Implements, new(environs.Upgrader))
}

func (s *environUpgradeSuite) TestEnvironUpgradeOperations(c *gc.C) {
	upgrader := s.env.(environs.Upgrader)
	ops := upgrader.UpgradeOperations(s.callCtx, environs.UpgradeOperationsParams{})
	c.Assert(ops, gc.HasLen, 1)
	c.Assert(ops[0].TargetVersion, gc.Equals, 1)
	c.Assert(ops[0].Steps, gc.HasLen, 1)
	c.Assert(ops[0].Steps[0].Description(), gc.Equals, "Create common resource deployment")
}

func (s *environUpgradeSuite) TestEnvironUpgradeOperationCreateCommonDeployment(c *gc.C) {
	upgrader := s.env.(environs.Upgrader)
	op0 := upgrader.UpgradeOperations(s.callCtx, environs.UpgradeOperationsParams{})[0]

	// The existing NSG has two rules: one for Juju API traffic,
	// and an application-specific rule. Only the latter should
	// be preserved; we will recreate the "builtin" SSH rule,
	// and the API rule is not needed for non-controller models.
	customRule := network.SecurityRule{
		Name: to.StringPtr("machine-0-tcp-1234"),
		SecurityRulePropertiesFormat: &network.SecurityRulePropertiesFormat{
			Description:              to.StringPtr("custom rule"),
			Protocol:                 network.SecurityRuleProtocolTCP,
			SourceAddressPrefix:      to.StringPtr("*"),
			SourcePortRange:          to.StringPtr("*"),
			DestinationAddressPrefix: to.StringPtr("*"),
			DestinationPortRange:     to.StringPtr("1234"),
			Access:                   network.SecurityRuleAccessAllow,
			Priority:                 to.Int32Ptr(102),
			Direction:                network.SecurityRuleDirectionInbound,
		},
	}
	securityRules := []network.SecurityRule{{
		Name: to.StringPtr("JujuAPIInbound"),
		SecurityRulePropertiesFormat: &network.SecurityRulePropertiesFormat{
			Description:              to.StringPtr("Allow API connections to controller machines"),
			Protocol:                 network.SecurityRuleProtocolTCP,
			SourceAddressPrefix:      to.StringPtr("*"),
			SourcePortRange:          to.StringPtr("*"),
			DestinationAddressPrefix: to.StringPtr("192.168.16.0/20"),
			DestinationPortRange:     to.StringPtr("17777"),
			Access:                   network.SecurityRuleAccessAllow,
			Priority:                 to.Int32Ptr(101),
			Direction:                network.SecurityRuleDirectionInbound,
		},
	}, customRule}
	nsg := network.SecurityGroup{
		SecurityGroupPropertiesFormat: &network.SecurityGroupPropertiesFormat{
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
	c.Assert(op0.Steps[0].Run(s.callCtx), jc.ErrorIsNil)
	c.Assert(s.requests, gc.HasLen, 3)

	expectedSecurityRules := []network.SecurityRule{{
		Name: to.StringPtr("SSHInbound"),
		SecurityRulePropertiesFormat: &network.SecurityRulePropertiesFormat{
			Description:              to.StringPtr("Allow SSH access to all machines"),
			Protocol:                 network.SecurityRuleProtocolTCP,
			SourceAddressPrefix:      to.StringPtr("*"),
			SourcePortRange:          to.StringPtr("*"),
			DestinationAddressPrefix: to.StringPtr("*"),
			DestinationPortRange:     to.StringPtr("22"),
			Access:                   network.SecurityRuleAccessAllow,
			Priority:                 to.Int32Ptr(100),
			Direction:                network.SecurityRuleDirectionInbound,
		},
	}, customRule}
	nsgId := `[resourceId('Microsoft.Network/networkSecurityGroups', 'juju-internal-nsg')]`
	subnets := []network.Subnet{{
		Name: to.StringPtr("juju-internal-subnet"),
		SubnetPropertiesFormat: &network.SubnetPropertiesFormat{
			AddressPrefix: to.StringPtr("192.168.0.0/20"),
			NetworkSecurityGroup: &network.SecurityGroup{
				ID: to.StringPtr(nsgId),
			},
		},
	}, {
		Name: to.StringPtr("juju-controller-subnet"),
		SubnetPropertiesFormat: &network.SubnetPropertiesFormat{
			AddressPrefix: to.StringPtr("192.168.16.0/20"),
			NetworkSecurityGroup: &network.SecurityGroup{
				ID: to.StringPtr(nsgId),
			},
		},
	}}
	addressPrefixes := []string{"192.168.0.0/20", "192.168.16.0/20"}
	templateResources := []armtemplates.Resource{{
		Type:     "Microsoft.Network/networkSecurityGroups",
		Name:     "juju-internal-nsg",
		Location: "westus",
		Properties: &network.SecurityGroupPropertiesFormat{
			SecurityRules: &expectedSecurityRules,
		},
	}, {
		Type:     "Microsoft.Network/virtualNetworks",
		Name:     "juju-internal-network",
		Location: "westus",
		Properties: &network.VirtualNetworkPropertiesFormat{
			AddressSpace: &network.AddressSpace{&addressPrefixes},
			Subnets:      &subnets,
		},
		DependsOn: []string{nsgId},
	}}

	var actual resources.Deployment
	unmarshalRequestBody(c, s.requests[2], &actual)
	c.Assert(actual.Properties, gc.NotNil)
	c.Assert(actual.Properties.Template, gc.NotNil)
	resources, ok := actual.Properties.Template.(map[string]interface{})["resources"].([]interface{})
	c.Assert(ok, jc.IsTrue)
	c.Assert(resources, gc.HasLen, len(templateResources))
}

func (s *environUpgradeSuite) TestEnvironUpgradeOperationCreateCommonDeploymentControllerModel(c *gc.C) {
	s.sender = nil
	env := openEnviron(c, s.provider, &s.sender, testing.Attrs{"name": "controller"})
	s.requests = nil
	upgrader := env.(environs.Upgrader)

	controllerTags := make(map[string]*string)
	trueString := "true"
	controllerTags["juju-is-controller"] = &trueString
	vms := []compute.VirtualMachine{{
		Tags: nil,
	}, {
		Tags: controllerTags,
	}}
	vmListSender := azuretesting.NewSenderWithValue(&compute.VirtualMachineListResult{
		Value: &vms,
	})
	vmListSender.PathPattern = ".*/virtualMachines"
	s.sender = append(s.sender, vmListSender)

	op0 := upgrader.UpgradeOperations(s.callCtx, environs.UpgradeOperationsParams{})[0]
	c.Assert(op0.Steps[0].Run(s.callCtx), jc.ErrorIsNil)
}

func (s *environUpgradeSuite) TestEnvironUpgradeOperationCreateCommonDeploymentControllerModelWithInvalidCredential(c *gc.C) {
	s.sender = nil
	s.requests = nil
	env := openEnviron(c, s.provider, &s.sender, testing.Attrs{"name": "controller"})
	upgrader := env.(environs.Upgrader)

	controllerTags := make(map[string]*string)
	trueString := "true"
	controllerTags["juju-is-controller"] = &trueString

	mockSender := mocks.NewSender()
	mockSender.AppendResponse(mocks.NewResponseWithStatus("401 Unauthorized", http.StatusUnauthorized))
	s.sender = append(s.sender, mockSender)

	c.Assert(s.invalidCredential, jc.IsFalse)
	op0 := upgrader.UpgradeOperations(s.callCtx, environs.UpgradeOperationsParams{})[0]
	c.Assert(op0.Steps[0].Run(s.callCtx), gc.NotNil)
	c.Assert(s.invalidCredential, jc.IsTrue)
}
