(upgrading-things)=
# Upgrading things
> See also: {ref}`upgrade-your-deployment`
<!--TODO Revisit. We might not need this doc in this form anymore.-->


In Juju, upgrades can happen at the level of the `juju` CLI client, the controller, the model, the application, and the machine. 

> See more:
> - {ref}`upgrade-juju`
> - {ref}`upgrade-a-controller`
> - {ref}`upgrade-a-model`
> - {ref}`upgrade-an-application`
> - {ref}`upgrade-a-machine`

Upgrades to the client, the controller, and the model are typically related: You upgrade the client by refreshing the `juju` snap, then you upgrade the controller and the model, which is done as follows:

1. If you're upgrading between
   - patch versions (e.g. 2.9.25 -> 2.9.26)
   - minor versions before 3.0 (e.g. 2.7 -> 2.8)

   you can upgrade in place via `upgrade-controller` and `upgrade-model`.

2. If you're upgrading between
   - major versions (e.g. 2.9 -> 3.0)
   - minor versions after 3.0 (e.g. 3.0 -> 3.1)

   you need to bootstrap a new controller, migrate your models to it, and then run `upgrade-model`. (This is because upgrades are risky, and model migration is a relatively safer way to upgrade than upgrading in place.) It is also important to pay attention to the allowed upgrade paths -- for example, to update from `juju v2.2` to `juju v3.0`, one must first upgrade the client, controller, and model to `juju v2.9` and then perform a second upgrade to `juju v3.0`. 

Application upgrades and machine upgrades are usually completely independent of this and of each other -- the former concerns the version of a charm and the latter the version of Ubuntu running on a machine. The only exception (relevant for upgrades to `3.0`) is when you upgrade across versions where the, e.g., a new controller has dropped support for, e.g., base (OS, series) required by some charm. In that case, before upgrading the controller, you'll want to make sure that all the existing machines (usually already attached to some application) have been upgraded to a supported series (`upgrade-machine`; going away in Juju 4) and also that any new machines provisioned for an application will use a supported series (`refresh <charm>`, `set-application-base <charm> <base>`). See more: {ref}`upgrade-your-deployment`.


## Agent software and related components

In general, the upgrade of the agent software is independent of the following:

-   Client software

    Although client and server software are independent, an upgrade of the agents is an opportune time to first upgrade the client software.

-   Charms

    Charms and agent versions are orthogonal to one another. There is no necessity to upgrade charms before or after an upgrade of the agents.

-   Running workloads

    Workloads running are independent of Juju so a downtime maintenance window is not normally required in order to perform an upgrade.

## Version nomenclature and the auto-selection algorithm

A version is denoted by:

`major.minor.patch`

For instance: `2.0.1`

When not specifying a version to upgrade to ('--version') an algorithm will be used to auto-select a version.

Rules:

1.  If the agent major version matches the client major version, the version selected is minor+1. If such a minor version is not available then the next patch version is chosen.

2.  If the agent major version does not match the client major version, the version selected is that of the client version.

To demonstrate, let the available online versions be: 1.25.1, 2.02, 2.03, 2.1, and 2.2. This gives:

-   client 2.03, agent 2.01 -&gt; upgrade to 2.02
-   client 1.25, agent 1.25 -&gt; upgrade to 1.25.1
-   client 2.1, agent 1.25 -&gt; upgrade to 2.1

The stable online agent software is found here: https://streams.canonical.com/juju/tools/agent/
