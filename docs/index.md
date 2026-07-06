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

**Point of entry:**
* Tutorial: {ref}`Get started with Juju <tutorial>`
* Installation: {ref}`Install Juju <install-juju>`

**Model-driven application lifecycle management:**
* Models: {ref}`Reference <model>` | {ref}`Manage models <manage-models>`
* Charmed applications: {ref}`Charm reference <charm>` | {ref}`Manage charms <manage-charms>` | {ref}`Application reference <application>` | {ref}`Manage applications <manage-applications>`
* Units: {ref}`Reference <unit>` | {ref}`Manage units <manage-units>`
* Bundles: {ref}`Reference <bundle>`
* Charm-defined functionality: {ref}`Actions <action>` | {ref}`Manage actions <manage-actions>` | {ref}`Relations <relation>` | {ref}`Manage relations <manage-relations>` | {ref}`Resources <charm-resource>` | {ref}`Manage charm resources <manage-charm-resources>` | {ref}`Configurations <application-configuration>` | {ref}`Configure an application <configure-an-application>`
* Offers (cross-model relations): {ref}`Reference <offer>` | {ref}`Manage offers <manage-offers>`
* Secrets: {ref}`Reference <secret>` | {ref}`Manage secrets <manage-secrets>` | {ref}`Manage secret backends <manage-secret-backends>`
* Scaling: {ref}`Reference <scaling>` | {ref}`Scale an application <scale-an-application>`
* Status: {ref}`Reference <status>`
* Removing things: {ref}`removing-things`
* Upgrading things: {ref}`upgrading-things`

**Core machinery:**
* Client — Juju CLI: {ref}`Reference <juju-cli>` | {ref}`Manage Juju <manage-juju>`
* Controller: {ref}`Reference <controller>` | {ref}`Manage controllers <manage-controllers>`
* Database: {ref}`Reference <database>` | {ref}`Manage the databases <manage-the-databases>` | {ref}`Juju DB REPL <juju-db-repl>`
* Agents: {ref}`Reference <agent>`
* Pebble: {ref}`Reference <pebble>`
* Hooks and hook commands: {ref}`Hook reference <hook>` | {ref}`Hook command reference <hook-command>`
* Scripts: {ref}`Reference <script>`
* Authentication and authorisation: {ref}`Users <user>` | {ref}`Manage users <manage-users>` | {ref}`Manage user access <manage-user-access>` | {ref}`SSH keys <ssh-key>` | {ref}`Manage SSH keys <manage-ssh-keys>`
* Logs: {ref}`Reference <log>`
* Telemetry: {ref}`Reference <telemetry>`
* Constraints: {ref}`Reference <constraint>`
* Placement directives: {ref}`Reference <placement-directive>`
* Juju as a distributed system: {ref}`juju-architecture`

**Clouds:**
* Basics: {ref}`Cloud reference <cloud>`
* Working with clouds: {ref}`Manage clouds <manage-clouds>`
* Credentials: {ref}`Reference <credential>` | {ref}`Manage credentials <manage-credentials>`
* Metadata: {ref}`Reference <metadata>` | {ref}`Manage metadata <manage-metadata>`

**Resources and interfaces:**
* Compute: {ref}`Reference <resource-compute>`, {ref}`Machine reference <machine>` | {ref}`Manage machines <manage-machines>`
* Networking — spaces: {ref}`Reference <space>` | {ref}`Manage spaces <manage-spaces>`
* Networking — subnets: {ref}`Reference <subnet>` | {ref}`Manage subnets <manage-subnets>`
* Storage: {ref}`Reference <storage>` | {ref}`Manage storage <manage-storage>` | {ref}`Manage storage pools <manage-storage-pools>`
* Zones: {ref}`Reference <zone>`

**Quality:**
* Security: {ref}`juju-security`
* Performance: {ref}`performance-with-juju`
* Observability: {ref}`Collect metrics about a controller <collect-metrics-about-a-controller>` | {ref}`Manage logs <manage-logs>`

**Enterprise features:**
* Juju Dashboard: {ref}`Reference <juju-dashboard>` | {ref}`Manage the Juju Dashboard <manage-the-juju-dashboard>`
* High availability: {ref}`Reference <high-availability>` | {ref}`Make a controller highly available <make-a-controller-highly-available>` | {ref}`Make an application highly available <make-an-application-highly-available>`
* IaC client — Terraform Provider for Juju: [Documentation](https://documentation.ubuntu.com/terraform-provider-juju/latest/)
* Global view, external identity provider, and ReBAC authorization — JAAS: [Documentation](https://documentation.ubuntu.com/jaas/latest/)

**Deployment lifecycle:**
* Set up: {ref}`set-up-your-deployment` | {ref}`set-things-up` | {ref}`set-up-your-deployment-offline <take-your-deployment-offline>` | {ref}`Bootstrap a controller <bootstrap-a-controller>` | {ref}`add-a-cloud` | {ref}`add-a-credential`
* Harden: {ref}`Harden your deployment <harden-your-deployment>`
* Upgrade: {ref}`Upgrade your deployment <upgrade-your-deployment>` | {ref}`Patch version <upgrade-your-juju-components-patch-version>` | {ref}`Minor or major version <upgrade-your-juju-components-minor-or-major-version>` | {ref}`From 3.6 to 4.0 <upgrade-your-juju-deployment-from-36-to-40>`
* Troubleshoot: {ref}`Troubleshoot your deployment <troubleshoot-your-deployment>`
* Tear down: {ref}`Tear things down <tear-things-down>`

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

* ### Releases

* [Roadmap & Releases](releasenotes/index.md)

### Governance and policies

* [Code of Conduct](https://ubuntu.com/community/code-of-conduct)

### Commercial support

Thinking about using Juju for your next project? [Get in touch](https://canonical.com/contact-us)!
