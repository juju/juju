// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package lease manages lease allocation and lifecycle for Juju workers.
// See the sections below for details about this package.
//
// See github.com/juju/juju/core/lease for lease types and store interface.
//
// # How this package works
//
// **Leases**: Leases provide distributed locking for Juju workers that need
// exclusive access to resources. Common uses include leadership election for
// application units and singular worker coordination across controllers.
//
// **Lease claiming and attribution**: Workers claim leases through the manager.
// Claims are either attributed (the worker receives the lease immediately) or
// blocked (the worker waits for the lease to become available). The manager
// tracks blocked claims and automatically attributes leases to waiting workers
// when current leases expire or are revoked.
//
// **Lease expiry and reattribution**: When a lease expires or is explicitly
// revoked, the manager selects one of the blocked workers and attributes the
// lease to them. This ensures fair distribution and prevents lease starvation.
//
// **Lease pinning during upgrades**: During application upgrades, workers can
// request that their lease be pinned. Pinned leases do not expire or get revoked
// during the upgrade, and their validity is refreshed when the upgrade completes.
// This prevents application units from losing leadership mid-upgrade, which could
// cause upgrade failures or inconsistent state.
//
// # How to use this package correctly
//
// **Lifecycle**: The Manager implements worker.Worker and uses tomb for lifecycle
// management. Callers MUST call Kill() to initiate shutdown and Wait() to block
// until shutdown completes. The manager waits up to 55 seconds for in-flight
// operations to complete before forcing shutdown.
package lease
