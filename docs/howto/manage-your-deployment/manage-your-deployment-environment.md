(manage-your-deployment-environment)=
# Manage your deployment environment

Whether you are a charm user looking to try out some charms or a charm author looking to develop a charm, you will likely need / want all of the following:

- `juju`,
- a localhost Kubernetes / machine cloud, and possibly also
- further deploy or develop tools,

ideally in an isolated test environment and with the confidence that you know not just how to set things up but also tear things down.

This document shows two basic ways to achieve all of that, one automatic (using an Ubuntu VM pre-equipped with most/all you'll need) and one manual (where you have the option to go without a Multipass VM).

```{tip}
The automatic path should be faster and safer for most.
```

(set-things-up)=
## Set things up

Get an isolated test environment; install the `juju` CLI client; get a cloud, add the cloud to `juju` and bootstrap a controller into the cloud; add a model on the controller:


``````{tabs}
`````{tab} automatically

1. [Install Multipass](https://multipass.run/docs/install-multipass), then use it with the `charm-dev` blueprint to launch a Juju-ready Ubuntu VM (below, `my-juju-vm`), and launch a shell into the VM:


```{note}
If on Windows: Note that Multipass can only be installed on Windows 10 Pro or Enterprise. If you are using a different version, please follow the manual path, omitting the Multipass step.
```
```{note}
This step may take a few minutes to complete (e.g., 10 mins).

This is because the command downloads, installs, (updates,) and configures a number of packages, and the speed will be affected by network bandwidth (not just your own, but also that of the package sources).

However, once it’s done, you’ll have everything you’ll need – all in a nice isolated environment that you can clean up easily.

> See more: [GitHub > multipass-blueprints > charm-dev.yaml](https://github.com/canonical/multipass-blueprints/blob/ae90147b811a79eaf4508f4776390141e0195fe7/v1/charm-dev.yaml#L134)

**Troubleshooting:** If this fails, run `multipass delete --purge my-juju-vm` to clean up, then try the launch line again.

```

```text

# Install Multipass. E.g., on a Linux with snapd:
sudo snap install multipass

# Launch a Multipass VM from the charm-dev blueprint:
$ multipass launch --cpus 4 --memory 8G --disk 50G --name my-juju-vm charm-dev

# Open a shell into the VM:
$ multipass shell my-juju-vm
Welcome to Ubuntu 22.04.4 LTS (GNU/Linux 5.15.0-100-generic x86_64)
# ...

# Type any further commands after the VM shell prompt:
ubuntu@my-juju-vm:~$

```


```{tip}

At any point:
- To exit the shell, press {kbd}`mod` + {kbd}`C` (e.g., {kbd}`Ctrl`+{kbd}`C`) or type `exit`.
- To stop the VM after exiting the VM shell, run `multipass stop charm-dev-vm`.
- To restart the VM and re-open a shell into it, type `multipass shell charm-dev-vm`.


```

2. Verify that the VM already has everything that you'll need to deploy charms with Juju: a localhost cloud (`microk8s` - a MicroK8s-based Kubernetes cloud for Kubernetes charms; `localhost` -- a LXD-based machine cloud for machine charms); the cloud is already known to `juju`; `juju` already has a controller bootstrapped on the cloud`; and there is a workload model on that controller that you can go ahead and deploy things to:


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



3. (If you are developing a charm or planning to also use a different Juju client:)

3a. Ensure you have all the necessary tools.


````{dropdown} Example: Charming tools

```text
# Verify that you have Charmcraft:
ubuntu@my-juju-vm:~$ charmcraft

# Verify that you have a version of Python that meets the requirements for Ops:
ubuntu@my-juju-vm:~$ python3 --version

# Take stock of ay other pre-installed Python packages:
ubuntu@my-juju-vm:~$ pip list # should show, e.g., requests, tox, toml, virtualenv

# Install anything else that's missing, e.g., docker:
ubuntu@my-juju-vm:~$ sudo addgroup --system docker
ubuntu@my-juju-vm:~$ sudo adduser $USER docker
ubuntu@my-juju-vm:~$ newgrp docker
ubuntu@my-juju-vm:~$ sudo snap install docker

```

> See more: [Charmcraft docs](https://canonical-charmcraft.readthedocs-hosted.com/en/stable/), [Ops docs](https://ops.readthedocs.io/en/latest/)

````

3b. On your workstation, create a directory for your files, then mount it to your Ubuntu VM:

````{dropdown} Example: Create and mount a charm directory

```text
# Create the charm directory:
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
````

`````

`````{tab} manually


1. (Optional:) [Install Multipass](https://multipass.run/docs/install-multipass), then use it with the `charm-dev` blueprint to launch a Juju-ready Ubuntu VM (below, `my-juju-vm`), and launch a shell into the VM:


```{note}
If on Windows: Note that Multipass can only be installed on Windows 10 Pro or Enterprise. At the same time, if you're developing a charm, you will want Charmcraft, and Charmcraft can currently only be installed on a Linux with `snapd` or on macOS.
```

```text

# Install Multipass. E.g., on a Linux with snapd:
$ sudo snap install multipass

# Launch a Multipass VM with Ubuntu:
$ multipass launch --cpus 4 --memory 8G --disk 50G --name my-juju-vm

# Open a shell into the VM:
$ multipass shell my-juju-vm

```


```{tip}
At any point:
- To exit the shell, press {kbd}`mod` + {kbd}`C` (e.g., {kbd}`Ctrl`+{kbd}`C`) or type `exit`.
- To stop the VM after exiting the VM shell, run `multipass stop charm-dev-vm`.
- To restart the VM and re-open a shell into it, type `multipass shell charm-dev-vm`.

```

2. (Whether you are a charm developer or a charm user:) Prepare everything you'll need to deploy charms with Juju: Install `juju; set up a localhost cloud (`microk8s` - a MicroK8s-based Kubernetes cloud for Kubernetes charms; `localhost` -- a LXD-based machine cloud for machine charms); add the cloud to `juju`; bootstrap a controller into the cloud; add a workload model on that controller that you can then deploy things to:


2a. Install `juju`. For example, on a Linux with `snapd`:

```text
sudo snap install juju
```

> See more: {ref}`manage-juju`

2b. Set up your cloud, add it to `juju`, then bootstrap a controller into the cloud:


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


3. (If you are developing a charm or planning to also use a different Juju client:)

3a. Ensure you have all the necessary tools.


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

> See more: [Charmcraft docs](https://canonical-charmcraft.readthedocs-hosted.com/en/stable/), [Ops docs](https://ops.readthedocs.io/en/latest/)

````

3b. (If you're using Multipass:) On your workstation, create a directory for your files, then mount it to your Ubuntu VM:

````{dropdown} Example: Create and mount a charm directory

```text
# Create the charm directory:
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
````

`````

``````

(tear-things-down)=
## Tear things down

``````{tabs}
`````{tab} automatically

Delete the Multipass VM:

```text
$ multipass delete --purge my-juju-vm
```

Uninstall Multipass.

> See more: [Multipass | Uninstall Multipass](https://multipass.run/docs/install-multipass#uninstall)

`````

`````{tab} manually

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

> See more: [Multipass | Uninstall Multipass](https://multipass.run/docs/install-multipass#uninstall)

`````

``````



