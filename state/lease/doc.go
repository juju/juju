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

  * Time passes at approximately the same rate for all clients. (Note that
    the clients do *not* have to agree what time it is, or what time zone
    anyone is in: just that 1s == 1s. This is likely to be true already.)

So long as the above holds true, the following statements will too:

  * A successful ClaimLease guarantees lease ownership until *at least* the
    requested duration after the start of the call. (It does *not* guaranntee
    any sort of timely expiry.)

  * A successful ExtendLease makes the same guarantees. (In particular, note
    that this cannot cause a lease to be shortened; but that success may
    indicate ownership is guaranteed for longer than requested.)

  * ExpireLease will only succeed when the most recent writer of the lease is
    known to believe the time is after the expiry time it wrote.

When expiring a lease (or determining whether it needs to be extended) we only
need to care about the writer, because everybody else is determining their own
skew relative only to the writer. That is, assuming 3 clients:

 1) knows "the real time"; wrote a lease at 01:00:00, expiring at 01:00:30
 2) is 20 seconds ahead; read the lease between 01:00:23 and 01:00:24
 3) is 5 seconds behind; read the lease between 00:59:56 and 00:59:57

x






*/
package lease
