---
myst:
  html_meta:
    description: "Step-by-step guides for managing Juju deployments, clouds, controllers, models, applications, and more. Key operations and common tasks."
---

(how-to-guides)=
# How-to guides

**Step-by-step guides** covering key operations and common tasks

```{toctree}
:maxdepth: 2
:hidden:

Upgrade your deployment from 3.6 to 4.0 <upgrade-your-juju-deployment-from-36-to-40>
Manage your deployment <manage-your-deployment>
Manage juju <manage-juju>
Add, update, and remove clouds and their regions <manage-clouds>
Manage credentials <manage-credentials>
Manage metadata <manage-metadata>
Bootstrap and operate controllers <manage-controllers>
Inspect, query, and modify the databases <manage-the-databases>
Set up, access, and upgrade the Juju dashboard <manage-the-juju-dashboard>
Manage secret backends <manage-secret-backends>
Manage logs <manage-logs>
Manage SSH keys <manage-ssh-keys>
Administer users and their access <manage-users>
Work with models <manage-models>
Manage charms <manage-charms>
Deploy and operate applications <manage-applications>
Find, specify, and view charm resources <manage-charm-resources>
Manage actions <manage-actions>
Add, inspect, and remove relations <manage-relations>
Share and consume offers <manage-offers>
Inspect, scale, and troubleshoot units <manage-units>
Manage secrets <manage-secrets>
Operate machines <manage-machines>
Manage storage <manage-storage>
Create, inspect, update, and remove storage pools <manage-storage-pools>
Manage spaces <manage-spaces>
List or move subnets <manage-subnets>
Define resource tags in a cloud <define-resource-tags-in-a-cloud>

```

```{tip}
Moving from Juju 3.6? Start with {ref}`Upgrade your deployment from 3.6 to 4.0 <upgrade-your-deployment-from-36-to-40>`.
```

(your-juju-deployment-the-birds-eye-view)=
## Your Juju deployment: the bird's eye view

The high-level logic of a Juju deployment, from day 0 to day 2. Start with {ref}`Manage your deployment <manage-your-deployment>`, or jump to a stage:

- **Set up**: {ref}`Standard <set-up-your-deployment>` • {ref}`Local testing and development <set-things-up>` • {ref}`Offline <take-your-deployment-offline>`
- **Maintain**: {ref}`Harden <harden-your-deployment>` • {ref}`Troubleshoot <troubleshoot-your-deployment>` • {ref}`Upgrade <upgrade-your-deployment>`
- **Tear down**: {ref}`Local testing and development <tear-things-down>`

## Set up Juju

Install the `juju` client, add a cloud to the client, bootstrap a Juju controller, connect further clouds to the client or an existing controller, set up the Juju dashboard, configure secret backends, configure logs.

- {ref}`Manage the juju CLI <manage-juju>`
- {ref}`Add, update, and remove clouds and their regions <manage-clouds>`
- {ref}`Manage credentials <manage-credentials>`
- {ref}`Manage metadata <manage-metadata>`
- {ref}`Bootstrap and operate controllers <manage-controllers>`
- {ref}`Inspect, query, and modify the databases <manage-the-databases>`
- {ref}`Set up, access, and upgrade the Juju dashboard <manage-the-juju-dashboard>`
- {ref}`Manage secret backends <manage-secret-backends>`
- {ref}`Manage logs <manage-logs>`

## Handle authentication and authorization

Set up SSH keys. Add users and control their access to controllers, clouds, models, or application offers.

- {ref}`Manage SSH keys <manage-ssh-keys>`
- {ref}`Administer users and their access <manage-users>`

## Deploy infrastructure and applications

Deploy, configure, integrate, scale, etc., charmed applications. This will automatically provision infrastructure, but you can customise it before, during, or after deploy too.

- {ref}`Manage charms or bundles <manage-charms>`
- {ref}`Find, specify, and view charm resources <manage-charm-resources>`
- {ref}`Deploy and operate applications <manage-applications>`
- {ref}`Manage actions <manage-actions>`
- {ref}`Add, inspect, and remove relations <manage-relations>`
- {ref}`Share and consume offers <manage-offers>`
- {ref}`Inspect, scale, and troubleshoot units <manage-units>`
- {ref}`Manage secrets <manage-secrets>`
- {ref}`Operate machines <manage-machines>`
- {ref}`Manage storage <manage-storage>`
- {ref}`Create, inspect, update, and remove storage pools <manage-storage-pools>`
- {ref}`Manage spaces <manage-spaces>`
- {ref}`List or move subnets <manage-subnets>`
