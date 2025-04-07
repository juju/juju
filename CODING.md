First, familiarize yourself with the following:

- https://go.dev/doc/effective_go
- https://go-proverbs.github.io


In addition to that, also consider the following:

### Do have integration tests but do not forget about unit tests

### Do not call up that which you cannot put down

Files, API connections, mongo sessions, database documents, goroutines,
workers in general, &c: if you started or created them, you need to
either stop them (or delete or destroy or whatever)... **or** hand on
responsibility for same to something else. This isn't C, but that
doesn't mean we're safe from resource leaks -- just that you need to
watch out for different classes of leak.

And you need to be aware that whatever code you're running is probably
viewed as such a resource by some other component: you have a duty to
clean yourself up promptly and gracefully when asked. `Worker`s are easy
-- just pay attention to your internal `Dying` channel -- but free funcs
that could block for any length of time should accept an `Abort` chan (and
return a special error indicating early exit).

Don't be the resource leak :).

See discussion of the `worker.Worker` interface below for a useful
perspective on this problem and a recipe for avoiding it.

## Juju-specific heuristics

The overarching concept of Juju is that the user describes the model
they want, and we wrangle reality to make it so. This overriding force
shapes *everything we do*, and failure to pay attention to it causes
serious problems. This section explores the impact of this approach on
the code you have to write.

### User operations must be *simple* and *valid*

What does that imply? First of all, it implies that user actions need to
be *simple* and *easy to validate*. Deploy a service, add a machine, run
an action, upgrade a charm: all these things can and should be
represented as the simplest possible record of user intent. That's the
easy bit -- so long as we're careful with mgo/txn, individual user
operations either happen or don't.

Screwing that bit up will break your feature from the word go, though,
so please keep it in mind. You must validate the changes sent by the
client, but you *must not* depend on the client for any further input or
clarification, because

**EVERYTHING FAILS**

and if you rely on the client sticking around to complete some process
you will end up stuck with inconsistent data in some paying client's
production DB, and you will have a Very Bad Day, replete with
opportunities to make it Much Much Worse.

Those days are *almost* as fun as they sound.

### Agent operations must be *resilient*

So, there are hard constraints we must not violate, but the user-input
problem is well bounded -- client, server, DB, done. Everything else
that juju does is happening in the real world, where networks go down
and processes die and disks fill up and senior admins drop hot coffee
down server vents... i.e. where

**EVERYTHING FAILS**

and we somehow have to make a million distinct things happen in the
right order to run, e.g., a huge openstack deployment on varying
hardware.

So, beyond the initial write of user intent to the DB, *all* the code
that's responsible for massaging reality into compliance has to be
prepared to retry, again and again, forever if necessary. This sounds
hard, and it *is* really hard to write components that retry all
possible failures, and it's a bloody *nightmare* to test them properly.
So: don't do that. It's a significant responsibility, and mixing it into
every type we write, as we happen to remember or not, is a prescription
for failure.

Thankfully, you actually *don't* have to worry about this in the usual
course of development; the only things you have to do are:

* (make sure your task is resumable/idempotent, which it always has to
  be whatever else you do, because **EVERYTHING FAILS**)
* encapsulate your task in a type that implements worker.Worker, and
  make a point of failing out immediately at the slightest provocation
* run it inside some agent's dependency.Engine, which will restart the
  task when it fails; and relax in the knowledge that the engine will
  also be restarted if it fails, or at least that it's not *your*
  problem if it isn't, and it will be addressed elsewhere

...and they're completely separable tasks, so you can address them one
by one and move forward with confidence in each part. Remember, though,
if you eschew any above step... you *will* have trouble. This is
because, in case you forgot,

**EVERYTHING FAILS**

and sooner or later your one-shot thing will be the one to fail, and you
will find that, whoops, having things happen reliably and in a sensible
order isn't quite as simple as it sounds; and if you have any sense you
will get your one-shots under proper management and devote your effort
to making your particular task idempotent or at least resumable, rather
than re-re-reinventing the reliability wheel and coming up with a
charming new variation on the pentagon, which is a waste of everyone's
time.

