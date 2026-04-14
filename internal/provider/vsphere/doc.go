// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package vsphere implements the VMware vSphere provider, registered with
// the environs registry under the name "vsphere".
//
// The vSphere provider targets an existing VMware environment rather than a
// cloud-native API. It communicates with a vCenter or ESXi endpoint via
// govmomi. Provisioning is driven by cloning VM templates that the provider
// downloads and caches on the vSphere datastore.
//
// See github.com/juju/juju/internal/provider/common for functionality common
// to all providers. See github.com/juju/juju/internal/provider for other
// providers. See github.com/juju/juju/environs for provider interfaces. See
// the sections below for vSphere package-wide behavior boundaries.
//
// # How the vsphere provider differs from other providers
//
//   - VMware environment targeting:
//     vSphere targets an existing VMware environment (vCenter or ESXi)
//     rather than a public cloud API.
//
//   - Template-driven provisioning:
//     Instances are created by cloning VM templates (VMDK/OVF). Templates
//     are downloaded once per Ubuntu release and cached in a controller-
//     scoped datastore directory for reuse.
//
//   - No provider-managed networking:
//     vSphere attaches VMs to existing port groups. Firewall operations
//     are not supported.
//
//   - vSphere-native topology:
//     Placement uses vSphere concepts -- datacenters, compute resources,
//     datastores, resource pools, and port groups.
//
//   - Session-scoped API access:
//     Each exported operation dials a new vCenter/ESXi session via
//     sessionEnviron, scoped to the lifetime of that call.
//
// # Configuration
//
// No configuration fields are required beyond the standard cloud endpoint
// and credentials. The following provider-specific model config keys are
// defined in config.go:
//
//   - primary-network: the primary port group VMs connect to. Defaults to
//     "VM Network" if not set.
//   - external-network: an additional port group whose resulting IP is
//     reported as the VM's public address.
//   - datastore: the datastore in which VMs are created. If not set,
//     provisioning fails unless exactly one datastore is available.
//   - force-vm-hardware-version: the HW compatibility version applied when
//     cloning a template. Must be >= the template version and supported by
//     the target compute resource.
//   - enable-disk-uuid: exposes consistent disk UUIDs to the VM
//     (disk.EnableUUID). Defaults to true.
//   - disk-provisioning-type: how the disk is provisioned during clone.
//     Allowed values are thickEagerZero (default), thick, and thin.
//
// # Instances and images
//
// Instances are created by cloning a VMDK template stored in the vSphere
// datastore. For each supported Ubuntu release the provider downloads an
// OVF/VMDK template, prepares it, and caches it under a controller-scoped
// templates directory. Subsequent deployments clone from the cached template.
//
// Because templates are cached and not automatically refreshed, apt update
// and apt upgrade run during first boot, which increases deploy time as the
// template ages. Templates can be deleted from the vSphere to force a fresh
// download. Specifying pre-configured template paths in model config can
// reduce bootstrap and deploy times.
//
// VMs are organised into vSphere folders. The folder hierarchy is:
//
//	<datacenter>/vm/Juju Controller (<controller-uuid>)/
//	    Model "<model-name>" (<model-uuid>)/
//	    templates/
//
// # Networking
//
// The vSphere provider does not manage networking resources. VMs are
// attached to existing port groups. The primary-network config key selects
// the primary port group; external-network selects an optional second port
// group whose IP is reported as the public address. Firewall operations are
// not supported. Network interface data for the instance-poller worker is
// sourced from the machine agent.
//
// # Instances and images
//
// Instances are created by cloning a VMDK template stored in the vSphere
// datastore. Templates are cached under a controller-scoped directory and
// reused across deployments.
//
// VMs are organised into vSphere folders per controller and model.
//
// # Storage
//
// Storage provider types and pool management delegate to the common IAAS
// storage helpers in internal/provider/common.
//
// # Regions and Availability Zones
//
// Regions map to vSphere datacenters, configured as the cloud region in
// the Juju cloud definition. Availability zones map to resource pools
// within compute resources in the datacenter. Zone names are derived from
// the vSphere inventory path relative to the host folder, with the
// "Resources" path segment stripped. Zone information is cached per
// session to reduce API calls.
//
// # Maintainer notes
//
// When changing vSphere provider behavior, preserve these invariants:
//
//   - provisioning depends on template caching; changes to template
//     discovery, download, or clone logic affect all deployments
//   - the provider does not manage networking; do not add firewall or
//     network allocation logic
//   - vSphere folder names have a maximum length of 80 characters; the
//     model name is truncated to 33 characters to stay within this limit
//   - storage delegates entirely to common IAAS storage helpers
package vsphere
