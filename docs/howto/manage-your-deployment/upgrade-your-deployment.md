(upgrade-your-deployment)=
# How to upgrade your deployment

> See also: {ref}`juju-roadmap-and-releases`

This document shows how to upgrade your deployment -- the general logic and order, whether you upgrade in whole or in part, whether you are on Kubernetes or machines.

This typically involves upgrading Juju itself -- the client, the controller (i.e., all the agents in the controller model + the internal database), and the models (i.e., all the agents in the non-controller models). Additionally, for all the applications on your models, you may want to upgrade their charm.

None of these upgrades are systematically related (e.g., compatibility between Juju component versions is based on overlap in the supported facades, and compatibility between charms and Juju versions is charm-specific, so to know if a particular version combination is possible you'll need to consult the release notes for all these various parts).

> See more: {ref}`upgrading-things`, {ref}`juju-cross-version-compatibility`, {ref}`juju-roadmap-and-releases`, individual charm releases

However, in principle, you should always try to keep all the various pieces up to date, the main caveats being that the Juju components are more tightly coupled to one another than to charms and that, due to the way controller upgrades work, keeping your client, controller, and models aligned is quite different if you're upgrading your Juju patch version vs. minor or major version.

## Upgrade your Juju components' patch version
> e.g., 2.9.49 -> 2.9.51

1. Upgrade the client's patch version to stable.
1. Upgrade the controller's patch version to the stable version.
1. For each model on the controller: Upgrade the model's patch version to the stable version. Optionally, for each application on the model: Upgrade the application's charm.


````{dropdown} Example workflow


```text
snap refresh juju --channel 3.3/stable
juju switch <target controller>
juju upgrade-controller
juju upgrade-model -m <target model>
juju refresh <charm>
```

````


> See more:
>
> - {ref}`upgrade-juju`
> - {ref}`upgrade-a-controller`
> - {ref}`upgrade-a-model`
> - {ref}`upgrade-an-application`


## Upgrade your Juju components' minor or major version
> i.e., 2.9 -> 3.0

See [Juju 3 | Upgrade your Juju components' minor or major version](https://documentation.ubuntu.com/juju/latest/howto/manage-your-deployment/upgrade-your-deployment/#upgrade-your-juju-components-minor-or-major-version)