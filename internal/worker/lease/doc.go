// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// Package lease, also known as the manager, manages the leases used by
// individual Juju workers.
//
// Workers will claim a lease, and they are either attributed (i.e., the workers
// gets the lease ) or blocked (i.e., the worker is waiting for a lease to
// become available).
// In the latter case, the manager will keep track of all the blocked claims.
// When a worker's lease expires or gets revoked, then the manager will
// re-attribute it to one of other workers, thus unblocking them and satisfying
// their claim.
// In the special case where a worker is upgrading an application, it will ask
// the manager to "pin" the lease. This means that the lease will not expire or
// be revoked during the upgrade, and the validity of the lease will get
// refreshed once the upgrade has completed. The overall effect is that the
// application unit does not lose leadership during an upgrade.
package lease
