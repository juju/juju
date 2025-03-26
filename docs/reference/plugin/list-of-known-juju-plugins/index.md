(list-of-known-juju-plugins)=
# List of known Juju plugins

```{toctree}
:hidden:
:glob:

*

```


Juju {ref}`plugins <plugin>` allow users to extend the Juju client with their own custom commands. This doc lists official Juju plugins, as well as some prominent third party ones.


## Official plugins

- {ref}`plugin-juju-metadata`

  This plugin provides tools for generating and validating image and tools metadata. It is used to find the correct image and agent binaries when bootstrapping a Juju model.

- `juju-wait-for`

  *NB: this is a plugin in Juju 2.9, but from Juju 3.0 onwards, it is a standard command.*

  This plugin allows you to wait for a specified model, machine, application or unit to reach a certain state (defined by a `jq` query against the output of `juju status --format=yaml`).




## Third party plugins


* [`juju-bundle`](https://github.com/knkski/juju-bundle): Juju plugin for easy bundle interactions

  This plugin allows building a bundle from charms config and metadata, and then deploying them. Options to publish the bundle in the charmstore are also available.

* [`juju-bmc`](https://launchpad.net/juju-bmc): juju plugin that adds a command to access a server out band management

  This plugin takes advantage of the integration between Juju and MaaS to obtain the relevant credentials to display them or to establish a connection (via udp/623 or tcp/22) to the BMC console of a bare metal server. The snap uses the MaaS CLI client to communicate with the MaaS API.

* [`juju-crashdump`](https://github.com/juju/juju-crashdump): Gather logs and other debugging info from a Juju model

  This plugin runs commands via SSH to collect evidence to help troubleshoot Juju units within a model.

* [`juju-kubectl`](https://github.com/canonical/juju-kubectl): Juju plugin for running kubectl

  This plugin tries to run kubectl commands against the K8s API of the current model. If the provider is inferred to run microk8s or CDK, the plugin will try to run specific commands (e.g. microk8s.kubectl). Otherwise, the plugin will try to copy the ~/config file from the kubernetes-master/0 unit.

* [`juju-lint`](https://launchpad.net/juju-lint): Linter for Juju models to compare deployments with configurable policy

  This plugin is intended to be run against a YAML dump of Juju status, a YAML dump of a Juju bundle (juju export-bundle), or a remote cloud or clouds via SSH.

* {ref}`plugin-juju-stash`: stash model names, makes moving between models super simple

  This plugin allows you to jump between models as if you have a stack; pushing and popping between models. This makes it possible to switch back and forth between models without having to remember their names.

* [`juju-verify`](https://github.com/canonical/juju-verify): verify the safety of maintenance operations
  
  This plugin allows a user to check whether it's safe to perform some disruptive maintenance operations on Juju units, like `shutdown` or `reboot` .

<!--
NB: this is superseded by the official juju wait-for plugin/command.

* [`juju-wait`](https://launchpad.net/juju-wait): Juju plugin to wait for environment steady state.

  This plugin is similar to the “watch” command in Linux. It monitors a Juju environment (via “juju status”) towards a certain status of the units.
-->

<!--
NB: should be moved to official plugins section.

 - [`juju-wait-for`](https://github.com/juju/juju/tree/develop/cmd/plugins/juju-wait-for): Juju plugin to wait for environment steady state.

   The new [juju-wait-for plugin](https://discourse.charmhub.io/t/plugin-wait-for/3695) is currently in alpha status. The plugin is more optimized for large deployments, by using the AllWatcher API to listen to new changes. This removes the need to call `juju status`, which is known to take a very long time in large deployments.
-->

<!--
### Deprecated

An old list of plugins can be found at https://github.com/juju/plugins/. Most of them were written in Bash or Python to support missing features in Juju 1.x.

* [juju-act](https://launchpad.net/juju-act): Improve the command line user experience of Juju Action

  This plugin made sense when `juju run-action --wait` was not supported. It combined running an action and showing the output from the queued action id.

  “--wait” is supported since 2017 ([LP#1445066](https://bugs.launchpad.net/juju/+bug/1445066))

* [DHX (hook debugging environment)](https://discourse.charmhub.io/t/dhx-a-customized-hook-debugging-environment-plugin/1114/1)

# Dead link. Not sure if it's deprecated but it doesn't make sense to list it now that we have juju debug-hook

* [juju-introspection-proxy](https://github.com/axw/juju-introspection-proxy): A proxy to Juju internal metrics

  This plugin was a personal effort to support Prometheus metrics to introspect Juju. Nowadays, Juju supports Prometheus metrics so this plugin is not needed anymore.

* juju-apply-sla: Unsupported. Repo not found, but a snap exists.

* [juju-matrix](https://github.com/juju-solutions/matrix): Automatic testing of big software deployments under various failure conditions. The repo has not received updates for a long time.

* juju-experts: Tools for Juju experts (Unsupported, Repo not found)

* juju-helpers: Juju plugins to ease a few pain points. This plugin has the same description as juju-bundle, and is maintained by the same author.

* [juju-remove](https://discourse.charmhub.io/t/new-plugin-juju-remove/2318)
-->
