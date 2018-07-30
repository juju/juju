// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

import (
	"strings"

	"github.com/Azure/azure-sdk-for-go/arm/compute"
	"github.com/Azure/azure-sdk-for-go/arm/network"
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
		ManagementClient: env.network,
	}
	allRules, err := networkSecurityRules(nsgClient, env.resourceGroup)
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

func isControllerEnviron(env *azureEnviron, ctx context.ProviderCallContext) (bool, error) {
	// Look for a machine with the "juju-is-controller" tag set to "true".
	client := compute.VirtualMachinesClient{env.compute}
	result, err := client.List(env.resourceGroup)
	if err != nil {
		return false, errorutils.HandleCredentialError(errors.Annotate(err, "listing virtual machines"), ctx)
	}

	if result.Value == nil {
		// No machines implies this is not the controller model, as
		// there must be a controller machine for the upgrades to be
		// running!
		return false, nil
	}

	for _, vm := range *result.Value {
		if toTags(vm.Tags)[tags.JujuIsController] == "true" {
			return true, nil
		}
	}
	return false, nil
}
