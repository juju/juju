# Implementing providers

This document describes how to implement an environment provider for Juju. For the remainder of this document we will
use the term "provider" to mean "environment provider", but be aware that there are additional types of providers (e.g.
storage providers).

## Overview

Providers are the bridge between Juju and the cloud environment in which Juju operates, and provides:

- allocation, deallocation, and querying of machines
- cloud-integrated network management

## Aspects of a provider

A provider is made up of several different parts:

- Configuration
- Creation (bootstrapping) and destruction of the environment
- Instance (machine) creation
- Instance querying
- Firewalling

These are encompassed by several interfaces in the Juju codebase:

- [environs.EnvironProvider](http://godoc.org/github.com/juju/juju/environs#EnvironProvider)
- [environs.Environ](http://godoc.org/github.com/juju/juju/environs#Environ)
- [instance.Instance](http://godoc.org/github.com/juju/juju/instance#Instance)

### Configuration

The first thing you should do when creating a new provider is identify the core configuration required, and then
implement the EnvironProvider interface. The EnvironProvider methods are described below.

Environment configuration is encapsulated
in [environs/config.Config](http://godoc.org/github.com/juju/juju/environs/config#Config), providing accessors for
common configuration attributes. Provider-specific configuration can be obtained through the `Config.UnknownAttrs`
method.

#### EnvironProvider.PrepareForBootstrap, EnvironProvider.PrepareForCreateEnvironment

These methods "prepare" an environment, which essentially means adding attributes to the provided configuration as
required. For most providers, PrepareForCreateEnvironment will be a no-op, simply returning the provided configuration
unmodified. Similarly, for most providers the PrepareForBootstrap method will be a call to PrepareForCreateEnvironment
followed by a call to Open.

#### EnvironProvider.Validate

Validate takes two configurations, and reports whether or not they are valid. There are two use-cases for this method:

- "Is this configuration valid in isolation?"
  In this case, the first argument will be a non-nil configuration, and the second will be nil.
- "Is this configuration valid, given the existing configuration?"
  In this case, the first argument will be the new configuration, and the second argument the old. You must check for
  invalid changes to configuration here.

It is also possible for the Validate method to return a *modified* configuration, which is often used to implement
configuration upgrades (i.e. upgrading configuration for older versions of a provider in a newer version). This exists
only because we do not have machinery dedicated to upgrading configuration, and should not be used lightly.

#### EnvironProvider.Open

Open returns an Environ for a given configuration.

#### EnvironProvider.SecretAttrs

SecretAttrs identifies which configuration attributes are secrets, which may only be shared with servers in the Juju
environment, and will be stripped from the environment configuration that is stored in the database. These are typically
only the credentials required to connect to the cloud provider.

#### EnvironProvider.RestrictedConfigAttributes

There is work ongoing to support multiple environments (i.e. separate sets of machines, services, units, etc.) within
the same cloud provider. RestrictedConfigAttributes identifies which attributes should not change between multiple
environments for the same Juju server, such as the cloud provider credentials.

#### EnvironProvider.BoilerplateConfig

BoilerplateConfig returns the provider-specific YAML boilerplate that is added to `environments.yaml` when running
`juju init`. The output should describe the possible configurations and default values.

### Bootstrapping and destruction of the environment

Before Juju can be useful, it needs to create the initial environment which contains a single Juju server, known as "
machine 0". The procedure by which this is created is known as "bootstrapping". Some of this is provider-independent,
and some is provider-specific; the latter is implemented via the `Environ.Bootstrap` method.

The bootstrap procedure is somewhat complicated, but many providers can simply defer to
the [provider/common.Bootstrap](http://godoc.org/github.com/juju/juju/provider/common#Bootstrap) function. Most of the
hard work is in creating machine instances.

Destroying an environment is just a matter of destroying any resources created in the cloud provider. You must make sure
that any resources you create can later be identified in order to be destroyed. The environment destruction procedure is
implemented via the `Environ.Destroy` method. There exists a common
function, [provider/common.Destroy](http://godoc.org/github.com/juju/juju/provider/common#Destroy), which providers may
use to destroy an environment. This will simply destroy each of the instances and storage in the environment; you must
handle handle any other resource cleanup separately.

### Instance creation and destruction

Easily the most involved part of a provider is dealing with how machine instances are created. There are many facets to
this, including:

- constraint validation and matching
- OS image selection
- tagging
- distribution over availability zones
- network configuration
- storage configuration

Machine instances are created via the `Environ.StartInstances` method, whose basic goal is to create a machine that:

- matches the requested constraints (e.g. have at least X amount of memory, Y number of CPU cores, Z amount of root disk
  space);
- runs an OS matching the specified "series" (i.e. an OS release, such as Ubuntu 14.04, or Windows Server 2012);
- can be identified later as being part of the environment;
- can be identified later as being a Juju server or not; and
- can communicate with other machines in the environment

Tagging is typically the preferred mechanism for identifying machines, and Juju provides a set of tags that should be
applied to machines when they are created, including tags to identify the environment that the machine is a part of (
using the environment's UUID); the Juju ID of the machine; and a special tag for Juju servers.

When a machine instance is created, StartInstance must report back the hardware characteristics to Juju. This enables
Juju to properly describe resources in the environment, both for reporting purposes, and also to enable assigning
services to machines based on their constraints, i.e. "assign this memory-hungry workload to a machine with a minimum of
X amount of memory".

One very important part of machine creation is customising the behaviour of the machine through "user data". Generally
we rely on [cloud-init](https://cloudinit.readthedocs.org/en/latest/) to configure machines; Juju provides a base
cloud-init configuration to StartInstance; StartInstance can make any additional modifications to it as necessary, and
then render it to the appropriate format and include it in the machine creation parameters to the cloud provider. How to
do this is very specific to cloud providers.

There are several additional, advanced features which are optional, which we will not describe here:

* placement (provider-specific directives on how/where to allocate machines, e.g. "in zone us-east-1a")
* distribution (distribution of machines across availability zones, for highly available services)
* networking (for managing multiple networks within the environment)
* storage (for requesting additional disks/volumes when allocating a machine)

Destroying instances is achieved via the `Environ.StopInstances` method. Note that while the name says "stop", it really
means *terminate*.

### Instance querying

A provider must be able to identify machine instances that it has created for an environment (`Environ.Instances` and
`Environ.AllInstances`), and it must be able to identify just the machines that are Juju servers (
`Environ.StateServerInstances`). In either case, the provider will yield provider-specific implementations of the
`instance.Instance` interface.

Implementations of `instance.Instance` are typically immutable snapshots with accessors for reporting the cloud-provider
specific ID (`Instance.Id`), the cloud-provider specific machine status (`Instance.Status`), and the machine's current
set of addresses (`Instance.Addresses`). In addition to these, `Instance` implementations must provide the `OpenPorts`,
`ClosePorts`, and `Ports` methods in order to support the "instance" firewall mode, which is described in the
Firewalling section below.

### Firewalling

Juju supports two modes of firewalling: global, and instance. Instance-level firewalling is preferable, as this provides
the greatest level of granularity; ports are opened and closed at a machine level. The "global" firewalling mode exists
for environments where instance-level firewalling is not possible, and applies to all machines in the environment.

To implement instance-level firewalling, the `Instance.OpenPorts`, `Instance.ClosePorts` and `Instance.Ports` methods
must be implemented. Otherwise, to support global firewalling, the `Environ` methods of the same names must be
implemented.