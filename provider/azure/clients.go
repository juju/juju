// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

import (
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/authorization/armauthorization/v3"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute/v4"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/keyvault/armkeyvault"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/network/armnetwork"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources"
)

func (env *azureEnviron) deployClient() (*armresources.DeploymentsClient, error) {
	return armresources.NewDeploymentsClient(env.subscriptionId, env.credential, &arm.ClientOptions{
		ClientOptions: env.clientOptions,
	})
}

func (env *azureEnviron) computeClient() (*armcompute.VirtualMachinesClient, error) {
	return armcompute.NewVirtualMachinesClient(env.subscriptionId, env.credential, &arm.ClientOptions{
		ClientOptions: env.clientOptions,
	})
}

func (env *azureEnviron) resourceGroupsClient() (*armresources.ResourceGroupsClient, error) {
	return armresources.NewResourceGroupsClient(env.subscriptionId, env.credential, &arm.ClientOptions{
		ClientOptions: env.clientOptions,
	})
}

func (env *azureEnviron) disksClient() (*armcompute.DisksClient, error) {
	return armcompute.NewDisksClient(env.subscriptionId, env.credential, &arm.ClientOptions{
		ClientOptions: env.clientOptions,
	})
}

func (env *azureEnviron) encryptionSetsClient() (*armcompute.DiskEncryptionSetsClient, error) {
	return armcompute.NewDiskEncryptionSetsClient(env.subscriptionId, env.credential, &arm.ClientOptions{
		ClientOptions: env.clientOptions,
	})
}

func (env *azureEnviron) imagesClient() (*armcompute.VirtualMachineImagesClient, error) {
	return armcompute.NewVirtualMachineImagesClient(env.subscriptionId, env.credential, &arm.ClientOptions{
		ClientOptions: env.clientOptions,
	})
}

func (env *azureEnviron) availabilitySetsClient() (*armcompute.AvailabilitySetsClient, error) {
	return armcompute.NewAvailabilitySetsClient(env.subscriptionId, env.credential, &arm.ClientOptions{
		ClientOptions: env.clientOptions,
	})
}

func (env *azureEnviron) resourceSKUsClient() (*armcompute.ResourceSKUsClient, error) {
	return armcompute.NewResourceSKUsClient(env.subscriptionId, env.credential, &arm.ClientOptions{
		ClientOptions: env.clientOptions,
	})
}

func (env *azureEnviron) resourcesClient() (*armresources.Client, error) {
	return armresources.NewClient(env.subscriptionId, env.credential, &arm.ClientOptions{
		ClientOptions: env.clientOptions,
	})
}

func (env *azureEnviron) providersClient() (*armresources.ProvidersClient, error) {
	return armresources.NewProvidersClient(env.subscriptionId, env.credential, &arm.ClientOptions{
		ClientOptions: env.clientOptions,
	})
}

func (env *azureEnviron) vaultsClient() (*armkeyvault.VaultsClient, error) {
	return armkeyvault.NewVaultsClient(env.subscriptionId, env.credential, &arm.ClientOptions{
		ClientOptions: env.clientOptions,
	})

}

func (env *azureEnviron) publicAddressesClient() (*armnetwork.PublicIPAddressesClient, error) {
	return armnetwork.NewPublicIPAddressesClient(env.subscriptionId, env.credential, &arm.ClientOptions{
		ClientOptions: env.clientOptions,
	})
}

func (env *azureEnviron) interfacesClient() (*armnetwork.InterfacesClient, error) {
	return armnetwork.NewInterfacesClient(env.subscriptionId, env.credential, &arm.ClientOptions{
		ClientOptions: env.clientOptions,
	})
}

func (env *azureEnviron) subnetsClient() (*armnetwork.SubnetsClient, error) {
	return armnetwork.NewSubnetsClient(env.subscriptionId, env.credential, &arm.ClientOptions{
		ClientOptions: env.clientOptions,
	})
}

func (env *azureEnviron) securityRulesClient() (*armnetwork.SecurityRulesClient, error) {
	return armnetwork.NewSecurityRulesClient(env.subscriptionId, env.credential, &arm.ClientOptions{
		ClientOptions: env.clientOptions,
	})
}

func (env *azureEnviron) securityGroupsClient() (*armnetwork.SecurityGroupsClient, error) {
	return armnetwork.NewSecurityGroupsClient(env.subscriptionId, env.credential, &arm.ClientOptions{
		ClientOptions: env.clientOptions,
	})
}

func (env *azureEnviron) roleDefinitionClient() (*armauthorization.RoleDefinitionsClient, error) {
	return armauthorization.NewRoleDefinitionsClient(env.credential, &arm.ClientOptions{
		ClientOptions: env.clientOptions,
	})
}

func (env *azureEnviron) roleAssignmentClient() (*armauthorization.RoleAssignmentsClient, error) {
	return armauthorization.NewRoleAssignmentsClient(env.subscriptionId, env.credential, &arm.ClientOptions{
		ClientOptions: env.clientOptions,
	})
}
