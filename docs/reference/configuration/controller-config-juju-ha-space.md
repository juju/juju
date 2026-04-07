# `juju-ha-space`

This document describes the `juju-ha-space` controller configuration key.

|Key|Type|Default|Valid values|Purpose|
|---|---|---|---|---|
|`juju-ha-space`|string|null||The network space within which the MongoDB replica-set should communicate.|

The `juju-ha-space` key is used to apply a network space to the controller.

The space associated with `juju-ha-space` is used for MongoDB replica-set communication when controller high availability is in use. When enabling HA, this option must be set when cluster members have more than one IP address available for MongoDB use, otherwise an error will be reported. Existing HA replica sets with multiple available addresses will report a warning instead of an error provided the members and addresses remain unchanged.

Using this option with the `bootstrap` or `enable-ha` commands effectively adds constraints to machine provisioning. These commands will emit an error if such constraints cannot be satisfied.

