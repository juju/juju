(manage-your-deployment)=
# How to manage your deployment

The goal of everything in Juju is to help you set up and maintain your cloud deployment, from day 0 to day 2, in the same unified way, on any cloud and even between clouds. This document covers the high-level logic.

(set-up-your-deployment)=
## Set up your deployment

To set up a cloud deployment with Juju, you need a cloud, Juju, and charms.

1. {ref}`Install the juju CLI client <install-juju>`.

2. Consult our {ref}`list of supported clouds <list-of-supported-clouds>` and prepare your cloud(s).

3. Add your {ref}`cloud definition(s) <add-a-cloud>` and {ref}`cloud credential(s) <add-a-credential>` to Juju and use your `juju` CLI client to {ref}`bootstrap a Juju controller <bootstrap-a-controller>` (control plane) into your cloud. Once the controller is up, you may connect further clouds directly to it.

4. Add {ref}`users <add-a-user>`, {ref}`SSH keys <add-an-ssh-key>`, {ref}`secret backends <add-a-secret-backend>`, etc.

5. Add {ref}`models <add-a-model>` (workspaces) to your controller, then start {ref}`deploying, configuring, integrating, scaling, etc., charmed applications <manage-applications>`. Juju takes care of the underlying infrastructure for you, but if you wish you can also customize {ref}`storage <add-storage>`, {ref}`networking <add-a-space>`, etc.


> See more: {ref}`administering-juju`, {ref}`building-with-juju`

(set-things-up)=
### Set up your deployment -- local testing and development

```{important}
The logic is always the same: set up an isolated environment; get Juju, a cloud, and charms; start deploying. However, for certain steps there is an automatic path that greatly facilitates things -- we strongly recommend you take it.


If you however wish to follow the manual path and to skip the blueprint or the entire Multipass VM: For best results try to stay very close to [the definition of the `charm-dev` blueprint](https://github.com/canonical/multipass-blueprints/blob/ae90147b811a79eaf4508f4776390141e0195fe7/v1/charm-dev.yaml#L134).

Depending on your use case you may also wish to install further Juju clients or charm development tools; we include those steps too, though feel free to skip them if they don't apply.
```

1. Create an isolated environment, as below:

