(tutorial)=
# Get started with Juju

Imagine your business needs a chat service such as Mattermost backed up by a database such as PostgreSQL. In a traditional setup, this can be quite a challenge, but with Juju you'll find yourself deploying, configuring, scaling, integrating, etc., applications in no time. Let's get started!

**What you'll need:**
- A workstation that has sufficient resources to launch a virtual machine with 4 CPUs, 8 GB RAM, and 50 GB disk space.

**What you'll do:**
- Set up an isolated test environment with Multipass and the `charm-dev` blueprint, which will provide all the necessary tools and configuration for the tutorial (a localhost machine cloud and Kubernetes cloud, Juju, etc.).

- Plan, deploy, and maintain a chat service based on Mattermost and backed by PostgreSQL on a local Kubernetes cloud with Juju.


## Set up an isolated test environment


See {ref}`Set up your deployment – local testing and development <set-things-up>`.

```{important}
We strongly recommend you follow the automatic path where you use a Multipass VM based on the `charm-dev` blueprint.

If you however decide to follow the manual path: Please make sure to stay very close to [the definition of the `charm-dev` blueprint](https://github.com/canonical/multipass-blueprints/blob/ae90147b811a79eaf4508f4776390141e0195fe7/v1/charm-dev.yaml#L134).

You only need to go up to the point where you have Juju, a cloud, a controller

```

## Plan


In this tutorial your goal is to set up a chat service on a cloud.

First, decide which cloud (i.e., anything that provides storage, compute, and networking) you want to use. Juju supports a long list of clouds; in this tutorial we will use a low-ops, minimal production Kubernetes called 'MicroK8s'. In a terminal, open a shell into your VM and verify that you already have MicroK8s installed (`microk8s version`).

> See more: {ref}`cloud`, {ref}`list-of-supported-clouds`, {ref}`cloud-kubernetes-microk8s`


Next, decide which charms (i.e., software operators) you want to use. Charmhub provides a large collection. For this tutorial we will use `mattermost-k8s`  for the chat service,  `postgresql-k8s` for its backing database, and `self-signed-certificates` to TLS-encrypt traffic from PostgreSQL.


> See more: {ref}`charm`, [Charmhub](https://charmhub.io/), Charmhub | [`mattermost-k8s`](https://charmhub.io/mattermost-k8s), [`postgresql-k8s`](https://charmhub.io/postgresql-k8s), [`self-signed-certificates`](https://charmhub.io/self-signed-certificates)


## Deploy


You will need to install a Juju client; on the client, add your cloud and cloud credentials; on the cloud, bootstrap a controller (i.e., control plane); on the controller, add a model (i.e., canvas to deploy things on; namespace); on the model, deploy, configure, and integrate the charms that make up your chat service.

The blueprint used to launch your VM has ensured that most of these things are already in place for you -- verify that you have a Juju client, that it knows about your MicroK8s cloud and cloud credentials, that the MicroK8s cloud already has a controller bootstrapped on it, and that the Microk8s controller already has a model on it.

Just for practice, bootstrap a new controller and model with more informative names -- a controller called `31microk8s` (reflecting the version of Juju that came with your VM and the cloud that the controller lives on) and a model called `chat` (reflecting the fact that we intend to use it for applications related to a chat service).

Finally, go ahead and deploy, configure, and integrate your charms.

Sample session (yours should look very similar):


```{tip}
Split your terminal window into three. In all, access your Multipass VM shell (`multipass shell my-juju-vm`) and then:

**Shell 1:** Keep using it as you've already been doing so far, namely to type the commands in this tutorial.

**Shell 2:**  Run `juju status --relations --watch 1s` to watch your deployment status evolve. (Things are all right if your `App Status` and your `Unit - Workload` reach `active` and your `Unit - Agent` reaches `idle`. See more: {ref}`status`.

**Shell 3:** Run `juju debug-log` to watch all the details behind your deployment status. (Especially useful when things don't evolve as expected. In that case, please get in touch.)
```


```text
# Verify that you have the juju client installed:
ubuntu@my-juju-vm:~$ juju version
3.1.8-genericlinux-amd64

# Verify that the client already knows about your microk8s cloud:
ubuntu@my-juju-vm:~$ juju clouds
# (Ignore the client-controller distinction for now --it'll make sense in a bit.)
Only clouds with registered credentials are shown.
There are more clouds, use --all to see them.

Clouds available on the controller:
Cloud     Regions  Default    Type
microk8s  1        localhost  k8s

Clouds available on the client:
Cloud      Regions  Default    Type  Credentials  Source    Description
localhost  1        localhost  lxd   1            built-in  LXD Container Hypervisor
microk8s   1        localhost  k8s   1            built-in  A Kubernetes Cluster


# Verify that the client already knows about your microk8s credentials:
ubuntu@my-juju-vm:~$ juju credentials
# (Ignore the client-controller distinction for now --it'll make sense in a bit.)
Controller Credentials:
Cloud     Credentials
microk8s  microk8s

Client Credentials:
Cloud      Credentials
localhost  localhost*
microk8s   microk8s*
ubuntu@my-juju-vm:~$ juju controllers
Use --refresh option with this command to see the latest information.

Controller  Model        User   Access     Cloud/Region         Models  Nodes    HA  Version
lxd         welcome-lxd  admin  superuser  localhost/localhost       2      1  none  3.1.8
microk8s*   welcome-k8s  admin  superuser  microk8s/localhost        2      1     -  3.1.8
ubuntu@my-juju-vm:~$

# Bootstrap a new controller:
ubuntu@my-juju-vm:~$ juju bootstrap microk8s 31microk8s
Creating Juju controller "31microk8s" on microk8s/localhost
Bootstrap to Kubernetes cluster identified as microk8s/localhost
Creating k8s resources for controller "controller-31microk8s"
Starting controller pod
Bootstrap agent now started
Contacting Juju controller at 10.152.183.71 to verify accessibility...

Bootstrap complete, controller "31microk8s" is now available in namespace "controller-31microk8s"

Now you can run
	juju add-model <model-name>
to create a new model to deploy k8s workloads.

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


## Maintain


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
> See more: [MicroK8s | Create a multi-node cluster](https://microk8s.io/docs/clustering)
```

> See more: {ref}`manage-applications` > Scale



## Tear down your test environment


To tear things down, remove your entire Multipass Ubuntu VM, then uninstall Multipass.

> See more: {ref}`tear-things-down`


