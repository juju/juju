(tutorial)=
# Get started with Juju

Juju is an application orchestration tool for charmed applications -- that is, applications whose installation, configuration, integration, scaling, upgrading, etc., operations have been defined in a way that Juju can interpret in software packages called 'charms'.

In this tutorial you will get acquainted with Juju and charms by deploying a chat service.

What you'll need:
- A workstation that has sufficient resources to launch a virtual machine with 4 CPUs, 8 GB RAM, and 50 GB disk space.
- Familiarity with a terminal.

What you'll do:
- Set up an isolated test environment with Multipass, then set up Juju with a localhost Kubernetes cloud, and use it to deploy, configure, and integrate the charms required to set up a chat service based on Mattermost and PostgreSQL.

## Set up an isolated test environment

When you're trying things out it's nice to work in an isolated test environment. Let's spin up an Ubuntu virtual machine (VM) with Multipass!

First, [install Multipass](https://documentation.ubuntu.com/multipass/en/latest/how-to-guides/install-multipass/).

Now, launch an Ubuntu VM and open a shell in the VM:

```{terminal}
:user:
:host:
:input: multipass launch --cpus 4 --memory 8G --disk 50G --name my-juju-vm

Launched: my-juju-vm

:input: multipass shell my-juju-vm

Welcome to Ubuntu 24.04.2 LTS (GNU/Linux 6.8.0-64-generic x86_64)

 * Documentation:  https://help.ubuntu.com
 * Management:     https://landscape.canonical.com
 * Support:        https://ubuntu.com/pro

 System information as of Fri Aug  1 09:46:41 CEST 2025

  System load:  0.24              Processes:             145
  Usage of /:   3.3% of 47.39GB   Users logged in:       0
  Memory usage: 3%                IPv4 address for ens3: 10.238.98.204
  Swap usage:   0%


Expanded Security Maintenance for Applications is not enabled.

18 updates can be applied immediately.
18 of these updates are standard security updates.
To see these additional updates run: apt list --upgradable

Enable ESM Apps to receive additional future security updates.
See https://ubuntu.com/esm or run: sudo pro status


To run a command as administrator (user "root"), use "sudo <command>".
See "man sudo_root" for details.


```

Anything you type after the VM shell prompt will run on the VM.

```{dropdown} Tips for usage

At any point:
- To exit the shell, press {kbd}`mod` + {kbd}`C` (e.g., {kbd}`Ctrl`+{kbd}`C`) or type `exit`.
- To stop the VM after exiting the VM shell, run `multipass stop my-juju-vm`.
- To restart the VM and re-open a shell into it, type `multipass shell my-juju-vm`.

```
```{dropdown} Tips for troubleshooting

If the VM launch fails, run `multipass delete --purge my-juju-vm` to clean up, then try the launch line again.

```

## Set up Juju

```{figure} tutorial-setup.svg
   :alt: Juju consists of a client and a controller and both need access to a cloud and to Charmhub

   _Juju consists of at least a client and a controller, and both need access to a cloud and to Charmhub._
```

The way Juju works is that you use a client to talk to a controller; the controller talks to {ref}`clouds <list-of-supported-clouds>` to provision infrastructure and to [Charmhub](https://charmhub.io/) (or a local source for charms) to get charms to deploy, configure, integrate, scale, upgrade, etc., applications on that infrastructure; and the controller itself must live on a cloud resource, so before you do any of that you must use the client to talk to a cloud and Charmhub to bootstrap the controller into the cloud. Let's go ahead and prepare all those pieces and make sure your Juju is good to go!


### Prepare your cloud

To Juju a cloud is anything that has an API where you can request compute, storage, and networking.

This includes traditional machine clouds (Amazon AWS, Google GCE, Microsoft Azure, but also MAAS, OpenStack, Oracle OCI, and LXD) as well as Kubernetes clusters (Amazon EKS, Google GKE, Microsoft AKS but also Canonical Kubernetes or MicroK8s).

Among these is MicroK8s, a low-ops, minimal production Kubernetes that you can also use to get a small, single-node localhost Kubernetes cluster ([see more](https://documentation.ubuntu.com/juju/3.6/reference/cloud/list-of-supported-clouds/the-microk8s-cloud-and-juju/)). Let's set it up on your VM:

```{terminal}
:user: ubuntu
:host: my-juju-vm

# Install the MicroK8s package:
:input: sudo snap install microk8s --channel 1.28-strict

2025-08-01T09:47:10+02:00 INFO Waiting for automatic snapd restart...
microk8s (1.28-strict/stable) v1.28.15 from Canonical✓ installed

# Add your user to the `microk8s` group for unprivileged access:

:input: sudo adduser $USER snap_microk8s

info: Adding user `ubuntu' to group `snap_microk8s' ...

# Give your user permissions to read the ~/.kube directory:

:input: sudo chown -f -R $USER ~/.kube

# Wait for MicroK8s to finish initialising:

:input: sudo microk8s status --wait-ready

microk8s is running
high-availability: no
  datastore master nodes: 127.0.0.1:19001
  datastore standby nodes: none
addons:
  enabled:
    dns                  # (core) CoreDNS
    ha-cluster           # (core) Configure high availability on the current node
    helm                 # (core) Helm - the package manager for Kubernetes
    helm3                # (core) Helm 3 - the package manager for Kubernetes
  disabled:
    cert-manager         # (core) Cloud native certificate management
    cis-hardening        # (core) Apply CIS K8s hardening
    community            # (core) The community addons repository
    dashboard            # (core) The Kubernetes dashboard
    host-access          # (core) Allow Pods connecting to Host services smoothly
    hostpath-storage     # (core) Storage class; allocates storage from host directory
    ingress              # (core) Ingress controller for external access
    mayastor             # (core) OpenEBS MayaStor
    metallb              # (core) Loadbalancer for your Kubernetes cluster
    metrics-server       # (core) K8s Metrics Server for API access to service metrics
    minio                # (core) MinIO object storage
    observability        # (core) A lightweight observability stack for logs, traces and metrics
    prometheus           # (core) Prometheus operator for monitoring and logging
    rbac                 # (core) Role-Based Access Control for authorisation
    registry             # (core) Private image registry exposed on localhost:32000
    rook-ceph            # (core) Distributed Ceph storage using Rook
    storage              # (core) Alias to hostpath-storage add-on, deprecated

# Enable the 'storage' and 'dns' addons (required for the Juju controller):
:input: sudo microk8s enable hostpath-storage dns

Infer repository core for addon hostpath-storage
Infer repository core for addon dns
WARNING: Do not enable or disable multiple addons in one command.
         This form of chained operations on addons will be DEPRECATED in the future.
         Please, enable one addon at a time: 'microk8s enable <addon>'
Enabling default storage class.
WARNING: Hostpath storage is not suitable for production environments.
         A hostpath volume can grow beyond the size limit set in the volume claim manifest.

deployment.apps/hostpath-provisioner created
storageclass.storage.k8s.io/microk8s-hostpath created
serviceaccount/microk8s-hostpath created
clusterrole.rbac.authorization.k8s.io/microk8s-hostpath created
clusterrolebinding.rbac.authorization.k8s.io/microk8s-hostpath created
Storage will be available soon.
Addon core/dns is already enabled

# Alias kubectl so it interacts with MicroK8s by default:
:input: sudo snap alias microk8s.kubectl kubectl

Added:
  - microk8s.kubectl as kubectl

# Ensure your new group membership is apparent in the current terminal:
# (Not required once you have logged out and back in again)
:input: newgrp snap_microk8s

# Since the juju package is strictly confined, you also need to manually create a path:
:input: mkdir -p ~/.local/share

```

Congratulations, your cloud is ready!

### Prepare Charmhub

If you have access to the internet you're all set -- Juju will be able to reach Charmhub!

### Set up the `juju` CLI client

In Juju a (user-facing) client is anything that can talk to a Juju controller. That currently includes the `juju` CLI, the `terraform` CLI when used with the `juju` provider plugin, two Python clients, and a JavaScript client. However, to get a Juju controller you need the `juju` CLI client and it must have access to a cloud and Charmhub. Let's go ahead and set it up!

In your VM, install the `juju` CLI client:

```{terminal}
:user: ubuntu
:host: my-juju-vm
:input: sudo snap install juju

juju (3/stable) 3.6.8 from Canonical✓ installed
```

Now, ensure the client has access to your cloud (i.e., knows where to find your cloud and has the credentials to access your cloud). For a localhost MicroK8s cloud installed from a strictly confined snap like ours, your `juju` client can read the local kubeconfig file and retrieve the cloud definition (and credentials) from there automatically, as you can verify:

```{terminal}
:user: ubuntu
:host: my-juju-vm
:input: juju clouds --client

# Output should show microk8s.
# If it doesn't: Exit the VM (`exit`), re-enter it (`multipass shell my-juju-vm`), then try again.

```

Ensure also that the client has access to Charmhub by launching a random query, e.g., "ingress", and asking for more information about one of the results it shows:

```{terminal}
:user: ubuntu
:host: my-juju-vm
:input: juju find ingress

# Output should show all the charms or charm bundles related to this query that are available on Charmhub.
# For best results always double-check Charmhub.

:input: juju info traefik-k8s

# Output should show name, publisher, quick description, integration endpoints, etc.
# For the full features and more info see directly the charm's page on Charmhub.
```

Our `juju` client is ready! Sneak a peek at what it can do (`juju help commands` -- you can pipe a query to zoom in on feature: `juju help commands | grep application`). Check status (`juju status` and `juju debug-log` -- not much to see there yet). Keep going...

```{tip}

Split your terminal window into 3 (or open 3 terminal windows). In all, access your Multipass VM shell (`multipass shell my-juju-vm`) and then:

**Shell 1:** Keep using it as you've already been doing so far, namely to type the commands in this tutorial.

**Shell 2:** Run `watch -n 1 -c juju status --relations --color` to watch your deployment status evolve. Things are all right if your `App Status` and your `Unit - Workload` reach `active` and your `Unit - Agent` reaches `idle`. To exit and return to the terminal prompt, press {kbd}`mod` + {kbd}`C` (e.g., {kbd}`Ctrl`+{kbd}`C`). See more: {ref}`status`

**Shell 3:** Run `juju debug-log -m controller` to watch all the details behind your deployment status. (Especially useful when things don't evolve as expected. In that case, please get in touch.)
```

### Set up a Juju controller

A Juju controller is your Juju control plane -- the entity that holds the Juju API server and Juju's database. Anything you do in Juju post-controller-setup goes through a Juju controller, and to work properly the controller needs access to a cloud and to Charmhub (or a local source of charms). Let's set it up!

In your VM, use your client and its access to the MicroK8s cloud to bootstrap a Juju controller:

```{terminal}
:user: ubuntu
:host: my-juju-vm
:input: juju bootstrap microk8s my-first-juju-controller
Creating Juju controller "my-first-juju-controller" on microk8s/localhost
Bootstrap to Kubernetes cluster identified as microk8s/localhost
Creating k8s resources for controller "controller-my-first-juju-controller"
Downloading images
Starting controller pod
Bootstrap agent now started
Contacting Juju controller at 10.152.183.180 to verify accessibility...

Bootstrap complete, controller "my-first-juju-controller" is now available in namespace "controller-my-first-juju-controller"

Now you can run
	juju add-model <model-name>
to create a new model to deploy k8s workloads.

```

This will use ingredients from your client (especially the Juju agent binaries), the `juju-controller` charm from Charmhub and a pod from MicroK8s (backed by your current node -- your VM) to give you a running Juju controller.


Take a moment to appreciate what this looks like:
- On the MicroK8s side (in MicroK8s' database) you've now got:
  1. a 'controller' namespace (`kubectl get namespaces`) holding
  1. a 'controller-0' pod (`kubectl -n controller-my-first-juju-controller get pods`) holding
  1. 3 containers: 'api-server', 'charm', 'mongodb' (`kubectl -n controller describe pod controller-0`).
- On the Juju side (in the Juju controller's database) you've now got:
  1. a Juju 'my-first-juju-controller' controller (i.e., control plane), with access to the 'microk8s' cloud (`juju show-controller my-first-juju-controller`), and holding
  1. a Juju model (i.e., workspace; corresponding to a Kubernetes namespace) implicitly called 'controller', using resources from the 'microk8s' cloud (`juju switch controller`, then `juju show-model controller`), and  holding
  1. a Juju application (i.e., deployed charm -- in this case, the `juju-controller` charm) called 'controller' with a Juju unit (i.e., replica; corresponding to a Kubernetes pod) called 'controller/0', deployed on a resource from the 'microk8s' cloud (`juju status`), and holding
  1. the same 3 containers we saw on the MicroK8s side (`juju ssh --container [api-server | charm | mongodb] 0`), which hold the various bits of a running Juju controller:
    - 'charm': contains a Juju unit agent and `juju-controller` charm code. The Juju unit agent sends requests to the Juju controller agent (see next item) to check the unit's target state against the state in the database (see two items down) and progresses the unit's state by executing the charm code.
    - 'api-server': contains Pebble, a lightweight process supervisor, and the Juju controller agent, which also wraps the Juju API server. The controller agent reads and writes to the database (see next item).
    - 'mongodb': contains Juju's database.

These details are specific to Kubernetes, bootstrap, and the controller application, but this is fundamentally how Juju operates for all clouds, all operations, and all applications: the cloud provides a resource and Charmhub provides charms, and the result is a resource that holds the Juju unit agent and the charm code, where the Juju unit agent always pings the Juju controller agent to reconcile state, then executes the charm code accordingly, and the charm reacts as it's been programmed to react, e.g., by installing a workload.

Back to our main concern, to be fully operational a controller needs access to a cloud and to Charmhub (or a local source for charms). We saw just now that our controller already has access to our 'microk8s' cloud -- this access was granted implicitly through bootstrap. Also, as before, so long as you're connected to the internet, your controller has access to Charmhub too. Your controller is all set!

At this point we could connect to it further clouds or set up the Juju dashboard. For the purpose of this tutorial, however, we will skip ahead to talking about users and permissions.

## Handle authentication and authorization

```{figure} tutorial-handle-auth.svg
   :alt: A user is any person that can log in to a Juju controller.

   _A user is any person that can log in to a Juju controller._
```

Your client and controller can already talk to a cloud and Charmhub, but they don't run on their own -- enter the user! In Juju, the user is any person that can log in to a controller, and what they can do can be controlled at the level of the controller or some of the smaller entities associated with that controller. As the entity that has bootstrapped the controller, you have automatically been logged in and given `superuser` access. (It's why you were able to SSH into the controller unit!) Let's verify:

```{terminal}
:user: ubuntu
:host: my-juju-vm
:input: juju whoami

Controller:  my-first-juju-controller
Model:       <no-current-model>
User:        admin

:input: juju show-user admin
user-name: admin
display-name: admin
access: superuser
date-created: 3 hours ago
last-connection: just now
```

At this point you could go ahead and add further users and control their permissions. However, for the purpose of this tutorial, we will skip ahead and deploy our chat service!

## Provision infrastructure and operate applications

```{figure} tutorial-provision-deploy.svg
   :alt: A user uses the client to talk to the controller to talk to the cloud and to Charmhub to provision infrastructure and to deploy and operate charmed applications.

   _A user uses the client to talk to the controller to talk to the cloud and to Charmhub to provision infrastructure and to deploy and operate charmed applications. The way this works is that any application unit created by Juju contains a unit agent that is constantly checking state with the controller agent and progressing the unit's state by executing the charm code to install, configure, etc., applications._
```

As you could see from the controller bootstrap step, anything you provision or deploy and operate with a Juju controller goes onto a workspace called a 'model'. Let's create the model that will hold our chat applications:

```{terminal}
:user: ubuntu
:host: my-juju-vm
:input: juju add-model my-chat-model
Added 'my-chat-model' model on microk8s/localhost with credential 'microk8s' for user 'admin'
```

This will automatically also switch you to that model.

Now, let's deploy, configure, and integrate the charmed applications that will make up our chat service:

First, [Mattermost](https://charmhub.io/mattermost-k8s):


```{terminal}
:user: ubuntu
:host: my-juju-vm
:input: juju deploy mattermost-k8s --constraints "mem=2G"
Deployed "mattermost-k8s" from charm-hub charm "mattermost-k8s", revision 27 in channel latest/stable on ubuntu@20.04/stable
```

<!--
A popular open source application for that is Mattermost ([see more](https://mattermost.com/)).  A search on Charmhub reveals there is already a suitable charm for this application, the `mattermost-k8s` charm ([see more](https://charmhub.io/mattermost-k8s)). Moreover, a quick look at this charm's docs shows that, to satisfy this application's dependency on PostgreSQL, this charm also supports easy integration with PostgreSQL through the `postgresql-k8s` charm ([see more](https://charmhub.io/postgresql-k8s)), though traffic from PostgreSQL must be TLS-encrypted, something that can be satisfied, for our tutorial purposes, through further integration with the application deployed from the `self-signed-certificates` charm ([see more](https://charmhub.io/self-signed-certificates)). -->

Now, its dependencies. Mattermost needs a PostgreSQL database, and [its charmed version supports an easy way to integrate with such a database](https://charmhub.io/mattermost-k8s/integrations#db). Let's deploy [PostgreSQL](https://charmhub.io/postgresql-k8s) in the recommended way, from track 14 with risk `stable`; with `--trust` -- i.e., permission to use our cloud credentials (this charm needs to create and manage some Kubernetes resources); because we're just playing around, setting [the `profile` config](https://charmhub.io/postgresql-k8s/configurations#profile) to `testing`, so we don't use too many resources; and, just for fun, with `-n 2`, that is, two replicas (in a real life setting you'll want to distribute them over multiple nodes -- something Juju would do automatically here too, except we're doing everything on a single node).

```{terminal}
:user: ubuntu
:host: my-juju-vm
:input: juju deploy postgresql-k8s --channel 14/stable --trust --config profile=testing -n 2
Deployed "postgresql-k8s" from charm-hub charm "postgresql-k8s", revision 495 in channel 14/stable on ubuntu@22.04/stable
```

Mattermost wants PostgreSQL status to be TLS-encrypted. There are a few ways to do that. Because we're just trying things out, we can use [Self Signed X.509 Certificates](https://charmhub.io/self-signed-certificates) (don't do this in production!). Let's deploy it and integrate it with our PostgreSQL to enable TLS encryption on our PostgreSQL cluster:

```{terminal}
:user: ubuntu
:host: my-juju-vm
:input: juju deploy self-signed-certificates
Deployed "self-signed-certificates" from charm-hub charm "self-signed-certificates", revision 317 in channel 1/stable on ubuntu@24.04/stable

# Two charmed application can be integrated with one another if they have endpoints that
# support the same interface (e.g., 'tls-certificates') and
# have opposite endpoint roles ('requires' vs. 'provides').
:input: juju integrate self-signed-certificates postgresql-k8s
```

Finally, time to integrate Postgresql with Mattermost:

```{terminal}
:user: ubuntu
:host: my-juju-vm
:input: juju integrate postgresql-k8s:db mattermost-k8s
```

While executing any of these commands returns automatically so you can execute the next, standing things up in the cloud takes a little bit of time; watch your progress with `juju status --relations --color --watch 1s`. Things are all set when the output looks similar to the one below:

```{terminal}
:user: ubuntu
:host: my-juju-vm
:input: juju status --relations --color

Model          Controller                Cloud/Region        Version  SLA          Timestamp
my-chat-model  my-first-juju-controller  microk8s/localhost  3.6.8    unsupported  13:26:28+02:00

App                       Version                         Status  Scale  Charm                     Channel        Rev  Address         Exposed  Message
mattermost-k8s            .../mattermost:v8.1.3-20.04...  active      1  mattermost-k8s            latest/stable   27  10.152.183.144  no
postgresql-k8s            14.15                           active      2  postgresql-k8s            14/stable      495  10.152.183.184  no
self-signed-certificates                                  active      1  self-signed-certificates  1/stable       317  10.152.183.160  no

Unit                         Workload  Agent  Address      Ports     Message
mattermost-k8s/0*            active    idle   10.1.32.142  8065/TCP
postgresql-k8s/0*            active    idle   10.1.32.139            Primary
postgresql-k8s/1             active    idle   10.1.32.140
self-signed-certificates/0*  active    idle   10.1.32.141

Integration provider                   Requirer                       Interface         Type     Message
postgresql-k8s:database-peers          postgresql-k8s:database-peers  postgresql_peers  peer
postgresql-k8s:db                      mattermost-k8s:db              pgsql             regular
postgresql-k8s:restart                 postgresql-k8s:restart         rolling_op        peer
postgresql-k8s:upgrade                 postgresql-k8s:upgrade         upgrade           peer
self-signed-certificates:certificates  postgresql-k8s:certificates    tls-certificates  regular
```

Time to test the results! From the output of `juju status`> `Unit` > `mattermost-k8s/0`, retrieve the IP address and the port and feed them to `curl` on the template `curl <IP address>:<port number>/api/v4/system/ping`. Given the IP we got above:

```{terminal}
:user: ubuntu
:host: my-juju-vm
:input: curl 10.1.32.142:8065/api/v4/system/ping
{"ActiveSearchBackend":"database","AndroidLatestVersion":"","AndroidMinVersion":"","IosLatestVersion":"","IosMinVersion":"","status":"OK"}
```

Congratulations, your chat service is up and running!

As before, take a moment to appreciate what this looks like:

- On the MicroK8s side (in MicroK8s' database) you've now got:
  1. a 'chat' namespace (`kubectl get namespaces`) holding
  1. pods named after Mattermost, PostgreSQL, and Self Signed Certificates (`kubectl -n chat pods`), each holding
  1. a 'charm' container and one or more workload-related containers (e.g., try `kubectl -n chat describe pod postgresql-k8s-0`).
- On the Juju side (in Juju controller's database) you've now got:
  1. a Juju model called 'chat', associated with the 'my-first-juju-controller' controller, using resources from the 'microk8s' cloud (`juju show-model chat`), and holding
  1. Juju applications for Mattermost, PostgreSQL, and Self Signed Certificates, with one or more Juju units, deployed on a resource from the 'microk8s' cloud (`juju status`), each holding
  1. a 'charm' container holding a Juju unit agent and charm code and one more more worload-related containers.
  1. a relation and relation databags holding the data the integrated applications read and write for the purpose of their integration.

Thus, whenever you type `juju deploy` into your `juju` CLI client, the client talks to the controller, which talks to clouds to get resources and to Charmhub to get charms, and the result is a resource that contains a Juju unit agent, charm code, and whatever else executing the charm code may install, e.g., PostgreSQL. Any subsequent operation affecting the deployed application follows a similar flow, with the unit agent always pinging the controller agent to find out what its unit's target state should be and then sending hooks to the charm to get it to react in ways that will make the unit's state match the target state, e.g., by installing or configuring something. Application integration works exactly the same, with the units of the applications involved in the relation exchanging data through the controller agent, infollowing a star topology.

At this point you can keep your current Juju setup to experiment further or proceed to the next section to tear things down.

## Tear everything down

```{tip}
To tear everything down at once, skip to the step where you delete your Multipass VM and uninstall Multipass.
```

Tear down your Juju deployment:

```{terminal}
:user: ubuntu
:host: my-juju-vm
# Destroy any models you've created
# (this will also remove applications along with their configs, relations, etc.,
# and the cloud resources associated with them):
:input: juju destroy-model my-chat-model --destroy-storage
WARNING This command will destroy the "my-chat-model" model and affect the following resources. It cannot be stopped.

 - 3 applications will be removed
  - application list: "mattermost-k8s" "postgresql-k8s" "self-signed-certificates"
 - 2 filesystems and 2 volumes will be destroyed

To continue, enter the name of the model to be unregistered: my-chat-model
Destroying model
Waiting for model to be removed, 3 application(s), 2 volume(s), 2 filesystems(s)..........
Waiting for model to be removed, 3 application(s), 2 volume(s), 1 filesystems(s).....
Waiting for model to be removed, 3 application(s), 2 volume(s).....
Waiting for model to be removed, 3 application(s), 1 volume(s)...
Waiting for model to be removed, 2 application(s).....
Waiting for model to be removed, 1 application(s)............
Waiting for model to be removed........
Model destroyed.

# Destroy any controllers you've created:
:input: juju destroy-controller my-first-juju-controller
WARNING This command will destroy the "my-first-juju-controller" controller and all its resources


To continue, enter the name of the controller to be unregistered: my-first-juju-controller
Destroying controller
Waiting for model resources to be reclaimed
All models reclaimed, cleaning up controller machines

# Uninstall the juju client:
:input: sudo snap remove juju
juju removed

# Reset Microk8s:
:input:  sudo microk8s reset
Disabling all addons
Disabling addon : core/cert-manager
Disabling addon : core/cis-hardening
Disabling addon : core/dashboard
Disabling addon : core/dns
Disabling addon : core/helm
Disabling addon : core/helm3
Disabling addon : core/host-access
Disabling addon : core/hostpath-storage
Disabling addon : core/ingress
Disabling addon : core/mayastor
Disabling addon : core/metallb
Disabling addon : core/metrics-server
Disabling addon : core/minio
Disabling addon : core/observability
Disabling addon : core/prometheus
Disabling addon : core/rbac
Disabling addon : core/registry
Disabling addon : core/rook-ceph
Disabling addon : core/storage
All addons are disabled.
Deleting the CNI
Cleaning resources in namespace default
Cleaning resources in namespace kube-node-lease
Cleaning resources in namespace kube-public
Cleaning resources in namespace kube-system
Removing CRDs
Removing PriorityClasses
Removing StorageClasses
Restarting cluster
Setting up the CNI

# Uninstall Microk8s:
:input: $ sudo snap remove microk8s
microk8s removed

# Remove your user from the snap_microk8s group:
:input: sudo gpasswd -d $USER snap_microk8s
Removing user ubuntu from group snap_microk8s
```

Now exit the VM (in your terminal type `exit`); then, from your host machine, delete the VM:

```{terminal}
:user:
:host:
:input: multipass delete --purge my-juju-vm

:input: multipass list
No instances found.
```

Finally, [uninstall Multipass](https://documentation.ubuntu.com/multipass/en/latest/how-to-guides/install-multipass/#uninstall).


## Next steps

This tutorial has introduced you to the basic things you can do with `juju`. But there is a lot more to explore:

| If you are wondering... | visit our...                          |
| ----------------------- | ------------------------------------- |
| "How do I...?"          | [How-to docs](howto/index)            |
| "What is...?"           | [Reference docs](reference/index)     |
| "Why...?", "So what?"   | [Explanation docs](explanation/index) |


