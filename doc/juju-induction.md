# Juju Developer Induction
This is an onboarding guide for new developers on the Juju project. It will
guide you through the developer documentation to help you understand how Juju is
put together. At the end there is a quiz to test your knowledge!

## Using Juju
First, make sure that you have a good idea of what Juju is, and what it can do.
Start by doing the Juju tutorial, and then try building a charm with the
charming tutorial.

Make full use of the Juju documentation to get an understanding of what Juju can
do, and make sure to report any gaps not covered by the docs to the team!

> See more: [Juju tutorial](https://juju.is/docs/juju/tutorial), [Charming
tutorial (machines)](https://juju.is/docs/sdk/write-your-first-machine-charm),
[Juju documentation](https://juju.is/)

## Understanding Juju
The Juju project was one of the first adopters of the Go language. In the early
days the ecosystem was for smaller and less mature. As a result, the Juju does
not always follow the standard Go way of doing things. The early developers
often had to come up with their own solutions. 

This is all to say, the way things are done in Juju is not necessary the normal
way. This document will guide you through what you need to know to understand
the components of Juju.

### The architecture of Juju
The [Architectural overview](architectural-overview.md) is the best place to
understand how Juju is put together. It is also useful to understand which binaries go
where, for this see the [List of Agents](dev/reference/agent.md).



### The Juju API
As mentioned in the Architectural overview, the API endpoints are served using
facades. For more information on Facades see: [Facades](facades.md).

### Workers and the dependency engine
Workers are generally in charge of long-running processes in juju. The
dependency engine managers which workers are running.
See [Workers](dev/reference/worker.md) and [The dependency
engine](dev/reference/dependency-package.md)

### State (mongo and DQLite)
The state of Juju is kept in MongoDB. For information about how juju interacts
with mongo, see [MongoDB and Consistency](MongoDB-and-Consistency.md).

### Cloud Providers
The cloud providers are the abstraction layer that allow juju to work on
different clouds e.g. aws, openstack, maas, ect.
[Implementing enviroment providers](implementing-environment-providers.md)

### Juju clients (cli, terraform, python-libjuju)
The Juju cli is the most used client of Juju, others include:
- [The Juju terraform provider](https://github.com/juju/terraform-provider-juju)
- [python-libjuju](https://github.com/juju/python-libjuju)
- [JAAS](https://jaas.ai)

### Relations

There are many more services that make use of the Juju API.
### Contributing
See [CONTRIBUING.md](CONTRIBUTING.md) for etiquette around pull requests. The
[read before contributing guide](read-before-contributing.md) has a lot of
useful tips about coding practice in Juju.

## Quiz time!
After reading the documentation above, write answers to the following questions
and send your answers to a member of the Juju team to check. This should help
fill in gaps in your knowledge.

- Describe what happens in Juju when you do `juju config mysql foo=bar`. Explain
  how it progresses from the CLI, to the controller, and eventually to the charm
  code itself.

- A “flag” worker runs when some condition is true and not when it is false. It
  loops forever and takes no action. What use does a “flag” worker have in Juju?

- Explain how facades determine which client versions are compatible with which
  controller versions.

- TODO: Question on the uniter

- TODO: Question on mongo

- TODO: Question on debugging