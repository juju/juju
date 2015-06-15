// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

/*

The lease package exists to implement distributed lease management on top of
mgo/txn, and to expose assert operations that allow us to gate other mgo/txn
transactions on lease state.

The core type is the Client, which is implemented such that:

  * multiple clients can collaborate so as not to disagree on the global truth
    of statements about leases, and no client will act to break a guarantee made
    by any other client
  * leases are namespaced, such that disparate sets of clients can use the same
    collection without interfering with one another





*/
package lease
