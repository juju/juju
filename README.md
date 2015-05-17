juju
====

juju is devops distilled.

Juju enables you to use [Charms](http://juju.ubuntu.com/charms) to deploy your application architectures to EC2, OpenStack,
Azure, HP your data center and even your own Ubuntu based laptop.
Moving between environments is simple giving you the flexibility to switch hosts
whenever you want — for free.

For more information, see the [docs](https://juju.ubuntu.com/docs/).

Getting started
===============

`juju` is written in Go (http://golang.org), a modern, compiled, statically typed,
concurrent language. This document describes how to build `juju` from source.

If you are looking for binary releases of `juju`, they are available from the Juju
stable PPA, `https://launchpad.net/~juju/+archive/stable`, and can be installed with:

    sudo apt-add-repository ppa:juju/stable
    sudo apt-get update
    sudo apt-get install juju

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
`$GOPATH/github.com/juju/juju`. The source for any dependent packages will
also be available inside `$GOPATH`. You can use `git pull --rebase`, or the 
less convenient `go get -u github.com/juju/juju/...` to update the source
from time to time.
If you want to know more about contributing to `juju`, please read the
[CONTRIBUTING](CONTRIBUTING.md) companion to this file.

Installing prerequisites
------------------------

You can use `make install-dependencies` or, if you prefer to install
them manually, check the Makefile target.

This will add some PPAs to ensure that you can install the required
golang and mongodb-server versions for precise onwards, in addition to the
other dependencies.


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

You should be able to bootstrap a local environment now with the following
(Note: the use of sudo for bootstrap here is only required for the local
provider because it uses LXC, which requires root privileges)

    juju init
    juju switch local
    sudo juju bootstrap

--upload-tools
--------------

The `juju` client program, and the juju 'tools' are deployed in lockstep. When a
release of `juju` is made, the compiled tools matching that version of juju
are extracted and uploaded to a known location. This consumes a release version
number, and implies that no tools are available for the next, development, version
of juju. Therefore, when using the development version of juju you will need to
pass an additional flag, `--upload-tools` to instruct the `juju` client to build
a set of tools from source and upload them to the environment as part of the
bootstrap process.

    juju bootstrap -e your-environment --upload-tools {--debug}


Installing bash completion for juju
===================================

    make install-etc

Will install Bash completion for `juju` cli to `/etc/bash_completion.d/juju`. It does
dynamic completion for commands requiring service, unit or machine names (like e.g.
juju status <service>, juju ssh <instance>, juju terminate-machine <machine#>, etc),
by parsing cached `juju status` output for speedup. It also does command flags
completion by parsing `juju help ...` output.
