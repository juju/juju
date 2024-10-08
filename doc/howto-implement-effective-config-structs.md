The config-struct idiom
=======================

As we work to make our dependencies more explicit across the board, we've been
converging on a useful default pattern to follow when creating or modifying
types. It involves defining a struct, usually called "[Something]Config", with
a few consistent properties and usage patterns.

* The config struct should be safe to copy and use by value. Avoid maps and
  slices in particular; and pointers that aren't nicely hidden behind
  interfaces; and, generally, anything that might be mutated accidentally
  from outside (once the struct has been used to create an instance).

* The config struct should have a Validate method, with a value receiver, that
  returns an error if any problems can be forseen. The instance constructor
  should be able to determine success merely by validating its config; but
  clients should also be able to validate their configs without needing to
  try to use them.

* The type should ideally *only* be constructed by passing a Config struct by
  value, which can be validated and then stored directly in a field on the
  type. This makes the type implementation easier to work with: at a glance,
  you can see the difference between immutable config and live runtime state,
  by the presence of absence of a `.config`.

* The config struct should specify *all* the type's dependencies, to eliminate
  the need for package-patching in tests. This may be upsetting, because it
  can lead to large config structs, but it's not *actually* broadening the
  interface at all. If the config struct looks bad, the code is *already*
  dependent on too many things; making that fact explicit is the first step
  towards fixing it (and also helps maintainers know what they're dealing
  with, leading to fewer accidental regressions in the future).

In addition to the benefits above, starting with a config struct *early* in a
type's life makes your own life much easier: you almost certainly will need
new config/dependencies as the type evolves, and designing the constructor to
easily accommodate this generally pays off handsomely.

Example
-------

I'm going to write a worker that implements the Skinner-box protocol so popular
in F2P games. When a client pushes a button, the Skinner box either delivers a
reward or does nothing; the outcome is chosen at random.

    package skinner

    import (
        // ...
    )

    // Button delivers button-press notifications to a skinner box.
    type Button interface {
        Presses() <-chan struct{}
    }

    // Random exposes random booleans.
    type Random interface {
        Choice() bool
    }

    // Output allows a skinner box to issue rewards.
    type Output interface {
        Reward()
    }

    // BoxConfig defines the client-specifiable features of a skinner.Box.
    type BoxConfig struct {

        // Button, Random, and Output are the obvious pieces of configuration
        // given the context, but the obvious ones are rarely the only things
        // that need to be configured.
        Button Button
        Random Random
        Output Output

        // As it happens, actually, they *are* the only things that need to be
        // configured; the worker implementation is not the point of this code.
        // But it shouldn't be too much of an imaginative leap to think of types
        // that do use time and logging, so we discuss them here.

        // If you use time at all, and a large number of things *do* use time,
        // you should interact with an explicit Clock dependency. It's kinda
        // forced here, but it's important enough to be worth crowbarring it
        // in.
        Clock clock.Clock

        // We don't usually make loggers explicit dependencies; but honestly we
        // should. There are a couple of places in the codebase with object-
        // granularity logging rather than package-level; adding more would be
        // a generally good thing.
        Logger loggo.Logger
    }

    // Validate should be the one place you ever need to check config. It
    // should avoid modifying the config and signal that fact -- accomplished
    // by using a value receiver -- and can return errors describing the found
    // issues without additional context.
    func (config BoxConfig) Validate() error {

        // This is boring, but I'm going to write it out in full...
        if config.Button == nil {
            return errors.NotValidf("Button not set")
        }
        if config.Random == nil {
            return errors.NotValidf("Random not set")
        }
        if config.Output == nil {
            return errors.NotValidf("Output not set")
        }
        if config.Clock == nil {
            // ...at least partly because this bit is particularly important.
            //
            // It's *very natural* to see this situation and try to make the
            // client's life easier by setting a default system clock (or a
            // default Random implementation, or whatever); but it's actually
            // a bad idea, and the reason to prefer a value receiver is to
            // guard against the temptation to do so.
            //
            // This is because *any* default privileges one particular use case
            // over all others, and us human beings are notoriously bad at
            // predicting which will be most important. Even when there are only
            // two clients -- test and live -- defaults cannot help but privilege
            // one over the other, and render the code comparatively either
            // awkward to test or awkward to use. And on top of that: we should
            // write our code as though it's going to be reused; every implicit
            // default subtly works against that goal.
            return errors.NotValidf("Clock not set")
        }
        if config.Logger == nil {
            // This even applies here -- it's *not our job* to provide defaults.
            // If the client wants to set a no-op logger, they can and should do
            // so; but *we* should not presume to know what the client wanted,
            // and should bail in the face of the slightest ambiguity.
            return errors.NotValidf("Logger not set")
        }

        // Heh, this method would have been a *great* place to use katco's
        // validation library to get nice composite errors.
        return nil
    }

    // Box is a worker that runs a skinner box.
    type Box struct {

        // config should not be embedded, even if it seems convenient. The broken
        // encapsulation hurts way more that the extra `.config`s.
        config BoxConfig

        // all the other fields are for the runtime state of the type. In this
        // case it's very light; the benefits of separating out the config will
        // become more clearly apparent as the runtime complexity grows.
        tomb tomb.Tomb
    }

    // NewBox returns a skinner box running as configured, or an error.
    func NewBox(config BoxConfig) (*Box, error) {
        if err := config.Validate(); err != nil {
            return nil, errors.Annotatef(err, "cannot create Box")
        }
        box := &Box{
            config: config,
            // ...plus whatever runtime fields might be needed...
        }
        go func() {
            defer box.tomb.Done()
            box.tomb.Kill(box.loop())
        }()
        return box, nil
    }

    // Kill is part of the worker.Worker interface.
    func (box *Box) Kill() {
        box.tomb.Kill()
    }

    // Wait is part of the worker.Worker interface.
    func (box *Box) Wait() error {
        return box.tomb.Wait()
    }

    // loop issues random rewards in response to button presses.
    func (box *Box) loop() error {
        for {
            select {
            case <-box.tomb.Dying():
                return tomb.ErrDying
            case _, ok := <-box.config.Button.Presses():
                if !ok {
                    return errors.New("button disconnected")
                }
                if box.config.Random.Choice() {
                    box.config.Output.Reward()
                }
            }
        }
        // ...and, yeah, just pretend I did something with Clock and Logger.
    }
