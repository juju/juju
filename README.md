<picture>
  <source media="(prefers-color-scheme: dark)" srcset="doc/juju-logo-dark.png?raw=true">
  <source media="(prefers-color-scheme: light)" srcset="doc/juju-logo.png?raw=true">
  <img alt="Juju logo next to the text Canonical Juju" src="doc/juju-logo.png?raw=true" width="30%">
</picture>

Juju is an open source application orchestration engine that enables any application operation (deployment, integration, lifecycle management) on any infrastructure (Kubernetes or otherwise) at any scale (development or production) in the same easy way (typically, one line of code), through special operators called ‘charms’.

[![juju](https://snapcraft.io/juju/badge.svg)](https://snapcraft.io/juju)
[![snap](https://github.com/juju/juju/actions/workflows/snap.yml/badge.svg)](https://github.com/juju/juju/actions/workflows/snap.yml)
[![build](https://github.com/juju/juju/actions/workflows/build.yml/badge.svg)](https://github.com/juju/juju/actions/workflows/build.yml)

||||
|-|-|- |
|:point_right: | [Juju](https://juju.is/docs/juju) | Learn how to quickly deploy, integrate, and manage charms on any cloud with Juju. <br>  _It's as simple as `juju deploy foo`, `juju integrate foo bar`, ..., on any cloud._ |
||||
|| [Charmhub](https://charmhub.io/) | Sample our existing charms on Charmhub. <br> _A charm can be a cluster ([OpenStack](https://charmhub.io/openstack-base), [Kubernetes](https://charmhub.io/charmed-kubernetes)), a data platform ([PostgreSQL](https://charmhub.io/postgresql-k8s), [MongoDB](https://charmhub.io/mongodb), etc.), an observability stack ([Canonical Observability Stack](https://charmhub.io/cos-lite)), an MLOps solution ([Kubeflow](https://charmhub.io/kubeflow)), and so much more._ |
||||
|| [Charm SDK](https://juju.is/docs/sdk) | Write your own charm! <br> _Juju is written in Go, but our SDK supports easy charm development in Python._  |


## Give it a try!

Let's use Juju to deploy, configure, and integrate some Kubernetes charms:


### Set up

You will need a cloud and Juju. The quickest way is to use a Multipass VM launched with the `charm-dev` blueprint. 

Install Multipass: [Linux](https://multipass.run/docs/installing-on-linux) | [macOS](https://multipass.run/docs/installing-on-macos) | [Windows](https://multipass.run/docs/installing-on-windows). On Linux:

```
sudo snap install multipass
```

Use Multipass to launch an Ubuntu VM with the `charm-dev` blueprint: 

```
multipass launch --cpus 4 --memory 8G --disk 30G --name tutorial-vm charm-dev 
```

Open a shell into the VM:

```
multipass shell tutorial-vm
```

Verify that you have Juju and two localhost clouds:

```
juju clouds
```

Bootstrap a Juju controller into the MicroK8s cloud:

```
juju bootstrap microk8s tutorial-controller
```

Add a workspace, or 'model':

```
juju add-model tutorial-model
```

### Deploy, configure, and integrate a few things

Deploy Mattermost:

```
juju deploy mattermost-k8s
```
> See more: [Charmhub | `mattermost-k8s`](https://charmhub.io/mattermost-k8s) 

Deploy PostgreSQL:

```
juju deploy postgresql-k8s --channel 14/stable --trust
```

> See more: [Charmhub | `postgresql-k8s`](https://charmhub.io/postgresql-k8s)

Enable security in your PostgreSQL deployment:

```
juju deploy tls-certificates-operator
juju config tls-certificates-operator generate-self-signed-certificates="true" ca-common-name="Test CA"
juju integrate postgresql-k8s tls-certificates-operator
```

Integrate Mattermost with PostgreSQL:

```
juju integrate mattermost-k8s postgresql-k8s:db
```

Watch your deployment come to life:

```
juju status --watch 1s
```

(Press `Ctrl-C` to quit. Drop the `--watch 1s` flag to get the status statically. Use the `--relations` flag to view more information about your integrations.)

### Test your deployment

When everything is in `active` or `idle` status, note the IP address and port of Mattermost and pass them to `curl`:

```
curl <IP address>:<port>/api/v4/system/ping
```

You should see the output below:

```
{"AndroidLatestVersion":"","AndroidMinVersion":"","IosLatestVersion":"","IosMinVersion":"","status":"OK"}
```
### Congratulations!

You now have a Kubernetes deployment consisting of a Mattermost backed by PosgreSQL with TLS-encrypted traffic!

### Clean up

Delete your Multipass VM:

```
multipass delete --purge tutorial-vm
```

Uninstall Multipass: [Linux](https://multipass.run/docs/installing-on-linux) | [macOS](https://multipass.run/docs/installing-on-macos) | [Windows](https://multipass.run/docs/installing-on-windows). On Linux:

```
snap remove multipass
```

## Next steps

### Learn more

- Read our [user docs](https://juju.is/docs/juju)
- Read our [developer docs](https://juju.is/docs/dev)

### Chat with us

Read our [Code of conduct](https://ubuntu.com/community/code-of-conduct) and:

- Join our chat: [Matrix](https://matrix.to/#/#charmhub-juju:ubuntu.com)
- Join our forum: [Discourse](https://discourse.charmhub.io/)


### File an issue

- Report a Juju bug on [Launchpad](https://bugs.launchpad.net/juju/+filebug)
- Raise a general https://juju.is/docs documentation issue on [Github | juju/docs ](https://github.com/juju/docs)

### Make your mark

- Read our [documentation contributor guidelines](https://discourse.charmhub.io/t/documentation-guidelines-for-contributors/1245) and help improve a doc 
- Read our [codebase contributor guidelines](doc/CONTRIBUTING.md) and help improve the codebase
