(juju-cli)=
# `juju` CLI

> See also: {ref}`manage-juju`


```{toctree}
:hidden:
juju-cli/list-of-juju-cli-commands/index
juju-cli/juju-environment-variables
```

<!--HARRY SAYS: THIS DOC IS MISSING A LOT OF DETAIL-->


<!--The Juju CLI is the client for bootstrapping Juju controllers, creating Juju models, deploying applications and managing these entities.-->

`juju` is the main CLI client of Juju that you can use to manage Juju {ref}`controllers <controller>`, whether as an administrator or as a regular user.

<!--This software connects to Juju controllers and is used to issue commands that deploy and manage application units running on cloud instances.-->

<!-- Commented out because it uses "cloud" as the collection of resources provided by what we would call a cloud.
![machine](https://assets.ubuntu.com/v1/865acefc-juju-client-2.png)
-->

## Directory

The `juju` directory is located, on Ubuntu, at `~/.local/share/juju`.

Aside from things like a credentials YAML file, which you are presumably able to recreate, this directory contains unique files such as Juju's SSH keys, which are necessary to be able to connect to a Juju machine. This location may also be home to resources needed by charms or models.

```{note}

On Microsoft Windows, the directory is in a different place (usually `C:\Users\{username}\AppData\Roaming\Juju`).

```

## Backward compatibility

`juju` has been designed to be backward compatible and can talk to older or newer existing controllers if the controller and the client are on the same major version (2.x and 3.x). As such, performing simple commands can be achieved without upgrading the client. At the same time, it is always recommended to be up-to-date with the client and controller where possible.



## Working locally

<!-- should cover LXD as well as MicroK8s-->

In the case of the localhost cloud (LXD), the cloud is a local LXD daemon housed within the same system as the Juju client:

![machine](https://assets.ubuntu.com/v1/1f5ba83e-juju-client-3.png)

LXD itself can operate over the network and Juju does support this (`v.2.5.0`).


## Environment variables

You can also configure the Juju client using various environment variables. For more, see {ref}`juju-environment-variables`.


## Plugins

The Juju client can be extended with plugins. For more, see {ref}`plugin`, {ref}`list-of-known-juju-plugins`.

## Roadmap and releases

Each Juju release is accompanied by a set of release notes that highlight the changes and bug fixes for each release. For more, see  {ref}`juju-roadmap-and-releases`.
