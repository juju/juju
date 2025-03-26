# Juju developer tutorial
This is an onboarding guide for new developers on the Juju project. It will
guide you through the developer documentation to help you understand how Juju is
put together.

This onboarding guide is not designed to be done all in one go. There is a lot
of content here which can quickly become overwhelming. It is recommended that you
use each section as an entry point to that part of Juju and consolidate the
knowledge by doing the exercise in each section and/or looking at a bug in that
area.

> See also: [Developer FAQ](../faq.md)

## Using Juju
First, make sure that you have a good idea of what Juju is, and what it can do.
Start by doing the Juju tutorial, and then try building a charm with the
charming tutorial.

Make full use of the Juju documentation to get an understanding of what Juju can
do, and make sure to report any gaps not covered by the docs to the team!

> See more: [Juju tutorial](https://juju.is/docs/juju/tutorial),
[Juju documentation](https://juju.is/)

## Understanding Juju
The Juju project was one of the first adopters of the Go language. In the early
days the ecosystem was far smaller and less mature. As a result, the Juju does
not always follow the standard Go way of doing things. The early developers
often had to come up with their own solutions so things may not be done as you
expect. This document will guide you through the way things are done in Juju.

Each section below explains a concept, or collection of concepts in Juju. Each
section includes an exercise to help consolidate the knowledge.

### The architecture of Juju
The [Architectural overview](../architectural-overview.md) is the best place to
understand how Juju is put together. It is also useful to understand which
binaries go where, for this see the [List of Agents](reference/agent.md).

**Exercise**: TODO

### The Juju API
As mentioned in the Architectural overview, the API endpoints are served using
facades. For more information on Facades see: [Facades](../facades.md).

**Exercise**: TODO

### State (MongoDB and DQLite)
The state of Juju is kept in MongoDB and DQLite. For information about how juju
interacts with mongo, see [MongoDB and Consistency](../MongoDB-and-Consistency.md).

DQLite: TODO

**Exercise**: TODO

### Workers and the dependency engine
Workers are generally in charge of long-running processes in juju. The
dependency engine managers which workers are running.
See [Workers](reference/worker.md) and [The dependency
engine](reference/dependency-package.md)

**Exercise**: TODO

### Cloud providers
The cloud providers are the abstraction layer that allow juju to work on
different clouds e.g. aws, openstack, MAAS, ect.
[Implementing environment providers](../implementing-environment-providers.md)

**Exercise**: TODO

### Juju clients (cli, terraform, python-libjuju)
The Juju CLI (`juju`) is the most used client of Juju, others include:
- [The Juju terraform provider](https://github.com/juju/terraform-provider-juju)
- [python-libjuju](https://github.com/juju/python-libjuju)
- [JAAS](https://jaas.ai)

There are many more services that make use of the Juju API.

**Exercise**: TODO

### Charms
Juju charms operate applications.

**Exercise:** build a charm by following a Charm SDK tutorial

> See more: (Write your first machine charm)[https://juju.is/docs/sdk/write-your-first-machine-charm]

### The Uniter

TODO

**Exercise**: TODO

### Contributing
See our [CONTRIBUTING.md](../CONTRIBUTING.md) for team etiquette around pull
requests. The [read before contributing guide](../read-before-contributing.md) has
a lot of useful tips about coding practice in Juju.

One way to get up to speed with current development is to look at our open PRs.
Feel free to comment on the PRs and ask questions about context and why things
are being done the way they are, along with anything else. 

> See more: [Juju pull requests](https://github.com/juju/juju/pulls)
## Quiz time!
After reading the documentation above, write answers to the following questions
and send your answers to a member of the Juju team to check. This should help
fill in gaps in your knowledge.

- (overview) Describe what happens in Juju when you do `juju config mysql foo=bar`. Explain
  how it progresses from the CLI, to the controller, and eventually to the charm
  code itself.

- (api) Explain how facades determine which client versions are compatible with which
  controller versions.

- TODO: Question on state

- (workers) A “flag” worker runs when some condition is true and not when it is false. It
  loops forever and takes no action. What use does a “flag” worker have in Juju?

- TODO: Question on the uniter

- TODO: Question on charms

- TODO: Question on debugging