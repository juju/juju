// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

/*

The lease package exists to implement distributed lease management on top of
mgo/txn, and to expose assert operations that allow us to gate other mgo/txn
transactions on lease state. This necessity has affected the package; but,
apart from leaking assertion operations, it functions as a distributed lease-
management system with various useful properties.

These properties of course rest upon assumptions; ensuring the validity of the
following statements is the job of the client.

  * The lease package has exclusive access to any collection it's configured
    to use. (Please don't store anything else in there.)

  * Given any (collection,namespace) pair, any client Id will be unique at any
    given point in time. (Run no more than one per namespace per server, and
    identify them according to where they're running).

  * Global time (i.e. the time returned by the provided global clock) passes
    no faster than wall-clock time.

So long as the above holds true, the following statements will too:

  * A successful ClaimLease guarantees lease ownership until *at least* the
    requested duration after the start of the call. (It does *not* guaranntee
    any sort of timely expiry.)

  * A successful ExtendLease makes the same guarantees. (In particular, note
    that this cannot cause a lease to be shortened; but that success may
    indicate ownership is guaranteed for longer than requested.)

  * ExpireLease will only succeed when the global time is after the
    (start+duration) written to the lease.


Schema design
-------------

For each namespace, we store a single document per lease. The lease document
holds the name, holder, global start time, duration, and writer of the lease.


Client usage considerations
---------------------------

  * Client operates at a relatively low level of abstraction. Claiming a held
    lease will fail, even on behalf of the holder; expiring an expired lease
    will fail; but at least we can allow lease extensions to race benignly,
    because they don't involve ownership change and thus can't break promises.

  * ErrInvalid is normal and expected; you should never pass that on to your
    own clients, because it indicates that you tried to manipulate the client
    in an impossible way. You can and should inspect Leases() and figure out
    what to do instead; that may well be "return an error", but please be sure
    to return your own error, suitable for your own level of abstraction.

  * You *probably* shouldn't ever need to actually call Refresh. It's perfectly
    safe to let state drift arbitrarily far out of sync; when you try to run
    operations, you will either succeed by luck despite your aged cache... or,
    if you fail, you'll get ErrInvalid and a fresh cache to inspect to find out
    recent state.

  * The expiry time returned via lease.Info is relative to the local system
    clock, not the global time. The expiry time should only be used for
    comparison with other local system clock times, and only with Go 1.9+
    (i.e. because Go 1.9 introduced monotonic time components.)

*/
package lease
