# `juju-mgmt-space`


This document describes the `juju-mgmt-space` controller configuration key.

|Key|Type|Default|Valid values|Purpose|
|---|---|---|---|---|
|`juju-mgmt-space`|string|null||The network space that agents should use to communicate with controllers.|

The `juju-mgmt-space` key is used to apply a network space to the controller.

The space associated with `juju-mgmt-space` affects the communication between {ref}`Juju agents <agent>` and their controllers by limiting the IP addresses of controller API endpoints to those in the space. If the chosen space results in a lack of agent:controller communication, then a fallback default allows for any IP address to be contacted by the agent. Juju client communication with controllers is unaffected by this option.

Using this option with the `bootstrap` or `enable-ha` commands effectively adds constraints to machine provisioning. These commands will emit an error if such constraints cannot be satisfied.


<!--
From List of controller configuration keys:
<h3 id="heading--controller-related-spaces">Controller-related spaces</h3>

There are two network spaces that can be applied to controllers and this is done by assigning a space name to options `juju-mgmt-space` and `juju-ha-space`. See {ref}`Network spaces <space>` for background information on spaces.

The space associated with `juju-mgmt-space` affects the communication between {ref}`Juju agents` and their controllers by limiting the IP addresses of controller API endpoints to those in the space. If the chosen space results in a lack of agent:controller communication then a fallback default allows for any IP address to be contacted by the agent. Juju client communication with controllers is unaffected by this option.

The space associated with `juju-ha-space` is used for MongoDB replica-set communication when {ref}`Controller high availability` is in use. When enabling HA, this option must be set when cluster members have more than one IP address available for MongoDB use, otherwise an error will be reported. Existing HA replica sets with multiple available addresses will report a warning instead of an error provided the members and addresses remain unchanged.

Using these options with the `bootstrap` or `enable-ha` commands effectively adds constraints to machine provisioning. These commands will emit an error if such constraints cannot be satisfied.

-->
