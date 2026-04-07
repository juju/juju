---
myst:
  html_meta:
    description: "Technical reference for Juju: APIs, specifications, CLI commands, architecture, and comprehensive documentation of all Juju components."
---

(reference)=
# Reference

Technical specifications, APIs, and comprehensive details of all Juju components.

## Cloud and Charmhub

Clouds provide compute resources for your infrastructure. Charmhub provides software packages (charms) for deploying and managing applications.

- {ref}`cloud` • {ref}`credential` • {ref}`metadata` • [Charmhub](https://charmhub.io/) • [`juju-controller` charm](https://charmhub.io/juju-controller)

## Client

Command-line and web interfaces for managing controllers, models, and deployments.

- {ref}`client` • {ref}`juju-cli` • {ref}`juju-web-cli` • {ref}`juju-dashboard`

## Controller

The central management service that coordinates between your client, clouds, and deployed resources.

- {ref}`controller` • {ref}`log` • {ref}`high-availability` • {ref}`scaling`

## User

User accounts provide authentication and authorization for accessing and managing Juju resources.

- {ref}`user`

## Infrastructure and applications

Juju automatically provisions infrastructure when you deploy applications, though you can also customize resources before, during, or after deployment.

You organize your work into workspaces called 'models'.

- {ref}`model`

To deploy and operate your applications, you use software packages called 'charms', which can be packaged together as 'bundles' and run as 'applications' composed of 'units'.

- {ref}`charm` • {ref}`bundle` • {ref}`application` • {ref}`unit`

These applications run on infrastructure resources that Juju provisions from your cloud: machines with storage, organized into spaces, subnets, and zones, controlled through constraints and placement directives. You can access machines using SSH keys.

- {ref}`machine` • {ref}`ssh-key` • {ref}`storage` • {ref}`space` • {ref}`subnet` • {ref}`zone` • {ref}`constraint` • {ref}`placement-directive`

On each machine, Juju installs an agent to execute the charm code and manage your workload.

- {ref}`agent` • {ref}`jujud` • {ref}`jujuc` • {ref}`containeragent` • {ref}`pebble` • {ref}`hook` • {ref}`hook-command`

You can run actions and scripts on deployed applications, integrate them through relations (including cross-model via offers), provide configuration and secrets, and scale them or enable high availability through units.

- {ref}`action` • {ref}`script` • {ref}`relation` • {ref}`offer` • {ref}`configuration` • {ref}`secret` • {ref}`high-availability` • {ref}`scaling`

## Other

- {ref}`juju` • {ref}`telemetry` • {ref}`removing-things` • {ref}`upgrading-things` • {ref}`worker`

```{toctree}
:titlesonly:
:glob:
:hidden:

*

```
