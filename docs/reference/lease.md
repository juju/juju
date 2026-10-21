---
myst:
  html_meta:
    description: "Juju lease reference: the time-limited claims that decide which controller node runs the singular workers, which unit leads an application, and which node creates a backup, with the records that keep them and the operations that claim, revoke, and pin them."
---

(lease)=
# Lease

A **lease** is a time-limited claim on one named thing, which the holder keeps by renewing it before it expires: as long as the lease is held, no other holder can take the thing. The {ref}`controller database <database>` keeps the leases of every {ref}`model <model>`. Juju uses leases to decide three things that only one may do at a time: which controller node runs the workers that a model needs once (the `singular-controller` type), which {ref}`unit <unit>` is the {ref}`leader unit <leader-unit>` of an application (the `application-leadership` type), and which controller node creates a backup archive at a time (the `backup-creation` type).

(lease-database-schema)=
## Lease database schema

```{ggarch}
:file: ../juju.ggarch
:view: Lease database schema
:caption: The controller database keeps one lease record for each claimed thing: its type, the model it belongs to, its name, its holder, when the holder took it, and when it expires. A lease belongs to one lease type (a type is held by many leases), and a lease has many pins (a pin belongs to one lease). A pin names an entity that needs the lease not to expire.
:alt: Three boxes of the same size in a row inside a group named Controller database, each with a filled table mark in the corner. Lease type is on the left, Lease is in the middle (1 to m), and Lease pin is on the right (1 to m).
```

The lease database schema is the set of records that the controller database keeps about leases. The records, with the rules that apply to each, are:

- **Lease:** One claimed thing: its type, the model it belongs to, its name, its holder, the moment the holder took it, and the moment it expires.
  - *Rules:*
    - A model has at most one lease of a type and name.
    - A holder and a name are not empty, and hold no whitespace and none of the characters `.`, `#`, and `$`.
- **Lease type:** The kind of thing the lease claims: `singular-controller`, `application-leadership`, or `backup-creation`.
  - *Rules:*
    - A type name appears once.
- **Lease pin:** One entity that needs a lease not to expire, named for the lease. A lease with a pin does not expire while the pin is there.
  - *Rules:*
    - A lease has each pinned entity once.

A lease is a row in the table `lease`. Its type is a row in the lookup table `lease_type`. A pin is a row in `lease_pin`. {ref}`juju-db-repl` shows the definition of any of them with `.ddl <name>`.

(lease-operations)=
## Lease operations

Lease operations are the actions that claim, keep, and release leases. The callers are the workers of the controller and the {ref}`unit agents <unit-agent>` of a model: a unit agent claims the leadership lease of its application (see {ref}`requesting unit leadership <unit-claiming-leadership>`), a worker of the controller claims the singular lease of a model, and the backup creator claims the backup lease of the controller model.

(lease-claiming)=
### Claiming leases

Claiming leases asks to hold a lease for a holder for a duration, or to keep it. A claim on a lease that nobody holds is granted; a claim by the holder extends the lease; a claim on a lease that another holder holds is denied.

The operation's required access, rules, and errors are:

- *Required access:* A unit agent for its application's leadership lease; a worker of the controller for a singular or a backup lease. No model access level applies.
- *Rules:*
  - The claim names the lease, the holder, and a duration.
  - The duration must be positive.
  - A granted lease expires at the end of its duration, unless its holder claims it again before that, or the lease is pinned.
  - A leadership lease is named for its {ref}`application <application>` and held by a unit name; a singular lease is named for its model and held by the controller node that runs the workers the model needs once; a backup lease is named for the controller and held by a controller node.
- *Errors:* The caller receives each from the claim:
  - **`lease claim denied`**: *Trigger:* Another holder holds the lease. *Remediation:* Wait for the lease to expire or be released, then claim it again.

(lease-revoking)=
### Revoking leases

Revoking leases releases a lease before it expires, so that another holder can claim it. The removal job of a unit revokes the unit's claim on its application's leadership lease when it removes the unit. The backup creator revokes the backup lease when a backup creation call finishes.

The operation's required access, rules, and errors are:

- *Required access:* The holder of the lease, or the controller itself for a removal. No model access level applies.
- *Rules:*
  - A revoked lease is gone at once: the next claim on it is granted.
- *Errors:* The caller receives each from the revoke:
  - **`lease not held`**: *Trigger:* The named holder does not hold the lease. *Remediation:* Claim the lease first.

(lease-pinning)=
### Pinning leases

Pinning leases keeps a lease from expiring while an entity needs it held, and unpinning lets it expire again. A pinned lease stays with its holder until every pin is removed.

The operation's required access and rules are:

- *Required access:* The controller itself. No client or agent calls it in 4.1.
- *Rules:*
  - A pin names a lease and an entity. Several entities can pin the same lease, each with its own pin.
  - A lease with no pin expires as usual once its duration ends.

(lease-and-model-migration)=
## Leases and model migration

A migration carries the leadership of a model's applications to the target controller, so that the leader units stay the leaders after the move. It carries nothing else: the times of the other leases are stale by the time the import runs, the pins are left behind, and a singular lease is held by a controller node of the source controller, so all of them are claimed anew on the target. See {ref}`Migrating models <model-migrating>`.
