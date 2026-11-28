(juju-cli)=
# `juju` CLI

```{ibnote}
See also: {ref}`manage-juju`
```

```{toctree}
:hidden:
juju-cli/list-of-juju-cli-commands/index
juju-cli/juju-environment-variables
juju-cli/plugin-juju-metadata
```

`juju` is the main CLI client of Juju that you can use to manage Juju {ref}`controllers <controller>`, whether as an administrator or as a regular user.

## Directory

The `juju` directory is located, on Ubuntu, at `~/.local/share/juju`.

Aside from things like a credentials YAML file, which you are presumably able to recreate, this directory contains unique files such as Juju's SSH keys, which are necessary to be able to connect to a Juju machine. This location may also be home to resources needed by charms or models.

```{note}

On Microsoft Windows, the directory is in a different place (usually `C:\Users\{username}\AppData\Roaming\Juju`).

```

(credential-stores-and-defaults)=
## Credential stores and defaults

The client stores named credentials and its per-cloud default in `credentials.yaml` in its data directory. The default applies only to that client. Selecting a different local default neither changes another workstation nor switches an existing model.

For credential commands that accept both destinations, `--client` targets the local store and `--controller NAME` targets that controller; the two flags together select both. Specify the destination for a non-interactive write. To compare independent copies, {ref}`inspect the credential in each store <view-details-about-a-credential>`.

The lookup order depends on the command. {ref}`Creating a model <command-juju-add-model>` and {ref}`switching a model's credential <command-juju-set-credential>` use different lookup rules.

## Backward compatibility

`juju` has been designed to be backward compatible and can talk to older or newer existing controllers if the controller and the client are on the same major version (e.g., 4.0 and 4.1). As such, performing simple commands can be achieved without upgrading the client. At the same time, it is always recommended to be up-to-date with the client and controller where possible.

## Working locally

In the case of the localhost cloud (LXD), the cloud is a local LXD daemon housed within the same system as the Juju client:

![machine](https://assets.ubuntu.com/v1/1f5ba83e-juju-client-3.png)

LXD itself can operate over the network and Juju does support this (`v.2.5.0`).

## Environment variables

You can also configure the Juju client using various environment variables. For more, see {ref}`juju-environment-variables`.

## Roadmap and releases

Each Juju release is accompanied by a set of release notes that highlight the changes and bug fixes for each release. For more, see  {ref}`releasenotes`.
