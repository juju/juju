// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

/*
Package globalclock provides clients for updating and reading the
global virtual time, stored in the MongoDB database.

Multiple global clock updaters may run concurrently, but concurrent
updates will fail. This simplifies failover in a multi-node controller,
while preserving the invariant that a global clock second is at least
as long as a wall-clock second.

Schema design
-------------

We maintain a single collection, with a single document containing
the current global time. Whenever time is to be advanced, we update
the document while ensuring that the global time has not advanced by
any other updater.
*/
package globalclock
