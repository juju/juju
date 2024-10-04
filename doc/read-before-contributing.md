# Read before contributing

This document is intended to help you write safe, dull, simple, boring,
pedestrian code. These qualities are valuable because programming is
hard, and debugging is harder, and debugging distributed systems is
harder still; but by writing all our code with a determinedly po-faced
and dyspeptic cynicism, and by tightly proscribing the interactions
between components, we can at least limit the insanity we have to deal
with at any one time.

There are valid exceptions to many of these maxims, but they are *rare*
and you should not violate them without a bone-deep understanding of why
they exist and what their purpose is. (Before tearing down a fence, know
why it was put up.)

## Programming 101

Practice this stuff consciously until you're doing it unconsciously. It
transcends languages and will serve you well wherever you go.

### Do Not Use Global Variables.

Seriously, please, **DO NOT USE GLOBAL VARIABLES**.

Do not even use *one* little unexported package-global variable, unless
you have explored the issue *in detail* with a *technical lead* and
determined that it's the *least* harmful approach.

One might think that one would not need to stress this in a group of
professional software developers. One would be heartbreakingly wrong;
so, for those who have not seen the light:

Our most fundamental limitation as software developers is in the number
of things we can consider simultaneously. We all know how hard it is to
deal with a func that takes 7 params -- that's a *lot* to think about at
once -- but it's far superior to a func that takes 5 params and uses 2
global variables, because the dependencies have been made explicit and
the code has been decoupled from the rolling maelstrom of secret
collaborators -- i.e. *everything else* that might *ever* read or write
that global variable.

Extracting global usage from existing code pays off handsomely and fast,
and opportunities to do so should pretty much always be taken. You never
have to fix the whole program at once: you just drop the global
reference, supply it as a parameter from the originating context, and
move on. It doesn't matter if there's now a new global reference one
level up: it's one step closer to the edge of the system, and one step
closer to being constructed explicitly, just once, and handed explicitly
to everything that needs it.

(Aside: environment vars are global variables too. If you need some
config from the environment, read it as early as possible and hand it
around explicitly like anything else.)

### Write Unit Tests

And that means you should write **unit tests**: *tests* for each *unit*
of functionality that will interact with other parts of the program.

Write integration tests and functional tests as well, for sure; but
they're never going to cover every possible weird race condition. Your
unit tests are responsible for that; you use them to imagine and induce
every situation you can imagine affecting your component, and you make
every effort to ensure it behaves sensibly in all circumstances.

