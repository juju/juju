(juju-environment-variables)=
# `juju` environment variables


This document lists the environment variables that are available on the Juju client in order to change its default behaviour.



## `GOCOOKIES`


The default location of the Go cookies file is `~/.go-cookies`. This variable can change that.

Example:

``` text
GOCOOKIES=/var/lib/landscape/juju-homes/1/.go-cookies
```

## `JUJU_CONTROLLER`


Used to specify the current Juju controller to use. This is overridden if the controller is specified on the command line using `-c CONTROLLER`.

(envvar-juju-data)=
## `JUJU_DATA`


This sets the path where Juju will look for its configuration files. You do not need to set this - by default Juju follows XDG guidelines and on Linux systems it will use the path:

``` text
~/.local/share/juju
```

## `JUJU_HOME` (deprecated)

For versions of Juju prior to 2.0, this variable indicated the 'home' directory where Juju kept configuration and other data.

    JUJU_HOME=~/.juju

## `JUJU_REPOSITORY` (deprecated)

For versions prior to 2.0, this variable set a local charms directory that Juju would search when deploying an application. The equivalent `--repository=/path/to/charms` switch (with `juju deploy`) was also available.

Both the environment variable and the switch are no longer functional in 2.x versions.

## `JUJU_LOGGING_CONFIG`


This setting takes effect on an environment only at bootstrap time. In stable Juju releases, agents are started with logging set to WARNING, and units are set to INFO. Development releases are set to DEBUG globally. Post bootstrap, on a running environment you can change the logging options to be more or less verbose. For example:

    juju model-config logging-config="juju=DEBUG; unit=WARNING"

## `JUJU_MODEL`


Used to specify the current Juju model to use. This is overridden if the model is specified on the command line using `-m MODEL`.

## `JUJU_DEV_FEATURE_FLAGS`


This setting takes effect on an environment only at bootstrap time. Unstable or pre-release features are enabled only when the feature flag is enabled prior to bootstrapping the environment.

    JUJU_DEV_FEATURE_FLAGS=<flag1,flag2> juju bootstrap

```{note}

Unforeseen and detrimental results can occur by enabling developmental features. Do not do so on production systems.

```

## `JUJU_STARTUP_LOGGING_CONFIG`

This setting takes effect on an environment only at bootstrap time, and is used to set the verbosity of the bootstrap process. For example, to troubleshoot a failure bootstrapping during provider development, you can set the log level to TRACE.

    JUJU_STARTUP_LOGGING_CONFIG=TRACE juju bootstrap

## `JUJU_CLI_VERSION`

This allows you to change the behaviour of the command line interface (CLI) between major Juju releases and exists as a compatibility flag for those users wishing to enable the newer behaviour of the Juju CLI. As the CLI output and behaviour is stable between minor releases of Juju, setting JUJU_CLI_VERSION will enable developers and users to preview the newer behaviour of the CLI.

    export JUJU_CLI_VERSION=2
    juju status

<!--BEN SAYS THIS SECTION BELONGS WITH SDK DOCS. BUT: IT'S OUT OF DATE AS IT USES `CHARM` INSTEAD OF `CHARMCRAFT`. SO IT SHOULD MAYBE JUST GO.
# Building

These variables are available to the `charm build` process.

<h4 id="heading--layer_path">CHARM_LAYERS_DIR (deprecated name: LAYER_PATH)</h4>

Sets the location to search for charm-layers. If no layer is found in this location, it defaults to searching the directory at the Juju Charm Layers Index (`https://github.com/juju/layer-index`) for the requested charm-layer.

    CHARM_LAYERS_DIR=$JUJU_REPOSITORY/layers

<h4 id="heading--interface_path">CHARM_INTERFACES_DIR (deprecated name: INTERFACE_PATH)</h4>

Sets the location to search for interface-layers. If no interface is found in this location, it defaults to searching the directory at the Juju Charm Layers Index (`https://github.com/juju/layer-index`) for the requested interface-layer.

    CHARM_INTERFACES_DIR=$JUJU_REPOSITORY/interfaces

-->

## Internal Use only

These exist for developmental purposes only.

### `JUJU_DUMMY_DELAY`

### `JUJU_NOTEST_MONGOJS`
