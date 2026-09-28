---
myst:
  html_meta:
    description: "Juju access reference: how access is granted and revoked, the permission record it creates, and the rules that bound it. Access declaration, persistence, execution, and rules."
---

(access)=
# Access

In Juju, **access** is the authority a {ref}`user <user>` holds to act on
an object: one grant, naming who is given what
{ref}`access level <user-access-levels>` on what kind of object: a
{ref}`cloud <cloud>`, the {ref}`controller <controller>`, a
{ref}`model <model>`, or an {ref}`application offer <offer>`. The
levels and the abilities they confer are defined in the user
reference (see {ref}`the access levels <user-access-levels>`); this
page is the grant itself.

Its neighbours: the {ref}`users <user>` whose names the grants carry,
the four kinds of object the grants hang off, and the
{ref}`controller database <database>` the permission records live in.

(the-access-declaration)=
## Access in the declaration layer

You grant access with `juju grant` (for models, for application
offers, and with no model or offer named, for controller levels) and
with `juju grant-cloud` for cloud levels; you revoke it with
`juju revoke` the same way. Which verb is open to you depends on the
object: changing controller access requires
{ref}`controller superuser access <user-access-controller-superuser>`;
changing model access requires
{ref}`model admin access <user-access-model-admin>` on that model or
controller superuser access; changing offer access requires model
admin access on the model the offer belongs to (or controller
superuser access); changing cloud access requires
{ref}`cloud admin access <user-access-cloud-admin>` on that cloud or
controller superuser access.

```{ibnote}
See also: {ref}`Juju | Manage user access <manage-user-access>`,
{ref}`Terraform Provider for Juju | Manage user access
<tfjuju:manage-user-access>`
```

The Terraform Provider for Juju can manage access in place of the
commands: the `juju_access_model` and `juju_access_offer` resources
cover model and offer access; cloud and controller access are covered
by the provider's `jaas_access_cloud` and `jaas_access_controller`
resources, which apply when using JAAS.

(the-access-records)=
(the-access-record)=
## Access in the persistence layer

Access is persisted in the {ref}`controller database <database>` as
the **permission** record: one record per grant, carrying its UUID,
the access level and the object kind (stored as records of their own
vocabularies), the object's name or UUID, and the granted user's
UUID. The natural key is the pair of object and user: a unique index
allows exactly one permission per user per object.

Three vocabularies bound the record: the access levels are seven
(`read`, `write`, `consume`, `admin`, `login`, `add-model`,
`superuser`); the object kinds are four (`cloud`, `controller`,
`model`, `offer`); and a combination lookup holds the valid pairs,
ten of them: `admin` and `add-model` for clouds, `login` and `superuser` for
controllers, `read`, `write` and `admin` for models, `read`,
`consume` and `admin` for offers (the same list the
{ref}`user reference <user-access-levels>` describes as abilities).
Database views resolve each record's identifiers into their type names,
one view per object kind and one more for the special
`everyone@external` user, whose grants act like a group: any
{ref}`external user <types-of-user>` inherits a level from
`everyone@external` whenever the external user's own is lower.

The permission service performs the writes: a grant creates the record
(or updates it to a higher level), a revoke updates the record down or
deletes it (revoking the `read` level deletes the record outright),
and granting to an external user that Juju has not seen yet creates
that user as a side effect, so the record has someone to name.

From the user's side, this same record set is the user's *reach* (see
{ref}`the user's persistence layer <the-users-persistence>`).

(the-access-states)=
### Access states

A permission has no state machine: the record exists, or it does not:
a revoke that takes the level back to `read` is the same write as the
one that removes the grant.

(types-of-access)=
### Types of access

The permission record carries two stored discriminators: the **access
level** and the **object kind**. The levels and the abilities each
confers are defined in the user reference
({ref}`the access levels <user-access-levels>`); which combinations
are valid is the schema's own lookup, listed under
{ref}`the access rules <the-access-rules-and-errors>`.

(the-access-machinery)=
## Access in the execution layer

Access has no machinery of its own: a grant is a pure write to the
permission record set, and nothing runs because a level was conferred. The
enforcement happens elsewhere and on demand: every gated API call a
client makes carries a permission check against these records, and the
checks resolve the `everyone@external` inheritance each time they read an
external user's access: there is no cached copy to invalidate.

(the-access-operations)=
### Access operations

You read access where you read users: `juju show-user` reports a
user's access levels, and `juju users` lists the users of a model
with theirs. Both are plain reads of the permission records through
the user manager facade: nothing is enqueued and nothing waits.

(the-access-watchers)=
### Access watchers

The access domain exposes no watch surfaces: user and permission
changes are read on demand, not watched (the same statement the user
reference makes; see {ref}`user watchers <the-user-watchers>`).

(the-access-rules-and-errors)=
## Access rules and errors

The rules a grant must satisfy:

- a grant must name one of the valid level-per-object combinations,
  the schema's combination lookup; the client
  validates before sending and refuses a controller level paired with
  a model or offer (`You have specified a controller access
  permission ...`);
- a user holds at most one level per object: the unique index on
  the permission record;
- a grant must raise the level: a user already holding the level (or
  a greater one) cannot be granted it again (`access or greater`);
- a user cannot change their own cloud access (`cannot change your
  own cloud access`);
- the last model admin cannot be removed (`user is the last model
  admin for 1 or more models`);
- an external user's effective level is the greater of their own
  grants and `everyone@external`'s, enforced at every read, not
  stored.

These rules are stated where they bite: the combination lookup and
the uniqueness in the schema, the level-hierarchy and self-change
guards at the facades that write the permission record, and the
last-admin rule where users are removed. The errors that encode
them: `access or greater`, `cannot change your own cloud access`,
`permission not valid`, `permission not found`, `access not found`,
`user is the last model admin for 1 or more models`.