---
relatedlinks: "[Juju &nbsp; ecosystem &nbsp; docs](https://juju.is/docs), [Terraform &nbsp; Provider &nbsp; Juju &nbsp; docs](https://documentation.ubuntu.com/terraform-provider-juju/), [JAAS &nbsp; docs](https://documentation.ubuntu.com/jaas/), [Jubilant &nbsp; docs](https://documentation.ubuntu.com/jubilant/), [Charmcraft &nbsp; docs](https://documentation.ubuntu.com/charmcraft/), [Ops &nbsp; docs](https://documentation.ubuntu.com/ops/), [Canonical &nbsp; Charmlibs &nbsp; docs](https://canonical-charmlibs.readthedocs-hosted.com/)"
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
```

Juju is an open source orchestration engine for software operators that enables the deployment, integration, and lifecycle management of applications in the cloud using special software operators called ‘charms’.

Juju and charms provide a simple, consistent, and repeatable way to install, provision, maintain, update, upgrade, and integrate applications on and across Kubernetes containers, Linux containers, virtual machines, and bare metal machines, on public or private cloud.

Application- and cloud-specific challenges can make operations complex, especially with sophisticated workloads in hybrid environments. Juju and charms abstract away that complexity, making all clouds and operations feel the same -- at any scale, on any cloud.

Whether you are a CIO or SysAdmin, DevOps engineer, or SRE, Juju helps you take control.

## In this documentation

- **Learn more about Juju:** {ref}`Architecture <juju-architecture>`, {ref}`Security <juju-security>`, {ref}`Performance <performance-with-juju>`
- **Set up Juju:** {ref}`Install juju <install-juju>`, {ref}`Bootstrap a controller <bootstrap-a-controller>`, {ref}`Connect a cloud <add-a-cloud>`, {ref}`Add a model <add-a-model>`
- **Handle authentication and authorization:** {ref}`SSH keys <manage-ssh-keys>`, {ref}`Users <manage-users>`
- **Deploy infrastructure and applications:** {ref}`Deploy <deploy-an-application>`, {ref}`Configure <configure-an-application>`, {ref}`Integrate <integrate-an-application-with-another-application>`, {ref}`Scale <scale-an-application>`, {ref}`Upgrade <upgrade-an-application>`, etc.


````{grid} 1 1 2 2

```{grid-item-card} [Tutorial](/index)
:link: tutorial/index
:link-type: doc

**Start here**: a hands-on introduction to Juju for new users
```

```{grid-item-card} [How-to guides](/index)
:link: howto/index
:link-type: doc

**Step-by-step guides** covering key operations and common tasks
```

````

````{grid} 1 1 2 2
:reverse:

```{grid-item-card} [Reference](/index)
:link: reference/index
:link-type: doc

**Technical information** - specifications, APIs, architecture
```

```{grid-item-card} [Explanation](/index)
:link: explanation/index
:link-type: doc

**Discussion and clarification** of key topics
```

````


(project-and-community)=
## Project and community

Juju is an open source project that warmly welcomes community projects, contributions, suggestions, fixes and constructive feedback.

* [Roadmap & Releases](./reference/juju/juju-roadmap-and-releases.md)
* [Code of Conduct ](https://ubuntu.com/community/code-of-conduct)
* [Join our chat](https://matrix.to/#/#charmhub-juju:ubuntu.com)
* [Join our forum ](https://discourse.charmhub.io/)
* [Report a bug](https://github.com/juju/juju/issues)
* [Contribute](https://github.com/juju/juju/blob/main/CONTRIBUTING.md)
* [Visit our careers page](https://juju.is/careers)

Thinking about using Juju for your next project? [Get in touch](https://canonical.com/contact-us)!