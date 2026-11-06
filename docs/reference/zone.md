---
myst:
  html_meta:
    description: "Juju availability zone reference: what a zone is, the records the model keeps about it, how a constraint and a placement directive name it, and the call that lists the zones of a cloud."
---

(zone)=
# Zone

```{ibnote}
See also: {ref}`Juju | Add a machine <add-a-machine>`, {ref}`Juju | Manage subnets <manage-subnets>`
```

An **availability zone** is a named failure domain of a {ref}`cloud <cloud>`, such as `us-east-1a`, in which a {ref}`subnet <subnet>` is available. A user names zones to say where Juju provisions the {ref}`machines <machine>` that it requests, so that an outage in one zone does not take every machine down. Juju stores a zone when it adds a subnet that is available in it.

(zone-database-schema)=
## Zone database schema

```{ggarch}
:file: ../juju.ggarch
:view: Zone database schema
:caption: One link row per pair: a subnet is available in many zones and a zone holds many subnets, because neither owns the other. The zone record itself is only a name, the cloud's failure domain. The subnet is another page's record, shown for context.
:alt: Three boxes of the same size in a row inside a group named Model database, each with a filled table mark in the corner. Availability zone is on the left, Subnet's zone is in the middle (1 to m), and Subnet is on the right (m to 1), with an outlined arrow mark for a record that another page describes.
```

The zone database schema is the set of records that the {ref}`model database <database>` keeps about availability zones. The records, with the rules that apply to each, are:

- **Availability zone:** A zone's name.
  - *Rules:*
    - A zone name is unique.
- **Subnet's zone:** The link that says a subnet is available in a zone.
  - *Rules:*
    - A subnet is linked to a zone at most once.
- **Subnet:** The record at the far end of the link. Another page owns it: see {ref}`subnet <subnet>`.

(zone-calls)=
## Zone calls

Zone calls are the calls a {ref}`client <client>` can take on zones through the controller API (the set of calls that clients and {ref}`agents <agent>` make to the controller). The only one lists the zones of the cloud.

(zone-listing)=
### Listing zones

Listing zones returns the availability zones that the cloud reports and whether each is available.

The caller, the rules, and the errors are:

- *Caller:* A client, for a user with {ref}`read access <user-access-model-read>` to the model. No block applies.
- *Rules:*
  - The answer comes live from the cloud, not from the model database, so it can name zones that the model has no subnet in.
  - A cloud that cannot report zones gives an empty list: Kubernetes, Azure, and manual clouds report none.
  - A LXD server that is not clustered reports one zone, the server.
- *Errors:*
  - **`getting provider availability zones: <reason>`**: *Trigger:* The cloud refused or failed the request. The client receives it with no error code. *Remediation:* Fix the credential or the connection to the cloud.
  - **`permission denied`**: *Trigger:* The caller lacks read access. The client receives it with the code `unauthorized access`. *Remediation:* Ask a model administrator for read access to the model.

(zone-requesting)=
### Requesting zones

```{ibnote}
See also: {ref}`Juju | Add a machine <add-a-machine>`
```

Requesting zones tells Juju where to start the instance of a machine. A request names zones in either of two ways. The way decides how many zones it can name:

- The {ref}`zones <constraint-zones>` constraint names a list of zones, and Juju provisions the machine in one of them. The constraint can apply to a model, an application, or a machine.
- The {ref}`zone <placement-directive-zone>` placement directive, `zone=<name>`, names one zone, and Juju provisions the machine in it.

The rules are:

- *Rules:*
  - A placement directive overrides a constraint: on EC2, GCE, OpenStack, and MAAS, a request that carries both is provisioned in the zone of the directive.
  - Without either, Juju spreads the machines of an application across the zones that the cloud reports as available.
  - The zone must be one that the cloud reports and that is available, as in `us-east-1a` on EC2.
  - On EC2, a placement in a different zone from an existing volume that the machine attaches is refused, because the volume could not be attached.
  - Clouds read zones differently. On EC2, GCE, OpenStack, and MAAS the value is a zone of the cloud. On LXD it is the name of a cluster member, and on OCI an availability domain. Azure reports no zones and places machines in availability sets instead. {ref}`list of supported clouds <list-of-supported-clouds>` gives the details for each cloud.
- *Errors:* The client receives each with no error code:
  - **`invalid availability zone "<name>"`**: *Trigger:* On EC2, a placement directive that names a zone the cloud does not report. GCE gives the same message followed by `not found`. *Remediation:* {ref}`List the zones <zone-listing>` and use one of them.
  - **`availability zone "<name>" is "<state>"`**: *Trigger:* On EC2, GCE, and LXD, the zone exists but is not available. GCE prints the state without quotes. *Remediation:* Name another zone.
  - **`cannot create instance with placement "<placement>", as this will prevent attaching the requested EBS volumes in zone "<zone>"`**: *Trigger:* On EC2, a placement in a different zone from a volume that the machine attaches. *Remediation:* Place the machine in the zone of the volume.

(zone-migration)=
## Zones and model migration

A migration recreates the availability zones of the model's subnets, with their links to those subnets, on the target controller.
