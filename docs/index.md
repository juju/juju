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

* **Learn more about Juju**: {ref}`Get started with Juju <tutorial>` • {ref}`Architecture <juju-architecture>` • {ref}`Security <juju-security>` • {ref}`Performance <performance-with-juju>`
* **Set up Juju**: {ref}`Install juju <install-juju>` • {ref}`Bootstrap a controller <bootstrap-a-controller>` • {ref}`Connect a cloud <add-a-cloud>`
* **Handle authentication and authorization**: {ref}`Add a user <add-a-user>` • {ref}`Manage user access <manage-user-access>`
* **Deploy infrastructure and applications**: {ref}`Deploy <deploy-an-application>` • {ref}`Configure <configure-an-application>` • {ref}`Integrate <integrate-an-application-with-another-application>` • {ref}`Scale <scale-an-application>` • {ref}`Upgrade <upgrade-an-application>`

## How this documentation is organised

This documentation uses the [Diátaxis documentation structure](https://diataxis.fr/).
- The {ref}`Tutorial <tutorial>` takes you step-by-step through deploying your first application with Juju.
- {ref}`How-to guides <how-to-guides>` provide step-by-step instructions for key operations and common tasks.
- {ref}`Reference <reference>` provides technical specifications, APIs, and comprehensive details of all Juju components.
- {ref}`Explanation <explanation>` offers discussion and clarification of key topics, providing background and context.

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