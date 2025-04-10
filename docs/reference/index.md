(reference)=
# Reference

<!--Technical information on APIs, specifications, command reference, internals.-->

```{toctree}
:titlesonly:
:glob:
:hidden:

*

```

Welcome to Juju Reference docs -- our cast of characters (tools, concepts, entities, and processes) for the Juju story!

When you install the  {ref}`juju-cli`, and give Juju access to your {ref}`cloud <cloud>` (which can be any cloud from our long {ref}`list of supported clouds <list-of-supported-clouds>`, Kubernetes or otherwise), your Juju client bootstraps a {ref}`controller <controller>` into the cloud.

The controller serves the Juju API. You can also interact with it via {ref}`the Juju dashboard <juju-dashboard>`, [the Terraform Provider for Juju](https://canonical-terraform-provider-juju.readthedocs-hosted.com), [Python Libjuju](https://pythonlibjuju.readthedocs.io/en/latest/), or [JAAS](https://canonical-jaas-documentation.readthedocs-hosted.com/). However you access it, the controller talks to clouds to provision infrastructure for you, and to [Charmhub](https://charmhub.io/) to retrieve {ref}`charms <charm>` for you.

If you can access a Juju controller, you are a Juju {ref}`user <user>`. Depending on your {ref}`access level <user-access-levels>`, you can add further clouds to the controller, or create workspaces known as {ref}`models <model>`, etc., and eventually use the clouds and the charms to deploy, configure, scale, upgrade, etc., {ref}`applications <application>`, or to integrate applications within and between workspaces models and clouds. Possibilities are endless -- for example, you can deploy PostgreSQL on a Charmed Kubernetes that's on a Charmed OpenStack that's on a bare metal cloud such as {ref}`MAAS <cloud-maas>`, and then integrate it with an observability stack, to get a cloud-native, observed database on a resource-optimized private cloud with little effort and time.

You don't have to worry about the infrastructure -- the Juju controller {ref}`agent <agent>` takes care of all of that automatically for you, with sensible defaults. But, if you care, Juju also lets you manually control {ref}`availability zones <zone>`, {ref}`machines <machine>`, {ref}`subnets <subnet>`, {ref}`spaces <space>`, {ref}`secret backends <secret-backend>`, {ref}`storage <storage>`, {ref}`SSH keys <ssh-key>`, etc.

Whatever you provision contains a Juju {ref}`agent <agent>`, which is constantly in contact with the {ref}`controller agent <controller-agent>` to realize the state you declare to the controller using your Juju client. It does this through {ref}`workers <worker>`, a well-known pattern for watching and reacting to changes in state whose asynchronous nature ensures that your cloud operations run smoothly and in parallel.


<!--
## Juju as a whole

- {ref}`juju`



## Tools

- {ref}`containeragent`
- {ref}`hook`
- {ref}`hook-command`
- {ref}`juju-cli`
- {ref}`juju-dashboard`
- {ref}`juju-web-cli`
- {ref}`jujuc`
- {ref}`jujud`
- {ref}`pebble`

## Entities

- {ref}`action`
- {ref}`agent`
- {ref}`application`
- {ref}`bundle`
- {ref}`charm`
- {ref}`cloud`
- {ref}`configuration`
- {ref}`cloud`
- {ref}`configuration`
- {ref}`constraint`
- {ref}`controller`
- {ref}`credential`
- {ref}`log`
- {ref}`machine`
- {ref}`metadata`
- {ref}`model`
- {ref}`offer`
- {ref}`placement-directive`
- {ref}`plugin`
- {ref}`relation`
- {ref}`script`
- {ref}`secret`
- {ref}`space`
- {ref}`ssh-key`
- {ref}`storage`
- {ref}`subnet`
- {ref}`unit`
- {ref}`user`
- {ref}`worker`
- {ref}`zone`

## Processes

- {ref}`removing-things`
- {ref}`scaling`
- {ref}`upgrading-things`


## Concepts

- {ref}`high-availability`
- {ref}`telemetry`
-->