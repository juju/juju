# `juju-ha-space`

This document describes the `juju-ha-space` controller configuration key.

|Key|Type|Default|Valid values|Purpose|
|---|---|---|---|---|
|`juju-ha-space`|string|null||The network space within which the MongoDB replica-set should communicate.|

The `juju-ha-space` key is used to apply a network space to the controller.

The space associated with `juju-ha-space` is used for MongoDB replica-set communication when controller high availability is in use. When enabling HA, this option must be set when cluster members have more than one IP address available for MongoDB use, otherwise an error will be reported. Existing HA replica sets with multiple available addresses will report a warning instead of an error provided the members and addresses remain unchanged.

Using this option with the `bootstrap` or `enable-ha` commands effectively adds constraints to machine provisioning. These commands will emit an error if such constraints cannot be satisfied.


<!--
From List of controller configuration keys:
<h3 id="heading--controller-related-spaces">Controller-related spaces</h3>

There are two network spaces that can be applied to controllers and this is done by assigning a space name to options `juju-mgmt-space` and `juju-ha-space`. See [Network spaces](/t/network-spaces/1157) for background information on spaces.

The space associated with `juju-mgmt-space` affects the communication between [Juju agents](/t/concepts-and-terms/1144#heading--agent) and their controllers by limiting the IP addresses of controller API endpoints to those in the space. If the chosen space results in a lack of agent:controller communication then a fallback default allows for any IP address to be contacted by the agent. Juju client communication with controllers is unaffected by this option.

The space associated with `juju-ha-space` is used for MongoDB replica-set communication when [Controller high availability](/t/controller-high-availability/1110) is in use. When enabling HA, this option must be set when cluster members have more than one IP address available for MongoDB use, otherwise an error will be reported. Existing HA replica sets with multiple available addresses will report a warning instead of an error provided the members and addresses remain unchanged.

Using these options with the `bootstrap` or `enable-ha` commands effectively adds constraints to machine provisioning. These commands will emit an error if such constraints cannot be satisfied.

-->
