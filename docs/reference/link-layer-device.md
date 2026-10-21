---
myst:
  html_meta:
    description: "Juju link-layer device reference: what a link-layer device is, the records the model keeps about network devices, IP addresses, DNS names, and the network node of a machine or unit, the changes that reach units, and the operations that list addresses, read a unit's addresses, set a machine's network configuration, provision container networking, and update addresses from the cloud."
---

(link-layer-device)=
# Link-layer device

```{ibnote}
See also: {ref}`Juju | Manage machines <manage-machines>`, {ref}`Terraform Provider for Juju | Manage machines <tfjuju:manage-machines>`
```

A **link-layer device** is a network interface — an Ethernet port, a bridge, a bond, or a loopback — that the {ref}`model database <database>` records for a machine, a unit, or a Kubernetes service, with the IP addresses on it. The devices, addresses, and DNS names of one entity hang from its **network node**. The {ref}`machine agent <machine-agent>` reports what it observes on its machine; a worker on the {ref}`controller <controller>` merges what the cloud reports.

(link-layer-device-database-schema)=
## Link-layer device database schema

```{ggarch}
:file: ../juju.ggarch
:view: Link-layer device database schema
:caption: The model database keeps one record for each network interface. A machine, a unit, and a Kubernetes service each name one network node, and a network node has many devices. A device has a type, a name, a MAC address, an MTU, a VLAN tag, and a parent (a device can be the parent of many devices). It has many DNS search domains, DNS servers, and routes. A device has many IP addresses; an address belongs to one device and one network node, and names at most one subnet. A provider device mark and a provider address mark each join one cloud ID to one device and one address. A network node has many FQDN links and hostname links; each link joins the node to one FQDN or one hostname, and several nodes can share one FQDN or one hostname. The machine, the unit, the Kubernetes service, and the subnet are described on their own pages and the figure shows them only for context.
:alt: Seventeen boxes of the same size inside a group named Model database, each with a filled table mark in the corner. Network node is in the middle. Machine is to its left, with Unit above it and Kubernetes service below it. FQDN link is above Network node with FQDN above it; Hostname link is below Network node with Hostname below it. Link-layer device is to the right of Network node, with DNS search domain above it and DNS server above that, and Provider device mark below it with Parent and then Route below. IP address is to the right of Link-layer device with Provider address mark below it, and Subnet is to the right of IP address.
```

The link-layer device database schema is the set of records that the {ref}`model database <database>` keeps about the network interfaces of a machine, a unit, or a Kubernetes service, the IP addresses on them, and the DNS names reported for them. The records, with the rules that apply to each, are:

- **Network node:** The record that a machine, a unit, or a Kubernetes service hangs its devices, addresses, and DNS names from. A unit names one network node: on a machine model it is the network node of the machine the unit runs on, so the units of one machine share a node, and on Kubernetes the unit has a node of its own.
  - *Rules:*
    - A machine names one network node, so a network node belongs to at most one machine.
    - A Kubernetes service names one network node, so a network node belongs to at most one Kubernetes service.
- **Link-layer device:** One network interface of a network node: its name, its MTU (the largest packet it passes), its MAC address, its type, its virtual port type, whether it starts on boot, whether it is up, whether traffic is routed out of it by default, the address of its default gateway, and its VLAN tag (the identifier of the virtual LAN it carries; 0 for a normal network, 1 to 4094 for a VLAN).
  - *Rules:*
    - A network node has each device name once.
    - The MAC address can be empty: a placeholder device has none.
- **Device type:** The kind of interface. The types are `unknown`, `loopback`, `ethernet`, `802.1q`, `bond`, `bridge`, `vxlan`, and `veth`.
- **Virtual port type:** The virtual switch port implementation of the device. The types are the empty string, for none, and `openvswitch`.
- **Parent:** The parent of a device, named as another device.
  - *Rules:*
    - A device appears once as a child.
- **DNS search domain:** One DNS search domain of a device.
  - *Rules:*
    - A device lists each search domain once.
- **DNS server:** One DNS server of a device.
  - *Rules:*
    - A device lists each DNS server once.
- **Route:** One route of a device: its destination CIDR (the block of addresses the route covers), its gateway IP, and its metric. No operation stores a route in 4.1.
  - *Rules:*
    - A device has each destination once.
