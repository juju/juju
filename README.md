juju
====

juju is devops distilled.

Juju enables you to use [Charms](https://jujucharms.com/docs/stable/charms) to deploy your
application architectures to EC2, OpenStack, Azure, GCE, your data center, and
even your own Ubuntu based laptop.  Moving between models is simple giving you
the flexibility to switch hosts whenever you want — for free.

For more information, see the [docs](https://jujucharms.com/docs/stable/getting-started).

Getting started
===============

`juju` is written in Go (http://golang.org), a modern, compiled, statically typed,
concurrent language. This document describes how to build `juju` from source.

If you are looking for binary releases of `juju`, they are available in the snap store

    snap install juju --classic

Installing Go
--------------

`Juju's` source code currently depends on Go 1.11. One of the easiest ways
to install golang is from a snap. You may need to first install
the [snap client](https://snapcraft.io/docs/core/install). Installing the golang
snap package is then as easy as

    snap install go --channel=1.11/stable --classic

You can read about the "classic" confinement policy [here](https://insights.ubuntu.com/2017/01/09/how-to-snap-introducing-classic-confinement/)

If you want to use `apt`, then you can add the [Golang Gophers PPA](https://launchpad.net/~gophers/+archive/ubuntu/archive) and then install by running the following

    sudo add-apt-repository ppa:gophers/archive
    sudo apt-get update
    sudo apt install golang-1.11

Alternatively, you can always follow the official [binary installation instructions](https://golang.org/doc/install#install)

Setting GOPATH
--------------

When working with the source of Go programs, you should define a path within
your home directory (or other workspace) which will be your `GOPATH`. `GOPATH`
is similar to Java's `CLASSPATH` or Python's `~/.local`. `GOPATH` is documented
online at `http://golang.org/pkg/go/build/` and inside the `go` tool itself

    go help gopath

Various conventions exist for naming the location of your `GOPATH`, but it should
exist, and be writable by you. For example

    export GOPATH=${HOME}/work
    mkdir $GOPATH

will define and create `$HOME/work` as your local `GOPATH`. The `go` tool itself
will create three subdirectories inside your `GOPATH` when required; `src`, `pkg`
and `bin`, which hold the source of Go programs, compiled packages and compiled
binaries, respectively.

Setting `GOPATH` correctly is critical when developing Go programs. Set and
export it as part of your login script.

Add `$GOPATH/bin` to your `PATH`, so you can run the go programs you install:

    PATH="$GOPATH/bin:$PATH"


Getting juju
============

The easiest way to get the source for `juju` is to use the `go get` command.

    go get -d -v github.com/juju/juju/...

This command will checkout the source of `juju` and inspect it for any unmet
Go package dependencies, downloading those as well. `go get` will also build and
install `juju` and its dependencies. To checkout without installing, use the
`-d` flag. More details on the `go get` flags are available using

    go help get

At this point you will have the git local repository of the `juju` source at
`$GOPATH/src/github.com/juju/juju`. The source for any dependent packages will
also be available inside `$GOPATH`. You can use `git pull --rebase`, or the
less convenient `go get -u github.com/juju/juju/...` to update the source
from time to time.
If you want to know more about contributing to `juju`, please read the
[CONTRIBUTING](CONTRIBUTING.md) companion to this file.

Installing prerequisites
------------------------

### *Making use of Makefile*

The `juju` repository contains a `Makefile`, which is the preferred way to install dependencies and other features.
It is advisable, when installing `juju` from source, to look at the [Makefile](./Makefile), located in `$GOPATH/src/github.com/juju/juju/Makefile`.

### *Dependencies*

Juju needs some dependencies in order to be installed and the preferred way to
collect the necessary packages is to use the provided `Makefile`.
The target `dep` will download the go packages listed in `Gopkg.lock`. The following bash code will install the dependencies.

    cd $GOPATH/src/github.com/juju/juju
    export JUJU_MAKE_GODEPS=true
    make dep

### *Runtime Dependencies*

You can use `make install-dependencies` or, if you prefer to install
them manually, check the Makefile target.

This will add some PPAs to ensure that you can install the required
golang and mongodb-server versions for precise onwards, in addition to the
other dependencies.

### *Build Dependencies*

Before you can build Juju, see
[Dependency management](CONTRIBUTING.md#dependency-management) section of
`CONTRIBUTING` to ensure you have build dependencies setup.


Building juju
=============

    go install -v github.com/juju/juju/...

Will build juju and install the binary commands into `$GOPATH/bin`. It is likely
if you have just completed the previous step to get the `juju` source, the
install process will produce no output, as the final executables are up-to-date.

If you do see any errors, there is a good chance they are due to changes in
juju's dependencies.  See the
[Dependency management](CONTRIBUTING.md#dependency-management) section of
`CONTRIBUTING` for more information on getting the dependencies right.


Using juju
==========

After following the steps above you will have the `juju` client installed in
`GOPATH/bin/juju`. You should ensure that this version of `juju` appears earlier
in your path than any packaged versions of `juju`, or older Python juju
commands. You can verify this using

    which juju

You should be able to bootstrap a local model now with the following:

    juju bootstrap localhost

Installing bash completion for juju
===================================

    make install-etc

Will install Bash completion for `juju` cli to `/etc/bash_completion.d/juju`. It does
dynamic completion for commands requiring service, unit or machine names (like e.g.
juju status <service>, juju ssh <instance>, juju terminate-machine <machine#>, etc),
by parsing cached `juju status` output for speedup. It also does command flags
completion by parsing `juju help ...` output.

Building Juju as a Snap Package
===============================

Building
--------
Make sure your snapcraft version is >= 2.26. Run `snapcraft` at the root of the repository. A snap will build.

Building with Local Changes
--------

Note that the default snapcraft.yaml file does a git clone of a local repository so if you need to include
any local changes, they have to be committed first as git ignores uncommitted changes during a local clone.

In some cases patches for dependencies are applied locally by invoking `patch` with snap scriptlets (see snapcraft.yaml).
This may cause successive rebuilds after `snapcraft clean -s build` to fail as patches will be applied
on an already patched code-base. In order to avoid that just clear all stages via `snapcraft clean`.

Current State
-------------
Classic mode.

Known Issues
------------
None. The snap shares your current credentials and environments as expected with a debian installed version.

Needed for confinement
----------------------
To enable strict mode, the following bugs need to be resolved, and the snap updated accordingly.

 * Missing support for abstract unix sockets (https://bugs.launchpad.net/snappy/+bug/1604967)
 * Needs SSH interface (https://bugs.launchpad.net/snappy/+bug/1606574)
 * Bash completion doesn't work (https://launchpad.net/bugs/1612303)
 * Juju plugin support (https://bugs.launchpad.net/juju/+bug/1628538)
 