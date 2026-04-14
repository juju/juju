// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package openstack implements the OpenStack provider, registered with the
// environs registry under the name "openstack".
//
// The provider communicates with OpenStack APIs via the goose client library
// (github.com/go-goose/goose). It uses Nova for instance lifecycle, Neutron
// for networking and firewalling, and Cinder for block storage. Image and
// agent metadata can be sourced from the Keystone service catalog in addition
// to the standard simplestreams sources.
//
// See github.com/juju/juju/internal/provider/common for functionality common
// to all providers. See github.com/juju/juju/internal/provider for other
// providers. See github.com/juju/juju/environs for all the interfaces a
// provider may implement.
//
// # How the openstack provider differs from other providers
//
//   - Keystone-based metadata discovery:
//     In addition to standard simplestreams sources, image and agent binary
//     metadata can be resolved from URLs published in the Keystone service
//     catalog ("product-streams" and "juju-tools" endpoints). These sources
//     are registered at package init time alongside the provider itself.
//
//   - Neutron-required networking:
//     The provider requires OpenStack Queens or newer because it depends on
//     the Neutron networking service. Clouds without a "network" endpoint in
//     the Keystone service catalog are not supported. Networking and
//     firewalling are both implemented via Neutron.
//
//   - Pluggable provider behaviour:
//     EnvironProvider accepts a ProviderConfigurator, FirewallerFactory, and
//     FlavorFilter at construction, allowing embedders to customise cloud-init
//     configuration, security group management, and flavor selection without
//     forking the provider.
//
//   - Root disk source selection:
//     The root disk can be sourced from either a local ephemeral disk
//     ("local") or a Cinder block volume ("volume"). The "volume" source
//     uses a block device mapping at boot time and defaults to 30 GiB.
//
// # Configuration
//
// No configuration fields are required beyond the standard cloud endpoint
// and credentials. The following provider-specific model config keys are
// defined in config.go:
//
//   - network: label or UUID of the Neutron network to attach instances to
//     when multiple networks exist.
//   - external-network: label or UUID of the external Neutron network used
//     to source floating IP addresses.
//   - use-default-secgroup: when true, assigns the OpenStack "default"
//     security group to new instances in addition to Juju-managed groups.
//     Defaults to false.
//   - use-openstack-gbp: when true, uses Neutron Group-Based Policy (GBP)
//     for instance networking. Defaults to false.
//   - policy-target-group: UUID of the GBP Policy Target Group to use when
//     use-openstack-gbp is true.
//
// Auth types supported: userpass (Keystone v2/v3) and access-key.
// Region can be detected from OS_REGION_NAME and OS_AUTH_URL environment
// variables.
//
// # Instances and images
//
// Instance types map to Nova flavors. Flavors are enumerated at constraint
// validation time and used to resolve instance-type constraints. A
// FlavorFilter can be provided to restrict which flavors are considered.
//
// Images are resolved via simplestreams, with an optional additional source
// from the Keystone "product-streams" catalog entry. Image selection is
// constrained by region, base OS, and architecture.
//
// The root disk is either local (Nova ephemeral) or a Cinder volume attached
// via block device mapping at boot. When root-disk-source=volume, a root disk
// constraint can be combined with an instance-type constraint.
//
// # Networking
//
// Networking is implemented exclusively via Neutron. At bootstrap, Juju
// verifies that the target cloud exposes a Neutron "network" endpoint;
// bootstrap fails if it does not.
//
// Instances are attached to one or more Neutron networks. When space
// constraints or endpoint bindings are present, the provider creates a Neutron
// port per subnet and attaches those ports to the new instance. Ports created
// for a failed instance are deleted before the error is returned.
//
// Floating IP addresses are optionally allocated and associated with instances
// when the allocate-public-ip constraint is set. A mutex is held across
// allocation and association to avoid duplicate assignment across concurrent
// StartInstance calls.
//
// Security groups are managed per machine via the Firewaller abstraction and
// cleaned up when instances are stopped. Model-wide and controller-wide groups
// are cleaned up on model and controller destruction.
//
// # Regions and Availability Zones
//
// Regions correspond to OpenStack regions and each has its own Keystone
// endpoint. Regions are defined in the Juju cloud configuration and can be
// auto-detected from the OS_REGION_NAME environment variable.
//
// Availability zones are sourced from the Nova API. Zone placement is
// constrained by volume attachment zones: all volumes being attached to a new
// instance must be in the same zone as the instance.
//
// # Storage
//
// Block storage is provided by Cinder via the "cinder" storage provider type
// (CinderProviderType). Volumes are created, attached, detached, and deleted
// through the Cinder API. Volume availability zones must be consistent with
// instance availability zones. An optional volume-type config key selects the
// Cinder volume type.
//
// # Maintainer notes
//
// When changing OpenStack provider behavior, preserve these invariants:
//
//   - the FirewallerFactory and ProviderConfigurator injections must remain
//     so that embedders can customise firewalling and cloud-init behavior
//   - Keystone metadata source registration in init() must stay in sync with
//     the provider registration
//   - root-disk-source=volume uses a block device mapping; changes to disk
//     configuration affect both local and volume root disk paths
//   - availability zone consistency between instances and volumes is
//     enforced at StartInstance time; do not relax this check
package openstack