[Install Multipass](https://multipass.run/docs/install-multipass). For example, on a Linux with `snapd`:

```text
$ sudo snap install multipass
```

```{important}
If on Windows: Note that Multipass can only be installed on Windows 10 Pro or Enterprise. If you are using a different version, please follow the manually, omitting the Multipass step.
```


Use Multipass to create an isolated environment:

``````{tabs}
`````{group-tab} automatically

Launch a VM called `my-juju-vm` using the `charm-dev` blueprint:

```{note}
This step may take a few minutes to complete (e.g., 10 mins).

This is because the command downloads, installs, (updates,) and configures a number of packages, and the speed will be affected by network bandwidth (not just your own, but also that of the package sources).

However, once it’s done, you’ll have everything you’ll need – all in a nice isolated environment that you can clean up easily. (See more: [GitHub > multipass-blueprints > charm-dev.yaml](https://github.com/canonical/multipass-blueprints/blob/ae90147b811a79eaf4508f4776390141e0195fe7/v1/charm-dev.yaml#L134).)

```

```text
$ multipass launch --cpus 4 --memory 8G --disk 50G --name my-juju-vm charm-dev

```

`````
`````{group-tab} manually
Launch a VM called `my-juju-vm`:

```text
$ multipass launch --cpus 4 --memory 8G --disk 50G --name my-juju-vm
```
`````
``````

Open a shell into the VM:

```text
$ multipass shell my-juju-vm
Welcome to Ubuntu 22.04.4 LTS (GNU/Linux 5.15.0-100-generic x86_64)
# ...
# Type any further commands after the VM shell prompt:
ubuntu@my-juju-vm:~$
```

```{dropdown} Tips for usage

At any point:
- To exit the shell, press {kbd}`mod` + {kbd}`C` (e.g., {kbd}`Ctrl`+{kbd}`C`) or type `exit`.
- To stop the VM after exiting the VM shell, run `multipass stop charm-dev-vm`.
- To restart the VM and re-open a shell into it, type `multipass shell charm-dev-vm`.

```
```{dropdown} Tips for troubleshooting
If the VM launch fails, run `multipass delete --purge my-juju-vm` to clean up, then try the launch line again.

```

2. Ensure you have the `juju` CLI client; on `juju`, a localhost cloud (`microk8s` - a MicroK8s-based Kubernetes cloud for Kubernetes charms; `localhost` -- a LXD-based machine cloud for machine charms); in the cloud, a Juju controller (i.e., control plane); on the controller, a model (i.e., workspace):

``````{tabs}
`````{group-tab} automatically

Thanks to the `charm-dev` blueprint, you should already have everything you need:

```text
# Verify that you have juju:
juju

# Verify that you have a Kubernetes and a machine cloud
# and they're already known to juju:
juju clouds

# Verify that you already have a controller bootstrapped into each:
juju controllers

# Switch to the preexisting workload model on the controller:
## For the MicroK8s cloud:
ubuntu@my-juju-vm:~$ juju switch microk8s:welcome-k8s

## For the LXD cloud:
ubuntu@my-juju-vm:~$ juju switch lxd:welcome-lxd

```
`````
`````{group-tab} manually

Install `juju`. For example, on a Linux with `snapd`:

```text
sudo snap install juju
```

> See more: {ref}`manage-juju`

Set up your cloud, add it to `juju`, then bootstrap a controller into the cloud:


````{dropdown} Example for MicroK8s, assuming a Linux with snapd:

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

# For MicroK8s, if you are working with an existing snap installation, and it is not strictly confined:
# (https://microk8s.io/docs/strict-confinement), you must also:
#
# # Share the MicroK8s config with Juju:
# sudo sh -c "mkdir -p /var/snap/juju/current/microk8s/credentials"
# sudo sh -c "microk8s config | tee /var/snap/juju/current/microk8s/credentials/client.config"
#
# # Give the current user permission to this file:
# sudo chown -f -R $USER:$USER /var/snap/juju/current/microk8s/credentials/client.config

# Register your MicroK8s cloud with Juju:
# Not necessary --juju recognises a localhost MicroK8s cloud automatically, as you can see by running 'juju clouds'.
juju clouds
# (If for any reason this doesn't happen, you can register it manually using 'juju add-k8s microk8s'.)

# Bootstrap a controller into your MicroK8s cloud:
juju bootstrap microk8s my-first-microk8s-controller


# Add a model to the controller:
juju add-model my-first-microk8s-model
# Reminder: In Kubernetes every Juju model corresponds to a namespace.

# Check the model's status:
juju status

# Happy deploying!

```
````

````{dropdown} Example for LXD, assuming a Linux that already has lxd:

```text
# lxd init --auto
lxc network set lxdbr0 ipv6.address none

# Register your LXD cloud with Juju:
# Not necessary --juju recognises a localhost LXD cloud automatically, as you can see by running 'juju clouds'.
juju clouds
# The LXD cloud appears under the name 'localhost'


# Bootstrap a controller into your LXD cloud:
juju bootstrap localhost my-first-lxd-controller

# Add a model to the controller:
juju add-model my-first-lxd-model

# Check the model's status:
juju status

# Happy deploying!

```
````
`````
``````

3. (If you are developing a charm or planning to also use a different Juju client:) Ensure you have all the necessary tools, for example, charming tools such as Charmcraft, Python, Tox, Docker, or additional Juju clients such as the Terraform Provider for Juju or JAAS:

````{dropdown} Example: Charming tools

```text
# Install Charmcraft:
$ sudo snap install charmcraft --classic

# Ensure you have a version of Python suitable for development with Ops (3.8+):
$ python3 --version

# Set up tox:
$ sudo apt update; sudo apt install python3 python3-pip
$ python3 -m pip install --user tox

# Set up Docker:
$ sudo addgroup --system docker
$ sudo adduser $USER docker
$ newgrp docker
$ sudo snap install docker

```
````

4. (If you are developing a charm or planning to also use a different Juju client, e.g., `terraform-provider-juju`:) Ensure any local files are accessible from your Multipass VM by creating a local directory and then mounting it to the Multipass VM. For example, if you're developing a charm:

```text
$ mkdir ~/my-charm

# Mount it to the Multipass VM:
$ multipass mount --type native ~/my-charm charm-dev-vm:~/my-charm

# Verify that it's indeed on the VM:
ubuntu@charm-dev-vm:~$ ls
my-charm  snap

# Going forward:
# - Use your host machine (on Linux, `cd ~/my-charm`) to create and edit your charm files. This will allow you to use your favorite local editor.
# - Use the Multipass VM shell (on Linux, `ubuntu@charm-dev-vm:~$ cd ~/my-charm`) to run Charmcraft and Juju commands.

```

5. Continue as usual by setting up users, storage, etc.; adding models; and deploying, configuring, integrating, etc., applications.

> See more: {ref}`administering-juju`, {ref}`building-with-juju`

(take-your-deployment-offline)=
### Set up your deployment -- offline

<!--This doc is intended to supersede https://discourse.charmhub.io/t/how-to-work-offline/1072 and the docs linked there.

IMO the doc has roughly the correct skeleton, though we may want to revisit the list of external services and we may want to include suggestions for server and proxy software, as in the now archived https://discourse.charmhub.io/t/offline-mode-strategies/1071.

When all is said and done, though, I feel the perspective still needs to be that of the constructs Juju provides, namely, the model-config keys, as it is that that will dictate whether you should plan to set up a local mirror or a proxy or rather download the resources beforehand.

PS Noticed some of the environment variables don't match what's in the list of model config keys. Does the envvar have to have a particular name, or can it be anything and it is something just by convention? Either way, we need to clarify.

Details:

https://discourse.charmhub.io/t/how-to-configure-juju-for-offline-usage/1068
>> we've incorporated the list of external sites and even added to it, but left out the detail about client-controller-machine and just linked to our ref docs on the bootstrap and deploy process -- though when you compare the list and those docs you realize those docs are missing some detail (cloud-images..., archive-..., and security-..., and container image registry)
>> we've incorporated and cleaned up the examples

https://discourse.charmhub.io/t/offline-mode-strategies/1071
>> This doc mentions a bunch of proxies and local mirrors that should be set, including suggestions for possible proxy software, and then the model-config keys that can be used to configure Juju to use those proxies / local mirrors.  The content duplicates some of the content in https://discourse.charmhub.io/t/how-to-configure-juju-for-offline-usage/1068  -- we've already incorporated all of that. However, we haven't yet incorporated the suggestions for server and proxy software.


https://discourse.charmhub.io/t/how-to-deploy-charms-offline/1069
>> This doc is all wrong. The current process would be to download the charms on a machine connected to the internet; move them to an offline machine; deploy locally. There is no mention of this here at all (as we don't support either proxies or mirrors?).

https://discourse.charmhub.io/t/how-to-install-snaps-offline/1179
>> This doc merely illustrates how to use the http-proxy model-config. We now also have more specific snap store proxy keys.

https://discourse.charmhub.io/t/how-to-use-the-localhost-cloud-offline/1070
>> This doc is merely featuring how to use the no-proxy key to exclude the localhost cloud from the list of things that you want to use a proxy.

-->

For an offline (to be more precise, proxy-restricted) deployment:

1. Set up a private cloud.

> See more: {ref}`List of supported clouds <list-of-supported-clouds>`

2. Figure out the list of external services required for your deployment and set up proxies / local mirrors for them. Depending on whether your deployment is on machines or Kubernetes, and on a localhost cloud or not, and which one, these services may include:

    - [https://streams.canonical.com](https://streams.canonical.com/) for agent binaries and LXD container and VM images;
    - [https://charmhub.io/](https://charmhub.io/) for charms, including the Juju controller charm;
    - [https://snapcraft.io/store](https://snapcraft.io/store) for Juju's internal database;
    - [http://cloud-images.ubuntu.com](http://cloud-images.ubuntu.com/) for base Ubuntu cloud machine images, and [http://archive.ubuntu.com](http://archive.ubuntu.com/) and [http://security.ubuntu.com](http://security.ubuntu.com/) for machine image upgrades;
    - a container image registry:
        - [https://hub.docker.com/](https://hub.docker.com/)
        - [https://gallery.ecr.aws/juju](https://gallery.ecr.aws/juju) (in Juju provide it as "public.ecr.aws")
        - [https://ghcr.io/juju](https://ghcr.io/juju)


3. Configure Juju to make use of the proxies / local mirrors you've set up by means of the following model configuration keys:

- {ref}`model-config-agent-metadata-url`
- {ref}`model-config-apt-ftp-proxy`
- {ref}`model-config-apt-http-proxy`
- {ref}`model-config-apt-https-proxy`
- {ref}`model-config-apt-mirror`
- {ref}`model-config-apt-no-proxy`
- {ref}`model-config-container-image-metadata-url`
- {ref}`model-config-ftp-proxy`
- {ref}`model-config-http-proxy`
- {ref}`model-config-https-proxy`
- {ref}`model-config-image-metadata-url`
- {ref}`model-config-juju-ftp-proxy`
- {ref}`model-config-juju-http-proxy`
- {ref}`model-config-juju-https-proxy`
- {ref}`model-config-juju-no-proxy`
- {ref}`model-config-no-proxy`
- {ref}`model-config-snap-http-proxy`
- {ref}`model-config-snap-https-proxy`
- {ref}`model-config-snap-store-assertions`
- {ref}`model-config-snap-store-proxy`
- {ref}`model-config-snap-store-proxy-url`


````{dropdown} Example: Configure the client to use an HTTP proxy


Set up an HTTP proxy, export it to an environment variable, then use the `http-proxy` model configuration key to point the client to that value.

<!--
``` text
export http_proxy=$PROXY_HTTP
```
-->

````

````{dropdown} Example: Configure all models to use an APT mirror


Set up an APT mirror, export it to the environment variable $MIRROR_APT, then set the `apt-mirror` model config key to point to that environment variable. For example, for a controller on AWS:

``` text
juju bootstrap --model-default apt-mirror=$MIRROR_APT aws
```

````

````{dropdown} Example: Have all models use local resources for both Juju agent binaries and cloud images


Get the resources for Juju agent binaries and cloud images locally; define and export export environment variables pointing to them; then set the `agent-metadata-url` and `image-metadata-url` model configuration keys to point to those environment variables. For example:

``` text
juju bootstrap \
    --model-default agent-metadata-url=$LOCAL_AGENTS \
    --model-default image-metadata-url=$LOCAL_IMAGES \
    localhost
```

````


````{dropdown} Example: Set up HTTP and HTTPS proxies but exclude the localhost cloud


Set up HTTP and HTTPS proxies and define and export environment variables pointing to them (below, `PROXY_HTTP` and `PROXY_HTTPS`); define and export a variable pointing to the IP addresses for your `localhost` cloud to the environment variable (below,`PROXY_NO`); then bootstrap setting the `http_proxy`, `https_proxy`, and `no-proxy` model configuration keys to the corresponding environment variable. For example:

```text
$ export PROXY_HTTP=http://squid.internal:3128
$ export PROXY_HTTPS=http://squid.internal:3128
$ export PROXY_NO=$(echo localhost 127.0.0.1 10.245.67.130 10.44.139.{1..255} | sed 's/ /,/g')

$ export http_proxy=$PROXY_HTTP
$ export https_proxy=$PROXY_HTTP
$ export no_proxy=$PROXY_NO

$ juju bootstrap \
--model-default http-proxy=$PROXY_HTTP \
--model-default https-proxy=$PROXY_HTTPS \
--model-default no-proxy=$PROXY_NO \
localhost lxd
```

````


4. Continue as usual by setting up users, storage, etc.; adding models; and deploying, configuring, integrating, etc., applications.

> See more: {ref}`administering-juju`, {ref}`building-with-juju`


(harden-your-deployment)=
## Harden your deployment

> See also: {ref}`juju-security`

Juju ships with sensible security defaults. However, security doesn't stop there.

### Harden the cloud

Use a private cloud.

> See more: {ref}`list-of-supported-clouds`

If you want to go one step further, take your cloud (and the entire deployment) offline.

> See more: {ref}`take-your-deployment-offline`

### Harden the client and the agent binaries

When you install Juju (= the `juju` CLI client + the Juju agent binaries) on Linux, you're installing it from a strictly confined snap. Make sure to keep this snap up to date.

> See more: [Snapcraft | Snap confinement](https://snapcraft.io/docs/snap-confinement), {ref}`manage-juju`, {ref}`juju-roadmap-and-releases`


### Harden the controller(s)

In a typical Juju workflow you allow your client to read your locally stored cloud credentials, then copy them to the controller, so that the controller can use them to authenticate with the cloud. However, for some clouds Juju now supports a workflow where your (client and) controller doesn't need to know your credentials directly -- you can just supply an instance profile (AWS) or a managed identity (Azure). One way to harden your controller is to take advantage of this workflow.

> See more: {ref}`bootstrap-a-controller`, {ref}`cloud-ec2`, {ref}`cloud-azure`

(Like all the cloud resources provisioned through Juju,) the cloud resource(s) (machines or containers) that a controller is deployed on by default run the latest Ubuntu LTS.  This Ubuntu is *not* CIS- and DISA-STIG-compliant (see more: [Ubuntu | The Ubuntu Security Guide](https://ubuntu.com/security/certifications/docs/usg)). However, it is by default behind a firewall, inside a VPC, with only the following three ports opened -- as well as hardened (through security groups) -- by default:

- (always:) `17070`, to allow access from clients and agents;
- (in high-availability scenarios): mongo
- (In high-availability scenarios): `controller-api-port`, which can be turned off (see {ref}`controller-config-api-port`).

When a controller deploys a charm, all the traffic between the controller and the resulting application unit agent(s) is [TLS](https://en.wikipedia.org/wiki/Transport_Layer_Security)-encrypted (each agent starts out with a CA certificate from the controller and, when they connect to the controller, they get another certificate that is then signed by the preshared CA certificate). In addition to that, every unit agent authenticates itself with the controller using a password.

> See more: [Wikipedia | TLS](https://en.wikipedia.org/wiki/Transport_Layer_Security)



<!--
```{caution}

On a MAAS cloud there is no MAAS-based firewall. In that case it is better to have your controller

```
-->

### Harden the user(s)

When you bootstrap a controller into a cloud, you automatically become a user with controller admin access. Make sure to change your password, and choose a strong password.

Also, when you create other users (whether human or for an application), take advantage of Juju's granular access levels to grant access to clouds, controllers, models, or application offers only as needed. Revoke or remove any users that are no longer needed.

> See more: {ref}`user`, {ref}`user-access-levels`, {ref}`manage-users`

### Harden the model(s)

Within a single controller, living on a particular cloud, you can have multiple users, each of which can have different models (i.e., workspaces or namespaces), each of which can be associated with a different credential for a different cloud. Juju thus supports multi-tenancy.

You can also restrict user access to a model and also restrict the commands that any user can perform on a given model.

> See more: {ref}`manage-models`

### Harden the applications

When you deploy (an) application(s) from a charm or a bundle, choose the charm / bundle carefully:

- Choose charms / bundles that show up in the Charmhub search – that means they’ve passed formal review – and which have frequent releases -- that means they're actively maintained.

- Choose charms that don’t require deployment with `--trust` (i.e., access to the cloud credentials). If not possible, make sure to audit those charms.

- Choose charms whose `charmcraft.yaml > containers > uid` and `gid` are not 0 (do not require root access). If not possible, make sure to audit those charms.

- *Starting with Juju 3.6:* Choose charms whose `charmcraft.yaml > charm-user` field set to `non-root`. If not possible, make sure to audit those charms.

- Choose charms that support secrets (see more:  {ref}`secret`).

(Like all the cloud resources provisioned through Juju,) the cloud resource(s) (machines or containers) that an application is deployed on by default run the latest Ubuntu LTS.  This Ubuntu is *not* CIS- and DISA-STIG-compliant (see more: [Ubuntu | The Ubuntu Security Guide](https://ubuntu.com/security/certifications/docs/usg)). However, it is by default behind a firewall, inside a VPC. Just make sure to expose application or application offer endpoints only as needed.

Keep an application's charm up to date.

> See more: {ref}`manage-charms`,  {ref}`manage-applications`

### Audit and observe

Juju generates agent logs that can help administrators perform auditing for troubleshooting, security maintenance, or compliance.

> See more: {ref}`log`

You can also easily collect metrics about or generally monitor and observe your deployment by deploying and integrating with the Canonical Observability Stack.

> See more: {ref}`collect-metrics-about-a-controller` (the same recipe -- integration with the [Canonical Observability Stack](https://charmhub.io/topics/canonical-observability-stack) bundle -- can be used to observe applications other than the controller)

(upgrade-your-deployment)=
## Upgrade your deployment

> See also: {ref}`juju-roadmap-and-releases`

This document shows how to upgrade your deployment -- the general logic and order, whether you upgrade in whole or in part, whether you are on Kubernetes or machines.

This typically involves upgrading Juju itself -- the client, the controller (i.e., all the agents in the controller model + the internal database), and the models (i.e., all the agents in the non-controller models). Additionally, for all the applications on your models, you may want to upgrade their charm.

None of these upgrades are systematically related (e.g., compatibility between Juju component versions is based on overlap in the supported facades, and compatibility between charms and Juju versions is charm-specific, so to know if a particular version combination is possible you'll need to consult the release notes for all these various parts).

> See more: {ref}`upgrading-things`, {ref}`juju-cross-version-compatibility`, {ref}`juju-roadmap-and-releases`, individual charm releases

However, in principle, you should always try to keep all the various pieces up to date, the main caveats being that the Juju components are more tightly coupled to one another than to charms and that, due to the way controller upgrades work, keeping your client, controller, and models aligned is quite different if you're upgrading your Juju patch version vs. minor or major version.

(upgrade-your-juju-components-patch-version)=
### Upgrade your Juju components' patch version
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
### Upgrade your Juju components' minor or major version
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

(troubleshoot-your-deployment)=
## Troubleshoot your deployment

From the point of view of the user, there are four basic failure scenarios:

1. Command that fails to return – things hang at some step (e.g., `bootstrap` or `deploy`) and eventually timeout with an error.
1. Command that returns an error.
1. Command that returns but, immediately after, `juju status` shows errors.
1. Things look fine but, at some later point, `juju status` shows errors.

In all cases you'll want to understand what's causing the error so you can figure out the way out:

- For (1)-(3) you can check the documentation for the specific procedure you were trying to perform right before the error -- you might find a troubleshooting box with the exact error message, what it means, and how you can solve the issue.

> See more:
>
> - The troubleshooting box at the end of {ref}`bootstrap-a-controller`
> - The troubleshooting box at the end of {ref}`migrate-a-model`
> - ...

- For (1)-(2) you can also retry the command with the global flags `--debug` and `--verbose` (best used together; for `bootstrap`, also use `--keep-broken` -- if a machine is provisioned, this will ensure that it is not destroyed upon bootstrap fail, which will enable you to examine the logs).
- For all of (1)-(4), you can examine the logs by
    - running `juju debug-log` (best used with `--tail`, because some errors are transient so the last lines tend to be the most relevant; also with  `–level=ERROR` and, if the point of failure is known, `–include ...` as well, to filter the output) or
    - examining the log files directly.

> See more: {ref}`command-juju-debug-log`, {ref}`log`, {ref}`manage-logs`

- For (3)-(4) the error might also be coming from a particular hook or action. In that case, use `juju debug-hooks` to launch a tmux session that will intercept matching hooks and/or actions. Then you can fix the error by manually configuring the workload, or editing the charm code. Once it is fixed you can run `juju resolved` to inform the charm that you have fixed the issue and it can continue.

> See more: {ref}`command-juju-debug-hooks`, {ref}`command-juju-resolved`

If none of this helps, use the information you've gathered to ask for help on our public [Charmhub Matrix chat](https://matrix.to/#/#charmhub:ubuntu.com) or our public [Charmhub Discourse forum](https://discourse.charmhub.io/t/welcome-to-the-charmed-operator-community).


(tear-down-your-deployment)=
## Tear down your deployment

(tear-things-down)=
### Tear down your deployment -- local testing and development

``````{tabs}
`````{group-tab} automatically
(tear-things-down-automatically)=

Delete the Multipass VM:

```text
$ multipass delete --purge my-juju-vm
```

Uninstall Multipass.

> See more: [Multipass | Uninstall Multipass](https://documentation.ubuntu.com/multipass/en/latest/how-to-guides/install-multipass/#uninstall)

`````

`````{group-tab} manually
(tear-things-down-manually)=

1. Tear down Juju:

```text
# Destroy any models you've created:
$ juju destroy-model my-model

# Destroy any controllers you've created:
$ juju destroy-controller my-controller

# Uninstall juju. For example:
$ sudo snap remove juju
```

2. Tear down your cloud. E.g., for a MicroK8s cloud:

```text
# Reset Microk8s:
$ sudo microk8s reset

# Uninstall Microk8s:
$ sudo snap remove microk8s

# Remove your user from the snap_microk8s group:
$ sudo gpasswd -d $USER snap_microk8s
```

3. If earlier you decided to use Multipass, delete the Multipass VM:

```text
multipass delete --purge charm-dev-vm
```

Then uninstall Multipass.

> See more: [Multipass | Uninstall Multipass](https://documentation.ubuntu.com/multipass/en/latest/how-to-guides/install-multipass/#uninstall)

`````

``````