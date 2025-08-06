(cloud-lxd)=
# The LXD cloud and Juju

<!--To see the older HTG-style doc, see version 39. Note that it may be out-of-date. -->

<!--
LXD is a hypervisor that provides system containers that are secure, lightweight, and easy to use. When your computer has LXD installed, Juju can operate the `localhost` cloud.
-->

This document describes details specific to using your existing LXD cloud with Juju.


````{dropdown} Expand to view how to get a LXD cloud quickly on Ubuntu


Your Ubuntu likely comes with LXD preinstalled. Configure it as below. Juju will then recognize it as the `localhost` cloud.

```text
lxd init --auto
lxc network set lxdbr0 ipv6.address none
```

````
> See more: [LXD](https://documentation.ubuntu.com/lxd/en/latest/)


```{dropdown} Expand to view some reasons to use a LXD cloud

The LXD cloud, especially when used locally, is great for: <p> - creating a repeatable deployment: Juju enables you to quickly iterate to construct the optimal deployment for your situation, then distribute that across your team <p> -- local development: Juju's localhost cloud can mirror the production ops environment (without incurring the costs involved with duplicating it) <p> - learning Juju: LXD is a lightweight tool for exploring Juju and how it operates <p> - rapid prototyping: LXD is great for when you're creating a new charm and want to be able to quickly provision capacity and tear it down

```

```{dropdown} Expand to find out why Docker wouldn't work

Juju expects to see an operating system-like environment, so a LXD system container fits the bill. Docker containers are laid out for a singular application process, with a self-contained filesystem rather than a base userspace image.

```

## Requirements

Juju `2.9.x`: LXD `5.0`<p> Juju `3.x.x`: LXD `5.x`

## Notes on `juju add-cloud`

Type in Juju: `lxd`

Name in Juju: `localhost`

## Notes on `juju add-credential`

**local LXD cloud:** If you are a Juju admin user: Already known to Juju. Run `juju bootstrap`, then `juju credentials` to confirm. (Pre-defined credential name in Juju: `localhost`.) Otherwise: Add manually as you would a remote. <p> **clustered LXD cloud**: In Juju, this counts as a remote cloud. You must add its definition to Juju explicitly. <p> **remote LXD cloud:** Requires the API endpoint URL for the remote LXD server.  <br> > See more: [LXD \| How to add remote servers](https://documentation.ubuntu.com/lxd/en/latest/remotes/)


### Authentication types

#### `certificate`
Attributes:
- server-cert: the path to the PEM-encoded LXD server certificate file (required)
- client-cert: the path to the PEM-encoded LXD client certificate file (required)
- client-key: the path to the PEM-encoded LXD client key file (required)

#### `interactive`
Attributes:
- trust-token: the LXD server trust token (optional, required if trust-password is not set).
<br>This is the recommended method for authenticating with a remote LXD server (see [LXD \| Adding client certificates using tokens](https://documentation.ubuntu.com/lxd/en/stable-5.0/authentication/#adding-client-certificates-using-tokens)).
<br>(Added in Juju 3.6.4)
- trust-password: the LXD server trust password (optional, required if trust-token is not set)

<!--
## Notes on `juju bootstrap`
-->

## Cloud-specific model configuration keys

### `project`
The LXD project name to use for Juju's resources.

| | |
|-|-|
| type | string |
| default value | "default" |
| immutable | false |
| mandatory | false |

## Supported constraints

```{note}
With LXD system containers, constraints are interpreted as resource *maximums* (as opposed to *minimums*). <p> There is a 1:1 correspondence between a Juju machine and a LXD container. Compare `juju machines` and `lxc list`.
```

| {ref}`CONSTRAINT <constraint>`         |                                                                                                                                                         |
|----------------------------------------|---------------------------------------------------------------------------------------------------------------------------------------------------------|
| conflicting:                           |                                                                                                                                                         |
| supported?                             |                                                                                                                                                         |
| - {ref}`constraint-allocate-public-ip` | &#10005;                                                                                                                                                |
| - {ref}`constraint-arch`               | &#10003;  <br> Valid values: `[host arch]`.                                                                                                             |
| - {ref}`constraint-container`          | &#10005;                                                                                                                                                |
| - {ref}`constraint-cores`              | &#10003;                                                                                                                                                |
| - {ref}`constraint-cpu-power`          | &#10005;                                                                                                                                                |
| - {ref}`constraint-image-id`           | &#10005;                                                                                                                                                |
| - {ref}`constraint-instance-role`      | &#10005;                                                                                                                                                |
| - {ref}`constraint-instance-type`      |                                                                                                                                                         |
| - {ref}`constraint-mem`                | The maximum amount of memory that a machine/container will have.                                                                                        |
| - {ref}`constraint-root-disk`          |                                                                                                                                                         |
| - {ref}`constraint-root-disk-source`   | &#10003;  <br> `root-disk-source` is the LXD storage pool for the root disk. The default LXD storage pool is used if root-disk-source is not specified. |
| - {ref}`constraint-spaces`             | &#10005;                                                                                                                                                |
| - {ref}`constraint-tags`               | &#10005;                                                                                                                                                |
| - {ref}`constraint-virt-type`          | &#10003;                                                                                                                                                |
| - {ref}`constraint-zones`              | &#10003;  <br> `zones` are the LXD node name(s).                                                                                                        |


## Supported placement directives

| {ref}`PLACEMENT DIRECTIVE <placement-directive>` |                                                                      |
|--------------------------------------------------|----------------------------------------------------------------------|
| - {ref}`placement-directive-machine`               | TBA                                                                  |
| - {ref}`placement-directive-subnet`                | &#10005;                                                             |
| - {ref}`placement-directive-system-id`             | &#10005;                                                             |
| - {ref}`placement-directive-zone`                  | &#10003;  <br> If there's no '=' delimiter, assume it's a node name. |




## Other notes

### Simple bootstrap of a remote LXD server

From Juju 2.9.5, the easiest method for bootstrapping a remote LXD server is to add the remote to your local LXC config then bootstrap with `juju`.

On the remote server:
```bash
# ensure the LXD daemon is listening on an accessible IP
lxc config set core.https_address '{ref}`::]'
# give the LXD daemon a trust password so the client can register credentials
lxc config set core.trust_password mytrustpassword
```

On the bootstrapping client:
```bash
# add the remote LXD server to the local LXC config
lxc remote add myremote 11.22.33.44 --password mytrustpassword
# bootstrap juju using the remote name in LXC
juju bootstrap myremote
```

```{note}
The bootstrapping client must be able to reach the remote LXD containers. This may require the setup of a bridge device with the hosts ethernet device.
```


### Non-admin user credentials


See {ref}`manage-credentials` for more details on how Juju credentials are used to share a bootstrapped controller.

To share a LXD server with other users on the same machine or remotely, the best method is to use LXC remotes. See "Simple bootstrap of a remote LXD server" above.

### Add resilience via LXD clustering


LXD clustering provides the ability for applications to be deployed in a high-availability manner. In a clustered LXD cloud, Juju will deploy units across its nodes. See more: [LXD | Clustering](https://documentation.ubuntu.com/lxd/stable-5.21/clustering/).

### Use LXD profiles from a charm


LXD Profiles allows the definition of a configuration that can be applied to any instance. Juju can apply those profiles during the creation or modification of a LXD container. See more: [Charmcraft | `lxd-profile.yaml](https://canonical-charmcraft.readthedocs-hosted.com/stable/reference/files/lxd-profile-yaml-file/).

### LXD images


LXD is image based: All LXD containers come from images and any LXD daemon instance (also called a "remote") can serve images. When LXD is installed a locally-running remote is provided (Unix domain socket) and the client is configured to talk to it (named 'local'). The client is also configured to talk to several other, non-local, ones (named 'ubuntu', 'ubuntu-daily', and 'images').

An image is identified by its fingerprint (SHA-256 hash), and can be tagged with multiple aliases.

For any image-related command, an image is specified by its alias or by its fingerprint. Both are shown in image lists. An image's *filename* is its *full* fingerprint, while an image *list* displays its *partial* fingerprint. Either type of fingerprint can be used to refer to images.

Juju pulls official cloud images from the 'ubuntu' remote (http://cloud-images.ubuntu.com) and creates the necessary alias. Any subsequent requests will be satisfied by the LXD cache (`/var/lib/lxd/images`).

Image cache expiration and image synchronization mechanisms are built-in.