- **IP address:** One address of a device: the address with its subnet mask, its type, the way it is configured, its origin, its scope, and whether it is a secondary address (not the primary address of its device) or a shadow address (one the cloud assigned rather than observed on a device).
  - *Rules:*
    - An address belongs to one device and to one network node.
    - The type is `ipv4` or `ipv6`.
    - The configuration type is one of `unknown`, `dhcp`, `dhcpv6`, `slaac`, `static`, `manual`, and `loopback`.
    - The origin is `machine` or `provider`: the machine reported the address, or the cloud did.
    - The scope is one of `unknown`, `public`, `local-cloud`, `local-machine`, and `link-local`.
    - An address can name no subnet.
- **Provider device mark:** The cloud's ID of a device, joined to one device.
  - *Rules:*
    - The cloud ID is not empty.
- **Provider address mark:** The cloud's ID of an address, joined to one address.
  - *Rules:*
    - The cloud ID is not empty.
- **FQDN:** One fully qualified domain name reported for a network node, with the scope in which it is reachable.
  - *Rules:*
    - The scope is `local-cloud` or `public`, never `local-host`.
    - A name with one scope appears once: the same name with another scope is another record.
- **FQDN link:** The link from a network node to one of its FQDNs.
  - *Rules:*
    - A network node lists a FQDN once.
    - Several network nodes can report the same name and scope, so they share one FQDN.
- **Hostname:** One hostname reported for a network node, with the scope in which it is reachable.
  - *Rules:*
    - The scope is `local-host`, `local-cloud`, or `public`.
    - A hostname appears once.
- **Hostname link:** The link from a network node to one of its hostnames.
  - *Rules:*
    - A network node lists a hostname record once.
    - Several network nodes can report the same hostname, so they share one record.
- **Machine, unit, Kubernetes service, and subnet:** The records at the far ends of a network node and of an address. Other pages own them: see {ref}`machine <machine>`, {ref}`unit <unit>`, {ref}`application <application>` for the Kubernetes service, and {ref}`subnet <subnet>`.

(link-layer-device-change-streams)=
## Link-layer device change streams

Link-layer device change streams are the notifications that tell the {ref}`unit agents <unit-agent>` of a model that the addresses of their unit or the endpoint bindings of its application changed. A change stream is delivered through a watcher (an API object that tells its holder when something it follows has changed). A unit agent calls `WatchUnitAddressesHash` to hear that the addresses of its unit have a new hash. The hash covers the addresses on the network node of the unit, or, on Kubernetes, on the network node of its application's cloud service, together with the endpoint bindings of the application (the space each of its endpoints is bound to). When the hash changes, the agent runs the {ref}`config-changed <hook-config-changed>` hook.

(link-layer-device-operations)=
## Link-layer device operations

Link-layer device operations are the actions on the network records of a machine, a unit, or a Kubernetes service: listing devices and addresses, reading the addresses of a unit, setting the network configuration of a machine, provisioning container networking, and updating addresses from the cloud. A {ref}`client <client>` lists devices and addresses through the controller API (the set of calls that clients and {ref}`agents <agent>` make to the controller). A **{ref}`machine agent <machine-agent>`** is the process that runs on a machine. It logs in as its machine and acts only for that machine. A **{ref}`unit agent <unit-agent>`** is the process that runs a unit's charm. It logs in as its unit and acts only for that unit. No model access level applies to an agent. No block applies to these operations.

(link-layer-device-listing)=
### Listing network devices and addresses

```{ibnote}
See also: {ref}`Juju | View the status of a machine <view-the-status-of-a-machine>`, {ref}`Juju | View details about a machine <view-details-about-a-machine>`, {ref}`Juju | Access a machine via SSH <access-a-machine-via-ssh>`
```

Listing network devices and addresses returns the devices of each machine of a model with the addresses on them, as part of the model's full status, and returns the address to reach a machine or a unit over SSH.

The operation's required access, rules, and errors are:

