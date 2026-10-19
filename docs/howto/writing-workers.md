---
myst:
  html_meta:
    description: "Learn about best practices for writing Juju workers, including tomb usage, channel management, and goroutine lifecycle control patterns."
---

(writing-workers)=
# Writing workers

If you're writing a worker -- and almost everything Juju does happens inside a worker -- you should be aware of the following guidelines. They're not necessarily comprehensive, and not *necessarily* to be followed without question; but if you're not following the advice on this page, you should have a very good reason.

These guidelines apply to every worker, whichever package it lives in. For the ready-made workers that react to watcher events (`NewNotifyWorker`, `NewStringsWorker`), see the package documentation of `core/watcher`.

* Keep the dying error inside the worker that owns it: return `w.catacomb.ErrDying()` (or `tomb.ErrDying` for a bare tomb) only from the loop of that worker. When a method hands it to a client, the client may pass it to its own `Kill`. A catacomb turns another catacomb's `ErrDying` into an error that stops the client, and because a parent kills itself with the error of any child that stops, it stops the parent as well. A bare `tomb.Tomb` panics, because it is not yet dying. Return an ordinary error from methods instead, for example one that names the stopped worker.

* If your worker has any methods outside the `worker.Worker` interface, write a custom worker instead of using a ready-made callback-style one. Those methods communicate with the main goroutine and *need* to know its state, so that they never hang forever. Wrap every channel send and receive in a select that includes `.Dying()`: a structure that seems to need a naked send or receive is most likely the wrong structure.

* If you're writing a custom worker, use a catacomb (`github.com/juju/worker/v5/catacomb`). It is the standard carrier: a catacomb is built on a tomb, so the lifetime mechanics below are the same, and it adds the coordination of child workers. A bare `tomb.Tomb` is correct only for a worker that can have no children.

* Make sure your worker calls `.tomb.Done()` exactly once, on every path.

* Select on `.Dying()`. Clients don't care whether the component is dying or dead; they care only that it is no longer functioning reliably and cannot fulfil their requests. Whatever started the component needs to know why it failed, but that parent is usually a different entity from the client calling methods.

* Prefer workers that collaborate correctly with themselves over `internal/worker/singular`, since a worker that only works as a singleton breaks when distributed.

* Let each worker hold its own state. A singleton is basically a global variable, except even worse, and a singleton responsible for goroutines is more horrible still.

## Example worker

Let's imagine a worker that reads values from a channel and passes them to a handler function. Here follows an annotated implementation:

```go
// Config defines the operation of a ValuePasser.
type Config struct {
    Values  <-chan int
    Handler func(int) error
}

// Validate returns an error if the config is not valid.
func (config Config) Validate() error {
    if config.Values == nil {
        return errors.Errorf("nil Values %w", coreerrors.NotValid)
    }
    if config.Handler == nil {
        return errors.Errorf("nil Handler %w", coreerrors.NotValid)
    }
    return nil
}

// ValuePasser reads int values from a channel and passes them to a handler.
type ValuePasser struct {

    // You must have at least a tomb, or a catacomb if you have child
    // workers, or doom yourself to re-implementing them badly.
    catacomb catacomb.Catacomb

    // It's very convenient to keep dependencies and configuration values
    // tucked away in their own struct for easy validation and many other
    // reasons.
    config Config

    // For runtime state, use your judgment re fields vs vars in the loop
    // method; but prefer fields for values that won't be overwritten.
    // Sophisticated workers often use vars in the loop method to control
    // which branches of the select can be taken, and that's hard enough
    // to follow without other variables polluting the namespace.
}

// NewValuePasser returns a ValuePasser configured as supplied.
func NewValuePasser(config Config) (*ValuePasser, error) {

    // This function should do three things:
    //  * Validate the configuration.
    if err := config.Validate(); err != nil {
        return nil, errors.Capture(err)
    }

    //  * Create the worker (and initialize any runtime fields).
    //    Note that the catacomb doesn't need initialisation; but you want to
    //    create a fully-configured worker, ready to go, in one step, so
    //    this is the point where you should initialize runtime fields.
    w := &ValuePasser{
        config: config,
        // maps, chans, whatever
    }

    //  * Launch the worker.
    err := catacomb.Invoke(catacomb.Plan{
        Name: "value-passer",
        Site: &w.catacomb,
        Work: w.loop,
    })
    if err != nil {
        return nil, errors.Capture(err)
    }

    //  * Return the worker.
    return w, nil
}

// loop is where most of the interesting stuff happens.
func (w *ValuePasser) loop() error {

    // If you've got detailed setup to do; watchers to be started, resource
    // cleanup to defer, etc, generally do it here. This is a very simple
    // worker so it doesn't do anything special and goes straight into the
    // standard select loop.

    for {
        select {

        // This bit is mandatory. If you get the signal that you're meant to
        // shut down, you return your catacomb's ErrDying to the launcher
        // func; this will then kill the catacomb with that error, which
        // (uniquely) does *not* overwrite a nil error.
        // You're thus free to call .catacomb.Kill(someError) -- or
        // .Kill(nil) -- elsewhere, and this case needn't worry about why
        // it's dying.
        case <-w.catacomb.Dying():
            return w.catacomb.ErrDying()

        // Here's where you need to pay most of your attention, because it'll
        // differ with each worker you write. The common features are that you
        // will *usually* just return errors at the slightest provocation
        // (retrying isn't your problem; someone else is responsible for
        // restarting you; and in general, *any* unknown error should be taken
        // to indicate that we *do not know* whether the last operation
        // succeeded or failed, and that we're fatally compromised).

        case value, ok := <-w.config.Values:
            if !ok {
                return errors.New("values channel closed unexpectedly")
                // Of course, a closed input channel might be expected, and
                // indicate that the task is complete; in that case, you can
                // return nil, which will stop the worker without error.
            }
            if err := w.config.Handler(value); err != nil {
                return errors.Errorf("handling %d: %w", value, err)
            }
        }
    }
}

// Kill is boilerplate and should look exactly like this.
func (w *ValuePasser) Kill() {
    w.catacomb.Kill(nil)
}

// Wait is boilerplate and should look exactly like this.
func (w *ValuePasser) Wait() error {
    return w.catacomb.Wait()
}
```
