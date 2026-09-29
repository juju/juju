---
myst:
  html_meta:
    description: "Technical reference for Juju: APIs, specifications, CLI commands, architecture, and comprehensive documentation of all Juju components."
---

(reference)=
# Reference

Technical specifications, APIs, and comprehensive details of all Juju components.

## Platform

Juju's platform is what you install and run: the clients you use, the controller they talk to, and the agents that carry out the work on your machines. Which versions of these components work together is covered by the cross-version compatibility reference.

- {ref}`juju-cross-version-compatibility`

### Client

You interact with Juju through a client -- a command-line or web interface for managing controllers, models, and deployments.

- {ref}`client`
- {ref}`juju-cli`
- {ref}`juju-web-cli`
- {ref}`juju-dashboard`
- {ref}`juju-db-repl`

### Controller

Clients connect to a controller -- the central management service that coordinates between clouds, [Charmhub](https://charmhub.io/), and your deployed resources. The controller records what you declare in its database and is itself deployed through the [`juju-controller` charm](https://charmhub.io/juju-controller).

- {ref}`controller`
- {ref}`database`

### Agents and charm runtime

On each machine, agents (`jujuagentd` on machines, `containeragent` on Kubernetes) execute charm code through hooks. Charms use hook commands (provided by `jujuc`) to interact with Juju. On Kubernetes, `containeragent` also orchestrates workload containers using Pebble.

- {ref}`agent`
- {ref}`jujud`
- {ref}`containeragent`
- {ref}`hook`
- {ref}`hook-command`
- {ref}`jujuc`
- {ref}`pebble`

## Users

Controller access requires user authentication, and what a user can do is a matter of authorization. User accounts cover both.

- {ref}`user`

## Cloud

Clouds provide the compute resources for your infrastructure, and Juju needs credentials to use them. Charms come from a second external source, [Charmhub](https://charmhub.io/).

- {ref}`cloud`
- {ref}`credential`
- {ref}`metadata`

## Models and applications

Within a controller, deployments are organized into models -- logical containers for applications, infrastructure, and their supporting components. Each model draws resources from a single cloud. Models contain applications deployed from charms or bundles and composed of units. Applications connect to each other through relations, and offers enable cross-model relations. Applications are configured through configuration, secrets, actions, and scripts, and charms may require resources.

- {ref}`model`
- {ref}`charm`
- {ref}`bundle`
- {ref}`application`
- {ref}`unit`
- {ref}`relation`
- {ref}`offer`
- {ref}`configuration`
- {ref}`charm-resource`
- {ref}`secret`
- {ref}`action`
- {ref}`script`

## Infrastructure

Supporting infrastructure -- machines and other compute resources, storage volumes, network spaces and subnets, and availability zones -- is provisioned from the cloud. Constraints and placement directives control how resources are selected and allocated. SSH keys provide access.

- {ref}`machine`
- {ref}`resource-compute`
- {ref}`storage`
- {ref}`space`
- {ref}`subnet`
- {ref}`zone`
- {ref}`constraint`
- {ref}`placement-directive`
- {ref}`ssh-key`

## Observability

Juju records what happens in a deployment and reports on its health.

- {ref}`log`
- {ref}`telemetry`
- {ref}`status`

## Cross-cutting processes

Some operations apply across many kinds of resource -- from individual units and applications to entire models and controllers.

- {ref}`scaling`
- {ref}`high-availability`
- {ref}`removing-things`
- {ref}`upgrading-things`

```{toctree}
:titlesonly:
:glob:
:hidden:

*

```
