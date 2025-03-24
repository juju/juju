(on-managing-complexity)=
# On managing complexity

Complexity kills software. Successfully developing software for the long term
is an exercise in managing complexity; once the whole picture becomes too big
to keep in your head, it becomes difficult to predict the ultimate impact of
even a simple change to a complex system.

This situation is dangerous; it manifests as fragile software, and makes it
very hard to estimate work accurately, and each of those is a serious problem
in its own right.


## Management from above

The most reassuring approach to managing complexity is to tie it down by sheer
weight of definition. Surely, if the desired system can be described, it can
then be coded to match the description? But:

1) The map never quite matches the territory.
2) If the map somehow *did* include all relevant features of the territory,
   it would be just as hard to hold it in your head.
3) ...and it would actually be *harder* to work with the description alone,
   because it's not even executable.
4) ...and then the market changes, the requirements change, and it only becomes
   harder to keep the whole thing in sync.

That's not to say that descriptions aren't valuable: they are. But their value
often lies in what is left out: the hyperfocus on a single view of the system
will elide the details in which the devils lie, and doesn't help a developer
to follow the subtle threads of cause and effect which trigger unintended
consequences.

And therefore it doesn't solve the big problems alluded to above, and we need
to look elsewhere.


## Management from below

So, what are these "subtle threads" that cause all the problems? They are,
literally, dependencies. Some of them are declared and thus relatively easy
to track (e.g. package dependencies); but some of them are much subtler, and
distressingly easy to introduce accidentally even when you know what to look
out for.

And this implies that there *will* always be these subtle threads creeping
through our systems: I don't believe there's a simple suite of techniques for
avoiding them completely, but there are a *bunch* of valuable heuristics that
we can apply; from the literature, from our experience in golang and other
languages, from descriptions and specifications, from historical accounts of
how X or Y component was developed...

...and in the absence of infallible guides, those heuristics are our best bet.
We make choices in the small that are *likely* to make our components cleanly
composable into comprehensible systems; and as we do this, imperfectly, we
observe our successes and failures (and others', if we're lucky); and we
improve our heuristics and gradually get better.

And, where possible, we serialize our own hard-developed heuristics into
advice for others; and we hope that the advice somehow helps our readers to
develop the good taste, mature judgement, and unreasoning paranoia that leads
to Good Code.


## Everything will fail

**TLDR:**

* Assume that your inputs will deliver pathological nonsense, and your
  clients will ignore any contract your code documents. Fail safe, in
  all such situations.
* You *will* see unknown errors; you cannot make *any* ironclad inferences
  in response; the only rational approach is to fail out (and build your
  algorithm to be robust in the face of repeated restarts at arbitrary
  times -- which will happen anyway regardless).
* Give your component as few responsibilities as possible, and restrict
  its capabilities as much as possible, to minimise the damage it can do
  when it goes wrong.
* Keep your concurrency handling out of your core logic. Each of those
  deserves care and attention, and deterministic testing of pathological
  situations.

**Background:**

