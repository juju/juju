---
myst:
  html_meta:
    description: "Juju architecture: the problem Juju solves, the core insight, the basic topology, and what happens under the hood."
---

(juju-architecture)=
# Juju architecture

:::{div} blurb
Deploying one application on one cloud is routine. Deploying ten across three clouds, and keeping them correct for years, is a lot of lifting. Juju helps you do the heavy lifting.
:::

## The problem

```{ggarch}
:file: ../juju.ggarch
:view: The problem
:caption: An SRE operates applications that run on cloud nodes. Every cloud, every application, and every link between applications adds its own work.
:alt: An SRE with an arrow labelled "operates" to a stack of applications. Each application carries a cloud badge, because it runs on a node from a cloud.
```

Running applications on clouds means provisioning infrastructure, configuring software, and connecting services. Good tools exist for each task, but each stops at the edge of its own layer or platform. Shared modules (reusable descriptions of infrastructure) provision machines, storage, and networks. Packaged definitions (reusable descriptions of an application and its resources) install an application. Operators (programs that automate an application's care after its first install) keep it running. That leaves you to bridge the gaps between them:

- **Clouds differ.** Machines, storage, and networks each have their own API. In a private cloud, you also build and run the cloud itself, along with the databases and monitoring around it.
- **Applications differ.** Installing, configuring, scaling, upgrading, and removing an application each work in their own way, so you re-encode that knowledge for every application.
- **Links differ.** Connecting two applications, such as a web service and its database, requires agreeing on endpoints, exchanging credentials, and handling changes on either side.

Because nothing describes the whole deployment at the level of its applications and how they connect, you become the integration engine. When a node fails or configuration drifts, you spot the gap and fix it, and the work grows with every cloud, every application, and every link you add.

## The insight

```{ggarch}
:file: ../juju.ggarch
:view: The insight
:caption: Juju sits between the user and the applications. The user declares what they want, and Juju operates the applications on nodes from clouds.
:alt: A horizontal chain from a user to Juju, drawn with an orange border and the Juju mark, and on to a stack of applications with a cloud badge. Arrows are labelled "declares intent to" and "operates".
```

Juju is an orchestrator (a program that coordinates many machines and services on your behalf): it sits between you and your infrastructure and takes on much of that work. You declare what you want at the level of the application: `juju deploy foo`, `juju config foo`, `juju integrate foo bar`, and so on.

- **Juju:** the machinery that works with any major type of cloud (public clouds, private clouds, and Kubernetes clusters), so the differences between clouds become Juju's concern. It runs charms, described next, to make your declarations happen there.
- **Charms:** software operators (code that encodes how to install, configure, scale, upgrade, and remove one specific application), so the differences between applications are written once, in the charm. Here foo and bar are charms. Charms also declare what they provide and require, and `juju integrate` connects them, so the endpoints, credentials, and change handling of a link are agreed between the charms.

Because charms connect to one another, a deployment can grow into a full stack of charms, from private cloud infrastructure (charmed OpenStack or charmed Kubernetes) to databases and observability tools. A declaration only helps if something keeps it and acts on it through every change and every failure. Juju stores the declaration as a goal state (the stored description of what you want) and works continuously to make reality match it.

## The basic topology

```{ggarch}
:file: ../juju.ggarch
:view: The topology
:caption: The user tells the client (anything that can talk to a controller, such as the `juju` CLI) what they want, and the client sends it to the controller (Juju's control plane), which stores it as the goal state (the stored description of what the user wants). The controller keeps models (isolated workspaces on one cloud that hold applications). For each model, the controller's workers (background programs, one per concern) get nodes from the cloud and charms from Charmhub (the charm store). An application (a charm deployed under a name) runs as one or more units. A unit is a node (a machine, or a pod on Kubernetes) with a unit agent (the Juju process that looks after the unit), the charm, and the workload (the software the charm runs). The unit agent asks the controller to notify it of changes that concern its unit. When one arrives, the unit agent fetches the current state and notifies the charm (this notification is a hook), which reacts, and then reports back. While it reacts, the charm can call hook commands (tools such as config-get, which reads configuration, and status-set, which reports status), and the unit agent serves them over a Unix domain socket.
:alt: A chain from a user through a client to a controller, with a stack of clouds above the controller and Charmhub below it. On the right, a model with a dotted border and a cloud badge contains an application, also with a dotted border, which contains a unit. The unit holds a unit agent, a charm, and a workload. Arrows are labelled "declares goal state to", "persists goal state in", "gets nodes from", "gets charms from", and "reconciles state with". Inside the unit, the unit agent runs the charm and the charm operates the workload.
```

To keep your deployment in sync with what you declared, Juju works in three steps, from left to right in the figure:

1. **Declaring the goal state.** You declare what you want through a client: the `juju` CLI, the Terraform provider for Juju, or another program that speaks the Juju API. The client sends the declaration to the controller, which stores it as the goal state, the single source of truth.
2. **Organising the work into models.** The controller organises the goal state into models. Each model lives on one cloud and holds your applications, each application runs as one or more units, and the controller's workers get the nodes from the cloud and the charms from Charmhub.
3. **Reconciling the real state.** Each unit agent learns when something concerning its unit changes, fetches the latest goal state, and lets the charm react through a hook. Detecting a gap and closing it, after every change, is called reconciliation.

This is the typical shape once a controller exists.

```{ibnote}
See more: {ref}`cloud`, {ref}`list-of-supported-clouds`, {ref}`client`, {ref}`controller`, {ref}`charm`, {ref}`agent`, {ref}`unit`
```

## Under the hood

Declaring works the same way on every cloud, but what happens behind the commands depends on whether the cloud is Kubernetes or machines. Each walkthrough starts before a controller exists.

``````{tabs}

`````{tab} Kubernetes

```{ggarch}
:file: ../juju.ggarch
:slides: juju add-k8s | juju bootstrap (Kubernetes) | juju add-model | juju deploy (Kubernetes) | juju config | juju integrate
:caption: The basic story of building a deployment on Kubernetes, one command at a time: connect the cluster, bootstrap a controller, add a model, deploy, configure, and integrate.
:slide-captions: The user starts by connecting a Kubernetes cluster to Juju. The client reads the cluster's endpoint and credential from the kubeconfig file (the file that tells tools how to reach a cluster) and saves both on the user's own machine, because no controller exists yet. | With the cluster connected, the user bootstraps (creates the first controller). No controller exists yet, so the client does the work itself: it asks the cluster to create the controller's StatefulSet (the Kubernetes object that runs it) and waits for the controller's API to come up. The controller then initialises its database: a replicated store (Dqlite) whose copies agree through a consensus protocol (Raft) and that records every change as a permanent transaction. From here on, the controller persists what the user declares. | Now the user adds a model (an isolated workspace where applications will live). The client declares it, and the controller persists it and creates a Kubernetes namespace for it. | Then the user deploys an application. The client declares it, and the controller persists it. The controller's workers (background programs, one per concern) ask the cluster to create the application's pods (the groups of containers that Kubernetes runs) and fetch the charm from Charmhub. In each pod, a unit agent (the containeragent program) starts, runs the charm's install hook, and reports the result. The workload runs in its own container, and the charm manages it through Pebble (a process supervisor in that container). | Next, the user configures the application. The controller persists the new value and notices the change in its database through its change stream (a feed of database changes), and it notifies the watchers concerned: each unit agent registered a watcher when it started (a long-lived request to be told about changes that matter to its unit). The notification carries no data, so the unit agent fetches the current state from the controller's API, runs the config-changed hook (the hook for configuration changes), and reports the result, which the controller persists in turn. The database is strongly consistent (its copies always agree), while the units are eventually consistent by design: notifications reach unit agents asynchronously, so units converge on the new value over time, and an agent that loses its connection catches up when it reconnects. | Finally, the user integrates two applications so that they work together. The controller persists the integration, and the watchers of both applications fire. The units of each application fetch the state and run their relation hooks (the hooks for changes in a link between applications). Every exchange between the applications passes through the controller. From here on, later changes such as scaling, upgrades, and removals follow the same loop.
:alt: Six sequence diagrams, one per command. In each, the user runs a juju command on the client. Add-k8s ends at the client. Bootstrap adds the cluster and the controller. Add-model adds the controller. Deploy adds Charmhub, the unit agent, and the charm. Config adds the unit agent and the charm. Integrate adds the unit agents of two applications.
```

`````

`````{tab} Machines

```{ggarch}
:file: ../juju.ggarch
:slides: juju add-cloud | juju add-credential | juju bootstrap | juju add-model | juju deploy | juju config | juju integrate
:caption: The basic story of building a deployment on machines, one command at a time: connect a cloud, add a credential, bootstrap a controller, add a model, deploy, configure, and integrate.
:slide-captions: The user starts by connecting a cloud to Juju. The client saves the cloud definition on the user's own machine, because no controller exists yet. Clouds that Juju already knows need only a credential, which is the next step. | Next, the user gives Juju a credential (the login that lets Juju use the cloud). The client saves it on the user's own machine too. | With the cloud connected, the user bootstraps (creates the first controller). No controller exists yet, so the client does the work itself: it asks the cloud for a node, installs the controller on it, and the controller initialises its database: a replicated store (Dqlite) whose copies agree through a consensus protocol (Raft) and that records every change as a permanent transaction. From here on, the controller persists what the user declares. | Now the user adds a model (an isolated workspace where applications will live). The client declares it, and the controller persists it. The model draws its nodes from the chosen cloud. | Then the user deploys an application. The client declares it, and the controller persists it. The controller's workers (background programs, one per concern) request a node from the cloud and fetch the charm from Charmhub. Once the node is ready, its unit agent starts (it runs inside the machine agent, the Juju process on each machine, jujuagentd), runs the charm's install hook, and reports the result. The node, the unit agent, the charm, and the workload the charm brings in together make a unit. | Next, the user configures the application. The controller persists the new value and notices the change in its database through its change stream (a feed of database changes), and it notifies the watchers concerned: each unit agent registered a watcher when it started (a long-lived request to be told about changes that matter to its unit). The notification carries no data, so the unit agent fetches the current state from the controller's API, runs the config-changed hook (the hook for configuration changes), and reports the result, which the controller persists in turn. The database is strongly consistent (its copies always agree), while the units are eventually consistent by design: notifications reach unit agents asynchronously, so units converge on the new value over time, and an agent that loses its connection catches up when it reconnects. | Finally, the user integrates two applications so that they work together. The controller persists the integration, and the watchers of both applications fire. The units of each application fetch the state and run their relation hooks (the hooks for changes in a link between applications). Every exchange between the applications passes through the controller. From here on, later changes such as scaling, upgrades, and removals follow the same loop.
:alt: Seven sequence diagrams, one per command. In each, the user runs a juju command on the client. Add-cloud and add-credential end at the client. Bootstrap adds a cloud and the controller. Add-model adds the controller. Deploy adds Charmhub, the unit agent, and the charm. Config adds the unit agent and the charm. Integrate adds the unit agents of two applications.
```

`````

``````

```{ibnote}
See more: {ref}`hook`, {ref}`hook-execution-guarantees`, {ref}`hook-command`, {ref}`relation`, {ref}`jujuc`, {ref}`pebble`, {ref}`jujud`, {ref}`containeragent`, {ref}`bootstrap-a-controller`, {ref}`command-juju-add-k8s`, {ref}`command-juju-add-cloud`, {ref}`command-juju-add-credential`, {ref}`command-juju-bootstrap`, {ref}`command-juju-add-model`, {ref}`command-juju-deploy`, {ref}`command-juju-config`, {ref}`command-juju-integrate`
```
