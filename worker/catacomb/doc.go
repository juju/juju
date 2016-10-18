// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

/*
Catacomb leverages tomb.Tomb to bind the lifetimes of, and track the errors
of, a group of related workers. It's intended to be close to a drop-in
replacement for a Tomb: if you're implementing a worker, the only differences
should be (1) a slightly different creation dance; and (2) you can later call
.Add(aWorker) to bind the worker's lifetime to the catacomb's, and cause errors
from that worker to be exposed via the catacomb. Oh, and there's no global
ErrDying to induce surprising panics when misused.

This approach costs many extra goroutines over tomb.v2, but is slightly more
robust because Catacomb.Add() verfies worker registration, and is thus safer
than Tomb.Go(); and, of course, because it's designed to integrate with the
worker.Worker model already common in juju.

Note that a Catacomb is *not* a worker itself, despite the internal goroutine;
it's a tool to help you construct workers, just like tomb.Tomb.

The canonical expected construction of a catacomb-based worker is as follows:

    type someWorker struct {
        config   Config
        catacomb catacomb.Catacomb
        // more fields...
    }

    func NewWorker(config Config) (worker.Worker, error) {

        // This chunk is exactly as you'd expect for a tomb worker: just
        // create the instance with an implicit zero catacomb.
        if err := config.Validate(); err != nil {
            return nil, errors.Trace(err)
        }
        w := &someWorker{
            config:   config,
            // more fields...
        }

        // Here, instead of starting one's own boilerplate goroutine, just
        // hand responsibility over to the catacomb package. Evidently, it's
        // pretty hard to get this code wrong, so some might think it'd be ok
        // to write a panicky `MustInvoke(*Catacomb, func() error)`; please
        // don't do this in juju. (Anything that can go wrong will. Let's not
        // tempt fate.)
        err := catacomb.Invoke(catacomb.Plan{
            Site: &w.catacomb,
            Work: w.loop,
        })
        if err != nil {
            return nil, errors.Trace(err)
        }
        return w, nil
    }

...with the standard Kill and Wait implementations just as expected:

    func (w *someWorker) Kill() {
        w.catacomb.Kill(nil)
    }

    func (w *someWorker) Wait() error {
        return w.catacomb.Wait()
    }

...and the ability for loop code to create workers and bind their lifetimes
to the parent without risking the common misuse of a deferred watcher.Stop()
that targets the parent's tomb -- which risks causing an initiating loop error
to be overwritten by a later error from the Stop. Thus, while the Add in:

    func (w *someWorker) loop() error {
        watch, err := w.config.Facade.WatchSomething()
        if err != nil {
            return errors.Annotate(err, "cannot watch something")
        }
        if err := w.catacomb.Add(watch); err != nil {
            // Note that Add takes responsibility for the supplied worker;
            // if the catacomb can't accept the worker (because it's already
            // dying) it will stop the worker and directly return any error
            // thus encountered.
            return errors.Trace(err)
        }

        for {
            select {
            case <-w.catacomb.Dying():
                // The other important difference is that there's no package-
                // level ErrDying -- it's just too risky. Catacombs supply
                // own ErrDying errors, and won't panic when they see them
                // coming from other catacombs.
                return w.catacomb.ErrDying()
            case change, ok := <-watch.Changes():
                if !ok {
                    // Note: as discussed below, watcher.EnsureErr is an
                    // antipattern. To actually write this code, we need to
                    // (1) turn watchers into workers and (2) stop watchers
                    // closing their channels on error.
                    return errors.New("something watch failed")
                }
                if err := w.handle(change); err != nil {
                    return nil, errors.Trace(err)
                }
            }
        }
    }

...is not *obviously* superior to `defer watcher.Stop(watch, &w.tomb)`, it
does in fact behave better; and, furthermore, is more amenable to future
extension (watcher.Stop is fine *if* the watcher is started in NewWorker,
and deferred to run *after* the tomb is killed with the loop error; but that
becomes unwieldy when more than one watcher/worker is needed, and profoundly
tedious when the set is either large or dynamic).

And that's not even getting into the issues with `watcher.EnsureErr`: this
exists entirely because we picked a strange interface for watchers (Stop and
Err, instead of Kill and Wait) that's not amenable to clean error-gathering;
so we decided to signal worker errors with a closed change channel.

This solved the immediate problem, but caused us to add EnsureErr to make sure
we still failed with *some* error if the watcher closed the chan without error:
either because it broke its contract, or if some *other* component stopped the
watcher cleanly. That is not ideal: it would be far better *never* to close.
Then we can expect clients to Add the watch to a catacomb to handle lifetime,
and they can expect the Changes channel to deliver changes alone.

Of course, client code still has to handle closed channels: once the scope of
a chan gets beyond a single type, all users have to be properly paranoid, and
e.g. expect channels to be closed even when the contract explicitly says they
won't. But that's easy to track, and easy to handle -- just return an error
complaining that the watcher broke its contract. Done.

It's also important to note that you can easily manage dynamic workers: once
you've Add()ed the worker you can freely Kill() it at any time; so long as it
cleans itself up successfully, and returns no error from Wait(), it will be
silently unregistered and leave the catacomb otherwise unaffected. And that
might happen in the loop goroutine; but it'll work just fine from anywhere.
*/
package catacomb
