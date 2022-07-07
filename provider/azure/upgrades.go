// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

import (
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/network/armnetwork"
	"github.com/juju/errors"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/context"
	"github.com/juju/juju/environs/tags"
	"github.com/juju/juju/provider/azure/internal/errorutils"
)

// UpgradeOperations is part of the upgrades.OperationSource interface.
func (env *azureEnviron) UpgradeOperations(context.ProviderCallContext, environs.UpgradeOperationsParams) []environs.UpgradeOperation {
	return []environs.UpgradeOperation{{
		providerVersion1,
		[]environs.UpgradeStep{
			commonDeploymentUpgradeStep{env},
		},
	}}
}

// commonDeploymentUpgradeStep adds a "common" deployment to each
// Environ corresponding to non-controller models.
type commonDeploymentUpgradeStep struct {
	env *azureEnviron
}

// Description is part of the environs.UpgradeStep interface.
func (commonDeploymentUpgradeStep) Description() string {
	return "Create common resource deployment"
}

// Run is part of the environs.UpgradeStep interface.
func (step commonDeploymentUpgradeStep) Run(ctx context.ProviderCallContext) error {
	env := step.env
	isControllerEnviron, err := isControllerEnviron(env, ctx)
	if err != nil {
		return errors.Trace(err)
	}
	if isControllerEnviron {
		// We only need to create the deployment for
		// non-controller Environs.
		return nil
	}

	// Identify the network security rules that exist already.
	// We will add these, excluding the SSH/API rules, to the
	// network security group template created in the deployment
	// below.
	securityGroups, err := env.securityGroupsClient()
	if err != nil {
		return errors.Trace(err)
	}
	allRules, err := existingSecurityRules(ctx, securityGroups, env.resourceGroup)
	if errors.IsNotFound(err) {
		allRules = nil
	} else if err != nil {
		return errors.Trace(err)
	}
	rules := make([]*armnetwork.SecurityRule, 0, len(allRules))
	for _, rule := range allRules {
		name := toValue(rule.Name)
		if name == sshSecurityRuleName || strings.HasPrefix(name, apiSecurityRulePrefix) {
			continue
		}
		rules = append(rules, rule)
	}

	return env.createCommonResourceDeployment(ctx, nil, rules)
}

// existingSecurityRules returns the network security rules for the internal
// network security group in the specified resource group. If the network
// security group has not been created, this function will return an error
// satisfying errors.IsNotFound.
func existingSecurityRules(
	ctx context.ProviderCallContext,
	nsgClient *armnetwork.SecurityGroupsClient,
	resourceGroup string,
) ([]*armnetwork.SecurityRule, error) {
	nsg, err := nsgClient.Get(ctx, resourceGroup, internalSecurityGroupName, nil)
	if err != nil {
		if errorutils.IsNotFoundError(err) {
			return nil, errors.NotFoundf("security group")
		}
		return nil, errors.Annotate(err, "querying network security group")
	}
	var rules []*armnetwork.SecurityRule
	if nsg.Properties != nil {
		rules = nsg.Properties.SecurityRules
	}
	return rules, nil
}

func isControllerEnviron(env *azureEnviron, ctx context.ProviderCallContext) (bool, error) {
	compute, err := env.computeClient()
	if err != nil {
		return false, errors.Trace(err)
	}
	// Look for a machine with the "juju-is-controller" tag set to "true".
	pager := compute.NewListPager(env.resourceGroup, nil)
	for pager.More() {
		next, err := pager.NextPage(ctx)
		if err != nil {
			return false, errorutils.HandleCredentialError(errors.Annotate(err, "listing virtual machines"), ctx)
		}
		for _, vm := range next.Value {
			if toValue(vm.Tags[tags.JujuIsController]) == "true" {
				return true, nil
			}
		}
	}
	return false, nil
}
