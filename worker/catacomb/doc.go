// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

/*
Catacomb leverages tomb.Tomb to bind the lifetimes of, and track the errors
of, a group of related workers. It's intended to be close to a drop-in
replacement for a Tomb: if you're implementing a worker, the only differences
should be (1) that a zero Catacomb is not valid, so you need to use New();
and (2) you can call .Add(someWorker) to bind the worker's lifetime to the
catacomb, and cause errors from that worker to be exposed via the catacomb.

This approach costs an extra goroutine over tomb.v2, but is slightly more
robust because Catacomb.Add() verfies worker registration, and is thus safer
than Tomb.Go(); and, of course, because it's designed to integrate with the
worker model already common in juju.

Note that a Catacomb is *not* a worker itself, despite the internal goroutine;
it's a tool to help you construct workers, just like tomb.Tomb.

The canonical expected construction of a catacomb-based worker is almost
identical to a tomb-based one, with s/tomb/catacomb/ and one extra line:

    func NewWorker(config Config) (worker.Worker, error) {
        if err := config.Validate(); err != nil {
            return nil, errors.Trace(err)
        }
        w := &someWorker{
            config:   config,
            catacomb: catacomb.New(), // This line is new.
        }
        go func() {
            defer w.catacomb.Done()
            w.catacomb.Kill(w.loop())
        }()
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
                return catacomb.ErrDying
            case change, ok := <-watch.Changes():
                 if !ok {
                     return watcher.EnsureErr(watch)
                 }
                 ...

...is not *obviously* superior to `defer watcher.Stop(watch, &w.tomb)`, it
does in fact behave better; and, furthermore, is more amenable to future
extension (watcher.Stop is fine *if* the watcher is started in NewWorker,
and deferred to run *after* the tomb is killed with the loop error; but that
becomes unwieldy when more than one watcher/worker is needed, and profoundly
tedious when the set is either large or dynamic).
*/
package catacomb
