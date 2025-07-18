(tutorial)=
# Get started with Juju

In this tutorial your goal is to set up a chat service on a cloud.

In traditional setups this can be quite a challenge; however, with Juju, its supported clouds, and charms, it's plug-and-play from day 0 to Day n.

Are you ready to take control of cloud? Let's get started!

What you'll need:
- A workstation that has sufficient resources to launch a virtual machine with 4 CPUs, 8 GB RAM, and 50 GB disk space.

What you'll do:
- Install Juju, prepare and connect a cloud, then deploy, configure,integrate, scale, and observe a chat service based on Mattermost and PostgreSQL using Juju and charms.

```{tip}
If at any point you get stuck: [get in touch](https://matrix.to/#/!bJwYQnTQivEaYPFwKL:ubuntu.com?via=ubuntu.com).
```

## Set up an isolated test environment

When you're trying things out it's nice to work in an isolated test environment. Let's spin up an Ubuntu virtual machine (VM) with Multipass!

First, [install Multipass](https://documentation.ubuntu.com/multipass/en/latest/how-to-guides/install-multipass/).

Now, launch an Ubuntu VM:

```text
multipass launch --cpus 4 --memory 8G --disk 50G --name my-juju-vm
```

Finally, open a shell in the VM:

```text
multipass shell my-juju-vm
```

Anything you type after the VM shell prompt will run on the VM.

```{dropdown} Tips for usage

At any point:
- To exit the shell, press {kbd}`mod` + {kbd}`C` (e.g., {kbd}`Ctrl`+{kbd}`C`) or type `exit`.
- To stop the VM after exiting the VM shell, run `multipass stop charm-dev-vm`.
- To restart the VM and re-open a shell into it, type `multipass shell charm-dev-vm`.

```
```{dropdown} Tips for troubleshooting
If the VM launch fails, run `multipass delete --purge my-juju-vm` to clean up, then try the launch line again.

```

## Prepare a cloud

To Juju a cloud is anything that has an API where you can request compute, storage, and networking. This includes traditional machine clouds (Amazon AWS, Google GCE, Microsoft Azure, but also Equinix Metal, MAAS, OpenStack, Oracle OCI, and LXD) as well as Kubernetes clusters (Amazon EKS, Google GKE, Microsoft AKS but also Canonical Kubernetes or MicroK8s). Among these is MicroK8s, a low-ops, minimal production Kubernetes that you can also use to get a small, single-node localhost Kubernetes cluster ([see more](https://documentation.ubuntu.com/juju/3.6/reference/cloud/list-of-supported-clouds/the-microk8s-cloud-and-juju/)). Let's set it up on your VM:

```text
# Install MicroK8s package:
sudo snap install microk8s --channel 1.28-strict

# Add your user to the `microk8s` group for unprivileged access:
sudo adduser $USER snap_microk8s

# Give your user permissions to read the ~/.kube directory:
sudo chown -f -R $USER ~/.kube

# Wait for MicroK8s to finish initialising:
sudo microk8s status --wait-ready

# Enable the 'storage' and 'dns' addons:
# (required for the Juju controller)
sudo microk8s enable hostpath-storage dns

# Alias kubectl so it interacts with MicroK8s by default:
sudo snap alias microk8s.kubectl kubectl

# Ensure your new group membership is apparent in the current terminal:
# (Not required once you have logged out and back in again)
newgrp snap_microk8s

# Since the juju package is strictly confined, you also need to manually create a path:
mkdir -p ~/.local/share

```

Congratulations, your cloud is ready!

## Install Juju

Juju is a distributed system that consists of one or more clients, one or more controllers, and one or more agents, where the client is typically on your workstation, the controller is something you bootstrap into a cloud using the client, and the agents are something you deploy implicitly every time you provision infrastructure or deploya applications with Juju. Let's install the `juju` CLI client and use it with our MicroK8s cloud to bootstrap a controller!

In your VM, install the `juju` CLI client:

```text
sudo snap install juju
```

In your VM, ensure that the client knows about your cloud (for your localhost MicroK8s from a strictly installed snap this should happen automatically), then use it to bootstrap a controller into the cloud:

```text
juju clouds --client
juju credentials --client
juju bootstrap microk8s my-first-controller
```

This will create a Juju controller (i.e., control plane; here: 'my-first-controller') implicitly connected to the MicroK8s cloud and holding a single Juju model (i.e., workspace; here: the 'controller' model) implicitly associated with the MicroK8s cloud to which a single Juju charm (i.e., software operator; here: the `juju-controller` charm) has been deployed as a single Juju application (i.e., whatever the charm deploys; here: the 'controller' application) with a single Juju unit (i.e., application replicae, here: the 'controller/0' unit). On Kubernetes, so, in our MicroK8s cloud this unit corresponds to a pod which, in this case, holds 3 containers which include, respectively, the following:

1. A Juju unit agent and `juju-controller` charm code.
1. The process supervisor Pebble and the controller agent running, among other things, the Juju API server.
1. A copy of `juju-db`, Juju's internal database, which stores all of the information that the controller knows about, including clouds, models, applications, as well as application units and -- as we shall see shortly -- application configurations, application integrations, etc.

Take some time to explore:

- Switch to the controller: `juju switch my-first-controller`.
- On the current controller, switch to the 'controller' model: `juju switch controller`
- On the current model, view the status of all of its contents: `juju status`.



<!--
```{dropdown} What exactly does this do?
When you use the `juju` CLI client to bootstrap a Juju controller into a Kubernetes cloud this, all of the following happens:

1. On the cluster / cloud side:
    1. The client will create a namespace and, in this namespace, resources for the controller.
    1. The cluster will provision pods by pulling the specified container images etc.
1. On the Juju side: This creates a 'controller' model (Kubernetes 'namespace'); verify: `juju models`. The model contains a 'controller' application (Kubernetes 'service') consisting of one 'controller/0' unit (a running instance of the application which, on Kubernets, is deployed to its own separate pod); verify: `juju status`. In the cloud this unit (pod) consists of three containers, one of which holds a Juju unit agent and the Juju controller charm code, one of which holds Pebble and the controller agent (that contains Juju's API server), and one of which holds Juju's internal database. You'll encounter the same basic setup when you deploy charmed applications too, as in its essence the 'controller' application is just a special kind of charmed application.
```
-->

Congratulations, your Juju client and controller are ready!

```{tip}
Split your terminal window into three. In all, access your Multipass VM shell (`multipass shell my-juju-vm`) and then:

**Shell 1:** Keep using it as you've already been doing so far, namely to type the commands in this tutorial.

**Shell 2:** Run `juju status --relations --color --watch 1s` to watch your deployment status evolve. (Things are all right if your `App Status` and your `Unit - Workload` reach `active` and your `Unit - Agent` reaches `idle`.)

**Shell 3:** Run `juju debug-log` to watch all the details behind your deployment status. (Especially useful when things don't evolve as expected. In that case, please get in touch.)
```

## Connect a cloud

Post-bootstrap, any operation you trigger through your client goes through the controller. That's why connecting a cloud to Juju fundamentally means connecting it to the controller. You can connect a cloud to a controller explicitly, if the controller already exists, or implicitly by bootstrapping a controller into it, like we did just now. Let's check that our implicit connection has indeed worked:

```text
juju clouds --controller
juju credentials --controller
```

Your controller can now use this cloud to get compute, storage, and networking resources (for Kubernetes clouds, this fundamentally means pods and containers), but also Charmhub to get charms. Time to deploy some charmed applications!

## Use charmed applications

### Deploy, configure, integrate

When we bootstrapped a controller earlier, we already saw a little bit how Juju works: we have workspaces called 'models' on which we deploy software operators known as 'charms' to get running workloads called 'applications' with replicas called 'units.

Because the Juju controller is fundamentally just a special kind of Juju application, the logic for deploying other charmed applications is pretty much the same: we create a model to which we deploying charms which will results in applications running on the resources that we get from our cloud.

First, let's create a model. As it will hold all our chat applications, let's call it 'chat':


```text
ubuntu@my-juju-vm:~$ juju add-model chat
Added 'chat' model on microk8s/localhost with credential 'microk8s' for user 'admin'
```

Now, let's deploy, configure, and integrate the charmed applications that will make up our chat service:

<!--
A popular open source application for that is Mattermost ([see more](https://mattermost.com/)).  A search on Charmhub reveals there is already a suitable charm for this application, the `mattermost-k8s` charm ([see more](https://charmhub.io/mattermost-k8s)). Moreover, a quick look at this charm's docs shows that, to satisfy this application's dependency on PostgreSQL, this charm also supports easy integration with PostgreSQL through the `postgresql-k8s` charm ([see more](https://charmhub.io/postgresql-k8s)), though traffic from PostgreSQL must be TLS-encrypted, something that can be satisfied, for our tutorial purposes, through further integration with the application deployed from the `self-signed-certificates` charm ([see more](https://charmhub.io/self-signed-certificates)). -->

```text
# Deploy mattermost-k8s as mattermost:
ubuntu@tutorial-vm:~$ juju deploy mattermost-k8s
Located charm "mattermost-k8s" in charm-hub, revision 27
Deploying "mattermost-k8s" from charm-hub charm "mattermost-k8s", revision 27 in channel stable on ubuntu@20.04/stable

# Deploy and configure postgresql-k8s as postgresql:
ubuntu@tutorial-vm:~$ juju deploy postgresql-k8s --channel 14/stable --trust --config profile=testing
Located charm "postgresql-k8s" in charm-hub, revision 193
Deploying "postgresql-k8s" from charm-hub charm "postgresql-k8s", revision 193 in channel 14/stable on ubuntu@22.04/stable

# Mattermost wants PostgreSQL status to be TLS-encrypted; deploy self-signed-certificates:
ubuntu@my-juju-vm:~$ juju deploy self-signed-certificates
Located charm "self-signed-certificates" in charm-hub, revision 72
Deploying "self-signed-certificates" from charm-hub charm "self-signed-certificates", revision 72 in channel stable on ubuntu@22.04/stable

# Integrate self-signed-certificates with postgresql:
ubuntu@tutorial-vm:~$ juju integrate self-signed-certificates postgresql-k8s

# Integrate postgresql with mattermost:
ubuntu@tutorial-vm:~$ juju integrate postgresql-k8s:db mattermost-k8s
```

While executing any of these commands returns automatically so you can execute the next, standing things up in the cloud takes a little bit of time; watch your progress in your `juju status` terminal window. Things are all set when `juju status` looks as below:

```
# Check your model's status:
ubuntu@my-juju-vm:~$ juju status --relations
Model  Controller  Cloud/Region        Version  SLA          Timestamp
chat   31microk8s  microk8s/localhost  3.1.8    unsupported  13:48:04+02:00

App                       Version                         Status  Scale  Charm                     Channel    Rev  Address         Exposed  Message
mattermost-k8s            .../mattermost:v8.1.3-20.04...  active      1  mattermost-k8s            stable      27  10.152.183.131  no
postgresql-k8s            14.10                           active      1  postgresql-k8s            14/stable  193  10.152.183.56   no
self-signed-certificates                                  active      1  self-signed-certificates  stable      72  10.152.183.119  no

Unit                         Workload  Agent  Address      Ports     Message
mattermost-k8s/0*            active    idle   10.1.32.155  8065/TCP
postgresql-k8s/0*            active    idle   10.1.32.152
self-signed-certificates/0*  active    idle   10.1.32.154

Integration provider                   Requirer                       Interface         Type     Message
postgresql-k8s:database-peers          postgresql-k8s:database-peers  postgresql_peers  peer
postgresql-k8s:db                      mattermost-k8s:db              pgsql             regular
postgresql-k8s:restart                 postgresql-k8s:restart         rolling_op        peer
postgresql-k8s:upgrade                 postgresql-k8s:upgrade         upgrade           peer
self-signed-certificates:certificates  postgresql-k8s:certificates    tls-certificates  regular
```

Now, from the output of `juju status`> `Unit` > `mattermost-k8s/0`, retrieve the IP address and the port and feed them to `curl` on the template below:

```text
curl <IP address>:<port number>/api/v4/system/ping
```

Sample session:

```text
ubuntu@my-juju-vm:~$ curl 10.1.32.155:8065/api/v4/system/ping
{"ActiveSearchBackend":"database","AndroidLatestVersion":"","AndroidMinVersion":"","IosLatestVersion":"","IosMinVersion":"","status":"OK"}
```

Congratulations, your chat service is up and running!

![Juju tutorial - Your deployment](tutorial.png)


*Your computer with your Multipass VM, your MicroK8s cloud, and a  live Juju controller (the 'charm' in the Controller Unit is the `juju-controller` charm) + a sample deployed application on it (the 'charm' in the Regular Unit stands for any charm that you might deploy). If in the Regular Application you replace the charm with `mattermost-k8s` and image a few more Regular Applications where you replace the charm with `postgresql-k8s` and, respectively, `self-signed-certificates`, and if you trace the path from `postgresql-k8s`'s Unit Agent through the Controller Agent to `self-signed-certificates`'s and, respectively, `mattermost-k8s` Unit Agent, you get a full representation of your deployment. (Note: After integration, the workloads may also know how to contact each other directly; still, all communication between their respective charms goes through the Juju controller and the result of that communication is stored in the database in the form of maps known as 'relation data bags'.)*


### Scale

Our Mattermost chat service is functional, but a database failure can be very costly. Let's scale its PostgreSQL database!

With Juju and charms, scaling an applications is simply a matter of telling Juju to increase the number of replicas, or units. Let's increase our PostgreSQL unit count to 3:

```text
ubuntu@my-juju-vm:~$ juju scale-application postgresql-k8s 3
postgresql-k8s scaled to 3 units

# Wait a minute for things to settle down, then check the result:
ubuntu@my-juju-vm:~$ juju status
Model  Controller  Cloud/Region        Version  SLA          Timestamp
chat   31microk8s  microk8s/localhost  3.1.8    unsupported  15:41:34+02:00

App                       Version                         Status  Scale  Charm                     Channel    Rev  Address         Exposed  Message
mattermost-k8s            .../mattermost:v8.1.3-20.04...  active      1  mattermost-k8s            stable      27  10.152.183.131  no
postgresql-k8s            14.10                           active      3  postgresql-k8s            14/stable  193  10.152.183.56   no
self-signed-certificates                                  active      1  self-signed-certificates  stable      72  10.152.183.119  no

Unit                         Workload  Agent      Address      Ports     Message
mattermost-k8s/0*            active    idle       10.1.32.155  8065/TCP
postgresql-k8s/0*            active    idle       10.1.32.152            Primary
postgresql-k8s/1             active    idle       10.1.32.158
postgresql-k8s/2             active    executing  10.1.32.159
self-signed-certificates/0*  active    idle       10.1.32.154

```


```{caution} In a production scenario:
You'll want to make sure that they are also properly distributed over multiple nodes. Our localhost MicroK8s doesn't allow us to do this (because we only have 1 node) but, if you clusterise MicroK8s, you can use it to explore this too!
```

### Observe

Our deployment hasn't really been up very long, but we'd still like to take a closer look at our controller, to see what's happening. Time for some observability!

In the Juju ecosystem the way to do this is via the Canonical Observability Stack, a collection of charms nicely configured and integrated that you can deploy in on line using the `cos-lite` bundle.

Let's create an 'observability' model:


```text
# Add a new model to hold your observability applications:
ubuntu@my-juju-vm:~$ juju add-model observability
Added 'observability' model on microk8s/localhost with credential 'microk8s' for user 'admin'

# Inspect the results:
ubuntu@my-juju-vm:~$ juju models
Controller: 34microk8s

Model           Cloud/Region        Type        Status     Units  Access  Last connection
chat            microk8s/localhost  kubernetes  available  5       admin  9 minutes ago
controller      microk8s/localhost  kubernetes  available  1       admin  just now
observability*  microk8s/localhost  kubernetes  available  6       admin  1 minute ago
```

Let's deploy to it the `cos-lite` bundle:


```text
# Deploy to it the cos-lite bundle:
ubuntu@my-juju-vm:~$ juju deploy cos-lite --trust
# Partial output:
Located bundle "cos-lite" in charm-hub, revision 11
Located charm "alertmanager-k8s" in charm-hub, channel latest/stable
Located charm "catalogue-k8s" in charm-hub, channel latest/stable
Located charm "grafana-k8s" in charm-hub, channel latest/stable
Located charm "loki-k8s" in charm-hub, channel latest/stable
Located charm "prometheus-k8s" in charm-hub, channel latest/stable
Located charm "traefik-k8s" in charm-hub, channel latest/stable
...
Deploy of bundle completed.
```

We'll want to use the Prometheus from this bundle to monitor our controller application, and that means we'll have to ensure these two applications can connect across model boundaries. In Juju that means that in the 'observability' model must offer Prometheus up for cross-model relations, then, on the 'controller' model we must integrate with it:

```
# On the 'observability' model, offer prometheus' metrics-endpoint endpoint
# for cross-model relations:
ubuntu@my-juju-vm:~$ juju offer prometheus:metrics-endpoint
Application "prometheus" endpoints [metrics-endpoint] available at "admin/observability.prometheus"

# Switch to the controller model
ubuntu@my-juju-vm:~$ juju switch controller
34microk8s:admin/observability -> 34microk8s:admin/controller

# Integrate the controller application with the prometheus offer:
ubuntu@my-juju-vm:~$ juju integrate controller admin/observability.prometheus

# Examine the result:
ubuntu@my-juju-vm:~$ juju status --relations
Model       Controller  Cloud/Region        Version  SLA          Timestamp
controller  34microk8s  microk8s/localhost  3.4.2    unsupported  17:08:10+02:00

SAAS        Status  Store       URL
prometheus  active  34microk8s  admin/observability.prometheus

App         Version  Status  Scale  Charm            Channel     Rev  Address  Exposed  Message
controller           active      1  juju-controller  3.4/stable   79           no

Unit           Workload  Agent  Address      Ports      Message
controller/0*  active    idle   10.1.32.161  37017/TCP

Integration provider         Requirer                     Interface          Type     Message
controller:metrics-endpoint  prometheus:metrics-endpoint  prometheus_scrape  regular
```

We'll want to also be able to look at the results using our Grafana dashboard. Let's use one of our Charmed Grafana's Juju actions to generate a password and an access URL:

```text
# Switch back to the observability model:
ubuntu@my-juju-vm:~$ juju switch observability
34microk8s:admin/controller -> 34microk8s:admin/observability

# Get an admin password for grafana:
ubuntu@my-juju-vm:~$ juju run grafana/0 get-admin-password
# Example output:
Running operation 1 with 1 task
  - task 2 on unit-grafana-0

Waiting for task 2...
admin-password: 0OpLUlxJXQaU
url: http://10.238.98.110/observability-grafana
```

Now, on your local machine, open a browser window and copy-paste the Grafana URL. In the username field, enter 'admin'. In the password field, enter the `admin-password`. If everything has gone well, you should now be logged in.

On the new screen, in the top-right, click on the Menu icon, then **Dashboards**. Then, on the new screen, in the top-left, click on **New**, **Upload dashboard JSON file**, and upload the JSON Grafana-dashboard-definition file below, then, in the IL3-2 field, from the drop-down, select the suggested `juju_observability...` option.

[Juju Controllers-1713888589960.json|attachment](https://discourse.charmhub.io/uploads/short-url/yOxvgum6eo3NmMxPaTRKLOLmbo0.json) (200.9 KB)


On the new screen, at the very top, expand the Juju Metrics section and inspect the results. How many connections to the API server does your controller show?

![Juju tutorial - Observe your controller](tutorial-observe.png)

Make a change to your controller (e.g., run `juju add-model test` to add another model and trigger some more API server connections) and refresh the page to view the updated results!

Congratulations, you now have a functional observability setup! But your controller is not the only thing that you can monitor -- go ahead and try to monitor something else, for example, your PostgreSQL!

## Tear down your test environment

To remove any trace of this tutorial, remove your entire Multipass Ubuntu VM (`multipass delete --purge my-charm-vm`, then [uninstall Multipass](https://documentation.ubuntu.com/multipass/en/latest/how-to-guides/install-multipass/#uninstall).





