(tutorial)=
# Get started with Juju

In this tutorial your goal is to set up a chat service on a cloud.

In traditional setups this can be quite a challenge; however, with charmed applications and their runtime Juju, it's plug-and-play from day 0 to Day n.

Are you ready to take control of cloud? Let's get started!

What you'll need:
- A workstation that has sufficient resources to launch a virtual machine with 4 CPUs, 8 GB RAM, and 50 GB disk space.

What you'll do:
- Get acquainted with the Juju paradigm for operations by deploying, configuring, integrate, and scaling a chat service based on [Mattermost](https://mattermost.com/) and backed by [PostgreSQL](https://www.postgresql.org/) using Juju and charms on a localhost Kubernetes cloud.


## Set up an isolated test environment

When you're trying things out it's nice to work in an isolated test environment. Let's spin up an Ubuntu virtual machine with (VM) Multipass!

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

## Install Juju

In your VM, install the `juju` CLI client:

```text
sudo snap install juju
```

## Connect a cloud

Juju supports a wide range of clouds -- whether traditional machine clouds or Kubernetes clusters.

Among these is MicroK8s, a low-ops, minimal production Kubernetes that you can also use to get a small, single-node localhost Kubernetes cluster on your VM:

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

The cloud is ready, but you need it to be connected to a Juju controller. If you had a controller that was ready deployed, at this point you could simply connect your cloud to it. You don't have that here, so connect your cloud to the client so it can deploy a controller, then connect to the controller to deploy further things:

```text
# Ensure you cloud is connected to your Juju client:
# (for your localhost MicroK8s from a strictly installed snap this should hapepn automatically):
juju clouds --client
juju credentials --client

# Ensure your cloud is connected to a Juju controller
# (because you don't have any ready controller you must bootstrap it):
juju bootstrap microk8s my-first-microk8s-controller
# When you do this all of the following happens:
# 1.On the cluster / cloud side:
# The client will create a namespace and, in this namespace, resources for the controller.
# The cluster will provision pods by pulling the specified container images etc.
# 2. On the Juju side:
# This creates a 'controller' model ( cf. namespace):
juju models
# Containing a 'controller' application (cf. service)
# consisting of one 'controller/0' unit (cf. pod):
juju status
# This unit (pod) consists of three containers,
# one of which holds a Juju unit agent and the Juju controller charm code,
# one of which holds Pebble and the controller agent (that contains Juju's API server), and
# one of which holds Juju's internal database
# The bootstrap process also automatically makes your cloud known to the controller:
juju clouds --controller
juju credentials --controller
```

Your cloud is set up and connected to a Juju controller. Time to deploy charmed applications!

## Use charmed applications

You have Juju, you've connected your cloud -- time to see the magic of charmed applications!

### Deploy

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

![Juju tutorial - Your deployment](tutorial.png)


*Your computer with your Multipass VM, your MicroK8s cloud, and a  live Juju controller (the 'charm' in the Controller Unit is the `juju-controller` charm) + a sample deployed application on it (the 'charm' in the Regular Unit stands for any charm that you might deploy). If in the Regular Application you replace the charm with `mattermost-k8s` and image a few more Regular Applications where you replace the charm with `postgresql-k8s` and, respectively, `self-signed-certificates`, and if you trace the path from `postgresql-k8s`'s Unit Agent through the Controller Agent to `self-signed-certificates`'s and, respectively, `mattermost-k8s` Unit Agent, you get a full representation of your deployment. (Note: After integration, the workloads may also know how to contact each other directly; still, all communication between their respective charms goes through the Juju controller and the result of that communication is stored in the database in the form of maps known as 'relation data bags'.)*


### Scale


A database failure can be very costly. Let's scale it!

Sample session:

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

As you might have guessed, the result of scaling an application is that you have multiple running instances of your application -- that is, multiple units.

```{caution} In a production scenario:
You'll want to make sure that they are also properly distributed over multiple nodes. Our localhost MicroK8s doesn't allow us to do this (because we only have 1 node) but, if you clusterise MicroK8s, you can use it to explore this too!
```

### Observe

Our deployment hasn't really been up very long, but we'd still like to take a closer look at our controller, to see what's happening. Time for observability!


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

# Offer prometheus' metrics-endpoint endpoint
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

On your local machine, open a browser window and copy-paste the Grafana URL. In the username field, enter 'admin'. In the password field, enter the `admin-password`. If everything has gone well, you should now be logged in.

On the new screen, in the top-right, click on the Menu icon, then **Dashboards**. Then, on the new screen, in the top-left, click on **New**, **Upload dashboard JSON file**, and upload the JSON Grafana-dashboard-definition file below, then, in the IL3-2 field, from the drop-down, select the suggested `juju_observability...` option.

[Juju Controllers-1713888589960.json|attachment](https://discourse.charmhub.io/uploads/short-url/yOxvgum6eo3NmMxPaTRKLOLmbo0.json) (200.9 KB)


On the new screen, at the very top, expand the Juju Metrics section and inspect the results. How many connections to the API server does your controller show?

![Juju tutorial - Observe your controller](tutorial-observe.png)

Make a change to your controller (e.g., run `juju add-model test` to add another model and trigger some more API server connections) and refresh the page to view the updated results!

Congratulations, you now have a functional observability setup! But your controller is not the only thing that you can monitor -- go ahead and try to monitor something else, for example, your PostgreSQL!

## Tear things down

Remove your entire Multipass Ubuntu VM:

```text
multipass delete --purge my-charm-vm
```

[Uninstall Multipass](https://documentation.ubuntu.com/multipass/en/latest/how-to-guides/install-multipass/#uninstall).





