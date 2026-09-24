---
myst:
  html_meta:
    description: "Learn to deploy and manage applications on Kubernetes or machines with Juju, an open-source orchestration engine for operators called charms."
relatedlinks: "[Charmcraft](https://documentation.ubuntu.com/charmcraft/), [Charmlibs](https://canonical-charmlibs.readthedocs-hosted.com/), [Concierge](https://github.com/canonical/concierge), [JAAS](https://documentation.ubuntu.com/jaas/), [Jubilant](https://documentation.ubuntu.com/jubilant/), [Ops](https://documentation.ubuntu.com/ops/), [Pebble](https://documentation.ubuntu.com/pebble/), [Terraform &nbsp; Provider &nbsp; for &nbsp; Juju](https://documentation.ubuntu.com/terraform-provider-juju/)"
---

(home)=
# Juju documentation

```{toctree}
:maxdepth: 2
:hidden:

Tutorial <tutorial/index>
howto/index
reference/index
explanation/index
For contributors <contributor/index>
releasenotes/index
```

Juju is an open source orchestration engine for software operators that enables the deployment, integration, and lifecycle management of applications in the cloud using special software operators called 'charms'.

Juju and charms provide a simple, consistent, and repeatable way to install, provision, maintain, update, upgrade, and integrate applications on and across Kubernetes containers, Linux containers, virtual machines, and bare metal machines, on public or private cloud.

Application- and cloud-specific challenges can make operations complex, especially with sophisticated workloads in hybrid environments. Juju and charms abstract away that complexity, making all clouds and operations feel the same -- at any scale, on any cloud.

Whether you are a CIO or SysAdmin, DevOps engineer, or SRE, Juju helps you take control.

## In this documentation

**Point of entry**

Start here if you're new to Juju.

* Tutorial: {doc}`Get started with Juju <tutorial/index>`
* Installation: {doc}`Install Juju <howto/manage-juju>`

**Models and charms**

Juju models business deployment logic through charms; charms describe how an application is deployed.

```{eval-rst}
.. domain:: Models and charms
   :suppress-warnings:

   .. slice:: Models

      :doc:`Reference <reference/model>` slice
      :doc:`Manage models <howto/manage-models>`
      :doc:`Model configuration keys <reference/configuration/list-of-model-configuration-keys>`

   .. slice:: Charmed applications

      :doc:`Charm reference <reference/charm>`
      :doc:`Manage charms <howto/manage-charms>`
      :doc:`Application reference <reference/application>`
      :doc:`Manage applications <howto/manage-applications>`
      :doc:`Bundle reference <reference/bundle>`

   .. slice:: Integration

      :doc:`Relations <reference/relation>`
      :doc:`Manage relations <howto/manage-relations>`
      :doc:`Offers <reference/offer>`
      :doc:`Manage offers <howto/manage-offers>`

   .. slice:: Configuration and secrets

      :doc:`Configuration <reference/configuration>`
      :doc:`Secrets <reference/secret>`
      :doc:`Manage secrets <howto/manage-secrets>`
      :doc:`Manage secret backends <howto/manage-secret-backends>`

   .. slice:: Actions and resources

      :doc:`Actions <reference/action>`
      :doc:`Manage actions <howto/manage-actions>`
      :doc:`Charm resources <reference/resource-charm>`
      :doc:`Manage charm resources <howto/manage-charm-resources>`

   .. slice:: Units

      :doc:`Unit reference <reference/unit>`
      :doc:`Manage units <howto/manage-units>`
      :doc:`Scaling <reference/scaling>`
```

**Juju's core machinery**

The CLI, controller, and agents form the engine that coordinates between the application and cloud layers.

```{eval-rst}
.. domain:: Juju's core machinery
   :suppress-warnings:

   .. slice:: Architecture

      :doc:`Juju architecture <explanation/juju-architecture>`

   .. slice:: Client

      :doc:`Reference <reference/juju-cli>` slice
      :doc:`Manage Juju <howto/manage-juju>`

   .. slice:: Controller

      :doc:`Reference <reference/controller>` slice
      :doc:`Manage controllers <howto/manage-controllers>`
      :doc:`Controller configuration keys <reference/configuration/list-of-controller-configuration-keys>`

   .. slice:: Database

      :doc:`Reference <reference/database>` slice
      :doc:`Manage the databases <howto/manage-the-databases>`
      :doc:`Juju DB REPL <reference/juju-db-repl>`

   .. slice:: Agents

      :doc:`Reference <reference/agent>` slice

   .. slice:: Pebble

      :doc:`Reference <reference/pebble>` slice

   .. slice:: Hooks and hook commands

      :doc:`Hook reference <reference/hook>`
      :doc:`Hook command reference <reference/hook-command>`

   .. slice:: Scripts

      :doc:`Reference <reference/script>` slice
```

**Enterprise features**

Additional capabilities for production and enterprise deployments, including access control, observability, and high availability.