Everything will fail. The hardware will fail; the network will fail; the power
will fail. Those are the easy ones. Your code will fail; your colleagues' code
will fail; the nexus of those failures will generate yet more failure. Then
some component gets an unknown error (that doesn't *really* indicate failure)
and drifts away from consensus reality, going on to issue poor instructions to
yet more components; and Bad Things eventuate.

Bitter experience teaches that we must write code in constant awareness of
these sad truths. Passing the tests on the happy path is not good enough;
before you're satisfied, you should be asking yourself:

* What will happen if my worker doesn't run?
* What will happen if several copies of my worker run at the same time?
* What won't happen if my worker doesn't run?
* Is there any point at which my worker can fail and be unable to recover?
* If I interact with any channel not wholly controlled by myself, how can
  that channel defy my expectations and break my code?
* What other parts of the system will react, or not react, if any of
  the above applies?
* What are *their* responsibilities? What reacts to *them*?
* What's the worst way that chain of components might misbehave?
* How can I avoid contributing to an ongoing failure, and hopefully render
  it less likely to be catastrophic?

Obviously, keeping track of the latter questions is a nightmare. For this
reason, testing your algorithms in isolation -- without allowing concurrency
concerns to creep in -- is critical. This allows you to deliberately subject
your code to pathological event patterns, and develop justified trust in its
safe behaviour under adverse conditions, before developing a concurrent harness
for it that marshals input and output over suitable channels...

...and then testing *that* harness for how it reacts to antisocial behaviour
on the parts of all its collaborators. Needless to say, this will be hard if
you don't keep your list of collaborators nice and short; but *everything*
will be hard if you don't do that.


### Streamline your dependencies


**TLDR:**

* Make your dependencies explicit.
    * This is IMPORTANT. The more hidden dependencies you have, the less
      helpful the following heuristics are.
* Per-file import count is a good heuristic for code quality.
    * A large group of very similar imports is probably fine.
    * Even a small number of very different imports is cause for concern.
* Per-package import count is a good heuristic for architectural quality.
    * Implicit/hidden dependencies contribute to this heuristic, but are
      very easy to miss, so they should be avoided wherever possible.
    * Per-file considerations also apply.

**Background:**

Our puny human brains can only think about a few things at once. To correctly
interact with a piece of code requires that you understand its relationships
with the rest of its system; when your import block -- a crude approximation
for your list of connected components -- becomes too long to see on one screen,
you definitely have a problem.

This applies in the large -- in a dependency graph of the system, it's the nodes
with many inputs that signal poor cohesion -- but also in the small, within a
package. Moving code between packages can be difficult; but moving code around
within a package -- between files, into new files -- is quick and easy, and
makes an appreciable difference to the comprehensibility of the code for you
and for everyone who comes after you (and will eventually contribute to any
package-level moves that come in the future).

Of course, package imports are not the full story. X code may implicitly depend
upon Y package by virtue of implementing an X interface supplied to X code; this
sort of dependency is largely harmless, so long as X is suitably paranoid about
its interfaces' implementors' potential bad behaviour (as it should be anyway).

But some implicit dependencies are very more insidious. In particular, any
package that exposes some mechanism for reading or writing global state is
worrisome, because it introduces a potential reverse dependency, in which *all*
that package's clients depend on all the package's other clients; and in which
the set of imports in distant code can control the eventual runtime behaviour.

The big problem with these constructs is that they masquerade as high-cohesion
nodes in the dependency graph (lots of clients, not many dependencies, what a
useful abstraction) but they act very differently. Some of those clients just use
the package, but some of them modify it, and there's no way to tell which
without inspecting all of them.


## Write in layers

**TLDR:**

* In an ideal world, you should be able to make a given change in a clear
  sequence of steps:
    * Start with the core model, and algorithms acting on the model types,
      and test it all in detail. No concurrency, no persistence, no apis:
      the business rules alone.
    * Figure out how to persistently represent the model in the database, and
      how to *reliably* apply *valid* changes to that representation.
    * Determine what capabilities the apiserver will need to expose, and
      implement suitable methods with pluggable authorization components.
    * Determine how to transiently represent the model over the wire, taking
      into account that future versions of the software WILL use different
      model types, and you'll have to accept old param formats forever.
    * Expose those methods via specific facades, supplying the tightest
      possible authorizations for the context.
    * Implement the api client layer (make it as thin as possible -- just
      convert wire types back into model types, taking compatibility into
      account).
    * ...and then you just use the pre-existing, well-tested, model-level
      algorithms to manipulate the world from the client side.
* In practice:
    * The persistence layer is ultimately the arbiter of business-logic-level
      consistency; any business rules you write elsewhere *must* be perfectly
      reflected here.
    * There's nothing stopping you Doing It Right at any other level
      (apiserver, params, apiclient)...
    * ...except the very last one, because that requires a storage-independent
      representation of the business rules, and that probably doesn't exist.
    * No easy answer. Keep reading.

**Background:**

The decision to tie the business rules to the storage layer was made very early
in the project's life, and has proven very hard to shake. The CLI and agents
originally all shared administrator access to the database, and the apiserver
layer alone was hard-won; and is, for all its imperfections, a great improvement
over what came before.

Layers are great, at least in theory. In practice, some layers insulate better
than others; and when a layer leaks or presents the wrong abstractions, the
separation of concerns begins to fail and the value of the layered model starts
to evaporate. Most of our layers have been introduced under time pressure; and in
parallel with overlapping development; and all of them have been misinterpreted
at one time of another, such that *all* the boundaries could do with firming up.

Nonetheless, the important layers and their intended responsibilities (and any
known fuzziness in the boundaries, and the reasons for same) are as follows.
Please consider the ultimate purpose of each piece of code you write, and do
your utmost to put it in the appropriate layer.


### Model

This layer does not exist in any concrete form. Elements of it are scattered at
the top level of the source tree (network, instance, etc) and probably elsewhere;
but they really ought to be collected together under a model directory that makes
*no* reference to any other concerns.

The benefits of doing this will accrue incrementally; the risk is that we will
fail to cleanly extract the model, and end up with divergent implementations of
the business rules. Therefore, for the present, make a point of extracting only
those business rules that can be validated in isolation; prefer to leave the
complex synchronisation inside state (or be aware that you're taking on a very
tricky project).


### Persistence

This layer is implemented in and under the top-level state package. It's not
just managing a representation of the business rules: it *is* the business rules.
Most of the package is concerned with carefully repeating the same logic in two
different domains: as in-memory checks on recent copies of mongodb documents,
and as mgo/txn-encoded assertions of those same facts. Needless to say, this
is very hard to keep track of.

It's also a problem that several parts of the database store model objects
directly (e.g. charm documents). This is really dangerous, because model
changes in apparently-unrelated packages (in extreme cases, not even part of
the same repository) can change our DB schema.

The obvious approach (to start wholesale extracting business rules from the
state package) is beset with pitfalls, because it inevitably involves moving
those two model representations further apart; and renders it functionally
impossible to keep the implementations in sync. We've already stepped onto
that path, but to double down on parallel implementations would doom us.

The only practical approach that seems likely to succeed is to incrementally
build a pure model representation of the business rules... and a *renderer*
for same, into a mgo/txn representation, such that all modifications to the
business rules are automatically reflected in the persistence logic.

Regardless: model and persistence are currently a single layer, and they're
very hard to untangle.

* If you see us storing model types without explicit conversion, fix it.
* If you see opportunities to extract business rules, investigate them if
  you like, but keep the changes small-scale to begin with.

### Services

...think "SOA", not so much juju services.

This layer is somewhat nascent; most of it lives in `apiserver/common`, and a
lot more code that should be part of it still sits in the other specific facade
implementations under `apiserver`. It's responsible for the model manipulation
mechanism (as opposed to policy, which is determined by the api requests as
authorized by the facade layer underneath).

The good bit is that the services layer manipulates model entities; the bad bit
is that the model entities are still persistence entities, but that's not an
especially serious restriction at this point -- it may prevent us from extracting
pure-model algorithms into the model layer, but this layer's responsibilities
are not especially onerous and it can bear the weight for now.

There's no good reason for most of the code in `apiserver/common` to be in a
single package. That's silly; we should have a bunch of subpackages broken up
according to the various capabilities they provide.


### Facades

...these are a bit like services as well, especially because a lot of them have
their service-layer implementations baked in directly. The job of a facade should
be twofold:

* expose a coherent set of capabilities, as implemented by the services layer,
  and tuned to the needs of some client.
* supply authorization information to the service implementations to prevent
  information from leaking or inappropriate changes from being applied.

...and it would probably be good if they were also to convert from param structs
to model entities before passing them up to the services layer. This wouldn't
play well with the extant embedding pattern (in which facade types just embed
auth-specialized service types -- accepting params types, not model types), but
that has other drawbacks anyway; in future, prefer to implement services in pure
model terms and make the facades responsible for parameter translation.

Oh, and they should more-or-less *always* be implemented to accept and return
arguments in bulk. Sooner or later their lack inevitably becomes inconvenient,
and changing facades is a hassle, because we need to support every version of
every facade we've ever used, for an uncomfortably long time.


### Params

Currently loosely organised under `apiserver/params`, this layer should have
almost no logic, but should contain static, immutable, unchanging representations
of the structs we send over the wire. It is NEVER ok to change a params struct;
it is never ok to use fields with types defined outside this layer.

You can (and will) add types; and you can change type names; but you can't change
field names, or field types. And, yeah, without logic it's not really a "layer"; it's
more a representation of the boundary between the facades and the api clients.


### Clients

Currently loosely organised under `api`, many of the client packages are a
dreadful mess. As noted above, the CLI and the agents were originally using
direct connections to MongoDB, and directly interacting with the entities
defined in the state package. In the interest of implementing an API layer
*without* rewriting almost everything in juju, we chose to implement the api
client code in such a way as to ape the existing state entities.

This was a dumb thing to do (for all that it may have been better than the
alternatives). Now we have all these "State" types that are really APIs, and
do nothing more than obfuscate the true capabilities of the facade, in the
interest of maintaining a remote-object-style approach to the code that
*itself* does us no favours.

Please make future api client code as thin as possible -- ideally just another
model/params translation layer, with version tracking -- and work to slim down
those that currently exist.


### Workers

Ultimately, workers should be manipulating pure model code, and communicating
with the api server in that language; today, they're pretty casual about using
params types willy-nilly. So long as they're not persisting those types, it's
not the worst problem in the codebase; still, think of the ideal worker as a
terribly narrow concurrency harness for a pure-model algorithm that was written
somewhere else.

As above, it's usually impractical to do this today, hence the widespread (ab?)
use of params structs for this purpose. Representing that data as true model
types, and running algorithms purely in those terms, will let us extract that
logic and write smaller, more reliable workers.


### Further thoughts

The terrifyingly tight coupling of the model objects to their persistence
concerns has an impact that distorts the whole codebase. We can, and should,
work to solidify the existing boundaries as we see opportunities to do so;
but it's clear that the largest impact we can have on global quality is to
extract business rules and types into model packages, and develop techniques
that let us reliably render them for mgo/txn persistence.


## Know your field

**TLDR:**

* Lots of smart people have written lots of smart things about software.
* Also, lots of dumb things. Read lots of both.
* Learn the difference. Cultivate good taste. If possible, evolve into
  pure energy and ascend to a higher state of being.

**Background:**

Seriously. Here's a ludicrously incomplete list of things worth reading.

* your beloved and well-thumbed copies of Clean Code; Code Complete; Design
  Patterns; Refactoring; etc
* effective go
* the seven (eight) (nine?) fallacies of distributed computing
* things by robert martin (uncle bob), in particular on SOLID design
* the zen of python
* things by steve yegge (stevey's drunken blog rants)
* http://c2.com/cgi/wiki
* goedel, escher, bach
* [Rails Conf 2012 Keynote: Simplicity Matters by Rich Hickey](https://www.youtube.com/watch?v=rI8tNMsozo0)

Some of them probably contradict each other. Both sides could be right at the
same time. Some of the best advice, followed blindly, will lead you into
perdition; and useful insights can come from surprising places. Mutable state,
for example, is pretty fundamental to what we do; but it's important to
understand the FP perspective of how and why it's evil, and to use it in full
knowedge of its dangers; and to avoid it when it's not necessary. Et cetera.