// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package instancepoller provides a worker that periodically polls the
// cloud provider for instance information and keeps Juju’s domain models
// up to date.
//
// ## Overview
//
// The instance poller is responsible for:
//
//   - Tracking machines that exist in a model and polling their backing
//     cloud instances for status and network interface information.
//   - Updating the cloud-specific instance status stored by Juju when the
//     provider-reported status changes (e.g. Pending → Running).
//   - Synchronising provider-sourced network interface information
//     (link-layer devices and their addresses) into Juju’s networking
//     domain to update provider-sourced ids.
//   - Adjusting polling cadence based on observed machine state so we poll
//     fast while a machine is coming up and slow down once it is stable.
//
// # Behaviour and polling strategy
//
// The worker maintains two polling groups:
//
//   - Short-poll group: machines without discovered addresses or that are
//     not yet fully started are polled frequently. The interval starts at
//     ShortPoll and is exponentially backed off (ShortPollBackoff) up to
//     ShortPollCap to avoid provider API churn while instances are
//     allocating/booting or temporarily missing.
//
//   - Long-poll group: once a machine is Started and has at least one
//     provider network interface/address, it is polled less frequently at
//     LongPoll intervals to detect drift in provider status or addresses.
//
// Machines can move between groups depending on the latest provider
// instance status and whether provider NICs are present. For example, a
// machine in the long-poll group that loses all provider NICs or whose
// provider status becomes Unknown will be moved back to the short-poll
// group for closer observation.
//
// # Services used by the worker and why
//
// The worker coordinates several services/interfaces.
//
//   - Environ: access to provider-specific datas, e.g. instances and network
//     interfaces to populate various provider ids, and status updates.
//
//   - StatusService: updates the instance status stored by Juju from provider
//     data
//
//   - MachineService: interacts with the machine changes to drive the polling queue,
//     and retrieves various information about the machines, as their life, method of provisionning,
//     and uuid and known devices.
//
//   - NetworkService: updates the network domain with the latest provider ids
//
// # Relationship with the machiner worker
//
// The instance poller and the machiner are complementary:
//
//   - The instance poller discovers link-layer devices and provider
//     addresses from the cloud provider (via Environ.NetworkInterfaces) and
//     writes them into the network domain using NetworkService.
//
//   - The machiner, running on each machine, reports locally observed
//     network devices and addresses. It is authoritative for what exists on
//     the guest OS and for non-provider addresses. The network domain then
//     reconciles the provider view (from the instance poller) with the
//     machine-reported view (from the machiner), ensuring that devices and
//     addresses are accurate and deduplicated.
//
// In practice, this means the instance poller is the source of provider
// topology (e.g., cloud NIC IDs, provider-assigned addresses), while the
// machiner fills in the OS-level details (names like eth0, additional
// addresses, MTUs). Both flows populate the same link-layer/device models
// so that Juju maintains a consistent and up-to-date network configuration
// for each machine.
package instancepoller
