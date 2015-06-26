// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

/*

The dependency package exists to address a general problem with shared resources
and the management of their lifetimes. Many kinds of software handle these issues
with more or less felicity, but it's particularly important the juju (a distributed
system that needs to be very fault-tolerant) handle them clearly and sanely.

Background
----------

A cursory examination of the various workers run in juju agents (as of 2015-04-20)
reveals a distressing range of approaches to the shared resource problem. A
sampling of techniques (and their various problems) follows:

  * enforce sharing in code structure, either directly via scoping or implicitly
    via nested runners (state/api conns; agent config)
      * code structure is inflexible, and it enforces strictly nested resource
        lifetimes, which are not always adequate.
  * just create N of them and hope it works out OK (environs)
      * creating N prevents us from, e.g., using a single connection to an environ
        and sanely rate-limiting ourselves.
  * use filesystem locking across processes (machine execution lock)
      * implementation sometimes flakes out, or is used improperly; and multiple
        agents *are* a problem anyway, but even if we're all in-process we'll need
        some shared machine lock...
  * wrap workers to start up only when some condition is met (post-upgrade
    stability -- itself also a shared resource)
      * lifetime-nesting comments apply here again; *and* it makes it harder to
        follow the code.
  * implement a singleton (lease manager)
      * singletons make it *even harder* to figure out what's going on -- they're
        basically just fancy globals, and have all the associated problems with,
        e.g. deadlocking due to unexpected shutdown order.

...but, of course, they all have their various advantages:

  * Of the approaches, the first is the most reliable by far. Despite the
    inflexibility, there's a clear and comprehensible model in play that has yet
    to cause serious confusion: each worker is created with its resource(s)
    directly available in code scope, and trusts that it will be restarted by an
    independent watchdog if one of its dependencies fails. This characteristic is
    extremely beneficial and must be preserved; we just need it to be more
    generally applicable.

  * The create-N-Environs approach is valuable because it can be simply (if
    inelegantly) integrated with its dependent worker, and a changed Environ
    does not cause the whole dependent to fall over (unless the change is itself
    bad). The former characteristic is a subtle trap (we shouldn't be baking
    dependency-management complexity into the cores of our workers' select loops,
    even if it is "simple" to do so), but the latter is important: in particular,
    firewaller and provisioner are distressingly heavyweight workers and it would
    be unwise to take an approach that led to them being restarted when not
    necessary.

  * The filesystem locking just should not happen -- and we need to integrate the
    unit and machine agents to eliminate it (and for other reasons too) so we
    should give some thought to the fact that we'll be shuffling these dependencies
    around pretty hard in the future. If the approach can make that task easier,
    then great.

  * The singleton is dangerous specifically because its dependency interactions are
    unclear. Absolute clarity of dependencies, as provided by the nesting approaches,
    is in fact critical.

The various nesting approaches give easy access to directly-available resources,
which is great, but will fail as soon as you have a sufficiently sophisticated
dependent that can operate usefully without all its dependencies being satisfied
(we have a couple of requirements for this in the unit agent right now). Still,
direct resource access *is* tremendously convenient, and we need some way to
access one service from another.

However, all of these resources are very different: for a solution that encompasses
them all, you kinda have to represent them as interface{} at some point, and that's
very risky re: clarity.


Problem
-------

The package is intended to implement the following developer stories:

  * As a developer, I want to provide a service provided by some worker to one or
    more client workers.
  * As a developer, I want to write a service that consumes one or more other
    workers' services.
  * As a developer, I want to choose how I respond to missing dependencies.
  * As a developer, I want to be able to inject test doubles for my dependencies.
  * As a developer, I want control over how my service is exposed to others.
  * As a developer, I don't want to have to typecast my dependencies from
    interface{} myself.
  * As a developer, I want my service to be restarted if its dependencies change.

That last one might bear a little bit of explanation: but I contend that it's the
only reliable approach to writing resilient services that compose sanely into a
comprehensible system. Consider:

  * Juju agents' lifetimes must be assumed to exceed the MTBR of the systems
    they're deployed on; you might naively think that hard reboots are "rare"...
    but they're not. They really are just a feature of the terrain we have to
    traverse. Therefore every worker *always* has to be capable of picking itself
    back up from scratch and continuing sanely. That is, we're not imposing a new
    expectation: we're just working within the existing constraints.
  * While some workers are simple, some are decidedly not; when a worker has any
    more complexity than "none" it is a Bad Idea to mix dependency-management
    concerns into their core logic: it creates the sort of morass in which subtle
    bugs thrive.

So, we take advantage of the expected bounce-resilience, and excise all dependency
management concerns from the existing ones... in favour of a system that bounces
workers slightly more often than before, and thus exercises those code paths more;
so, when there are bugs, we're more likely to shake them out in automated testing
before they hit users.

We'd also like to implement these stories, which go together, and should be
added when their absence becomes inconvenient:

  * As a developer, I want to be prevented from introducing dependency cycles
    into my application. [NOT DONE]
  * As a developer trying to understand the codebase, I want to know what workers
    are running in an agent at any given time. [NOT DONE]
  * As a developer, I want to add and remove groups of workers atomically, e.g.
    when starting the set of state-server workers for a hosted environ; or when
    starting the set of workers used by a single unit. [NOT DONE]


Solution
--------

Run a single dependency.Engine at the top level of each agent; express every
shared resource, and every worker that uses one, as a dependency.Manifold; and
install them all into the top-level engine.

When installed under some name, a dependency.Manifold represents the features of
a node in the engine's dependency graph. It lists:

  * The names of its dependencies (Inputs).
  * How to create the worker representing the resource (Start).
  * How (if at all) to expose the resource as a service to other resources that
    know it by name (Output).

...and allows the developers of each independent service a common mechanism for
declaring and accessing their dependencies, and the ability to assume that they
will be restarted whenever there is a material change to their accessible
dependencies.


Usage
-----

In each worker package, write a `manifold.go` containing the following:

    type ManifoldConfig struct {
        // The names of the various dependencies, e.g.
        APICallerName   string
        MachineLockName string
    }

    func Manifold(config ManifoldConfig) dependency.Manifold {
        // Your code here...
    }

...and take care to construct your manifolds *only* via that function; *all*
your dependencies *must* be declared in your ManifoldConfig, and *must* be
accessed via those names. Don't hardcode anything, please.

If you find yourself using the same manifold configuration in several places,
consider adding helpers to worker/util, which includes mechanisms for simple
definition of manifolds that depend on an API caller; on an agent; or on both.


Concerns and mitigations thereof
--------------------------------

The dependency package will *not* provide the following features:

  * Deterministic worker startup. As above, this is a blessing in disguise: if
    your workers have a problem with this, they're using magical undeclared
    dependencies and we get to see the inevitable bugs sooner.
    TODO(fwereade): we should add fuzz to the bounce and restart durations to
    more vigorously shake out the bugs...
  * Hand-holding for developers writing Output funcs; the onus is on you to
    document what you expose; produce useful error messages when they supplied
    with unexpected types via the interface{} param; and NOT to panic. The onus
    on your clients is only to read your docs and handle the errors you might
    emit.

*/
package dependency