### time.Now Is The Winter Of Our Discontent

You remember the thing about global variables? They're really just an
example of *mutable global state*, which `time.Now` and `time.After` and
so on hook into. And if you depend on this particular mutable state, you
will write poor tests, because they will be inherently and inescapably
timing-dependent, and forever trying to balance reliability with speed.
That's a terrible situation to be in: tests absolutely require *both*
properties, and it's a strategic error to place them in opposition to
one another.

So: always, *always*, **always** configure your code with an explicit
`clock.Clock`. You can supply `clock.WallClock` in production (it's one
of those globals that we are slowly but surely migrating towards the
edges...), but your tests *must* use a `*testing.Clock`; and use its
`Alarms()` method to synchronise with the SUT (you'll get one event on
that channel for every `Now()`, `NewTimer()`, and `Timer.Reset()` backed
by the `*testing.Clock`; you can use this to ensure you only `Advance()`
the clock when you know it's waiting).

(You will find loads of old code that *is* timing-dependent, and loads
of bad tests that do terribad things like patch a global delay var to,
say, 10ms, and set off a chain of events and wait, uhh, 50ms, and
verify that, well, between 3 and 7 things triggered in that time so on
the balance of probability it's probably OK to- **NO** it is *not OK*!)

But with a proper testing clock, you can set up and test *sensible*
scenarios like, say, a 24h update-check schedule; and you can verify
that waiting for 23:59:59.999999999 does *not* trigger, and waiting for
24h *does*. Checking boundary conditions is always a good idea.

### worker.Worker Is A Sweet Ass-Abstraction

Just two methods: `Kill()`, and `Wait() error`. Kill gives control of a
lifetime; Wait informs you of its end; the two are intimately linked,
and indeed sometimes used together, but each in fact stands effectively
alone (it is very common to find Kill and Wait invoked from different
goroutines). The interface binds them together mainly just because *you*
have to implement both of them to be valuable, even if some particular
clients only *actually* need you for Kill alone, for example.

You will find yourself implementing a lot of workers, and that the Kill
and Wait methods are the purest one-line boilerplate each:

    func (w *Worker) Kill() {
        w.catacomb.Kill(nil)
    }

    func (w *Worker) Wait() error {
        return w.catacomb.Wait()
    }

...and that if you follow the advice on writing workers and config
structs found in the wiki, your constructor will be pretty boilerplatey
itself:

    func New(config Config) (*Worker, error) {
        if err := config.Validate(); err != nil {
            return nil, errors.Trace(err)
        }
        worker := &Worker{
            config: config,
            other:  make(someRuntimeFieldPerhaps),
        }
        err := catacomb.Invoke(catacomb.Plan{
            Site: &worker.catacomb,
            Work: worker.loop,
        })
        if err != nil {
            return nil, errors.Trace(err)
        }
        return worker, nil
    }

...leaving all the hard and/or situation-specific work to your `loop() error`
func (and anything else it calls). See `go doc ./worker/catacomb` for
further discussion of effective worker lifetime management.

Also, when writing workers, use the `workertest` package. `CleanKill`,
`DirtyKill`, `CheckKilled` and `CheckAlive` are all one-liners that will
protect you from the consequences of many easy mistakes in SUT or test.

### Use dependency.Engine And catacomb.Catacomb

See `go doc ./worker/catacomb` and `go doc ./worker/dependency` for
details; the TLDRs are roughly:

* `dependency.Engine` allows you to run a network of
  interdependent tasks, each of which is represented by a
  `dependency.Manifold` which knows how to run the task, and what
  resources are needed to do so.
* Tasks are started in essentially random order, and restarted when
  any of their dependencies either starts or stops; or when they
  themselves stop. This converges pretty quickly towards a stable set
  of workers running happily; and (sometimes) a few that are
  consistently failing, and all of whose dependents are dormant
  (waiting to be started with an available dependency).
* Two tasks that need to *share* information in some way should
  generally *not* depend on one another: they should share a
  dependency on a resource that represents the channel of
  communication between the two. (The direction of information flow is
  independent of the direction of dependency flow, if you like.)
* However, you *can* simplify workers that depend on mutable
  configuration, by making them a depend upon a resource that
  supplies that information to clients, but also watches for changes,
  and bounces itself when it sees a material difference from its
  initial state (thus triggering dependent bounces and automatic
  reconfiguration with the fresh value). See `worker/lifeflag` and
  `worker/migrationflag` for examples; and see `worker/environ` for
  the Tracker implementation (mentioned above) which takes advantage
  of `environs.Environ` being goroutine-safe to share a single value
  between clients and update it in the background, thus avoiding
  bounces.
* You *might* want to run your own `dependency.Engine`, but you're
  rather more likely to need to add a task to the `Manifolds` func in
  the relevant subpackages of `cmd/jujud/agent` (depending on what
  agent the task needs to run in).

...and:

* `catacomb.Catacomb` allows you to robustly manage the lifetime of a
  `worker.Worker` **and** any number of additional non-shared Workers.
* See the boilerplate in the worker.Worker section, or the docs, for
  how to invoke it.
* To *use* it effectively, remember that it's all about responsibility
  transfer. `Add` takes *unconditional* responsibility for a supplied
  worker: if the catacomb is `Kill`ed, so will be that worker; and if
  worker stops with an error, the catacomb will itself be `Kill`ed.
* This means that worker can register private resources and forget
  about them, rather than having to worry about their lifetimes; and
  conversely it means that those resources need implement *only* the
  worker interface, and can avoid having to leak lifetime information
  via inappropriate channels (literally).

Between them, they seem to cover most of the tricky situations that come
up when considering responsibility transfer for workers; and since you
can represent just about any time-bounded resource as a worker, they
make for a generally useful system for robustly managing resources that
exist in memory, at least.

#### All Our Manifolds Are In The Wrong Place

...because they're in worker packages, alongside the workers, and thus
severely pollute the context-independence of the workers, which can and
should stand alone.

The precise purpose of a manifold is to encapsulate a worker for use in
a *specific context*: one of the various agent dependency engines. It's
at the manifold level that we define the input resources, and at the
manifold level that we (should) filter worker-specific errors and render
them in a form appropriate to the context.

(For example, some workers sometimes return `dependency.ErrMissing` or
`dependency.ErrUninstall` -- this is, clearly, a leak of engine-specific
concerns into the wrong context. The worker should return, say,
`local.ErrCannotRun`: and the manifold's filter should convert that
appropriately, because it's only at that level that it makes sense to
specify the appropriate response. The worker really shouldn't know it's
running in a `dependency.Engine` at all.)

Next time someone has a moment while doing agent work, they should just
dump all the manifold implementations in appropriate subpackages of
`./cmd/jujud/agent` and see where that takes us. Will almost certainly
be progress...

### Use Watchers But Know What You're Doing

Juju uses a lot of things called watchers, and they aren't always
consistent. Most of the time, the word refers to a type with a Changes
channel, from which a single client can receive a stream of events; the
semantics may vary but the main point of a watcher is to represent
changing state in a form convenient for a select loop.

There are two common forms of watcher, distinguished by whether they
implement the interface defined in the `./watcher` package, or the one
defined in the `./state` package. All your workers should be using the
former, and they should be used roughly like this:

    func (w *Worker) loop() error {
        watch, err := w.config.Facade.Watch()
        if err != nil {
            return errors.Trace(err)
        }
        if err := w.catacomb.Add(watch); err != nil {
            return errors.Trace(err)
        }
        for {
            select {
            case <-w.catacomb.Dying():
                return w.catacomb.ErrDying()
            case value, ok := <-watch.Changes():
                if !ok {
                    return errors.New("watcher closed channel")
                }
                if err := w.handle(value); err != nil {
                    return errors.Trace(err)
                }
            }
        }
    }

(note that nice clean responsibility transfer to catacomb)

...but even the state watchers, which close their channels when they
stop and cause a surprising amount of semantic mess by doing so, share a
fundamental and critical feature:

#### Watchers Send Initial Events

Every watcher has a Changes channel; no watcher should be considered
functional until it's delivered one event on that channel. That event is
the baseline against which subsequent changes should be considered to be
diffs; it's the point at which you can read the value under observation
and know for sure that the watcher will inform you at least once if it
changes.

One useful feature of the initial events, exemplified in the watching
worker example above, is that the loop doesn't need to distinguish
between the first event and any others: every event *always* indicates
that you should read some state and respond to it. If you're handling
"initial" data differently to subsequent events you're almost certainly
doing at least one of them wrong.

A lot of watchers send only `struct{}`s, indicating merely that the
domain under observation is no longer guaranteed to be the same as
before; several more deliver `[]string`s identifying entities/domains
that are no longer guaranteed to have the same state as before; others
deliver different information, and some even include (parts of) the
state under observation packaged up for client consumption.

This technique is tempting but usually ends up slightly janky in
practice, for a few relatively minor reasons that seem to add up to the
level of actual annoyance:

* you almost always need a representation for "nothing there right
  now", which yucks up the type you need to send (vs notification of
  existence change like any other, and nonexistence notified on read
  with the same errors you'd always have).
* the more complex the data you send, the harder it is to aggregate
  events correctly at any given layer; trivial notifies, though, can
  be safely compressed at any point and still work as expected.
* any data you send will be potentially degraded by latency, and you
  might need to worry about that in the client; pure notifications
  are easier to get right, because the handler *always* determines
  what to do by requesting fresh domain state.
* the more opinionated the data you send, the less useful it is to
  future clients (and the more likely you are to implement *almost*
  the same watcher 5 times for 5 clients, and to only fix the common
  bug in 3 or 4 of them), and, well, it's just asking for trouble
  unless you already understand *exactly* what data you're going to
  need.
* you really don't understand exactly what data you're going to need,
  and a watcher format change is almost certainly an api change, and
  you don't need that hassle as well. If you get a notify watcher
  wrong, so it doesn't watch quite enough stuff, you can easily fix
  the bug by associating a new notifywatcher to handle the data you
  missed. (those events might be a tad enthusiastic at times, but your
  clients all signed up for at-least-once -- you're not breaking any
  contracts -- and you're also free to sub in a tighter implementation
  that still-more-closely matches the desired domain when it becomes
  useful to do so.) In short, notifications are a lot easier to tune
  as your understanding grows.

Regardless, even if you do end up in a situation where you want to send
data-heavy events, make sure you still send initial events. You're
pretty free to decide what changes you want to report; but you're not
free to skip the initial sync that your clients depend on to make use of
you.

#### State Watchers Are Tricky

For one, they implement the evil watcher interface that closes their
channel, and it's hard to rearchitect matters to fix this; for another,
they use the *other* layer of watching I haven't mentioned yet, and that
drags in a few other unpleasant concerns.

The most important thing to know when writing a state watcher is that
you *have* to play nice with the underlying substrate (implemented in
`state/watcher`, and with whom you communicate by registering and
unregistering channels) otherwise *you can block all its other clients*.
Yes, that is both bizarre and terrifying, but there's not much we can do
without serious rework; for the moment, just make sure you (1) aggregate
incoming watcher events before devoting any processing power to handling
them and (2) keep your database accesses (well, anything that keeps you
out of your core select loop) to an *absolute minimum*.

This is another reason to implement notification watchers by default --
everything you do in the process of converting the document-level change
notification stream into Something Else increases the risk you run of
disrupting the operation of other watchers in the same system. Merely
turning the raw stream into business-logic-level change notifications is
quite enough responsibility for one type, and there is depressingly
little to be gained from making this process any more complex or
error-prone than it already is.

(Also, mgo has the entertaining property of panicking when used after
the session's been closed; and state watcher lifetimes are not cleanly
associated with the lifetime of the sessions they might copy if they
were to read from the DB at just the wrong moment: e.g. while handling
an event delivered *just* before the underlying watcher was killed
(after which we have no guarantee of db safety). And the longer you
spend reading, the worse it potentially is. Be careful, and for
goodness' sake dont *write* anything in a watcher.)

#### The Core Watcher Is A Lie

It's just polling every 5s and spamming a relevant subset of what it
sees to all the channels the watchers on the next layer up have
registered with it. This is, ehh, not such a big deal -- it's sorta
dirty and inelegant and embarrassing, but it's worked well enough for
long enough that I think it's demonstrated its adequacy.

However, it plays merry hell with your tests if they're using a real
watcher under the hood, because the average test will take *much* less
than 5s to write a change it expects to see a reaction to, and the
infrastructure won't consider it worth mentioning for almost a full 5s,
which is too long by far.

So, `state.State` has a `StartSync` method that gooses the watcher into
action. If you're testing a state watcher directly, just `StartSync` the
state you used to create it; and when triggering syncs in JujuConnSuite
tests, use the suite's `BackingState` field to trigger watchers for the
controller model, and go via the `BackingStatePool` to trigger hosted
watcher syncs. Sorry :(.

## Feature-specific heuristics

When you're trying to get Juju to do something new, you'll need to write
code to cover a lot of responsibilities. Most of them are relatively
easy to discharge, but you still have to do *something* -- not much will
happen by magic.

There's a lengthy discussion of the layers and proto-layers that exist
in juju in the "Managing Complexity" doc in the wiki; this won't cover
the exact same ground, so go and read that too.

### Know what you are modelling

You've probably been given a description of the feature that describes a
subset of the desired UX, and doesn't really cover anything else. This
is both a blessing and a curse; it gives you freedom to write good
internal models, but obscures what those models need to contain.

You'll know the maxim "every problem has a solution that is simple,
obvious, and wrong"? That applies here in *spades*. There is information
that the user wants to see; and there is data that you need to persist:
and the two are almost certainly *not* actually the same, even once
you've eliminated any substrate-specific storage details. (I like to
point people at http://thecodelesscode.com/case/97 -- but that really
only touches on the general problem.)

It only gets harder when you have a feature that's exposed to two
entirely different classes of user -- i.e. the things we expose to
charmers should *almost certainly* not map precisely to the things we
expose to users. They're different people with different needs, working
in *very* different contexts; any model tuned to the needs of one group
will shortchange the other.

So, what do we do? We understand that whatever we expose to a user is
inherently a "view" (think MVC), and that what we *model* probably both
wants and needs to be a bit more sophisticated. After all, what is our
*job* if not to package up messy reality and render it comprehensible to
our users?

### YAGNI... But, Some Things You Actually Will

Of course, your model *will* grow and evolve with your feature; don't
try to anticipate every user request. Know that there *will* be changes,
but don't imagine you know what they'll be: just try to keep it clean
and modular to the extent that you can in service of future updates.

However, there are some things that you *do* need to do even if no user
explicitly requests them; and which are *very* hard to tack on after the
fact. Specifically: **you must write consistent data** to mongodb. This
is not optional; and nor, sadly, is it easy. You might be used to fancy
schmancy ACID transactions: we don't have none of that here.

There's plenty of documentation of its idiosyncracies -- see
doc/hacking-state.txt, and the "MongoDB and Consistency" and "mgo/txn
example" pages on the wiki, not to mention the package's own
documentation; so **please** read and understand all that **before** you
try to write persistence code.

And if you understand it all just by reading it, and you are not chilled
to the bone, you are either a prodigous genius or a fool-rushing-in.
Make sure that any state code you write is reviewed by someone who
understands the context enough to fear it.

Bluntly: you need referential integrity. It's a shame you don't get it
for free, but if you're landing even a *single branch* that does not
fulfil these responsibilities, you are setting us up for the sort of
failure that requires live DB surgery to fix.  "Usually works under easy
conditions" is not good enough: you need to shoot for "still acts
sensibly in the face of every adverse condition I can subject it to", by
making use of the tools (`SetBeforeHooks` et al) that give you that
degree of control.

### Model Self-Consistent Concepts

Don't implement creation without deletion. Don't make your getters
return a different type from that accepted by your setters. Don't
expose methods at different abstraction levels on the same type.

I'm not quite sure what the common thread of those mistakes is, but I
think it often flows from a failure to reset perspective. You're
implementing a user-facing feature, and you write an apiserver facade
with a CreateFoo method, and you want to register and expose it to
finish your card so you just implement the minimum necessary in state to
satisfy today's requirements and you move on, unheeding of the technical
debt you have lumbered the codebase with.

Where did you go wrong? By registering your apiserver code, and thus
creating a hard requirement for an implementation, which you then
half-assed to get it to land. You should have taken it, reviewed it,
landed it; and realigned your brain to working in the state layer,
before starting a fresh branch with (1) background knowledge of an
interface you'll want to conform to but (2) your mental focus on asking
"how does the persistence model need to change in order to accommodate
this new requirement and remain consistent".

And you should probably land *that* alone too; and only register the
apiserver facade in a separate branch, at which point you can do any
final spit and polish necessary to align interface and implementation.

(If you're changing a facade that's already registered, you can't do
that; but if you're changing a facade that's already registered you are
Doing It Wrong because you are knowingly breaking compatibility. Even
just adding to a facade should be considered an api version change,
because clients have a right to be able to know if a method is there
-- or a parameter will be handled, or a result field will be set -- just
by looking at the version. Forcing them to guess is deeply deeply
unhelpful.)

### Internal Facades Map To Roles

If you're designing an API that will be used by some worker running in
an agent somewhere, design your facade *for that worker* and implement
your authorization to allow only (the agent running) that worker access.
If multiple workers need the same capabilities, expose those
capabilities on multiple facades; but implement the underlying
functionality just once, and specialize them with facade-specific
authorization code. For example, see `apiserver/lifeflag`, which is
implemented entirely by specializing implementations in
`apiserver/common`.

(Note that `apiserver/common` is way bloated already: don't just
mindlessly add to it, instead extract capabilities into their own
subpackages. And if you can think of a better namespace for this
functionality -- `apiserver/capabilities` perhaps? I'd be very much open
to changing it.)

This is important so we have some flexibility in how we arrange
responsibilities -- it allows us to move a worker from one place to
another and reintegrate it by *only* changing that facade's auth
expectations; you don't have to worry about creating ever-more-complex
(and thus likely-to-be-insecure) auth code for a facade that serves
multiple masters.

### External Facades Are More Like Microservices

They're all running in one process, but the choices informing their
grouping should be made from the same perspective. In particular, you
should try to group methods such that related things change at the same
time, so that you avoid triggering api-wide version bumps. For example,
a Service facade and a Machine facade will contain service- or machine-
specific functionality, but neither should contain functionality shared
with the other. For example, status information: a Status facade that
lets you get detailed statuses for any set of tag-identified entities is
a much better idea than implementing Status methods on each entity-
specific facade, because then you're free to evolve status functionality
without churning all the other facade api versions.

The fact that they're still all talking to the same monolithic state
implementation underneath is a bit of a shame, maybe, but it will do us
no harm to structure our public face after the architecture we'd like
rather than the one we have.

### All Facades Are Attack Vectors

Remember, controllers are stuffed with delicious squishy client secrets:
i.e. lots of people's public cloud credentials, which are super-useful
if you ever feel like, say, running a botnet. Compromised admins are an
obvious problem, but most will expose only their own credentials;
compromised *controller* admins are a much worse problem, but we can't
directly address that in code: that leaves us with compromised *agents*,
which are a very real threat.

Consider the size of the attack surface, apart from anything else. Every
unit we deploy runs third-party code in the form of the charm; *and* of
the application it's running; and *either* of those could be insecure or
actively malicious, and could plausibly take complete control of the
machine it's running on, and perfectly impersonate the deployed agent.
That is: you should *assume* that any agent you deploy is compromised,
and avoid exposing any capabilities *or information* that is not
*required* by one of the workers that you *independently* know *should*
be running in that agent. Honestly, "never trust the client" remains
good advice at every scale and in every domain I can think of so far:
it's another of those know-it-in-your-bones things.

(Yes, *any* information, even if you currently believe that some
colocated worker will already have access to that information. Workers
do occasionally move, and the flexibility is valuable; we'd rather they
didn't leave inappropriate capabilities around when they leave.)

So, be careful when writing facades; and when investigating existing
one, be sensitive to the possibility that they might be overly
permissive; and take all reasonable opportunities to tighten them up.
Let `ErrPerm` be your watchword; you may find it helpful to imagine
yourself in the persona of a small-minded and hate-filled minor
bureaucrat, gleefully stamping **VOID** on everything within reach.

### All Facades Should Accept Bulk Arguments

Please, JFDI. It's not actually hard: you expose, say, a `Foobar` method
that accepts the bulk args and loops over them to check validity and
authorization; and then it hands the acceptable ones on to an internal
`oneFoobar` method that does the specific work. (Or, if you have an
opportunity to improve performance by doing the internal work in bulk,
you can take that opportunity directly without changing the API.)

There are several reasons to prefer bulk arguments:

* if all your args are bulk, nobody needs to remember which ones are
* implementing your apiserver/common capabilities in bulk makes them
  easier to reuse in the cases where you do need bulk args
* those cases are more common than you might think -- and even if you
  don't strictly *need* them in any given instance, that's often just
  a failure of imagination.

Consider, for example, *any* agent worker that deals with a set of
entities. deployer, provisioner, firewaller, uniter, probably a bunch of
others. They're pretty much all seeing lists of entities and then
processing them one by one: check life, read some other data, handle
them. And this sucks! We really would kinda like to be able to scale a
bit: when you get 100 machines and you want to know all of their life
values and provisioning info, it is ludicrous to make 200 requests
instead of 2.

And because we already implemented a bulk `common.LifeGetter` you can
get that for free; and because you then implemented, say,
`common/provisioning.Getter`, then the next person who needs to divest
the provisioner of some of its too-many responsibilities will be able to
reuse that part, easily, in its own Facade. (And she'll also thank you
for having moved `common.LifeGetter` to `common/life.Getter`, which you
did because you are a good and conscientious person, alongside whom we
are all proud to work.)

### CLI Commands Must Not Be Half-Assed

They're *what our users see*. For $importantThing's sake, please, think
them through. Consider expectations, and conventions, and just about
every possible form of misuse you can imagine.

In particular, misunderstanding the nature of the cmd.Command interface
will trip you up and lead to you badly undertesting your commands. They
have *multiple responsibilities*: they are responsible both for
*interpreting user intent*, by parsing arguments, and by *executing*
that intent by communicating with a controller and one or more of its
external facades.

Both of these responsibilities are important, and conflating them makes
your life harder: the fact that it's convenient to put the two
responsibilities into a single type does not necessarily make it
sensible to erase their distinctions when validating how they work. So,
strongly consider extracting an *exported, embedded* type that
implements Run on its own; and using SetFlags and Init purely to
configure that type. You can then exercise your arg-parsing in detail,
and ensure it only generates valid Run types; and exercise your Run
method in detail, secure that it can't be affected by any internal state
that the SetFlags/Init phase might have set. (Composition FTW!)

#### CLI Implementation Thoughts

**DO NOT USE GLOBAL VARIABLES** (like os.Stdout and os.Stderr). You're
supplied with a cmd.Context; use it, and test how it's used. Also:

* your Info params should be documented consistently with the rest of
  the codebase; don't include options, they're generated automatically.
* stderr is for telling a human what's going on; stdout is for
  machine-consumable results. don't mix them up.
* if you write to stdout, make sure you implement a --format flag and
  accept both json and yaml. (don't write your own, see cmd.Output)
* positional args should generally not be optional: if something's
  optional, represent it as an option.
    * positional args *can* be optional, I suppose, but basically
      *only* when we decide that the rest of the command is in a
      position to perfectly infer intent; and that decision should not
      be taken too lightly. Before accepting it, come up with a clear
      reason why it shouldn't be an option with a default value, and
      write that down somewhere: in the comments, at least, if not the
      documentation.

...and again, please, test this stuff to absolute destruction.
Determining user intent is *quite* hard enough a problem without mixing
execution concerns in.
