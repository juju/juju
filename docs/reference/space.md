---
myst:
  html_meta:
    description: "Juju network space reference: logical grouping of subnets for traffic segmentation, security, performance, and regulatory compliance."
---

(space)=
# Space

```{ibnote}
See also: {ref}`manage-spaces`
```

A Juju **(network) space** is a logical grouping of {ref}`subnets <subnet>` that can communicate with one another.

A space is used to help segment network traffic for the purpose of:
* Network performance
* Security
* Controlling the scope of regulatory compliance

## Spaces as constraints and bindings

Spaces can be specified as {ref}`constraints <constraint>` -- to determine what subnets a machine is connected to -- or as application endpoint bindings -- to determine the subnets used by application relations.

A binding associates an {ref}`application endpoint <application-endpoint>` with a space. This restricts traffic for the endpoint to the subnets in the space. By default, endpoints are bound to the space specified in the `default-space` model configuration value. The name of the default space is "alpha".

Constraints and bindings affect application deployment and machine provisioning as well as the subnets a machine can talk to.

Endpoint bindings can be specified during deployment with `juju deploy --bind` or changed after deployment using the `juju bind` command.

## Support for spaces in Juju providers

Support for spaces may vary from one cloud to another. For cloud-specific details, see the networking behavior section in each cloud's reference doc: {ref}`Amazon EC2 <cloud-ec2>`, {ref}`Microsoft Azure <cloud-azure>`, {ref}`OpenStack <cloud-openstack>`, {ref}`LXD <cloud-lxd>`, {ref}`MAAS <cloud-maas>`, {ref}`Unmanaged <cloud-unmanaged>`.

