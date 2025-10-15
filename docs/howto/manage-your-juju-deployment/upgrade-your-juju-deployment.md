(upgrade-your-deployment)=
# Upgrade your Juju deployment

```{ibnote}
See also: {ref}`juju-roadmap-and-releases`

```
This document shows how to upgrade your deployment -- the general logic and order, whether you upgrade in whole or in part, whether you are on Kubernetes or machines.

This typically involves upgrading Juju itself -- the client, the controller (i.e., all the agents in the controller model), and the models (i.e., all the agents in the non-controller models). Additionally, for all the applications on your models, you may want to upgrade their charm.

None of these upgrades are systematically related.

```{ibnote}
See more: {ref}`upgrading-things`, {ref}`juju-cross-version-compatibility`, {ref}`juju-roadmap-and-releases`, individual charm releases
```

However, in principle, you should always try to keep all the various pieces up to date, the main caveats being that the Juju components are more tightly coupled to one another than to charms and that, due to the way controller upgrades work, keeping your client, controller, and models aligned is quite different if you're upgrading your Juju patch version vs. minor or major version.

(upgrade-your-juju-components-patch-version)=
## Upgrade your Juju components' patch version

```{ibnote}
e.g., `3.4.4` &rarr; `3.4.5`
```

1. Upgrade the client's patch version to stable. For example:

```text
snap refresh juju --channel 3.3/stable
```

```{ibnote}
See more: {ref}`upgrade-juju`
```

2. Upgrade the controller's patch version to the stable version. For example:

```text
juju switch mycontroller
juju upgrade-controller
```

```{ibnote}
See more: {ref}`upgrade-a-controllers-patch-version`
```


3. For each model on the controller: Upgrade the model's patch version to the stable version. Optionally, for each application on the model: Upgrade the application's charm. For example:

```text
juju upgrade-model -m mymodel
juju refresh mycharm
```

```{ibnote}
See more: {ref}`upgrade-a-model`, {ref}`upgrade-an-application`
```

(upgrade-your-juju-components-minor-or-major-version)=
## Upgrade your Juju components' minor or major version

```{ibnote}
e.g., `3.5` &rarr; `3.6` or  `2.9` &rarr; `3.0`
```

```{caution}
For best results, perform a {ref}`patch upgrade <upgrade-your-juju-components-patch-version>` first.
```

1. Upgrade your client to the target minor or major. For example:


```text
snap refresh juju --channel=<target controller version>
```

```{ibnote}
See more: {ref}`upgrade-juju`
```


2. It is not possible to upgrade a controller's minor or major version in place. Use the upgraded client to bootstrap a new controller of the target version, then clone your old controller's users, permissions, configurations, etc., into the new controller.

```{ibnote}
See more: {ref}`upgrade-a-controllers-minor-or-major-version`
```

If you care about a specific base, use configs (e.g., for the controller machine: `juju bootstrap aws aws-new --bootstrap-base=<base>`; for new machines on migrated models: `juju model-config -m <model name> default-base=<base>`; for new machines on new models: `juju model-defaults default-base=<base>`).

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

```{ibnote}
See more: {ref}`upgrade-a-model`, {ref}`upgrade-an-application`
```

4. Help your users connect to the new controller by resetting their password and sending them the registration link for the new control that they can use to connect to the new controller. For example:

```text
juju change-user-password <user> --reset
# >>> Password for "<user>" has been reset.
# >>> Ask the user to run:
# >>>     juju register
# >>> MEcTA2JvYjAWExQxMC4xMzYuMTM2LjIxNToxNzA3MAQgJCOhZjyTflOmFjl-mTx__qkvr3bAN4HAm7nxWssNDwETBnRlc3QyOQAA
# When they use this registration string, they will be prompted to create a login for the new controller.

```

```{ibnote}
See more: {ref}`manage-a-users-login-details`
```