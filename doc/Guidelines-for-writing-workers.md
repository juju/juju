# Writing workers

If you're writing a worker -- and almost everything that juju does happens inside a worker -- you should be aware of the
following guidelines. They're not necessarily comprehensive, and not *necessarily* to be followed without question; but
if you're not following the advice on this page, you should have a very good reason and have talked it through with
fwereade.

* If you really just want to run a dumb function on its own goroutine, use `worker.NewSimpleWorker`.

* If you just want to do something every \<period\>, use `worker.NewPeriodicWorker`.

* If you want to react to watcher events, you should probably use `worker.NewNotifyWorker or worker.NewStringsWorker`.

* If your worker has any methods outside the `worker.Worker` interface, DO NOT use any of the above callback-style
  workers. Those methods, that need to communicate with the main goroutine, *need* to know that goroutine's state, so
  that they don't just hang forever.

* To restate the previous point: basically *never* do a naked channel send/receive. If you're building a structure that
  makes you think you need them, you're most likely building the wrong structure.

* If you're writing a custom worker, and not using a `tomb.Tomb`, you are almost certainly doing it wrong. Read
  the [blog post](http://blog.labix.org/2011/10/09/death-of-goroutines-under-control), or just
  the [code](http://launchpad.net/tomb) -- it's less than 200 lines and it's about 50% comments.

* If you're letting `tomb.ErrDying` leak out of your workers to any clients, you are definitely doing it wrong -- you
  risk stopping another worker with that same error, which will quite rightly panic (because that tomb is *not* yet
  dying).

* If it's possible for your worker to call `.tomb.Done()` more than once, or less than once, you are *definitely* doing
  it very very wrong indeed.

* If you're using `.tomb.Dead()`, you are very probably doing it wrong -- the only reason (that I'm aware of) to select
  on that `.Dead()` rather than on `.Dying()` is to leak inappropriate information to your clients. They don't care if
  you're dying or dead; they care only that the component is no longer functioning reliably and cannot fulfil their
  requests. Full stop. Whatever started the component needs to know why it failed, but that parent is usually not the
  same entity as the client that's calling methods.

* If you're using `worker/singular`, you are quite likely to be doing it wrong, because you've written a worker that
  breaks when distributed. Things like provisioner and firewaller only work that way because we weren't smart enough to
  write them better; but you should generally be writing workers that collaborate correctly with themselves, and
  eschewing the temptation to depend on the funky layer-breaking of singular.

* If you're passing a \*state.State into your worker, you are almost certainly doing it wrong. The layers go worker->
  apiserver->state, and any attempt to skip past the apiserver layer should be viewed with *extreme* suspicion.

* Don't try to make a worker into a singleton (this isn't particularly related to workers, really, singleton is enough
  of an antipattern on its own). Singletons are basically the same as global variables, except even worse, and if you
  try to make them responsible for goroutines they become more horrible still.

## Example worker

Let's imagine a worker that reads values from some channel and passes them into some function. Here follows an annotated
implementation:

```go
// Config defines the operation of a ValuePasser.
type Config struct {
    Values  <-chan int
    Handler func(int) error
}

// Validate returns an error if the config is not valid.
func (config Config) Validate() error {
    if config.Values == nil {
        return errors.NotValidf("nil Values")
    }
    if config.Handler == nil {
        return errors.NotValidf("nil Handler")
    }
    return nil
}

// ValuePasser reads int values from a channel and passes them to a handler.
type ValuePasser struct {

    // You must have at least a tomb, or a catacomb if you have child 
    // workers, or doom yourself to re-implementing them badly.
    tomb tomb.Tomb

    // It's very convenient to keep dependencies and configuration values
    // tucked away in their own struct for easy validation and many other
    // reasons. See howto page elsewhere on the wiki.
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
        return errors.Trace(err)
    }

    //  * Create the worker (and initialize any runtime fields).
    //    Note that Catacomb doesn't need initialisation; but you want to
    //    create a fully-configured worker, ready to go, in one step, so
    //    this is the point where you should initialize runtime fields.
    worker := &ValuePasser{
        config: config,
        // maps, chans, whatever
    }

    //  * Launch the worker.
    err := worker.tomb.Go(worker.loop)
    if err != nil {
        return nil, errors.Trace(err)
    }

    //  * Return the worker.
    return worker, nil
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
        // shut down, you return your catacomb's ErrDying to the launcher func;
        // this will then kill the tomb with that error, which (uniquely)
        // does *not* overwrite a nil tomb error.
        // You're thus free to call .tomb.Kill(someError) -- or .Kill(nil) --
        // elsewhere, and this case needn't to worry about why it's dying.
        case <-w.tomb.Dying():
            return tomb.ErrDying
        
        // Here's where you need to pay most of your attention, because it'll
        // differ with each worker you write. The common features are that you
        // will *usually* just return errors at the slightest provocation
        // (retrying isn't your problem; someone else is responsible for
        // restarting you; and in general, *any* unknown error should be taken
        // to indicate that we *do not know* whether the last operation succeeded
        // or failed, and that we're fatally compromised).

        case value, ok := <- w.values:
            if !ok {
                return errors.New("values channel closed unexpectedly")
                // Of course, a closed input channel might be expected, and
                // indicate that the task is complete; in that case, you can
                // return nil, which will stop the worker without error.
            }
            if err := w.handler(value); err != nil {
                return errors.Annotatef(err, "cannot handle %d", value)
            }
        }
    }
}

// Kill is boilerplate and should look exactly like this.
func (w *ValuePasser) Kill() {
    w.tomb.Kill(nil)
}

// Wait is boilerplate and should look exactly like this.
func (w *ValuePasser) Wait() error {
    return w.tomb.Wait()
}
```