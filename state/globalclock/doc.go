// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

/*

Package globalclock provides clients for updating and observing the
global virtual time, stored in the MongoDB database.

Juju ensures that at most one agent is updating the global clock at
any one time. That agent will progress the global virtual time (GVT)
such that a GVT second is at least as long as a wall-clock second.

Schema design
-------------

We maintain a single, capped collection for storing documents containing
the current time. The collection permits a maximum of 2 documents, to
support tailing the collection without invalidating tailable iterators.

Whenever time is to be progressed, we insert a new document. The document
contains the current global virtual time.

*/
package globalclock
