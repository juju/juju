---
myst:
  html_meta:
    description: "Juju user reference: authentication, access levels, permissions, and user management for controllers, clouds, models, and offers. User declaration, persistence (the record, access levels, states, types), execution, and rules."
---

(user)=
# User
```{audience} user
```

In Juju, a **user** is any person able to log in to a Juju {ref}`controller <controller>`.

```{note}
Juju users are not related in any way to the client system users.
```

```{note}

A user's username and password are entirely different from the credentials referenced in `juju` commands such as `add-credential`---those are about access to a cloud, whereas these are about access to a Juju controller.

```

Users sit at the centre of Juju's access model: they log in to a {ref}`controller <controller>`, own {ref}`credentials <credential>` for clouds, and are granted access levels on {ref}`clouds <cloud>`, {ref}`models <model>`, and {ref}`application offers <offer>`; the {ref}`secrets <secret>` they create record them as the owner.

(the-users-declaration)=
## Users in the declaration layer

You add a user to a controller through a Juju client (`juju
add-user`), and you manage their login details the same way:
register the controller to set the first password, set or reset a
password, disable or re-enable authentication, and grant or revoke
access. A single Juju client can hold several users, but only one can
be logged in at a time.

A user logs in to a Juju controller with a username and a password.
The user created implicitly by the bootstrap gets the username `admin`
and is prompted to create a password on first login; a user created
explicitly gets the username assigned when they are added and sets
their login details when they register the new controller with their
Juju client.

```{ibnote}
See also: {ref}`Juju | Manage users <manage-users>`, {ref}`Terraform Provider for Juju | Manage users <tfjuju:manage-users>`
```

(the-users-persistence)=
(the-user-record)=
## Users in the persistence layer

In the controller database, a user is a record: its name (one active
user per name), its display name, whether it is an
{ref}`external <types-of-user>` identity, who created it, and whether
it has been removed. The authentication records hang beside it: the
salted password hash, the activation key a new user sets their
password with, and the disabled-authentication flag. The `admin` user
is seeded implicitly at bootstrap; every other user is added
explicitly.

The user service performs the writes: adding a user creates the
record and issues an **activation key** (a user who is given a
password directly skips the activation step); setting or resetting a
password writes a new hash (a reset issues a new activation key);
disabling authentication locks the user out without removing them;
re-enabling restores access; and a user from an identity provider is
imported in bulk or ensured on first use -- the record is created,
marked external, if it does not exist yet.

The user's *reach* is the **permission** table: one row per grant,
naming who is granted (the user) what access level on what kind of
object -- a {ref}`cloud <cloud>`, the {ref}`controller <controller>`,
a {ref}`model <model>`, or an {ref}`application
offer <offer>`. The permission service's writes create, update, or
delete those rows, and a grant must name one of the allowed
level-per-object combinations -- a lookup of its own: `login` and
`superuser` for controllers, `add-model` and `admin` for clouds,
`read`, `write` and `admin` for models, `read`, `consume` and `admin`
for offers.

(user-access-levels)=
### User access levels

A Juju user may have different abilities, according to the access level they have been granted. This document describes the various access levels and the corresponding abilities.

#### Valid access levels for controllers

(user-access-controller-login)=
##### `login`

Granted: Via `juju register.

Abilities: Log in to the controller.

(user-access-controller-superuser)=
##### `superuser`

Granted: Automatically by bootstrapping a controller or by having the username 'admin'.

Abilities: Do anything that it is possible to do at the level of a controller.

```{note}
A person logged into the `jaas` controller automatically has the login access level. This is automatically granted via ` juju grant login everyone@external`.
```

```{note}
Since multiple controllers—and therefore multiple controller administrators—are possible, there is no such thing as an overarching "Juju administrator". Nevertheless, a user with the superuser access level is usually what people refer to as "the admin".
```

#### Valid access levels for clouds

A controller can manage models on many clouds. With cloud-level access you can give a user permission to access one cloud but not another related to that controller.

(user-access-cloud-add-model)=
##### `add-model`

Granted: Via {ref}`command-juju-grant-cloud`.

Abilities: Add a model. Grant another user model-level permissions.

(user-access-cloud-admin)=
##### `admin`

Granted: Via {ref}`command-juju-grant-cloud`.

Abilities: You can do anything that it is possible to do at the level of a cloud.

#### Valid access levels for models

(user-access-model-read)=
##### `read`

Granted: Via {ref}`command-juju-grant`.

Abilities: View the content of a model without changing it. Use any of the read commands.

(user-access-model-write)=
##### `write`

Granted: Via {ref}`command-juju-grant`.

Abilities: Deploy and manage applications on the model.

(user-access-model-admin)=
##### `admin`

Granted: Via {ref}`command-juju-grant`.

Abilities: Do anything that it is possible to do at the level of a model.

#### Valid access levels for application offers

(user-access-offer-read)=
##### `read`

Granted: Via {ref}`command-juju-grant`.

Abilities: View offers during a search with {ref}`command-juju-find-offers`.

(user-access-offer-consume)=
##### `consume`

Granted: Via {ref}`command-juju-grant`.

Abilities: Relate an application to the offer.

(user-access-offer-admin)=
##### `admin`

Granted: Via {ref}`command-juju-grant`.

Abilities: You can do anything that it is possible to do at the level of an offer.

(the-user-states)=
### User states

A user's record carries two toggles rather than a life cycle: the
**authentication disabled** flag (the user exists but cannot log in)
and the **removed** flag (the name is retired; its records are kept
for audit and the name cannot be re-created while the removed row is
the active one). There is no alive/dying/dead for users -- disable
and remove are direct writes.

(types-of-user)=
### Types of user

The user record carries one discriminator: the **external** flag. A
**local user** is created in Juju and authenticates against Juju's
stored password records; an **external user** comes from an identity
provider outside Juju (its name carries the provider's qualifier --
`alice@external`), is imported or ensured on first use, and is the
same record shape with external authentication. The `admin` user
bootstrap creates is local, with the controller's `superuser` level;
a user created explicitly starts with the controller `login` level --
they can register the controller and log in, and nothing more, until
granted a higher level.

(the-users-execution)=
## Users in the execution layer

By the time the command returns, the user's record exists -- and no
login has happened and no permission has been checked yet. A user has
no machinery of their own: what acts on the records is the controller
itself -- the authentication at login and the permission check on
every request.

(the-user-watchers)=
### User watchers
```{audience} juju-dev
```

The user domain exposes no watch surfaces: user and permission
changes are read on demand (the client lists access when it needs it),
not watched.

(the-user-rules-and-errors)=
## User rules and errors
```{audience} charm-dev
```

The rules a **user identity** must satisfy:

- the user name must be a valid user name (lowercase, or the
  `name@qualifier` form for external identities);
- the last model admin cannot be disabled -- every model keeps an
  administrator.

The errors that encode them: `user not found`, `user name not
valid`, `user unauthorized`, `user authentication disabled`.
