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

Start here if you're new to Juju.

* Tutorial: {ref}`Get started with Juju <tutorial>`
* Installation: {ref}`Install Juju <install-juju>`

**Models and charms**

Juju models business deployment logic through charms; charms describe how an application is deployed.

* **Models**: {ref}`Overview <model>` | {ref}`Manage models <manage-models>`
* **Charmed applications**: {ref}`Charm reference <charm>` | {ref}`Manage charms <manage-charms>` | {ref}`Application reference <application>` | {ref}`Manage applications <manage-applications>` | {ref}`Bundle reference <bundle>`
* **Application operations**: {ref}`Actions <action>` | {ref}`Manage actions <manage-actions>` | {ref}`Relations <relation>` | {ref}`Manage relations <manage-relations>` | {ref}`Offers <offer>` | {ref}`Manage offers <manage-offers>` | {ref}`Charm resources <charm-resource>` | {ref}`Manage charm resources <manage-charm-resources>` | {ref}`Configurations <application-configuration>` | {ref}`Configure an application <configure-an-application>` | {ref}`Secrets <secret>` | {ref}`Manage secrets <manage-secrets>` | {ref}`Manage secret backends <manage-secret-backends>`
* **Units**: {ref}`Unit reference <unit>` | {ref}`Manage units <manage-units>` | {ref}`Scaling <scaling>` | {ref}`Scale an application <scale-an-application>`

**Juju's core machinery**

The controller, agents, and CLI form the engine that coordinates between the application and cloud layers.

* **Architecture**: {ref}`Juju architecture <juju-architecture>`
* **Client — Juju CLI**: {ref}`Reference <juju-cli>` | {ref}`Manage Juju <manage-juju>`
* **Controller**: {ref}`Reference <controller>` | {ref}`Manage controllers <manage-controllers>` | {ref}`Bootstrap a controller <bootstrap-a-controller>`
* **Database**: {ref}`Reference <database>` | {ref}`Manage the databases <manage-the-databases>` | {ref}`Juju DB REPL <juju-db-repl>`
* **Agents**: {ref}`Reference <agent>`
* **Pebble**: {ref}`Reference <pebble>`
* **Hooks and hook commands**: {ref}`Hook reference <hook>` | {ref}`Hook command reference <hook-command>`
* **Scripts**: {ref}`Reference <script>`

**Enterprise features**

Additional capabilities for production and enterprise deployments, including access control, observability, a web dashboard, high availability, and integrations with JAAS and Terraform.

* **Authentication and authorisation**: {ref}`Users <user>` | {ref}`Manage users <manage-users>` | {ref}`Manage user access <manage-user-access>` | {ref}`SSH keys <ssh-key>` | {ref}`Manage SSH keys <manage-ssh-keys>`
* **High availability**: {ref}`Reference <high-availability>` | {ref}`Make a controller highly available <make-a-controller-highly-available>` | {ref}`Make an application highly available <make-an-application-highly-available>`
* **Observability and monitoring**: {ref}`Collect metrics about a controller <collect-metrics-about-a-controller>` | {ref}`Manage logs <manage-logs>` | {ref}`Logs reference <log>` | {ref}`Telemetry reference <telemetry>`
* **Juju Dashboard**: {ref}`Reference <juju-dashboard>` | {ref}`Manage the Juju Dashboard <manage-the-juju-dashboard>`
* IaC client — Terraform Provider for Juju: [Documentation](https://documentation.ubuntu.com/terraform-provider-juju/latest/)
* Global view, external identity provider, and ReBAC authorisation — JAAS: [Documentation](https://documentation.ubuntu.com/jaas/latest/)

**Clouds**

Juju provisions and manages the cloud resources — machines, networking, storage — that applications run on.

* Basics: {ref}`Cloud reference <cloud>`
* Working with clouds: {ref}`Manage clouds <manage-clouds>`
* Credentials: {ref}`Credential reference <credential>` | {ref}`Manage credentials <manage-credentials>`
* Metadata: {ref}`Simplestreams metadata <metadata>` | {ref}`Manage metadata <manage-metadata>`
* Compute: {ref}`Resource (compute) <resource-compute>` | {ref}`Machine reference <machine>` | {ref}`Manage machines <manage-machines>` | {ref}`Constraints <constraint>` | {ref}`Placement directives <placement-directive>`
* Zones: {ref}`Zone reference <zone>`
* Networking — spaces: {ref}`Space reference <space>` | {ref}`Manage spaces <manage-spaces>`
* Networking — subnets: {ref}`Subnet reference <subnet>` | {ref}`Manage subnets <manage-subnets>`
* Storage: {ref}`Storage reference <storage>` | {ref}`Manage storage <manage-storage>` | {ref}`Manage storage pools <manage-storage-pools>`

**Security and performance**

Guidance on securing and optimising your Juju deployment.

* Security: {ref}`Juju security <juju-security>` | {ref}`Harden your deployment <harden-your-deployment>`
* Performance: {ref}`performance-with-juju`

**Deployment lifecycle**

End-to-end procedures for standing up, maintaining, and tearing down a Juju deployment.

* Set up: {ref}`Set up your deployment <set-up-your-deployment>` | {ref}`Set up for local testing <set-things-up>` | {ref}`Set up offline <take-your-deployment-offline>` | {ref}`Add a cloud <add-a-cloud>` | {ref}`Add a credential <add-a-credential>`
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
