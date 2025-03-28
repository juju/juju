(upgrade-your-deployment)=
# How to upgrade your deployment

> See also: {ref}`juju-roadmap-and-releases`

This document shows how to upgrade your deployment -- the general logic and order, whether you upgrade in whole or in part, whether you are on Kubernetes or machines.

This typically involves upgrading Juju itself -- the client, the controller (i.e., all the agents in the controller model + the internal database), and the models (i.e., all the agents in the non-controller models). Additionally, for all the applications on your models, you may want to upgrade their charm.

None of these upgrades are systematically related (e.g., compatibility between Juju component versions is based on overlap in the supported facades, and compatibility between charms and Juju versions is charm-specific, so to know if a particular version combination is possible you'll need to consult the release notes for all these various parts).

> See more: {ref}`upgrading-things`, {ref}`juju-cross-version-compatibility`, {ref}`juju-roadmap-and-releases`, individual charm releases

However, in principle, you should always try to keep all the various pieces up to date, the main caveats being that the Juju components are more tightly coupled to one another than to charms and that, due to the way controller upgrades work, keeping your client, controller, and models aligned is quite different if you're upgrading your Juju patch version vs. minor or major version.

## Upgrade your Juju components' patch version
> e.g., 3.4.4 -> 3.4.5

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
> e.g., 3.5 -> 3.6 or  2.9 -> 3.0

```{caution}
For best results, perform a patch upgrade first.
```

1. Upgrade your client to the target minor or major.
1. It is not possible to upgrade a controller's minor or major version in place. Use the upgraded client to bootstrap a new controller of the target version.
1. Clone your old controller's users, permissions, configurations, etc., into the new controller (only supported for machine controllers). 
1. Migrate your old controller's models to the new controller and upgrade them to match the version of the new controller. Optionally, for each application on the model: Upgrade the application's charm.
1. Help your users connect to the new controller.

````{dropdown} Example workflow

```text
# Upgrade the client to the version you want for your controller:
snap refresh juju --channel=<target controller version>

# Use the new client to bootstrap a controller:
juju bootstrap <cloud> newcontroller

# Create a backup of the old controller's controller model 
# and make note of the path to the backup file:
juju create-backup -m oldcontroller:controller
# Sample output:
# >>> ...
# >>>  Downloaded to juju-backup-20221109-090646.tar.gz

# Download the stand-alone juju-restore tool:
wget https://github.com/juju/juju-restore/releases/latest/download/juju-restore
chmod +x juju-restore

# Switch to the new controller's controller model:
juju switch newcontroller:controller

# Copy the juju-restore tool to the primary controller machine:  
juju scp juju-restore 0:

# Copy the backup file to the primary controller machine:
juju scp <path to backup> 0:

# SSH into the primary controller machine:
juju ssh 0

# Start the restore with the '--copy-controller' flag:
./juju-restore --copy-controller <path to backup>
# Congratulations, your <old version> controller config has been cloned into your <new version> controller.

# Switch to the old controller:
juju switch oldcontroller

# Migrate your models to the new controller:
juju migrate <model> newcontroller


# Switch to the new controller:
juju switch newcontroller

# Upgrade the migrated models to match the new controller's agent version:
juju upgrade-model --agent-version=<new controller's agent version>


# Reset the users' passwords to get a new registration string
# that they can use to connect to the new controller:
juju change-user-password <user> --reset
# >>> Password for "<user>" has been reset.
# >>> Ask the user to run:
# >>>     juju register 
# >>> MEcTA2JvYjAWExQxMC4xMzYuMTM2LjIxNToxNzA3MAQgJCOhZjyTflOmFjl-mTx__qkvr3bAN4HAm7nxWssNDwETBnRlc3QyOQAA
# When they use this registration string, they will be prompted to create a login for the new controller.

```

````

> See more:
> 
> - {ref}`upgrade-juju`
> - {ref}`upgrade-a-controller`
> - {ref}`upgrade-a-model`
> - {ref}`upgrade-an-application`