```{eval-rst}
.. domain:: Enterprise features
   :suppress-warnings:

   .. slice:: Authentication and authorisation

      :doc:`Users <reference/user>`
      :doc:`Manage users <howto/manage-users>`
      :doc:`SSH keys <reference/ssh-key>`
      :doc:`Manage SSH keys <howto/manage-ssh-keys>`

   .. slice:: High availability

      :doc:`Reference <reference/high-availability>` slice

   .. slice:: Observability and monitoring

      :doc:`Manage logs <howto/manage-logs>`
      :doc:`Logs reference <reference/log>`
      :doc:`Telemetry reference <reference/telemetry>`

   .. slice:: Juju Dashboard

      :doc:`Reference <reference/juju-dashboard>` slice
      :doc:`Manage the Juju Dashboard <howto/manage-the-juju-dashboard>`
```

**Clouds**

Juju provisions and manages the cloud resources — machines, networking, storage — that applications run on.

```{eval-rst}
.. domain:: Clouds
   :suppress-warnings:

   .. slice:: Basics

      :doc:`Cloud reference <reference/cloud>`
      :doc:`List of supported clouds <reference/cloud/list-of-supported-clouds/index>`

   .. slice:: Working with clouds

      :doc:`Manage clouds <howto/manage-clouds>`

   .. slice:: Credentials

      :doc:`Credential reference <reference/credential>`
      :doc:`Manage credentials <howto/manage-credentials>`

   .. slice:: Metadata

      :doc:`Simplestreams metadata <reference/metadata>`
      :doc:`Manage metadata <howto/manage-metadata>`

   .. slice:: Zones

      :doc:`Zone reference <reference/zone>`

   .. slice:: Compute

      :doc:`Resource (compute) <reference/resource-compute>`
      :doc:`Machine reference <reference/machine>`
      :doc:`Manage machines <howto/manage-machines>`
      :doc:`Constraints <reference/constraint>`
      :doc:`Placement directives <reference/placement-directive>`

   .. slice:: Networking

      :doc:`Space reference <reference/space>`
      :doc:`Manage spaces <howto/manage-spaces>`
      :doc:`Subnet reference <reference/subnet>`
      :doc:`Manage subnets <howto/manage-subnets>`

   .. slice:: Storage

      :doc:`Storage reference <reference/storage>`
      :doc:`Manage storage <howto/manage-storage>`
      :doc:`Manage storage pools <howto/manage-storage-pools>`
```

**Security and performance**

Guidance on securing and optimising your Juju deployment.

```{eval-rst}
.. domain:: Security and performance
   :suppress-warnings:

   .. slice:: Security

      :doc:`Juju security <explanation/juju-security>`

   .. slice:: Performance

      :doc:`Performance with Juju <explanation/juju-performance>`
```

**Deployment lifecycle**

End-to-end procedures for standing up, maintaining, and tearing down a Juju deployment.

```{eval-rst}
.. domain:: Deployment lifecycle
   :suppress-warnings:

   .. slice:: Set up

      :doc:`Set up your deployment <howto/manage-your-juju-deployment/set-up-your-juju-deployment>`
      :doc:`Set up for local testing <howto/manage-your-juju-deployment/set-up-your-juju-deployment-local-testing-and-development>`
      :doc:`Set up offline <howto/manage-your-juju-deployment/set-up-your-juju-deployment-offline>`

   .. slice:: Harden

      :doc:`Harden your deployment <howto/manage-your-juju-deployment/harden-your-juju-deployment>`

   .. slice:: Upgrade

      :doc:`Upgrade your deployment <howto/manage-your-juju-deployment/upgrade-your-juju-deployment>`
      :doc:`From 3.6 to 4.0 <howto/upgrade-your-juju-deployment-from-36-to-40>`

   .. slice:: Troubleshoot

      :doc:`Troubleshoot your deployment <howto/manage-your-juju-deployment/troubleshoot-your-juju-deployment>`

   .. slice:: Tear down

      :doc:`Tear things down <howto/manage-your-juju-deployment/tear-down-your-juju-deployment-local-testing-and-development>`
```

## How this documentation is organised

This documentation uses the [Diátaxis documentation structure](https://diataxis.fr/).
- The {doc}`Tutorial <tutorial/index>` takes you step-by-step through deploying your first application with Juju.
- {doc}`How-to guides <howto/index>` provide step-by-step instructions for key operations and common tasks.
- {doc}`Reference <reference/index>` provides technical specifications, APIs, and comprehensive details of all Juju components.
- {doc}`Explanation <explanation/index>` offers discussion and clarification of key topics, providing background and context.

(project-and-community)=
## Project and community

Juju is an open source project that warmly welcomes community projects, contributions, suggestions, fixes and constructive feedback.

### Get involved

* [Join our chat](https://matrix.to/#/#charmhub-juju:ubuntu.com)
* [Join our forum ](https://discourse.charmhub.io/)
* [Report a bug](https://github.com/juju/juju/issues)
* [Contribute](https://github.com/juju/juju/blob/main/CONTRIBUTING.md)
* [Visit our careers page](https://canonical.com/careers/engineering)

### Releases

* [Roadmap & Releases](releasenotes/index.md)

### Governance and policies

* [Code of Conduct](https://ubuntu.com/community/code-of-conduct)

### Commercial support

Thinking about using Juju for your next project? [Get in touch](https://canonical.com/contact-us)!
