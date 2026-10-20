---
myst:
  html_meta:
    description: "Juju availability zone reference: what a zone is, the records the model keeps about it, how a constraint and a placement directive name it, and the operation that lists the zones of a cloud."
---

(zone)=
# Zone

```{ibnote}
See also: {ref}`Juju | Add a machine <add-a-machine>`, {ref}`Juju | Manage subnets <manage-subnets>`
```

An **availability zone** is a named failure domain of a {ref}`cloud <cloud>`, such as `us-east-1a`, in which a {ref}`subnet <subnet>` is available. A user names zones to say where Juju provisions the hardware that it requests, so that an outage in one zone does not take every {ref}`machine <machine>` down. Juju learns zones from the cloud together with subnets.

(zone-data-model)=
## Zone data model

```{ggarch}
:file: ../juju.ggarch
:view: Zone data model
:caption: The model database keeps one record for each availability zone, and one link record for each pair of a zone and a subnet that is available in it. A zone has many links and a subnet has many links, so a zone holds many subnets and a subnet is available in many zones. The subnet is described on its own page and the figure shows it only for context: all boxes are records, and the one with a second, outlined arrow mark in the corner is the record that another page describes.
:alt: Three boxes of the same size in a row inside a group named Model database, each with a filled table mark in the corner. Availability zone is on the left, Subnet's zone is in the middle (1 to m), and Subnet is on the right (m to 1), with an outlined arrow mark for a record that another page describes.
```

The zone data model is the set of records that the {ref}`model database <database>` keeps about availability zones. The records, with the rules that apply to each, are:

- **Availability zone:** A zone's name.
  - *Rules:*
    - A name appears once.
- **Subnet's zone:** The link that says a subnet is available in a zone.
  - *Rules:*
    - A subnet is linked to a zone once.
- **Subnet:** The record at the far end of the link. Another page owns it: see {ref}`subnet <subnet>`.

A zone is a row in the table `availability_zone`, and the link to a subnet is a row in `availability_zone_subnet`. {ref}`juju-db-repl` shows the definition of any of them with `.ddl <name>`. A machine's cloud instance records the zone it runs in, and that record belongs to the {ref}`machine <machine>` page.

(zone-operations)=
## Zone operations

Zone operations are the actions a {ref}`client <client>` can take on zones through the controller API (the set of calls that clients and {ref}`agents <agent>` make to the controller). The only one lists the zones of the cloud. A user chooses zones when requesting hardware, and the sections below describe both ways of naming one.

(zone-listing)=
### Listing zones

Listing zones returns the availability zones that the cloud reports and whether each is available.

The operation's required access, rules, and errors are:

- *Required access:* {ref}`Read access <user-access-model-read>` to the model. No block applies.
- *Rules:*
  - The answer comes live from the cloud, not from the model database, so it can name zones that the model has no subnet in.
  - A cloud that cannot report zones gives an empty list: Kubernetes, Azure, and manual clouds report none.
- *Errors:*
  - **`getting provider availability zones: <reason>`**: *Trigger:* The cloud refused or failed the request. The client receives it with no error code. *Remediation:* Fix the credential or the connection to the cloud.
  - **`permission denied`**: *Trigger:* The caller lacks read access. The client receives it with the code `unauthorized access`. *Remediation:* Ask a model administrator.

(zone-requesting)=
### Requesting zones

```{ibnote}
See also: {ref}`Juju | Add a machine <add-a-machine>`
```

Requesting zones tells Juju where to start the instance of a machine. A request names zones in either of two ways, and the way decides how many it can name:

- The {ref}`zones <constraint-zones>` constraint names a list of zones, and Juju provisions the machine in one of them. The constraint can apply to a model, an application, or a machine.
- The {ref}`zone <placement-directive-zone>` placement directive, `zone=<name>`, names one zone, and Juju provisions the machine in it.

The rules are:

- *Rules:*
  - A placement directive overrides a constraint: on EC2, GCE, OpenStack, and MAAS, the clouds whose code was read for this page, a request that carries both is provisioned in the zone of the directive.
  - Without either, Juju spreads the machines of an application across the zones that the cloud reports as available.
  - The zone must be one that the cloud reports and that is available, as in `us-east-1a` on EC2.
  - On EC2, a placement in a different zone from an existing volume that the machine attaches is refused, because the volume could not be attached.
  - Clouds read zones differently. On EC2, GCE, OpenStack, and MAAS the value is a zone of the cloud. On LXD it is the name of a cluster member, and on OCI an availability domain. Azure does not support the `zones` constraint: Juju spreads machines over availability sets instead. {ref}`list of supported clouds <list-of-supported-clouds>` gives the details for each cloud.
- *Errors:* The client receives each with no error code:
  - **`invalid availability zone "<name>"`**: *Trigger:* A placement directive that names a zone the cloud does not report. *Remediation:* {ref}`List the zones <zone-listing>` and use one of them.
  - **`availability zone "<name>" is "<state>"`**: *Trigger:* The zone exists but is not available. *Remediation:* Name another zone.
  - **`cannot create instance with placement "<placement>", as this will prevent attaching the requested EBS volumes in zone "<zone>"`**: *Trigger:* On EC2, a placement in a different zone from a volume that the machine attaches. *Remediation:* Place the machine in the zone of the volume.

## Zones and model migration

A migration copies the availability zones and their links to subnets with the model.
