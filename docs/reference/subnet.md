---
myst:
  html_meta:
    description: "Juju subnet reference: IP address ranges in CIDR notation, grouped into network spaces for application networking."
---

(subnet)=
# Subnet

```{ibnote}
See also: {ref}`manage-subnets`
```

```{ggarch}
:file: ../juju.ggarch
:view: Network spaces
:no-legend:
:caption: A subnet is a CIDR range that belongs to 0..1 space; the application default binding and per-endpoint bindings point at spaces.
:alt: Application record to space record to subnet record.
```



A **subnet** is a range of IP addresses in CIDR notation.

Subnets can be grouped to form a {ref}`space <space>`. A subnet can only be in one space.
