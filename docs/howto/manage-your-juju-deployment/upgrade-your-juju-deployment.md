(upgrade-your-deployment)=
# Upgrade your Juju deployment

> See also: {ref}`juju-roadmap-and-releases`

This document shows how to upgrade your deployment -- the general logic and order, whether you upgrade in whole or in part, whether you are on Kubernetes or machines.

This typically involves upgrading Juju itself -- the client, the controller (i.e., all the agents in the controller model), and the models (i.e., all the agents in the non-controller models). Additionally, for all the applications on your models, you may want to upgrade their charm.

None of these upgrades are systematically related.

> See more: {ref}`upgrading-things`, {ref}`juju-cross-version-compatibility`, {ref}`juju-roadmap-and-releases`, individual charm releases

However, in principle, you should always try to keep all the various pieces up to date, the main caveats being that the Juju components are more tightly coupled to one another than to charms and that, due to the way controller upgrades work, keeping your client, controller, and models aligned is quite different if you're upgrading your Juju patch version vs. minor or major version.

(upgrade-your-juju-components-patch-version)=
## Upgrade your Juju components' patch version
> e.g., 3.4.4 -> 3.4.5

1. Upgrade the client's patch version to stable. For example:

```text
snap refresh juju --channel 3.3/stable
```

> See more: {ref}`upgrade-juju`

2. Upgrade the controller's patch version to the stable version. For example:

```text
juju switch mycontroller
juju upgrade-controller
```

> See more: {ref}`upgrade-a-controllers-patch-version`


3. For each model on the controller: Upgrade the model's patch version to the stable version. Optionally, for each application on the model: Upgrade the application's charm. For example:

```text
juju upgrade-model -m mymodel
juju refresh mycharm
```

> See more: {ref}`upgrade-a-model`, {ref}`upgrade-an-application`

(upgrade-your-juju-components-minor-or-major-version)=
## Upgrade your Juju components' minor or major version
> e.g., 3.5 -> 3.6 or  2.9 -> 3.0

```{caution}
For best results, perform a patch upgrade first.
```

1. Upgrade your client to the target minor or major. For example:


```text
snap refresh juju --channel=<target controller version>
```
> See more: {ref}`upgrade-juju`


2. It is not possible to upgrade a controller's minor or major version in place. Use the upgraded client to bootstrap a new controller of the target version, then clone your old controller's users, permissions, configurations, etc., into the new controller (for machine controllers, using our backup and restore tooling). For example:

```text
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

```

> See more: {ref}`upgrade-a-controllers-minor-or-major-version`

3. Migrate your old controller's models to the new controller and upgrade them to match the version of the new controller. Optionally, for each application on the model: Upgrade the application's charm. For example:

```text
# Switch to the old controller:
juju switch oldcontroller

# Migrate your models to the new controller:
juju migrate <model> newcontroller

# Switch to the new controller:
juju switch newcontroller

# Upgrade the migrated models to match the new controller's agent version:
juju upgrade-model --agent-version=<new controller's agent version>

# Upgrade the applications:
juju refresh mycharm
```

> See more: {ref}`upgrade-a-model`, {ref}`upgrade-an-application`

4. Help your users connect to the new controller by resetting their password and sending them the registration link for the new control that they can use to connect to the new controller. For example:

```text
juju change-user-password <user> --reset
# >>> Password for "<user>" has been reset.
# >>> Ask the user to run:
# >>>     juju register
# >>> MEcTA2JvYjAWExQxMC4xMzYuMTM2LjIxNToxNzA3MAQgJCOhZjyTflOmFjl-mTx__qkvr3bAN4HAm7nxWssNDwETBnRlc3QyOQAA
# When they use this registration string, they will be prompted to create a login for the new controller.

```

> See more: {ref}`manage-a-users-login-details`
