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
// The provider creates each model, including the controller model, in a separate resource group.
// The resource group is named after the model using the pattern juju-<model name>-uuid and
// contains all artefacts for the model, including:
//   - virtual machines
//   - disks
//   - networks and subnets
//   - security groups and rules
//   - public IP addresses
//   - availability sets
//   - key vaults
//
// When a user destroys a model, the provider deletes the model's resource group.
//
// # Provisioning resources
//
// During bootstrap, a deployment API client is used to deploy an [Azure resource manager]
// template which contains all compute, network, and storage resources for the controller.
// After bootstrap, the provider creates API clients for the resource types managed by Juju.
// All API clients use the same options which define the retry and logging behaviour.
//
// # Resiliency
//
// Unlike most other providers, the Azure provider does not currently support availability zones.
// Instead, for each application, the provider creates an [Azure availability set] named after the application.
//
// When a machine is created to host a unit of the application, the machine will join that availability set.
// Azure ensures that machines in an availability set are not automatically rebooted at the same time (i.e. for
// infrastructure upgrades), and are allocated to redundant hardware, to avoid faults bringing down all
// application units simultaneously.
//
// At the same time, it is important to note that availability sets are scoped to a single Azure region.
// Thus, they are designed to protect against failures within that region but do not provide protection
// against a regional outage.
//
// Availability sets are similar to "availability zones", but dissimilar enough that they do not fit into
// Juju's abstraction of zones. In particular, charms cannot query what "zone" they are in on Azure.
//
// # Instances
//
// In Azure, Juju machines are represented by virtual machine instances. Due to Azure requirements,
// there are some peculiarities relating to the listing and deletion of instances that requires
// some explanation. To prevent leaking resources, the provider must continue to report instances
// until all of the associated resources are deleted: VM, NIC, public IP address, etc. The most
// obvious thing to do would be to delete the VM last, but this is not possible. A VM must have at
// least one NIC attached; it is not possible to delete a NIC while it is attached to a VM.
// Thus the NICs must be deleted after the VM. When we delete an instance, we first delete the VM
// and then the remaining resources. We leave the NICs last, and tag NICs with the name (instance ID)
// of the machines they were created for, so that their presence indicates the presence of an
// instance in spite of there being no corresponding Virtual Machine.
//
// # Networking
//
// Each model has its own [Azure virtual network] called (by default) "juju-internal-network", and a
// single 10.0.0.0/16 subnet within that network called (by default) "juju-internal-subnet". Note that
// these networks are not routable between models; Juju agents will communicate with the controllers
// using their public addresses. Each machine is created with a single NIC, attached to the internal
// subnet. Unless the "allocate-public-ip" constraint is set to false, the NIC is assigned
// an [Azure public IP address].
//
// # Exposing applications
//
// Each model is given its own [Azure network security group] called "juju-internal-nsg", attached
// to the model's [Azure virtual network]. The rules provide the allowed ingress to model applications
// according to what ports should be opened once the application is exposed.
//
// # Encrypted disks
//
// Where an encrypted disk is required for workload storage, the provider creates an
// [Azure disk encryption set] and [Azure key vault] according to the requirements
// of the Juju storage pool created to define the encrypted disk configuration.
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
