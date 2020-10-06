// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

import (
	stdcontext "context"
	"strings"

	"github.com/Azure/azure-sdk-for-go/services/compute/mgmt/2019-07-01/compute"
	"github.com/Azure/azure-sdk-for-go/services/network/mgmt/2018-08-01/network"
	"github.com/Azure/go-autorest/autorest/to"
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
	nsgClient := network.SecurityGroupsClient{
		BaseClient: env.network,
	}
	allRules, err := existingSecurityRules(nsgClient, env.resourceGroup)
	if errors.IsNotFound(err) {
		allRules = nil
	} else if err != nil {
		return errors.Trace(err)
	}
	rules := make([]network.SecurityRule, 0, len(allRules))
	for _, rule := range allRules {
		name := to.String(rule.Name)
		if name == sshSecurityRuleName || strings.HasPrefix(name, apiSecurityRulePrefix) {
			continue
		}
		rules = append(rules, rule)
	}

	env.mu.Lock()
	storageAccountType := env.config.storageAccountType
	env.mu.Unlock()
	return env.createCommonResourceDeployment(ctx, nil, rules, storageAccountTemplateResource(
		env.location, nil,
		env.storageAccountName,
		storageAccountType,
	))
}

// existingSecurityRules returns the network security rules for the internal
// network security group in the specified resource group. If the network
// security group has not been created, this function will return an error
// satisfying errors.IsNotFound.
func existingSecurityRules(
	nsgClient network.SecurityGroupsClient,
	resourceGroup string,
) ([]network.SecurityRule, error) {
	sdkCtx := stdcontext.Background()
	nsg, err := nsgClient.Get(sdkCtx, resourceGroup, internalSecurityGroupName, "")
	if err != nil {
		if isNotFoundResult(nsg.Response) {
			return nil, errors.NotFoundf("security group")
		}
		return nil, errors.Annotate(err, "querying network security group")
	}
	var rules []network.SecurityRule
	if nsg.SecurityRules != nil {
		rules = *nsg.SecurityRules
	}
	return rules, nil
}

func isControllerEnviron(env *azureEnviron, ctx context.ProviderCallContext) (bool, error) {
	// Look for a machine with the "juju-is-controller" tag set to "true".
	client := compute.VirtualMachinesClient{env.compute}
	sdkCtx := stdcontext.Background()
	result, err := client.ListComplete(sdkCtx, env.resourceGroup)
	if err != nil {
		return false, errorutils.HandleCredentialError(errors.Annotate(err, "listing virtual machines"), ctx)
	}

	if result.Response().IsEmpty() {
		// No machines implies this is not the controller model, as
		// there must be a controller machine for the upgrades to be
		// running!
		return false, nil
	}

	for ; result.NotDone(); err = result.NextWithContext(sdkCtx) {
		if err != nil {
			return false, errors.Annotate(err, "iterating machines")
		}
		vm := result.Value()
		if to.String(vm.Tags[tags.JujuIsController]) == "true" {
			return true, nil
		}
	}
	return false, nil
}
