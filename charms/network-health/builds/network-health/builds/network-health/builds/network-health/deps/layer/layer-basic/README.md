# Overview

This is the base layer for all charms [built using layers][building].  It
provides all of the standard Juju hooks and runs the
[charms.reactive.main][charms.reactive] loop for them.  It also bootstraps the
[charm-helpers][] and [charms.reactive][] libraries and all of their
dependencies for use by the charm.

# Usage

To create a charm layer using this base layer, you need only include it in
a `layer.yaml` file:

```yaml
includes: ['layer:basic']
```

This will fetch this layer from [interfaces.juju.solutions][] and incorporate
it into your charm layer.  You can then add handlers under the `reactive/`
directory.  Note that **any** file under `reactive/` will be expected to
contain handlers, whether as Python decorated functions or [executables][non-python]
using the [external handler protocol][].

### Charm Dependencies

Each layer can include a `wheelhouse.txt` file with Python requirement lines.
For example, this layer's `wheelhouse.txt` includes:

```
pip>=7.0.0,<8.0.0
charmhelpers>=0.4.0,<1.0.0
charms.reactive>=0.1.0,<2.0.0
```

All of these dependencies from each layer will be fetched (and updated) at build
time and will be automatically installed by this base layer before any reactive
handlers are run.

Note that the `wheelhouse.txt` file is intended for **charm** dependencies only.
That is, for libraries that the charm code itself needs to do its job of deploying
and configuring the payload.  If the payload itself has Python dependencies, those
should be handled separately, by the charm.

See [PyPI][pypi charms.X] for packages under the `charms.` namespace which might
be useful for your charm.

### Layer Namespace

Each layer has a reserved section in the `charms.layer.` Python package namespace,
which it can populate by including a `lib/charms/layer/<layer-name>.py` file or
by placing files under `lib/charms/layer/<layer-name>/`.  (If the layer name
includes hyphens, replace them with underscores.)  These can be helpers that the
layer uses internally, or it can expose classes or functions to be used by other
layers to interact with that layer.

For example, a layer named `foo` could include a `lib/charms/layer/foo.py` file
with some helper functions that other layers could access using:

```python
from charms.layer.foo import my_helper
```

### Layer Options

Any layer can define options in its `layer.yaml`.  Those options can then be set
by other layers to change the behavior of your layer.  The options are defined
using [jsonschema][], which is the same way that [action paramters][] are defined.

For example, the `foo` layer could include the following option definitons:

```yaml
includes: ['layer:basic']
defines:  # define some options for this layer (the layer "foo")
  enable-bar:  # define an "enable-bar" option for this layer
    description: If true, enable support for "bar".
    type: boolean
    default: false
```

A layer using `foo` could then set it:

```yaml
includes: ['layer:foo']
options:
  foo:  # setting options for the "foo" layer
    enable-bar: true  # set the "enable-bar" option to true
```

The `foo` layer can then use the `charms.layer.options` helper to load the values
for the options that it defined.  For example:

```python
from charms import layer

@when('state')
def do_thing():
  layer_opts = layer.options('foo')  # load all of the options for the "foo" layer
  if layer_opts['enable-bar']:  # check the value of the "enable-bar" option
      hookenv.log("Bar is enabled")
```

You can also access layer options in other handlers, such as Bash, using
the command-line interface:

```bash
. charms.reactive.sh

@when 'state'
function do_thing() {
    if layer_option foo enable-bar; then
        juju-log "Bar is enabled"
        juju-log "bar-value is: $(layer_option foo bar-value)"
    fi
}

reactive_handler_main
```

Note that options of type `boolean` will set the exit code, while other types
will be printed out.

# Hooks

This layer provides hooks that other layers can react to using the decorators
of the [charms.reactive][] library:

  * `config-changed`
  * `install`
  * `leader-elected`
  * `leader-settings-changed`
  * `start`
  * `stop`
  * `upgrade-charm`
  * `update-status`

Other hooks are not implemented at this time. A new layer can implement storage
or relation hooks in their own layer by putting them in the `hooks` directory.

**Note:** Because `update-status` is invoked every 5 minutes, you should take
care to ensure that your reactive handlers only invoke expensive operations
when absolutely necessary.  It is recommended that you use helpers like
[`@only_once`][], [`@when_file_changed`][], and [`data_changed`][] to ensure
that handlers run only when necessary.

# Layer Configuration

This layer supports the following options, which can be set in `layer.yaml`:

  * **packages**  A list of system packages to be installed before the reactive
    handlers are invoked.

  * **use_venv**  If set to true, the charm dependencies from the various
    layers' `wheelhouse.txt` files will be installed in a Python virtualenv
    located at `$CHARM_DIR/../.venv`.  This keeps charm dependencies from
    conflicting with payload dependencies, but you must take care to preserve
    the environment and interpreter if using `execl` or `subprocess`.

  * **include_system_packages**  If set to true and using a venv, include
    the `--system-site-packages` options to make system Python libraries
    visible within the venv.

An example `layer.yaml` using these options might be:

```yaml
includes: ['layer:basic']
options:
  basic:
    packages: ['git']
    use_venv: true
    include_system_packages: true
```


# Reactive States

This layer will set the following states:

  * **`config.changed`**  Any config option has changed from its previous value.
    This state is cleared automatically at the end of each hook invocation.

  * **`config.changed.<option>`** A specific config option has changed.
    **`<option>`** will be replaced by the config option name from `config.yaml`.
    This state is cleared automatically at the end of each hook invocation.

  * **`config.set.<option>`** A specific config option has a True or non-empty
    value set.  **`<option>`** will be replaced by the config option name from
    `config.yaml`.  This state is cleared automatically at the end of each hook
    invocation.

  * **`config.default.<option>`** A specific config option is set to its
    default value.  **`<option>`** will be replaced by the config option name
    from `config.yaml`.  This state is cleared automatically at the end of
    each hook invocation.

An example using the config states would be:

```python
@when('config.changed.my-opt')
def my_opt_changed():
    update_config()
    restart_service()
```


# Actions

This layer currently does not define any actions.


[building]: https://jujucharms.com/docs/devel/authors-charm-building
[charm-helpers]: https://pythonhosted.org/charmhelpers/
[charms.reactive]: https://pythonhosted.org/charms.reactive/
[interfaces.juju.solutions]: http://interfaces.juju.solutions/
[non-python]: https://pythonhosted.org/charms.reactive/#non-python-reactive-handlers
[external handler protocol]: https://pythonhosted.org/charms.reactive/charms.reactive.bus.html#charms.reactive.bus.ExternalHandler
[jsonschema]: http://json-schema.org/
[action paramters]: https://jujucharms.com/docs/stable/authors-charm-actions
[pypi charms.X]: https://pypi.python.org/pypi?%3Aaction=search&term=charms.&submit=search
[`@only_once`]: https://pythonhosted.org/charms.reactive/charms.reactive.decorators.html#charms.reactive.decorators.only_once
[`@when_file_changed`]: https://pythonhosted.org/charms.reactive/charms.reactive.decorators.html#charms.reactive.decorators.when_file_changed
[`data_changed`]: https://pythonhosted.org/charms.reactive/charms.reactive.helpers.html#charms.reactive.helpers.data_changed
