// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

import (
	"github.com/Azure/azure-sdk-for-go/arm/compute"
	"github.com/Azure/azure-sdk-for-go/arm/network"
	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/to"
	"github.com/juju/errors"
	"github.com/juju/version"

	"github.com/juju/juju/environs"
	"github.com/juju/juju/environs/tags"
)

// UpgradeOperations is part of the upgrades.OperationSource interface.
func (env *azureEnviron) UpgradeOperations(environs.UpgradeOperationsParams) []environs.UpgradeOperation {
	return []environs.UpgradeOperation{{
		version.MustParse("2.2-alpha1"),
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
func (step commonDeploymentUpgradeStep) Run() error {
	env := step.env
	isControllerEnviron, err := isControllerEnviron(env)
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
	nsgClient := network.SecurityGroupsClient{env.network}
	allRules, err := networkSecurityRules(
		nsgClient, env.callAPI, env.resourceGroup,
	)
	if errors.IsNotFound(err) {
		allRules = nil
	} else if err != nil {
		return errors.Trace(err)
	}
	var rules []network.SecurityRule
	for _, rule := range allRules {
		switch to.String(rule.Name) {
		case to.String(sshSecurityRule.Name):
		case to.String(apiSecurityRule.Name):
		default:
			rules = append(rules, rule)
		}
	}

	env.mu.Lock()
	storageAccountType := env.config.storageAccountType
	env.mu.Unlock()
	return env.createCommonResourceDeployment(
		nil, storageAccountType, rules,
	)
}

func isControllerEnviron(env *azureEnviron) (bool, error) {
	// Look for a machine with the "juju-is-controller" tag set to "true".
	client := compute.VirtualMachinesClient{env.compute}
	var result compute.VirtualMachineListResult
	if err := env.callAPI(func() (autorest.Response, error) {
		var err error
		result, err = client.List(env.resourceGroup)
		return result.Response, err
	}); err != nil {
		return false, errors.Annotate(err, "listing virtual machines")
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
