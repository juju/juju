Juju charms
===========

This package parses juju charms.

## Anatomy of a charm

A bare minimum charm consists of a directory containing a series of files and
directories that inform Juju how to correctly handle and execute a charm.

The [charming docs](https://discourse.juju.is/c/docs/charming-docs) provide more
advanced information and tutorials.
The following is a very simple basic configuration guide.

### `metadata.yaml`

The `metadata.yaml` is a required charm configuration yaml. It is expected that
every charm contains a `metadata.yaml`.

An example of a very basic `metadata.yaml`:

```yaml
name: example
summary: Example charm
description: |
    A contrived descriptive example of a charm
    description that has multiple line.
tags:
    - example
    - misc
series:
    - focal
    - bionic
```

### `config.yaml`

`config.yaml` is an optional configuration yaml for a charm. The configuration
allows the author to express a series of options for the user to configure the 
modelled charm software.

An example of a `config.yaml`:

```yaml
options:
    name:
        default:
        description: The name of the example software
        type: string
```

It is expected that for every configuration option, there is a name and a type.
All the other fields are optional.

The type can be either a `string`, `int`, `float` or `boolean`. Everything else
will cause Juju to error out when reading the charm.

### `revision`

The `revision` is used to indicate the revision of a charm. It expects that only
an integer exists in that file.

### `lxd-profile.yaml`

The `lxd-profile.yaml` is an optional configuration yaml. It allows the author
of the charm to configure a series of LXD containers directly from the charm.

An example of a `lxd-profile.yaml`:

```yaml
config:
  security.nesting: "true"
  security.privileged: "true"
  linux.kernel_modules: openvswitch,nbd,ip_tables,ip6_tables
devices:
  kvm:
    path: /dev/kvm
    type: unix-char
  mem:
    path: /dev/mem
    type: unix-char
  tun:
    path: /dev/net/tun
    type: unix-char
```

### `version`

The `version` file is used to indicate the exact version of a charm. Useful for
identifying the exact revision of a charm.
