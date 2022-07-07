// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure_test

import (
	stdcontext "context"
	"net/http"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute/v2"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/network/armnetwork"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources"
	"github.com/Azure/go-autorest/autorest/mocks"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/provider/azure"
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
		Sender:           azuretesting.NewSerialSender(&s.sender),
		RequestInspector: &azuretesting.RequestRecorderPolicy{Requests: &s.requests},
		CreateTokenCredential: func(appId, appPassword, tenantID string, opts azcore.ClientOptions) (azcore.TokenCredential, error) {
			return &azuretesting.FakeCredential{}, nil
		},
	})
	s.env = openEnviron(c, s.provider, &s.sender)
	s.requests = nil
	s.invalidCredential = false
	s.callCtx = &context.CloudCallContext{
		Context: stdcontext.TODO(),
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
	customRule := &armnetwork.SecurityRule{
		Name: to.Ptr("machine-0-tcp-1234"),
		Properties: &armnetwork.SecurityRulePropertiesFormat{
			Description:              to.Ptr("custom rule"),
			Protocol:                 to.Ptr(armnetwork.SecurityRuleProtocolTCP),
			SourceAddressPrefix:      to.Ptr("*"),
			SourcePortRange:          to.Ptr("*"),
			DestinationAddressPrefix: to.Ptr("*"),
			DestinationPortRange:     to.Ptr("1234"),
			Access:                   to.Ptr(armnetwork.SecurityRuleAccessAllow),
			Priority:                 to.Ptr(int32(102)),
			Direction:                to.Ptr(armnetwork.SecurityRuleDirectionInbound),
		},
	}
	securityRules := []*armnetwork.SecurityRule{{
		Name: to.Ptr("JujuAPIInbound"),
		Properties: &armnetwork.SecurityRulePropertiesFormat{
			Description:              to.Ptr("Allow API connections to controller machines"),
			Protocol:                 to.Ptr(armnetwork.SecurityRuleProtocolTCP),
			SourceAddressPrefix:      to.Ptr("*"),
			SourcePortRange:          to.Ptr("*"),
			DestinationAddressPrefix: to.Ptr("192.168.16.0/20"),
			DestinationPortRange:     to.Ptr("17777"),
			Access:                   to.Ptr(armnetwork.SecurityRuleAccessAllow),
			Priority:                 to.Ptr(int32(101)),
			Direction:                to.Ptr(armnetwork.SecurityRuleDirectionInbound),
		},
	}, customRule}
	nsg := armnetwork.SecurityGroup{
		Properties: &armnetwork.SecurityGroupPropertiesFormat{
			SecurityRules: securityRules,
		},
	}

	vmListSender := azuretesting.NewSenderWithValue(&armcompute.VirtualMachineListResult{})
	vmListSender.PathPattern = ".*/virtualMachines"
	nsgSender := azuretesting.NewSenderWithValue(&nsg)
	nsgSender.PathPattern = ".*/networkSecurityGroups/juju-internal-nsg"
	deploymentSender := azuretesting.NewSenderWithValue(&armresources.Deployment{})
	deploymentSender.PathPattern = ".*/deployments/common"
	s.sender = append(s.sender, vmListSender, nsgSender, deploymentSender)
	c.Assert(op0.Steps[0].Run(s.callCtx), jc.ErrorIsNil)
	c.Assert(s.requests, gc.HasLen, 3)

	var actual armresources.Deployment
	unmarshalRequestBody(c, s.requests[2], &actual)
	c.Assert(actual.Properties, gc.NotNil)
	c.Assert(actual.Properties.Template, gc.NotNil)
	resources, ok := actual.Properties.Template.(map[string]interface{})["resources"].([]interface{})
	c.Assert(ok, jc.IsTrue)
	c.Assert(resources, gc.HasLen, 2)
}

func (s *environUpgradeSuite) TestEnvironUpgradeOperationCreateCommonDeploymentControllerModel(c *gc.C) {
	s.sender = nil
	env := openEnviron(c, s.provider, &s.sender, testing.Attrs{"name": "controller"})
	s.requests = nil
	upgrader := env.(environs.Upgrader)

	controllerTags := make(map[string]*string)
	trueString := "true"
	controllerTags["juju-is-controller"] = &trueString
	vms := []*armcompute.VirtualMachine{{
		Tags: nil,
	}, {
		Tags: controllerTags,
	}}
	vmListSender := azuretesting.NewSenderWithValue(&armcompute.VirtualMachineListResult{
		Value: vms,
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

	unauthSender := mocks.NewSender()
	unauthSender.AppendAndRepeatResponse(mocks.NewResponseWithStatus("401 Unauthorized", http.StatusUnauthorized), 3)
	s.sender = append(s.sender, unauthSender, unauthSender, unauthSender)

	c.Assert(s.invalidCredential, jc.IsFalse)
	op0 := upgrader.UpgradeOperations(s.callCtx, environs.UpgradeOperationsParams{})[0]
	c.Assert(op0.Steps[0].Run(s.callCtx), gc.NotNil)
	c.Assert(s.invalidCredential, jc.IsTrue)
}
