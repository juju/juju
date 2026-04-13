---
myst:
  html_meta:
    description: "Technical reference for Juju: APIs, specifications, CLI commands, architecture, and comprehensive documentation of all Juju components."
---

(reference)=
# Reference

Technical specifications, APIs, and comprehensive details of all Juju components.

## Platform

Juju is an orchestration platform that includes the CLI client, agent binaries, and supporting infrastructure for managing cloud deployments.

- {ref}`juju`

## Cloud and Charmhub

In Juju, deployments draw from two external sources. Clouds provide compute resources for your infrastructure. Charmhub provides charms -- operators for deploying and managing applications.

- {ref}`cloud`
- {ref}`credential`
- {ref}`metadata`
- [Charmhub](https://charmhub.io/)
- [`juju-controller` charm](https://charmhub.io/juju-controller)

## Client

In Juju, you interact with these resources through clients -- command-line and web interfaces for managing controllers, models, and deployments.

- {ref}`client`
- {ref}`juju-cli`
- {ref}`juju-web-cli`
- {ref}`juju-dashboard`

## Controller

Clients connect to a controller -- the central management service that coordinates between clouds, Charmhub, and your deployed resources.

- {ref}`controller`
- {ref}`log`
- {ref}`telemetry`
- {ref}`high-availability`
- {ref}`scaling`

## Users

Controller access requires user authentication. User accounts provide authentication and authorization for managing Juju resources.

- {ref}`user`

## Infrastructure and applications

Once authenticated, users work with deployments. Within a controller, deployments are organized into models -- logical containers for applications, infrastructure, and their supporting components. Each model draws resources from a single cloud.

- {ref}`model`

Models contain applications deployed from charms and composed of units (individual instances).

- {ref}`charm`
- {ref}`application`
- {ref}`unit`

Applications connect to each other through relations endpoints -- integration points between compatible interfaces. Offers enable cross-model relations.

- {ref}`relation`
- {ref}`offer`

Applications are managed through configuration, secrets, actions, and scripts, and support scaling and high availability. Charms may require resources.

- {ref}`configuration`
- {ref}`charm-resource`
- {ref}`secret`
- {ref}`action`
- {ref}`script`
- {ref}`high-availability`
- {ref}`scaling`

Supporting infrastructure -- machines, storage volumes, network spaces and subnets, and availability zones -- is provisioned from the cloud. Constraints and placement directives control how resources are selected and allocated. SSH keys provide access.

- {ref}`machine`
- {ref}`storage`
- {ref}`constraint`
- {ref}`placement-directive`
- {ref}`space`
- {ref}`subnet`
- {ref}`zone`
- {ref}`ssh-key`

On each machine, agents (`jujud` on machines, `containeragent` on Kubernetes) execute charm code through hooks. Charms use hook commands (provided by `jujuc`) to interact with Juju. On Kubernetes, `containeragent` also orchestrates workload containers using Pebble.

- {ref}`agent`
- {ref}`jujud`
- {ref}`hook`
- {ref}`hook-command`
- {ref}`jujuc`
- {ref}`containeragent`
- {ref}`pebble`
Lifecycle management

Removing and upgrading are cross-cutting operations that apply across multiple resource types -- from individual units and applications to entire models and controllers.

- {ref}`telemetry`
- {ref}`removing-things`
- {ref}`upgrading-things`

```{toctree}
:titlesonly:
:glob:
:hidden:

*

```