If you think it's hard to test actual units, your units are too big. If
you want to write internal tests, your units are too big. (Or possibly,
in either case, you're screwing about with globals. If so, stop it.)

The underlying insight is: tests exist to fail. If a test has never
failed, it has value only in potentia; and its value is only realised,
for better or for worse, when it does fail. If the failure uncovers a
real problem, it delivers actual value; but every spurious failure
contributes to a drip-feed of negative value.

(Oh, and spurious successes are a vast flood of negative value: they
build up slowly and silently until someone notices you've been
shipping a broken feature for 6 months, at which point you suddenly have
to book all that negative value at once and scramble to stay afloat.)

But, regardless: as discussed, every test is a cost and a risk. Any test
that does not cleanly and clearly map to a failure of the *unit* to meet
expectations is especially risky, because it comes with a large extra
analysis cost every time it fails. So: be thoughtful about the tests you
do write, and make each test case as lean and focused as possible.

Ultimately, your tests will be judged by their failures, and by how
easily others can manipulate them to change or add to the SUT's
behavioural constraints. Behaviour behaviour behaviour. Behaviour.

### Test Behaviour, Not Implementation

This means, at a minimum:

* do not write tests in `package foo`, write them in `package foo_test`
* do not use the `export_test.go` mechanism at all
* do not come up with any other scheme for touching unexported bits
* do not export something just to test it, unless you give clients the
  exact same degree of control that your tests have *and require them
  to exercise it*.

...but if you're creative you'll find other ways to make this mistake.
Regardless, by forbidding internal access; not using globals (*especially*
not global func vars, they're at least as bad as any other global); and not
privileging *either* the tests *or* the runtime clients, we can write a
set of executable specifications for how a component will behave under
various circumstances. This is the single most valuable thing you can
have when refactoring or debugging.

* When refactoring, it's valuable because it allows you to *actually
  refactor*, i.e. change the code without changing the tests, and
  have a reasonable degree of confidence in the result.

* When debugging, it's valuable because the existence of good tests
  makes it easy to write *more* good tests: if you're changing
  behaviour (which you *will* have to do in response to bugs) you
  already have a framework designed to express the component's
  possible interactions with *all* its collaborators.

Some developers maintain that internal tests deliver value, and it's not
*impossible* for them to do so; but they are a continual stumbling-block
for future refactoring efforts, and they are **not** an adequate
replacement for *any* behaviour test.

(And their tendency to keep passing when the component as a whole is
failing to meet the responsibility that the tests appear to be
validating is *poisonous*. If you don't think this is a big deal, I envy
your innocence.)

### SOLID Is Not Just For Objects

Dave Cheney did a good talk on this sometime earlyish in 2016. To hit
the high points (and add my own spin, I think I disagree a bit with some
of his interpretations...):

#### Single Responsibility Principle

Do one thing, do it well. If some exported unit is described as doing "X
and Y", or "X or Y", it's failing SRP unless its *sole purpose* is the
concatenation of, or choice between, X and Y.

#### Open/Closed Principle

Types should be open for extension but closed for modification. Golang
is designed to make it easy to do the right thing here, so just follow
community practice, and embrace the limitations of embedding -- it
really is helping you to write better code.

In particular, if you're planning to do anything that reminds you of
OO-style inheritance, you're almost certainly failing here.

#### Liskov Substitution Principle

Roughly speaking, anywhere you can use a type, you should be able to use
a subtype of same without breaking; and I'm pretty sure golang basically
gives us this for free by eschewing inheritance. You still have to pay
some attention to the semantics of any interface you're claiming some
type implements, but that's an irreducible problem (given the context)
and seems to have relatively minor impact in practice. I wave my hands
and move on.

#### Interface Segregation Princple

Group your interfaces' capabilities so that they cover one concern, and
cover it well. Your underlying types might not -- when working with any
sort of legacy code, they surely will not -- but you don't have to care:
it's far better to supply the same bloated type via 3 separate interface
params than it is to accept a single bloated interface just because you
need to consume a badly-written type.

*Sometimes* you want to combine these tiny interfaces, when the
capabilities are sufficiently bound to one another already -- see
`package io`, for example -- but you should feel nervous and mildly guilty
as you do so, and the more methods you end up with the worse you should
feel.

#### Dependency Inversion Principle

Depend on abstractions, not concretions: that is to say, basically, only
accept *capabilities* via interfaces. Certain parts of the codebase have
been written so as to make it as difficult as possible to rewrite them
to conform to this principle; please chip away at the problem as you
can, all the same.

Also, think about what you really depend on. If you just *use* some
resource, accept that `Resource` directly, and don't muck about with
`ResourceFactory` or `NewResourceFunc` unless you really *need* to defer
that logic. SRP, remember -- unless resource creation is fundamental to
what you're doing, it's best to let someone else handle it.

### Code Is Written For Humans To Read

...and only incidentally for machines to execute. It is more important
that your code be *clear* than that it be elegant or performant; and
those goals are only very rarely in opposition to clarity anyway.

And people will be reading your code while trying to solve some bug, and
they'll be in a hurry, and it will be easy for them to miss nuances in
the code and end up making things worse instead of better. If you've written
proper tests then they have at least some guardrails, but if your code
doesn't do exactly what it looks like it does at first glance you've
given them pit traps as well.

In particular:

* comment your code
    * explain what problem you're solving
    * ideally, also explain what associated problems you're not, and
      why, and how/where they should probably be addressed
    * point out the tricky bits in the implementation
    * if you did something bad, or inherited something bad, add a note
      making clear that it's bad -- that way there's half a chance
      people won't see it and unthinkingly duplicate it, and maybe a
      1-in-2^16 chance that they'll spot an opportunity to fix it
    * oh, and, *read* existing comments and **update them**

* think hard about names
    * words mean things, pick ones that do not egregiously violate
      your readers' expectations
    * excessively long names are bad; vague, unclear, or misleading
      names are much worse
    * when choosing names, think about your clients
        * they don't need to know how your func works -- they need to know *what it does*
        * they don't need to know that field's implementation -- they need to know *what it's for*
    * receivers and loop index vars can still be very short, and
      everyone knows what `err` is
    * other vars? please just say what they are

* avoid unnecessary nesting
    * `return`/`continue` early on failure
    * use intermediate variables
    * do not nest more than two func definitions
    * extract internal funcs at the drop of a hat

* avoid unnecessary indirection
    * indirection in service of abstraction is the noblest form; but
      be sure you know the difference, and that you really *have*
      abstracted something and not just made it less direct
    * callbacks are for frameworks, try not to write frameworks until
      the need is clear and present (i.e. when you are extracting an
      implicit framework that's proven its value and isn't amenable to
      a more direct implementation. Please don't try to write them
      from scratch)
    * DI *doesn't* mean you pass factories around -- making a type
      repsonsible for *creating* all its dependencies (as well as
      *using* them) violates SRP.
        * DI infrastructure code probably will use some factories, this
          is not the same thing, leaking them unnecessarily is bad

* avoid clever scoping tricks
    * named returns are ok for deferring error handling
    * avoid using them in other circumstances
    * definitely don't ever do the magic naked return thing
    * don't shadow vars that mean something in the enclosing scope:
      maybe *you* will never screw up `=` vs `:=` but the rest of us
      aren't so smart.

### Concurrency Is Hard

Seriously, there are *so many* ways to screw up a simple goroutine with
a trivial select loop. Gentle misuse of channels can block forever, or
panic; data races can inject subtly corrupt data where you'd never
expect it.

Luckily, there's a Go Proverb to fit the situation: "don't communicate
by sharing memory, share memory by communicating". I think this is
terribly insightful in a way that few explanations really bring home,
and I'm not sure I'll do any better, but it's worth a try:

If you even think about sharing memory, you're in the wrong paradigm.
Channels excel at *transferring responsibility*: you should consider
whatever you put in a channel to be *gone*, and whatever you received to
be *yours*. If you want to be literal, yes, memory is being shared; but
that can happen safely as a side-effect of the robust communication via
channel. Focus on getting the communication right, and you get safe
memory-sharing for free.

Locks etc embody the opposite approach -- two components each have
direct access to the same memory at the same time, and they have to
directly manage the synchronisation and be mindful of lock-ordering
concerns, and it's *really* easy to screw up. Responsibility transfer via
channel is still quite screwable, don't get me wrong, but it's much
easier to detect mistakes locally: so ideally you *always* own *all* the
data in scope, but *only* until you hand it on to someone else, and it's
relatively easy to check that invariant by inspection (for example, vars
not zeroed after they're sent are suspect).

Of course, *sometimes* you'll need to use one of the package sync
constructs, but... probably not as a first resort, please. And talk to
someone about it first -- it's certainly *possible* that you're in a
situation where you really do want to synchronise rather than
orchestrate, and where a judiciously deployed sync type *can*
significantly simplify convoluted channel usage, but it's rare. (And be
sure you really are simplifying: if you're not careful, the
interactions between locks and channels can be... entertaining.)

### Do Not Call Up That Which You Cannot Put Down

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

### Align Responsibilities And Capabilities

Interfaces define capabilities; by inspecting the capabilities exposed
to a component, you can establish bounds on what it can possibly affect
and/or be affected by. (If you're not familiar with the term "light
cone", look it up, it's exactly the same idea.)

When framed in these terms, it seems clear that the fewer capabilities
we expose to a component, the easier it is to analyse that component,
and hopefully that's reason enough to narrow your interfaces as you
go; but if you embrace the concept, you can take it one step further, by
treating every *capability exposure* as a *responsibility transfer*.
That is to say, you *only* supply a capability to a thing you *know* will
use it.

In concrete, juju-specific terms, consider the `environ.Tracker` worker.
It's a worker, so it has `Kill` and `Wait` methods; and it also exposes
an `Environ` to clients that need one; but the sets of clients are
non-overlapping. When you construct such a worker, you get a concrete
type exposing both sets of capabilities, and thus become responsible for
their discharge; so you put the type under management as a `Worker` alone,
and hand on the `Environ` capability to whatever else needs it (specifically,
in this case, via its Manifold's Output func).

Similarly, the various worker implementations that start `Watcher`s will
pass the `worker.Worker` responsibility into the `Catacomb`, leaving only
the `Changes()` channel for the loop func to worry about.

And if you *don't* hand on some responsibility/capability, and you don't
leave a comment explaining why, there's local evidence to suggest that
you done goofed; either by handing too many capabilities into the
context to begin with, or by dropping a responsibility on the floor.

I'm not sure what concrete advice this boils down to, though. It's more
a mode of analysis, I guess -- looking at code explicitly through these
eyes can be useful, and can betray opportunities to tighten it up that
might not be otherwise apparent.

### Don't Trust Anyone, Least Of All Yourself

Whether through negligence, through weakness, or through their own
deliberate fault, our collaborators will do strange things. Maybe
they'll close a channel they swore they wouldn't; maybe they'll call
methods in unexpected orders; maybe they'll return invalid data without
an error.

The only thing you can be reasonably sure of is that you *will* be
abused by a collaborator, directly or indirectly, at some point; and
you'll probably be responsible for inflicting your own weirdness at some
point. The traditional advice is to be liberal in what you accept and
conservative in what you emit; in this scenario, with potential chains
of dependency running from component to component across the system,
it's safest to be conservative across the board.

It's *much* better to fail early and loud when you see the first warning
signs than it is to fail late when the damage is already irrevocable.

And about not trusting yourself: just because you're writing the code on
both sides of the contract, *don't* kid yourself that you're safe. The
next person to change the code won't remember all the implicit rules of
sanity: if you're not enforcing them, you're conspiring to break them.

### Config Structs Are Awesome

Multiple parameters are pretty nice at times, but the weight of evidence
leans in favour of making *most* of your funcs accept either 0 args or

1. The sensible limit is probably, uh, 3 maybe? ...but even then, it's
   actually depressingly rare to nail a 3-param signature such that you
   never need to update it; and callables tend to accumulate parameters,
   anyway.

So, when you're exporting any callable that you expect to churn a bit in
its lifetime (i.e., pretty much always) just take the few extra seconds
to represent the params as a type. There's a howto wiki page somewhere.
If you defer it until the first instance of actual churn, that's fine,
it's a judgment call; but if you find yourself rewriting an exported
signature even once, just take the time to replace it with a struct.
(And then future changes will be much easier.)

Also, take a couple of minutes to see if you're rediscovering a
widely-used type; and make an effort to give it a name that's a proper
noun: `FrobnosticateParams`  or -`Args` is generally a pretty terrible
choice, it's super-context-specific and betrays very little intent. For
example:

    type CreateMachineArgs struct {
        Cloud       string
        Placement   string
        Constraints constraints.Value
    }

    func CreateMachine(args CreateMachineArgs) (Machine, error)

...is, ehh, comprehensible enough, I suppose. But that's an *awful
name*! You can immediately make it a bit better by just calling the type
what it really is:

    type MachineSpec struct {
        Cloud       string
        Placement   string
        Constraints constraints.Value
    }

    func CreateMachine(spec MachineSpec) (Machine, error)

...and then, as a bonus, you get a type that doesn't hurt your eyes when
it ends up being a generally useful concept and passed around elsewhere.
I have found the following nouns generally better than either Args or
Params, in various contexts:

* `Spec` (implies creating distant resources, e.g. in the db, perhaps?)
* `Config` (implies sufficient dependencies/info to perform a task?)
* `Context` (implies you're a callback?)
* `Request` (implies, well, a direct request)
* `Selector` (implies request for several things, possibly to set up a
  Request?)

...but please don't treat this as a prescription: find the right names
and use them. e.g. a `LeaseClaim`, or whatever; and often, indeed, just
a string is quite good enough.

    func (repo *Repo) Get(name string) (Value, bool)

...is just fine as it is, because it probably really *won't* ever change
significantly.

One caveat: do not *casually* modify types that are used as config
structs, and pay particular heed to the names-are-for-clients advice
above. You may well have several different types that vary in only one
or two fields: that certainly shouldn't result in consolidation to *one*
type *alone*, but it may help you to discover a single type that is widely
used on its own, and occasionally alongside one or two other parameters.
Resist the temptation to consolidate so far that a type's validity depends
upon its context.

### Defaults Are A Trap

"OK", you think, "I will write a config struct, but it's a *terrible*
hassle for the client to supply all these values, we *know* what task
we're really doing, so we'll just accept nil values and insert
`clock.WallClock`, or `"/var/lib/juju"`, or `defaultAttemptStrategy`, or
whatever, as appropriate".

This is wrong for several reasons:

* you're smearing the client's responsibilities into what should be a
  context-agnostic tool
* you're making it harder to validate client code, because it
  specifies a bare minimum and just trusts the implementation to do
  what it meant
* you're privileging one use case (runtime, in the context you're
  currently imagining) over another current one (the tests)... *and*
  over all future uses to which your system might be put

...and it only rarely even aids readability, because the vast majority
of types where it's easy to make this mistake are only constructed once
or twice anyway. (When a type has many clients, the folly of defaults is
more clearly pronounced, so they tend to slip away. Or, quite often, to
remain in place, waiting to trip up someone who underconfigures the type
in some new context and would *really* have appreciated an immediate
"you didn't say how long to wait" error rather than discovering that
you've picked a wildly inappropriate default.)

Note that zero values are fine, encouraged even -- *so long as they are
valid as such*. The go proverb is "make the zero value useful", and so
you should: but a zero value that's magically interpreted as a non-zero
value is *not* a "useful zero value", it's just in-band signalling and
comes with all the usual drawbacks. Know the difference.

(Sometimes you will create Foos over and over again in the same context,
such that you only want to specify the important parameters at the point
of use. It's fine to write a local `newFoo(params, subset)` func: just
don't pollute the Foo implementation with your concerns. Similarly, if
you're really sure that you *can* supply useful defaults: expose them as
a package func returning a pre-filled config, and make it the client's
*explicit* choice to defer configuration elsewhere.)

### Process Boundaries Are Borders To Hell

* The stuff in the database will not be what you expect
* The stuff stored on disk will not be what you expect
* The server you talk to will not be what you expect
* The client talking to you will not be what you expect

Seriously. *Everything* you read should be assumed to be corrupt and/or
out of date, and probably malicious to boot. Get used to it.

In particular, don't *ever* serialize types that aren't *explicitly*
designed for exactly the serialization you're performing. Once you
serialize, you open up the likelihood that it'll be deserialized by a
different version of the code; it is *unsafe* to read it into any type
that even *could*  be different. The juju codebase has serious problems
here -- various api params types and mongodb doc types still include,
e.g., structs defined in the charm package that could change at any
time.

This is also why you need to be obsessive about updating API versions as
you change their capabilities and/or behaviour. It's hard enough
communicating in the first place without worrying about whether the
other side might support a field that got added without triggering a
corresponding api change.

## Golang-specific Considerations

Go and read Effective Go, it's full of useful stuff. But also read this.

### Interfaces Are Awesome

They really, really are -- they create an environment in which creating
(so)LID types is the path of least resistance. But while they are an
exceptionally useful tool, they are *not* the right tool for *every*
job, and injudiciously chosen interfaces can hamper understanding as
much as good ones can aid it.

#### Awesome Interfaces Are Small

One method is a great size for an interface; 3 or 4 is probably
perfectly decent; more than that and you're probably creating an
interface to cover up for some bloated concrete type that you're
replacing so you can test the recipient.

And that's tolerable -- it's certainly progress -- but it's also quite
possibly a missed opportunity. Imagine a config struct that currently
references some horrible gnarly type:

    type Config struct {
        State *state.State
        Magic string
    }

...and an implementation that uses a bunch of methods, such that you
extract the following interface to give (note: not real State methods,
methods returning concrete types with unexported fields are a goddamn
nightmare for testing and refactoring, this is heavily idealized for
convenience):

    type Backend interface {
        ListUnits() ([]string, error)
        GetUnits([]string) ([]Unit, error)
        DestroyUnits([]string) error
        ListMachines() ([]string, error)
        GetMachines([]string) ([]Machine, error)
        DestroyMachines([]string) error
    }

    type Config struct {
        Backend Backend
        Magic   string
    }

This is great, because you can now test your implementation thoroughly
by means of a mock implementing the Backend interface, but it's actually
still pretty unwieldy.

If we go a step further, though, according to the capabilities that are
most intimately connected:

    type UnitBackend interface {
        ListUnits() ([]string, error)
        GetUnits([]string) ([]Unit, error)
        DestroyUnits([]string) error
    }

    type MachineBackend interface {
        ListMachines() ([]string, error)
        GetMachines([]string) ([]Machine, error)
        DestroyMachines([]string) error
    }

    type Config struct {
        Units    UnitBackend
        Machines MachineBackend
        Magic    string
    }

...the scope of the type's responsibilities is immediately clearer: the
capabilities exposed to it are immediately clearer; and it's also easier
to build more modular test scaffolding around particular areas.

#### Separate Interfaces From Implementations

If you see an interface defined next to a type that implements that
interface, it's probably wrong; if you see a constructor that returns an
interface instead of a concrete type, it's almost certainly wrong. That
is to say:

    func NewThingWorker() (*ThingWorker, error)

...is much better than:

    func NewThingWorker() (worker.Worker, error)

...*even if* `ThingWorker` only exposes `Worker` methods. This is because
interfaces are designed to hide behaviour, but we don't want to hide
behaviour from our creator -- *she* needs to know everything we can do,
and she's responsible for packaging up those capabilities neatly and
delivering them where they're needed.

If the above is too abstract: don't define an interface next to an
implementation of that interface, otherwise you force people to import
the implementation when all they need is the interface... (or, they just
take the easier route and define their own interface, rendering yours
useless).

Aside: you will occasionally find yourself needing factory types or even
funcs, which exist purely to *abstract* the construction of concrete
types. These are not *quite* the same as constructors, so the advice
doesn't apply; and it's boring but easy to wrap a constructor to return
an interface anyway, and these sorts of funcs are verifiable by
inspection:

    func NewFacade(apiCaller base.APICaller) (Facade, error) {
        facade, err := apithing.NewFacade(apiCaller)
        if err != nil {
            return nil, errors.Trace(err)
        }
        return facade, nil
    }

    func NewWorker(config Config) (worker.Worker, error) {
        worker, err := New(config)
        if err != nil {
            return nil, errors.Trace(err)
        }
        return worker, nil
    }

...to the point where you'll often see this sort of func deployed
alongside a `dependency.Manifold`, tucked away in an untested `shim.go`
to satisfy the higher goal of rendering the manifold functionality
testable in complete isolation.

#### *Super*-Awesome Interfaces Are Defined On Their Own

If you've created an interface, or a few, with the pure clarity of, say,
`io.Reader` et al, it's probably worth building a package around them.
It'd be a good place for funcs that manipulate those types, and for
documentation and discussion of best practice when implementing... and
it *might* even have a couple of useful implementations...

But you're not going to write something as generally useful as
`io.Reader` -- or, at least, you're not going to get it right first
time. It's like pattern mining: when a whole bunch of packages declare
the same interfaces with the same semantics, that's a strong indication
that it's a *super*-awesome abstraction and it might be a good idea to
promote it. One implementation, one client, and a vision of the future?
Not reason enough. Let the client define the interface, lightly shim if
necessary, and let the implementation remain ignorant of what hoops it's
jumping through.

#### Awesome Interfaces Return Interfaces

...or fully-exported structs, at any rate. But the moment you define an
*interface* method to return anything that can't be fully swapped out by
an alternative implementation, you lose the freedom that interfaces give
you. (And kinda miss the point of using interfaces in the first place.)

But wait! Earlier, you said that constructors should return concrete
types; I can think of 100 methods that are essentially doing just the
same thing a constructor would. Is that really a bad idea?

I'm not *sure*, but: yes, I think it is. Constructors want to be free
funcs that explicitly accept all the context they need; *if* you later
discover a situation where you need to pass around a construction
capability, either on a factory interface or just as a factory func that
returns an interface, it's trivial to implement a shim that calls the
constructor you need -- and which is verifiably correct by inspection.

#### Hidden-Nil Interface Values Will Ruin Your Day

Smallest self-contained code sample I could think of (would have posted
a playground link, but, huh, share button apparently not there):

    package main
    
    import (
        "fmt"
    )
    
    // Things have Names.
    type Thing interface {
        Name() string
    }
    
    // thing is a Thing.
    type thing struct {
        name string
    }
    
    func (thing *thing) Name() string {
        return thing.name
    }
    
    // SafePrint prints Thing Names safely. (Har har.)
    func SafePrint(thing Thing) {
        if thing != nil {
            fmt.Println(thing.Name())
        } else {
            fmt.Println("oops")
        }
    }
    
    func main() {
        var thing0 = &thing{"bob"}
        var thing1 Thing
        var thing2 *thing
    
        SafePrint(thing0) // not nil, fine
        SafePrint(thing1) // nil, but fine
        SafePrint(thing2) // nil pointer, but non-nil interface: panic!
    }

This is a strong reason to be unimaginatively verbose and avoid directly
returning multiple results -- for example, if you're writing a shim for:

    func New() (*ThingWorker, error)

... to expose it as a worker.Worker:

    func NewWorker() (worker.Worker, error) {
        return New()
    }

...is a minor unexploded bomb just waiting for someone to make decisions
based on `w != nil` rather than `err == nil` and, yay, panic. And, sure,
they shouldn't do that: but you shouldn't introduce hidden nils. Always
just go with the blandly verbose approach:

    func NewWorker() (worker.Worker, error) {
        worker, err := New()
        if err != nil {
            return nil, errors.Trace(err)
        }
        return worker, nil
    }

...and, honestly, don't even take the shortcut when you're "sure" the
func you're calling returns an interface. People change things, and the
compiler won't protect you when someone fixes that constructor, far from
your code.

And for the same reason, *never* declare a func returning a concrete
error type, it's ludicrously likely to get missed and wreck someone's
day. Probably your own.

#### Don't Downcast Or Type-Switch Or Reflect

...unless you *really* have to. And even then, look into what it'd take
to rework your context to avoid it; and if/when defeated, be *painfully*
pedantic about correctness. `default:` on every switch, `, ok` on every
cast, and be prepared to spend several lines of code for every operation
on a reflected value.

(OK, there *are* legitimate uses for all these things -- but they're
infrastructurey as anything, and intrinsically hairy and hard to
understand; and really *really* best just avoided until you're backed
into a corner that nobody can help you out of. Thanks.)

### Structs Are Pretty Cool

Because structs are the underlying bones of an implementation, there are
different sets of advice that apply more or less strongly in different
circumstances -- it's hard to identify many practices that are
inherently either bad or good -- but I've still got a few heuristics.

#### Either Export Your Fields Or Don't

...but try to avoid mixing exported and unexported fields in the same
struct. Any exported field could be written by anyone; any unexported
field that's meant to be consistent with any exported one is thus at
risk.

Mixed exportedness also sends mixed messages to your collaborators --
they *can* create and change instances via direct access, but they don't
have full control, and that's a bit worrying. Better to just decide
whether your type is raw data (export it all) or a token representing
internal state (hide fields, expose methods).

#### If You Export Fields, Expose A `Validate() error` Method

By exporting fields, you're indicating that collaborators can usefully
use this type. If you're going to do that, it's only polite to let them
validate what they're planning to do without forcing them to *actually
do it* -- if, say, some complex operation spec requires a heavyweight
renderer to run it, it's just rude to force your client to create that
renderer before telling them the job never had a chance in the first place.

And give it a value receiver to signal that it's not going to change
the instance; and make a point of not changing anything, even if some of
the fields are themselves references. In fact, more generally:

#### If You Export Fields, You're A Value Type

If you have methods, accept value receivers. If you're returning the
type, return a value not a pointer. If you're accepting the type, accept
a value not a pointer.

If your type has exported maps or slices, beware, because they will not
be copied nicely by default; make a point of copying them (*including*
any nested reference types...) whenever your type crosses a boundary you
control.

### Channels Are Awesome But Deadly

They're fantastic, powerful, and *low-level*. Within a single narrow
context, orchestrating requests and responses and updates and changes,
there is no finer tool: but they *only* work right when both sides agree
on how they're meant to be used, so they should not be *exposed* without
great paranoid care.

#### Channels Should Be Private

In fact, almost the only channels you should ever see exported are
`<-chan struct{}`s used to broadcast some single-shot event. They're
pretty safe -- neither sender nor receiver has much opportunity to mess
with the other.

The other places you see them are basically all related to watchers of
one class or another; we'll talk about them in the juju-specific bits
further down. For now just be aware that they come with a range of
interesting drawbacks.

#### Channels Live In Select Statements

It is *vanishingly rare* for it to be a good idea to unconditionally
block on a channel, whether sending or receiving or ranging. That means
you *absolutely* depend on your collaborators to act as you expect, and
that's a bad idea -- mismatched expectations can leave you blocked
forever, and you never want that.

#### Buffers Are A Trap

It is depressingly common for people to design subtly-flawed channel
protocols... and respond to observed flakiness by buffering the channel.
This leaves the algorithm exactly as flaky as before, but explodes the
space of possible states and makes it orders of magnitude harder to
debug. Don't do that.

#### Naked Sends Into Buffers Are OK

...as long as they're locally verifiable by inspection. It's pretty rare
to have a good reason to do this, but it can be tremendously useful in
tests.

### Nil Channels Are Powerful

...because they go so nicely with select statements. There's a *very*
common select-loop pattern in juju that looks something like:

    var out outChan
    var results []resultType
    for {
        select {
        case <-w.abort:
            return errAborted
        case value := <- w.in:
            results = append(results, value)
            out = w.out
        case out <- results:
            results = nil
            out = nil
        }
    }

...or, more concretely, for a goroutine-safe public method that invokes
onto an internal goroutine:

    func (w *Worker) GetThing(name string) (Thing, error) {
        response := make(chan thingResponse)
        requests := w.thingRequests
        for {
            select {
            case <-w.catacomb.Dying():
                return errors.New("worker stopping")
            case requests <- thingRequest{name, response}:
                requests = nil
            case result := <-response:
                if result.err != nil {
                    return nil, errors.Trace(result.err)
                }
                return result.thing, nil
            }
        }
    }

Understand what it's doing, observe instances of it as you read the
code, reach for channel-nilling as a tool of first resort when
orchestrating communication between goroutines.

It's interesting to compare these with lock-based approaches. A simple:

    w.mu.Lock()
    defer w.mu.Unlock()

...at the top of (most) exported methods *is* an effective way of
synchronising access to a resource, at least to begin with; but it's not
always easy to extend as the type evolves. Complex operations that don't
*need* shared memory *all* the time end up with tighter locking; and
then all internal methods require extra care to ensure they're always
called with the same lock state (because any of them might be called
from more than one place).

And then maybe you're tempted into an attempt to orchestrate complex
logic with *multiple* locks, which is *very very* hard to get right --
in large part because locks take away your ability to verify individual
methods' correctness independent of their calling context. And that's
dangerous for the same reason that globals are poisonous: the implicit
factors affecting your code are much harder to track and reason about
than the explicit ones. It's worth learning to think with channels.

#### Channels Transfer Responsibility

It's perfectly fine to send mutable things down channels -- *so long as*
you immediately render yourself incapable of mutating that state.
(Usually, you just set something to nil.) The result-aggregation example
above (with `var results []resultType`) does exactly that: on the branch
where results are successfully handed over to whatever's reading from
`w.out`, we set `results = nil` not just to reset local state but to
render it visibly impossible for the transfer to contribute to a new
race condition.

Similarly, once you get a value out of a channel you are solely
responsible for it. Treat it as though you created it, and either tidy
it up or hand responsibility on to something else.

(Note: this is another reason not to buffer channels. Values sitting in
channel buffers are in limbo; you're immediately depending on non-local
context to determine whether those values ever get delivered, and that
renders debugging and analysis needlessly brain-melting. Merely knowing
that a value has been delivered does *not* imply it's been handled, of
course: the analysis remains non-trivial, but at least it's tractable.)

#### Channels Will Mess You Up

This is just by way of a reminder: misused channels can and will panic
or block forever. Neither outcome is acceptable. Only if you treat them
with actual thought and care will you see the benefits.

### Know What References Cost

...and remember that maps and slices are references too. This is really
an extension of the attitude that informs no-globals: every reference
that leaves your control, whether as a parameter or a result, should be
considered suspect -- *anyone* can change it, from any goroutine, from
arbitrarily distant code. So, especially for raw/pure data: the cost of
copying structs is *utterly negligible* anyway, and is *vastly* cheaper
than the dev time wasted tracking down -race reports (if you're lucky)
or weird unreproducible runtime corruption (if you're not).

Evidently, you often *do* need to pass references around, and probably
most of your methods will still take pointer receivers: don't twist your
code to avoid pointers. But be acutely aware of the costs of radically
enlarging the scope that can affect your data. If you're dealing with
dumb data -- maps, slices, plain structs -- strongly prefer to copy
them rather than allow any other component to share access to them.

### Errors Are Important

Really, they are. You'll see a lot of variations on a theme of:

    if err != nil {
        return errors.Trace(err)
    } 

...but those variations are important, and often quite interesting. For
example, that form implies that it's seeing a "can't-happen" error --
one completely outside the code's capacity to handle. It's so common
precisely *because* most code is not designed to handle every possible
error (and it shouldn't be!). But what it *does* need is to be mindful
of everything that could go wrong; and those error stanzas punctuate the
flow of code by delineating the points at which things *could* go wrong,
and thus leaving you in the best possible position to design robust,
resumable algorithms.

And, of course, it's in the variation that you see actual error
*handling*. One particular error in this context implies inconsistent
state: push that up a level for possible retry. Another means that we've
already completed the task, continue to skip ahead to the next entry.
Etc etc. Yes: errors are for flow control. Everyone says not to use
exceptions for flow control, conveniently forgetting that that's exactly
what they do; the trouble is only that it's such an opaque mechanism to
casual inspection that sane use of exceptions *requires* that you
tightly proscribe how they're used (just like sane use of `panic`/`recover`,
it would seem...).

By putting the error-handling front and centre, Golang puts you in a
position where you both can and must decide what to do based on your
errors; the error stanza is an *intentional* evocation of "I cannot
handle this condition", where a mere failure to `try:` may or may not be
deliberate.

And yeah, it's repetitive, nobody likes that: but a lot of what's great
about Golang comes from its steadfast refusal to add magic just to
reduce boilerplate.

#### Nobody Understands Unknown Errors

When you get an unknown error, that signals one thing and one thing
only: that you *do not know what happened*. I'll say it again, for
emphasis, because understanding this is so desperately critical to
writing code that behaves properly in challenging circumstances:

**An unknown error means you DO NOT KNOW what happened.**

In particular: an operation that returns an unknown error has **NOT**
(necessarily) **FAILED**. Seriously! This is important! When you get an
error you don't recognise, all you know is that you *do not know what's
going on* -- to proceed assuming *either* success *or* failure will
sometimes be wrong. (Yes, *usually* it's harmless to assume failure --
that is certainly the common case. But the weirdest bugs live in the
shadows of the assumptions you don't even know you're making, and this
is a really common mistake.)

The *only* safe thing to do with an unknown error is to *stop what
you're doing* and *report it up the chain*. If you're familiar with
languages that use exceptions, you should already be comfortable with
this approach; you know that, for example, `except: pass` is a Bad Idea,
and you know that as soon as an error happens you'll stop what you're
doing because the exception machinery will enforce that.

The thing further up the chain then gets to decide whether to retry or
to report it further; this is situational, but you generally shouldn't
have to worry about it while you're writing the code that might be
retried. (If you follow the juju-specific advice below then your workers
will always be retried anyway. You might at some point decide you need
finer-grained retry logic; this might be OK, but be *sure* you need it
before you dive in, it's hard to write and hard to test.)

#### Nobody Understands Transient Errors

It is tediously common for developers, faced with a reliability problem,
to start off by thinking "well, we need to detect transient errors".

If you do that, you've already screwed up: you're slicing the set of all
possible errors along the wrong line, and implicitly placing the
infinitude of unknown errors in the set of "permanent" errors; you thus
doom yourself to a literally Sisyphean task of repeatedly discovering
that there's *one more* error characteristic that you should treat as a
signal of transience.

What you actually have to do is assume that *all* errors are transient
until/unless you know otherwise: that is, your job is to detect and
handle or report the *permanent* errors, the ones that stand no chance
of being addressed without out-of-band intervention.

There will probably still be cases you miss, so it doesn't guarantee
that your error handling will stand unchanged forever; but what it
*does* mean is that your component can be fairly resilient in the face
of reality, and might not develop a reputation for collapsing in a
useless heap every time it encounters a new situation.

As noted above, juju has mechanisms for taking these issues off your
plate: see discussion of `dependency.Engine` in particular.

#### Trace Errors With Abandon

Tracebacks are a crutch to work around unsophisticated error design.
Sadly nobody really seems to know how to design errors so nicely as to
work around the need for *something* to tell you what code generated the
error; so, pretty much always, return the `errors.Trace` of an error.

(There *are* situations when a Trace is not quite ideal: consider
`worker.Worker.Wait`, which is *reporting* an error from another
context rather than *returning* an error from a failed operation. But
the annoyance of extra traces pales into insignificance beside that of
*missing* traces, so the two-word advice is "always Trace".)

#### Annotate Errors With Caution

Imagine you're a user, and you try to do something, and you see
something like this.

    ERROR: cannot add unit riak/3: cannot assign "riak/3" to machine "7": machine 7 no longer alive: not found: machine 7: machine not found.

Horrible, innit? Inconsistent, stuttery, verbose... alarmingly familiar.
We have a couple of ways to avoid this; one is to always test exact
error messages instead of hiding the gunk away behind `.*`s, but that
only goes so far, because our tests are less and less full-stacky these
days and many of the contributing components are not participating in
the unit tests. It can save individual components from the shame of
contributing, at least, and that counts for something.

The other option, which can feel terribly unnatural, but seems to have
good results in practice, is to *staunchly* resist the temptation to
include any context that is known to your *client*. So, say you're
implementing `AssignUnitToMachine(*Unit, *Machine)`, it is *not your
job* to tell the client that you "cannot assign" anything: the client
already knows what you're trying to do. For this to work, though, you
have to recognise the precise seams in your application where you *do*
need to insert context, and this is essentially the same "good error
design" problem that humans don't really seem to have cracked yet.

It also suffers badly from needing the whole chain of error
handling to be written in that style: i.e. the choice between `Trace` and
`Annotate` is not verifiable by local inspection alone. The *important*
thing is that you *think* about the errors you're producing, and remain
aware that eventually some poor user will see it and have to figure out
what they need to do.

#### React To *Specific Causes*

The `github.com/juju/errors` package is great at what it does, but it
includes a few pre-made error types (`NotFound`, `NotValid`, `AlreadyExists`,
etc) that are potential traps. They're all fine to *use*, but you need
to be aware that by choosing *such* a generic error you're effectively
saying "nobody but a user can usefully react to this".

Why's that? *Because* they're so general, and anyone can feel reasonably
justified in creating them, heedless of the chaos they can cause if
misinterpreted. If you're seeing a `CodeNotFound` out of the apiserver,
does that *necessarily* mean that the entity you were operating on was
not found? Or does it mean that some other associated entity was not
found? Or even, why not, some useful intermediate layer lost track of
some implementation detail and informed you via a `NotFound`? You don't
know, and you can't know, what your future collaborators will do: your
best bet, then, is to react only to the most precise errors you can.
`errors.IsNotFound(err)` could mean anything; but if you see an
`errors.Cause(err) == foobar.ErrNotBazQuxed` you can be as certain as
it's actually possible to be that the real problem is as stated. (It's
true that collaborators *could* still bamboozle you with inappropriate
errors -- but an `errors.NotFoundf` in `package foo` is never going to
ring alarms the way a `totallydifferentpackage.ErrSomethingOrOther`
will. At least you've got a *chance* of spotting the latter.)

Note also that the `errors.IsFoo` funcs are subtly misnamed: they're
really `IsCausedByFoo`. This is nice and helpful *but* some packages
declare their own `IsFoo` funcs, for their own errors, and *don't*
necessarily check causes. Therefore, helpful as the errors package's
default behaviour may be, don't rely on it -- it's a habit that will
eventually steer you invisibly wrong. When handling errors, *always*
extract the `Cause` and react to that; only if no special handling is
appropriate should you fall back to returning a `Trace` of the original
error.

## Juju-specific Heuristics

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

## Feature-Specific Heuristics

When you're trying to get juju to do something new, you'll need to write
code to cover a lot of responsibilities. Most of them are relatively
easy to discharge, but you still have to do *something* -- not much will
happen by magic.

There's a lengthy discussion of the layers and proto-layers that exist
in juju in the "Managing Complexity" doc in the wiki; this won't cover
the exact same ground, so go and read that too.

### Know What You Are Modelling

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
