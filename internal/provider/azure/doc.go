// Copyright 2024 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package azure implements the Azure provider, registered with the
// environs registry under the name "azure". The provider implements
// the [github.com/juju/juju/environs.Environ] interface, which defines
// methods for provisioning compute, network, and storage resources.
//
// This document describes some key implementation details specific to the Azure provider.
//
// # SDK
//
// The provider implementation is built using the [Azure SDK].
//
// # Models
//
// The provider creates all models, including the controller model, in a resource group.
// The resource group contains all artefacts for the model, including:
//   - virtual machines
//   - disks
//   - networks and subnets
//   - security groups and rules
//   - public IP addresses
//   - availability sets
//   - key vaults
//
// # Provisioning resources
//
// During bootstrap, a deployment API client is used to deploy an [Azure resource manager]
// template which contains all compute, network, and storage resources for the controller.
// After bootstrap, the provider creates API clients for particular resource types defined in the SDK.
// All API clients use the same options which define the retry and logging behaviour.
//
// # Resiliency
//
// Unlike most other providers, the Azure provider does not currently support availability zones.
// Instead, the provider creates for each application an [Azure availability set] named after the application.
// Availability sets are scoped to a single Azure region. They are designed to protect against failures
// within that region but do not provide protection against a regional outage.
//
// # Machine addresses
//
// Unless the "allocate-public-ip" constraint is set to false, each virtual machine
// is assigned an [Azure public IP address],
//
// # Encrypted Disks
//
// Where an encrypted disk is required for workload storage, the provider creates an
// [Azure disk encryption set] and [Azure key vault] according to the requirements
// of the Juju storage pool created to define the encrypted disk configuration.
//
// # Exposing applications
//
// The provider adds rules to the [Azure network security group] attached to the model's
// [Azure virtual network]. The rules provide the allowed ingress to model applications
// according to what ports should be opened once the application is exposed.
//
// [Azure SDK]: https://github.com/Azure/azure-sdk-for-go
// [Azure resource manager]: https://learn.microsoft.com/en-us/azure/azure-resource-manager/management/overview
// [Azure availability set]: https://learn.microsoft.com/en-us/azure/virtual-machines/availability-set-overview
// [Azure disk encryption set]: https://learn.microsoft.com/en-us/azure/virtual-machines/disk-encryption
// [Azure key vault]: https://learn.microsoft.com/en-us/azure/key-vault/general/overview
// [Azure virtual network]: https://learn.microsoft.com/en-us/azure/virtual-network/virtual-networks-overview
// [Azure network security group]: https://learn.microsoft.com/en-us/azure/virtual-network/network-security-groups-overview
// [Azure public IP address]: https://learn.microsoft.com/en-us/azure/virtual-network/ip-services/public-ip-addresses
package azure
