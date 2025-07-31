(tutorial)=
# Get started with Juju

Imagine your business needs a chat service such as Mattermost backed up by a database such as PostgreSQL. In a traditional setup, this can be quite a challenge, but with Juju you'll find yourself deploying, configuring, scaling, integrating, etc., applications in no time. Let's get started!

**What you'll need:**
- A workstation that has sufficient resources to launch a virtual machine with 4 CPUs, 8 GB RAM, and 50 GB disk space.

**What you'll do:**
- Set up an isolated test environment with Multipass and the `charm-dev` blueprint, which will provide all the necessary tools and configuration for the tutorial (a localhost machine cloud and Kubernetes cloud, Juju, etc.).

- Plan, deploy, and maintain a chat service based on Mattermost and backed by PostgreSQL on a local Kubernetes cloud with Juju.

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

## Set up Juju

### Prepare a cloud

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

### Get acquainted with Charmhub

### Set up the `juju` CLI client

Juju is a distributed system that consists of one or more clients, one or more controllers, and one or more agents, where the client is typically on your workstation, the controller is something you bootstrap into a cloud using the client, and the agents are something you deploy implicitly every time you provision infrastructure or deploya applications with Juju. Let's install the `juju` CLI client and use it with our MicroK8s cloud to bootstrap a controller!

In your VM, install the `juju` CLI client:

```text
sudo snap install juju
```

Needs access to a cloud and Charmhub. Happens automatically:

```text
juju clouds --client
juju find ...
```


```{tip}
Split your terminal window into three. In all, access your Multipass VM shell (`multipass shell my-juju-vm`) and then:

**Shell 1:** Keep using it as you've already been doing so far, namely to type the commands in this tutorial.

**Shell 2:** Run `juju status --relations --color --watch 1s` to watch your deployment status evolve. (Things are all right if your `App Status` and your `Unit - Workload` reach `active` and your `Unit - Agent` reaches `idle`.)

**Shell 3:** Run `juju debug-log` to watch all the details behind your deployment status. (Especially useful when things don't evolve as expected. In that case, please get in touch.)
```

### Set up a Juju controller

Needs to live on a cloud resource:

```text
juju credentials --client
juju bootstrap microk8s my-first-controller
```

Like anything deployed with Juju, it is represented in Juju's database on a model with a unit... [need to be careful here to keep things simple -- this should be just a preview of the basic Juju machinery, no more]

Tip: That means you can also for the most part treat it like any other application you deploy with Juju, e.g., you can observe it with COS.

Needs access to a cloud and Charmhub. Happens implicitly:

## Handle authentication and authorization

Your client and controller can already talk to a cloud and Charmhub, but they don't run on their own -- enter the user! In Juju, the user is any entity that can log in to a controller, and what they can be controlled at the level of the controller or at the level of the clouds, models, or application offer associated with that controller.

Verify that you're the `admin` user.... Verify that you have controller superuser access.

## Provision infrastructure and deploy, configure, integrate, scale, etc. applications

Add a model for your chat applications.

Deploy Mattermost. Mattermost needs a PostgreSQL database, and traffic from this database needs to be TLS-encrypted, so let's also deploy ... A database failure can be costly, so let



```text
# Create a new model:
ubuntu@my-juju-vm:~$ juju add-model chat
Added 'chat' model on microk8s/localhost with credential 'microk8s' for user 'admin'

# Deploy mattermost-k8s:
ubuntu@tutorial-vm:~$ juju deploy mattermost-k8s
Located charm "mattermost-k8s" in charm-hub, revision 27
Deploying "mattermost-k8s" from charm-hub charm "mattermost-k8s", revision 27 in channel stable on ubuntu@20.04/stable

# Deploy and configure postgresql-k8s:
ubuntu@tutorial-vm:~$ juju deploy postgresql-k8s --channel 14/stable --trust --config profile=testing
Located charm "postgresql-k8s" in charm-hub, revision 193
Deploying "postgresql-k8s" from charm-hub charm "postgresql-k8s", revision 193 in channel 14/stable on ubuntu@22.04/stable

# Deploy self-signed-certificates:
ubuntu@my-juju-vm:~$ juju deploy self-signed-certificates
Located charm "self-signed-certificates" in charm-hub, revision 72
Deploying "self-signed-certificates" from charm-hub charm "self-signed-certificates", revision 72 in channel stable on ubuntu@22.04/stable

# Integrate self-signed-certificates with postgresql-k8s:
ubuntu@tutorial-vm:~$ juju integrate self-signed-certificates postgresql-k8s

# Integrate postgresql-k8s with mattermost-k8s:
ubuntu@tutorial-vm:~$ juju integrate postgresql-k8s:db mattermost-k8s

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


From the output of `juju status`> `Unit` > `mattermost-k8s/0`, retrieve the IP address and the port and feed them to `curl` on the template below:

```text
curl <IP address>:<port number>/api/v4/system/ping
```

Sample session:

```text
ubuntu@my-juju-vm:~$ curl 10.1.32.155:8065/api/v4/system/ping
{"ActiveSearchBackend":"database","AndroidLatestVersion":"","AndroidMinVersion":"","IosLatestVersion":"","IosMinVersion":"","status":"OK"}
```

Congratulations, your chat service is up and running!

```{caution} In a production scenario:
You'll want to make sure that the units are also properly distributed over multiple nodes. Our localhost MicroK8s doesn't allow us to do this (because we only have 1 node) but, if you clusterise MicroK8s, you can use it to explore this too!
> See more: [MicroK8s | Create a multi-node cluster](https://microk8s.io/docs/clustering)
```

> See more: {ref}`manage-applications` > Scale


## Tear down your Juju deployment

Follow the steps below, for practice, or skip to the next section to tear all down simply by deleting your Multipass VM.

```text
# Destroy any models you've created:
$ juju destroy-model my-model

# Destroy any controllers you've created:
$ juju destroy-controller my-controller

# Uninstall juju. For example:
$ sudo snap remove juju
```

```text
# Reset Microk8s:
$ sudo microk8s reset

# Uninstall Microk8s:
$ sudo snap remove microk8s

# Remove your user from the snap_microk8s group:
$ sudo gpasswd -d $USER snap_microk8s
```

## Tear down your test environment

To remove any trace of this tutorial, remove your entire Multipass Ubuntu VM (`multipass delete --purge my-charm-vm`, then [uninstall Multipass](https://documentation.ubuntu.com/multipass/en/latest/how-to-guides/install-multipass/#uninstall).


