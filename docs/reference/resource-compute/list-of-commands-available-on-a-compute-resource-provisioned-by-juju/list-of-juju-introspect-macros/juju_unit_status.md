(juju_unit_status)=
# `juju_unit_status`

The `juju_unit_status` introspection function was introduced in 2.9.

In 2.9 the machine and unit agents were combined into a single process running on Juju deployed machines.  This tools allows you to see the status of agents running inside of that single process.  Example output:

```text
agent: machine-6
units:
  lxd/0: running
  neutron-openvswitch/0: running
  nova-compute/0: running
```
