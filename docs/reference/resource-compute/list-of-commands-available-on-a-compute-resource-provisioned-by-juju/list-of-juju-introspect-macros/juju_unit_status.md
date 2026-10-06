---
myst:
  html_meta:
    description: "Check status of unit agents running within a single Juju process using juju_unit_status introspect function."
---

(juju_unit_status)=
# `juju_unit_status`

The `juju_unit_status` introspection function shows the status of the agents running inside a machine agent process. On Juju deployed machines, the machine and unit agents run in that single process.  Example output:

```text
agent: machine-6
units:
  lxd/0: running
  neutron-openvswitch/0: running
  nova-compute/0: running
```
