# `juju-mgmt-space`

This document describes the `juju-mgmt-space` controller configuration key.

|Key|Type|Default|Valid values|Purpose|
|---|---|---|---|---|
|`juju-mgmt-space`|string|null||The network space that agents should use to communicate with controllers.|

The `juju-mgmt-space` key is used to apply a network space to the controller.

The space associated with `juju-mgmt-space` affects the communication between {ref}`Juju agents <agent>` and their controllers by limiting the IP addresses of controller API endpoints to those in the space. If the chosen space results in a lack of agent:controller communication, then a fallback default allows for any IP address to be contacted by the agent. Juju client communication with controllers is unaffected by this option.

Using this option with the `bootstrap` or `enable-ha` commands effectively adds constraints to machine provisioning. These commands will emit an error if such constraints cannot be satisfied.

