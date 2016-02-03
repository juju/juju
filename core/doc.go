// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

/*
Package core exists to hold concepts and pure logic pertaining to juju's domain.
We'd call it "model code" if we weren't planning to rename "environ" to "model";
but that'd be quite needlessly confusing, so "core" it is.

This is a necessarily broad brush; if anything, it's mmost important to be aware
what should *not* go here. In particular:

  * if it makes any reference to MongoDB, it should not be in here.
  * if it's in any way concerned with API transport, or serialization, it should
    not be in here.
  * if it has to do with the *specifics* of any resource *substrate* (compute,
    storage, networking, ...) it should not be in here.

...and more generally, when adding to core:

  * it's fine to import from any subpackage of "github.com/juju/juju/core"
  * but *never* import from any *other* subpackage of "github.com/juju/juju"
  * don't you *dare* introduce mutable global state or I will hunt you down
  * like a dog

...although, of course, *moving* code into core is great, so long as you don't
drag in forbidden concerns as you do so. At first glance, the following packages
are good candidates for near-term corification:

  * constraints (only dependency is instance)
  * instance (only dependency is network)
  * network (already core-safe)
  * watcher-excluding-legacy (only depends on worker[/catacomb])
  * worker/catacomb
  * worker/dependency
  * worker-excluding-other-subpackages

...and these have significant core-worthy content, but will be harder to extract:

  * environs[/config]-excluding-registry
  * storage-excluding-registry (depends only on instance and environs/config)
  * workload

...and, last but most, state, which deserves especially detailed consideration,
because:

  * it is by *far* the largest repository of business logic.
  * much of the business logic is horribly entangled with mgo concerns
  * plenty of bits -- pure model validation bits, status stuff, unit/machine
    assignment rules, probably a thousand more -- will be easy to extract

...but plenty of other bits will *not* be easy: in particular, all the business
rules that concern consistency are really tricky, and somewhat dangerous, to
extract, because (while those rules and relationshipps *are* business logic) we
need to be able to *render* them into a mgo/txn representation to ensure DB
consistency. If we just depend on implementing the state bits to match, rather
than *use*, the core logic, we're basically completely screwed.

The one place we address these concerns is in the core/lease.Token interface,
which includes functionality for communicating with the implementation of
lease.Client currently in play; where the state code which is responsible for
creating a mongo-based client is not entirely unjustified in making use of the
trapdoor to extract mgo.txn operations from lease.Token~s passed back in.

There's probably some sort of generally-useful abstraction to be extracted there,
but I'm not sure what it is yet.
*/
package core