- *Required access:* For the full status, {ref}`read access <user-access-model-read>` to the model, or {ref}`superuser access <user-access-controller-superuser>` to the controller. For the SSH addresses, {ref}`admin access <user-access-model-admin>` to the model, or superuser access to the controller.
- *Rules:*
  - The full status lists, for each machine, each device by name, with its MAC address, its gateway, its DNS servers, its spaces, whether it is up, and the addresses on it.
  - Addresses configured as loopback are left out. A device whose addresses are all loopback is left out as well.
  - A machine whose instance is not yet recorded lists no devices.
  - The full status also lists each machine's own addresses with their scopes. Its DNS name is the first of them.
  - The SSH addresses of a machine are the addresses on its devices. A unit names the machine it runs on, whose addresses are then used.
  - The public address is the address whose scope ranks first: `public` ranks first, then `local-cloud`, then `unknown`. `local-machine` and `link-local` never rank. Within one rank, IPv4 ranks before IPv6.
  - The private address ranks `local-cloud` first, then `public` and `unknown`. Within one rank, IPv4 ranks before IPv6.
  - A request can ask for every address. The answer holds each address once, sorted with public addresses first.
- *Errors:* The client receives each in the result for the request:
  - **`permission denied`**: *Trigger:* The caller lacks the access above. The client receives it with the code `unauthorized access`. *Remediation:* Ask a model administrator.
  - **`no public address(es)`** and **`no private address(es)`**: *Trigger:* No address of the machine ranks for the requested kind. The client receives them with no error code. *Remediation:* Wait for the machine agent to report the machine's network configuration, or reach the machine another way.
  - **`unit %q not found`** and **`machine %q: machine not found`**: *Trigger:* A named unit, or a named machine, does not exist. The client receives the unit error with the code `not found`; the machine error carries no code. *Remediation:* List the machines and use an existing one.

(link-layer-device-reading)=
### Reading the addresses of a unit

Reading the addresses of a unit returns the public and the private address of a unit to its agent. The addresses of a unit are the addresses on its network node; on Kubernetes they also include the addresses on the network node of its application's cloud service.

The operation's required access, rules, and errors are:

- *Required access:* A unit agent, for its own unit.
- *Rules:*
  - The public address is the first address whose scope is `public`. Failing that, `local-cloud` ranks next, then `unknown`.
  - The private address is the first address whose scope is `local-cloud`. Failing that, `public` ranks next, then `unknown`.
  - Within one rank, IPv4 ranks before IPv6.
- *Errors:* The unit agent receives each in the result for the unit:
  - **`"<unit>" has no public address set`** and **`"<unit>" has no private address set`**: *Trigger:* No address of the unit ranks for the requested kind. The agent receives them with the code `no address set`. *Remediation:* Wait for the machine agent, or for the cloud, to report the unit's addresses.
  - **`unit %q not found`**: *Trigger:* The unit does not exist. The agent receives it with the code `not found`. *Remediation:* Use the unit that the agent runs.
  - **`permission denied`**: *Trigger:* The caller asks about a unit it does not run. The agent receives it with the code `unauthorized access`. *Remediation:* Use the unit that the agent runs.

(link-layer-device-setting)=
### Setting the network configuration of a machine

Setting the network configuration of a machine stores the interfaces that a machine agent observes on its machine, with the addresses on them and the DNS settings of each device. The machiner worker reports whenever the addresses of its machine change. The worker that creates containers on a host machine reports the host's configuration before it creates one.

The operation's required access, rules, and errors are:

- *Required access:* A machine agent, for its own machine.
- *Rules:*
  - A report with no interfaces changes nothing.
  - The devices and addresses arrive with the origin `machine`.
  - A device that the report carries for the first time is added. A device that it carries again is updated.
  - A device that the report no longer carries is deleted when it has no addresses and no provider device mark. A device that is the parent of a device that survives is kept.
  - Each device names its parent by name. A parent that appears in neither the report nor the stored devices is dropped with a warning.
  - The DNS search domains and the DNS servers of each observed device are replaced as a whole.
  - An address keeps the subnet it already names. Otherwise the subnet whose CIDR contains the address is used, when exactly one CIDR matches. When several subnets share the CIDR, a new /32 subnet for IPv4 or /128 subnet for IPv6 is created in the alpha {ref}`space <space>`, and the instance poller reconciles it later.
  - When no subnet matches, a machine that was added manually gets the subnet of the address added; otherwise the address names no subnet.
  - An address that the report no longer carries, with the origin `machine`, is deleted.
