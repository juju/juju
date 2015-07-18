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
    anyone is in: just that 1s == 1s. This is likely to be true already if
    you use lease.SystemClock{}.)

So long as the above holds true, the following statements will too:

  * A successful ClaimLease guarantees lease ownership until *at least* the
    requested duration after the start of the call. (It does *not* guaranntee
    any sort of timely expiry.)

  * A successful ExtendLease makes the same guarantees. (In particular, note
    that this cannot cause a lease to be shortened; but that success may
    indicate ownership is guaranteed for longer than requested.)

  * ExpireLease will only succeed when the most recent writer of the lease is
    known to believe the time is after the expiry time it wrote.


Remarks on clock skew
---------------------

When expiring a lease (or determining whether it needs to be extended) we only
need to care about the writer, because everybody else is determining their own
skew relative only to the writer. That is, assuming 3 clients:

 A) knows "the real time"; wrote a lease at 01:00:00, expiring at 01:00:30A
 B) is 20 seconds ahead; read the lease between 01:00:23B and 01:00:24B
 C) is 5 seconds behind; read the lease between 00:59:57C and 00:59:58C

...then B cannot infer an expiry time earlier than 01:00:54L (=01:00:34A) and
C cannot infer an expiry time earlier than 01:00:28C (=01:00:33A). If A fails
to expire its lease, then C will trigger first and try to expire it, and most
likely succeed; and when C succeeds, B's subsequent attempt to expire the
lease will certainly fail, because C has updated both the clock document and
the lease document and invalidated B's assertions.

So B can and does then Refresh; and sees the lease document written by C, and
now needs only to consider its offset relative to C in order to Do The Right
Thing.


Schema design
-------------

For each namespace, we store a single clock document; and one additional
document per lease. The lease document holds the name, holder, expiry, and
writer of the lease; the clock document contains the most recent time
acknowledged by each client that has written to the namespace.

Every transaction that the lease package makes is gated on a write to the
clock document (which *must* precede any lease operations) which acks a
recent time and fails if it appears to be going backward in time (this could
happen if we crashed at the wrong moment and left a transaction queued but
unprepared for some time: we definitely don't want to accept those operations).

The fact that the clock document is involved in every transaction renders it a
per-namespace bottleneck, but the ability to discard outdated transactions is
valuable; and the centralised record of acknowledged times mitigates the impact
of client failure.

That is to say: assuming client C wrote lease L at time T, and wrote lease M
at time U (later than T); and then failed; then a fresh client D will be able
to expire lease L earlier (by U-T) than it could infer with the information in
lease L alone.

(We could ofc still calculate that by storing a written time in each lease
document, but it'd be more hassle to collate the data, harder to inspect the
database, and would only be able to make much weaker anti-time-travel promises
than we can manage with the clock doc.)


Client usage considerations
---------------------------

  * Client operates at a relatively low level of abstraction. Claiming a held
    lease will fail, even on behalf of the holder; expiring an expired lease
    will fail; but at least we can allow lease extensions to race benignly,
    because they don't involve ownership change and thus can't break promises
    (so long as our skew logic is correct).

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


*/
package lease
