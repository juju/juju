(compile-and-run-juju-agents-on-different-architectures)=
# Compile and run Juju agents on different architectures

It's often common practice that a Juju developer needs to test out new Juju code
on machines that are not the same operating system or architecture to that of
their current host. Alternatively in this workflow it is commonplace to want to perform Juju upgrades.

Examples of this would be:
- ubuntu/amd64 -> ubuntu/s390x
- ubuntu/amd64 -> centos/amd64
- macos/amd64 -> ubuntu/amd64
- macos/arm64 -> ubutnu/amd64

Doing this has been difficult in the past but with changes to the juju `Makefile`
we can now build simple streams for multiple platforms and versions.


## Create a local development simplestreams repository

We start by telling the Juju build systems that we would like a local simple
streams repository created for a given set of Go style os/arch pairs. For this
example we would like a simple stream of our development changes that have
artifacts for `linux/amd64` and `linux/arm64`.

The Juju build system is going place artifacts in a simplestream directory. You
can optionally specify where you would like this directory to be by exporting
`JUJU_METADATA_SOURCE` in your environment or simply let the build system choose
for you.

```text
export JUJU_METADATA_SOURCE=<local_simplestreams_path>
```

This is created by running:

```text
AGENT_PACKAGE_PLATFORMS="linux/amd64 linux/arm64" make simplestreams
```

This process will build all the juju agent binaries for the platforms specified
above and package them into a simple streams repository.

The end of the output for this `make` command will also provide the user with an
`export` statement that the user should run to help the juju bootstrap command
automatically find this local simple streams repository. Example output:

```text
export JUJU_METADATA_SOURCE="...."
```

You can ignore the above export requirement if you have previously done this in
a previous step.

You can now bootstrap using this simple streams repository using the usual:

```text
juju bootstrap cloudx
```

```{note}
You may need to specify additional `bootstrap-constrains` to help juju
choose the correct architecture.
```

##  Recompile, upload, update, and run the binary in the the controller

Often once we have a bootstrapped controller we want to upgrade the controller
for testing or time reasons.

To do this we now need to run the `make simplestreams` target again but this
time we also want to supply the build system with a build number.

```text
JUJU_BUILD_NUMBER=1 AGENT_PACKAGE_PLATFORMS="linux/amd64 linux/arm64" make simplestreams
```

```{note}
A key way to tell if this has worked is by looking at the output of
the command to confirm the new version is being used.
```

You will also be prompted to export the `JUJU_METADATA_SOURCE` again. This step
can safely be ignored if previously done from a previous step.

Next we need to get these new version artifacts on to the juju controller that
we want to upgrade.

```text
juju sync-agent-binary --version=<version>
```
**Where `<version>` is the Juju version we are upgrading to including the build
number from above.**

We can now perform the upgrade using:

```text
juju upgrade-controller --agent-version <version>
```
**Where `<version>` is the Juju version we are upgrading to including the build
number from above.**

## Current limitations & future changes
- Does not support OCI artifacts including upgrading of OCI deployed controllers
  and agents.
- Would like to integrate work by @hpidcock for a simplestreams server that
  continuously builds and updates.