- *Errors:* The client receives each in the answer of the call:
  - **`permission denied`**: *Trigger:* The caller is not the machine agent of the machine. A report about a machine that does not exist fails the same way. The client receives it with the code `unauthorized access`. *Remediation:* Use the machine that the agent runs.
  - **`unsupported device type: "<type>"`**, **`unsupported virtual port type: "<type>"`**, **`unsupported address type: "<type>"`**, **`unsupported address config type: "<type>"`**, **`unsupported address origin: "<type>"`**, and **`unsupported address scope: "<type>"`**: *Trigger:* The report names a type or a scope that the model database does not know. The client receives them with no error code. *Remediation:* Report the interfaces that the machine observes.
  - **`invalid IP address "<address>"`**: *Trigger:* An address is not an IP address in CIDR notation. The client receives it with no error code. *Remediation:* Write each address with its subnet mask.

(link-layer-device-provisioning)=
### Provisioning container networking

Provisioning container networking reads the devices of a host machine to plan the bridges that the host needs for containers and the network configuration that a container starts with. The machine agent of a host machine calls it for each container it creates.

The operation's required access, rules, and errors are:

- *Required access:* A machine agent, for its own machine, which is the host of the containers named in the request.
- *Rules:*
  - The request names containers. The answer has one result for each.
  - The devices of the host machine are grouped by the {ref}`space <space>` of their addresses. The positive space constraints of a container and the spaces that its application is bound to pick the devices to bridge.
  - A space whose host devices already include a bridge, or an Open vSwitch port, needs no new bridge.
  - A device can be bridged when it is not a loopback and it has no parent.
  - With local container networking, no host device is bridged: the default LXD bridge carries the container, every device is configured for DHCP, and at most one device is created with the default bridge as its parent.
  - When the cloud handles container networking and can allocate addresses, the cloud allocates a static address for each device of the container.
- *Errors:* The client receives each in the result for the container:
  - **`machine %q not found`**: *Trigger:* A named container matches no machine. The client receives it with the code `not found`. *Remediation:* List the machines and use an existing one.
  - **`host machine %q is not provisioned`**: *Trigger:* The cloud should allocate addresses, but the host machine has no instance ID. The client receives it with the code `not provisioned`. *Remediation:* Wait for the host machine to finish provisioning.
  - **`host %q has no available device in space(s) %v`**: *Trigger:* A space that the container needs has no host device left to bridge. The client receives it with no error code. *Remediation:* Add a device to that space, or change the container's space constraints.
  - **`permission denied`**: *Trigger:* The caller is not the machine agent of the host machine. The client receives it with the code `unauthorized access`. *Remediation:* Use the machine that the agent runs.

(link-layer-device-updating)=
### Updating a machine's addresses from the cloud

Updating a machine's addresses from the cloud merges what the cloud reports about the interfaces of a machine into the records of that machine. The instance poller, a worker on the controller, runs it for each machine that has an instance ID.

The operation's required access and rules are:

- *Required access:* No client calls this operation. The instance poller runs it for every machine that reports an instance ID.
- *Rules:*
  - The cloud reports devices and addresses. They are merged into the devices and addresses of the machine's network node.
  - A cloud can know a device by its hardware address only. Such a device takes the name of the stored device with the same hardware address.
  - A new address from the cloud is added with the origin `provider`. The cloud's IDs of the device and of the address are stored as provider marks.
  - An address that the cloud no longer reports keeps its record with the origin `machine`.
  - The subnet of an address is set again from the cloud's subnet ID when the address falls inside that subnet's CIDR.
  - A /32 or /128 subnet that no address and no provider uses any more is deleted.
- *Errors:* None reaches a client. The worker logs the failure and polls again.

## Link-layer devices and model migration

A {ref}`migration <model-migrating>` moves the network nodes of a model with their devices, addresses, DNS settings, provider marks, FQDNs, and hostnames. A cloud service that the model keeps for an application becomes a network node with a placeholder device, so its addresses hang from that node. An address that arrives without a subnet mask keeps the mask of its subnet when the model has one, and otherwise the mask of its type: /32 for IPv4, /128 for IPv6.