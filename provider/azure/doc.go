// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package azure implements the Azure provider, registered with
// environs under the name "azure". The provider implements the
// [github.com/juju/juju/environs.Environ] interface, which defines
// methods for provisioning compute, network, and storage resources.
//
// Here we describe some key implementation details specific to the Azure provider.
//
// # SDK
//
// The provider implementation is built using the [Azure SDK].
//
// # Credential Types
//
// The supported credential types are:
//   - service-principal-secret
//   - managed-identity
//
// The recommended way to create a credential with which to bootstrap is
// to run
//
//	juju add-credential azure
//
// and follow the prompts. Choosing "interactive" is the best choice for
// setting up a service principal credential as the browser is used to
// log into an Azure account and the credential attributes are filled
// in automatically.
//
// # Resource Groups
//
// All models, including the controller model, are created in a resource group.
// The resource group contains all artefacts for the model, including:
//   - virtual machines
//   - disks
//   - networks and subnets
//   - security groups and rules
//   - public IP addresses
//   - availability sets
//   - key vaults
//
// # Provisioning Resources
//
// During bootstrap, a deployment API client is used to deploy an [Azure Resource Manager]
// template which contains all compute, network, and storage resources for the controller.
// After bootstrap, API clients are created for particular resource types defined in the SDK.
// All API clients use the same options which define the retry and logging behaviour.
//
// # Availability Sets
//
// Each application has created for it an [Azure Availability Set] named after the application.
//
// # Machine Addresses
//
// Each virtual machine is assigned an [Azure Public IP Address] by default, unless the
// "allocate-public-ip" constraint is set to false.
//
// # Encrypted Disks
//
// Where an encrypted disk is required for workload storage, an [Azure Disk Encryption Set] and
// [Azure Key Vault] is created according to the requirements of the Juju storage pool created
// to define the encrypted disk configuration.
//
// # Exposing Applications
//
// The [Azure Network Security Group] attached to the model's [Azure Virtual Network] has rules added as
// required to provide the allowed ingress to model applications according to what ports should
// be opened once the application is exposed.
//
// [Azure SDK]: https://github.com/Azure/azure-sdk-for-go
// [Azure Resource Manager]: https://learn.microsoft.com/en-us/azure/azure-resource-manager/management/overview
// [Azure Availability Set]: https://learn.microsoft.com/en-us/azure/virtual-machines/availability-set-overview
// [Azure Disk Encryption Set]: https://learn.microsoft.com/en-us/azure/virtual-machines/disk-encryption
// [Azure Key Vault]: https://learn.microsoft.com/en-us/azure/key-vault/general/overview
// [Azure Virtual Network]: https://learn.microsoft.com/en-us/azure/virtual-network/virtual-networks-overview
// [Azure Network Security Group]: https://learn.microsoft.com/en-us/azure/virtual-network/network-security-groups-overview
// [Azure Public IP Address]: https://learn.microsoft.com/en-us/azure/virtual-network/ip-services/public-ip-addresses
package azure
