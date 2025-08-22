<picture>
  <source media="(prefers-color-scheme: dark)" srcset="docs/.sphinx/_static/logos/juju-logo-dark.png?raw=true">
  <source media="(prefers-color-scheme: light)" srcset="docs/.sphinx/_static/logos/juju-logo.png?raw=true">
  <img alt="Juju logo next to the text Canonical Juju" src="docs/.sphinx/_static/logos/juju-logo.png?raw=true" width="30%">
</picture>

Juju is an open source application orchestration engine that enables any application operation (deployment, integration, lifecycle management) on any infrastructure (Kubernetes or otherwise) at any scale (development or production) in the same easy way (typically, one line of code), through special operators called ‘charms’.

[![juju](https://snapcraft.io/juju/badge.svg)](https://snapcraft.io/juju)
[![snap](https://github.com/juju/juju/actions/workflows/snap.yml/badge.svg)](https://github.com/juju/juju/actions/workflows/snap.yml)
[![build](https://github.com/juju/juju/actions/workflows/build.yml/badge.svg)](https://github.com/juju/juju/actions/workflows/build.yml)


## Give it a try!

Let's use Juju to deploy, configure, and integrate some Kubernetes charms:


### Set up

You will need a cloud and Juju. The quickest way is to use a Multipass VM launched with the `charm-dev` blueprint.

[Install Multipass](https://canonical.com/multipass/docs/install-multipass). On Linux:

```
sudo snap install multipass
```

Use Multipass to launch an Ubuntu VM with the `charm-dev` blueprint:

```
multipass launch --cpus 4 --memory 8G --disk 50G --name tutorial-vm charm-dev
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
watch -n 1 -c juju status --color
```

Use the `--relations` flag to view more information about your integrations.
Use the `--storage` flag to view more information about your storages.

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

[Uninstall Multipass](https://canonical.com/multipass/docs/install-multipass). On Linux:

```
snap remove multipass
```

## Next steps

- Read the [docs](https://canonical-juju.readthedocs-hosted.com).
- Read our [Code of conduct](https://ubuntu.com/community/code-of-conduct) and join our [chat](https://matrix.to/#/#charmhub-juju:ubuntu.com) and [forum](https://discourse.charmhub.io/) or [open an issue](https://github.com/juju/juju/issues).
- Read our [CONTRIBUTING guide](./CONTRIBUTING.md) and contribute!
- Foobar
