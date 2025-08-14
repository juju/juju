
(set-things-up)=
# Set up your deployment - local testing and development

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
If on Windows: Note that Multipass can only be installed on Windows 10 Pro or Enterprise. If you are using a different version, please follow the manual path, omitting the Multipass step.
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
- To stop the VM after exiting the VM shell, run `multipass stop my-juju-vm`.
- To restart the VM and re-open a shell into it, type `multipass shell my-juju-vm`.

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
# Not necessary -- juju recognises a localhost MicroK8s cloud automatically, as you can see by running 'juju clouds'.
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
# Not necessary -- juju recognises a localhost LXD cloud automatically, as you can see by running 'juju clouds'.
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
$ multipass mount --type native ~/my-charm my-juju-vm:~/my-charm

# Verify that it's indeed on the VM:
ubuntu@my-juju-vm:~$ ls
my-charm  snap

# Going forward:
# - Use your host machine (on Linux, `cd ~/my-charm`) to create and edit your charm files. This will allow you to use your favorite local editor.
# - Use the Multipass VM shell (on Linux, `ubuntu@my-juju-vm:~$ cd ~/my-charm`) to run Charmcraft and Juju commands.

```

5. Continue as usual by setting up users, storage, etc.; adding models; and deploying, configuring, integrating, etc., applications.
