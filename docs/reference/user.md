---
myst:
  html_meta:
    description: "Juju user reference: authentication, access levels, permissions, and user management for controllers, clouds, models, and offers. The user record, user types, states, operations, and rules."
---

(user)=
# User
```{audience} user
```

```{ibnote}
See also: {ref}`manage-users`
```

In Juju, a **user** is any person able to log in to a Juju {ref}`controller <controller>`.

```{note}
Juju users are not related in any way to the client system users.
```

```{note}

A user's username and password are entirely different from the credentials referenced in `juju` commands such as `add-credential`---those are about access to a cloud, whereas these are about access to a Juju controller.

```

```{important}

Multiple users can be accommodated by the same Juju client. However, there can only be one user logged in at a time.

```

(the-users-records)=
## What Juju stores

(the-user-record)=
### The user's identity

In the controller database, a user is a record: its name (one active
user per name), its display name, whether it is an
{ref}`external <types-of-user>` identity, who created it, and whether
it has been removed. The authentication records hang beside it: the
salted password hash, the activation key a new user sets their
password with, and the disabled-authentication flag.

Users are created in two ways: implicitly by bootstrapping a controller into a cloud (the `admin` user) or explicitly by adding a user to a controller (`juju add-user`).

A user logs in to a Juju controller using a username and a password. The user created implicitly gets the username `admin` and  is prompted to create a password the first time they attempt to log out. A user created explicitly gets the username assigned to them when being added (via `juju add-user`) and is prompted to create login details when they register the new controller with their Juju client.

(the-user-in-the-data-model)=
### The user in the data model

The user's own records are the identity and authentication records;
the user's *reach* is the **permission** table: one row per grant,
naming who is granted (the user) what access level on what kind of
object -- a {ref}`cloud <cloud>`, the {ref}`controller <controller>`,
a {ref}`model <model>`, or an {ref}`application
offer <offer>`. The allowed level-per-object combinations are a
lookup of their own: `login` and `superuser` for controllers,
`add-model` and `admin` for clouds, `read`, `write` and `admin` for
models, `read`, `consume` and `admin` for offers.

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

(the-users-machinery)=
## What happens in the background

A user has no machinery of their own: what acts on the user's records
is the controller itself -- the permission checks on every request and
the authentication at login.

(the-user-operations)=
### How the user changes

#### Adding a user

Adding a user (`juju add-user`) creates the record and issues an
**activation key**: the new user completes registration by setting
their password with it (a user who is given a password directly skips
the activation step).

#### Passwords and authentication

Setting or resetting a password writes a new hash (a reset issues a
new activation key -- users cannot reset their own password
directly); disabling authentication locks the user out without
removing them; re-enabling restores access.

#### Granting access

Grants are the permission service's writes: create, update or delete a
permission row -- the user's access level on a cloud, the controller,
a model, or an offer (the access levels above).

#### Importing external users

Users from an identity provider are imported in bulk or ensured on
first use -- the record is created (marked external) if it does not
exist yet.

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
- one active user per name -- a removed name cannot be re-created
  while its removed record stands.

The rules a **user mutation** must satisfy:

- a password reset always goes through an activation key -- a user
  cannot reset their own password directly;
- the last model admin cannot be disabled -- every model keeps an
  administrator;
- grants must name an allowed level for the object kind (the matrix
  above).

The errors that encode them: `user not found`, `user name not
valid`, `user unauthorized`, `user authentication disabled`.

(related-entities-user)=
## Entities related to the user

- A user logs in to **the controller**; its database keeps the
  user and permission records (see {ref}`controller <controller>`).
- **Clouds, models and offers** are the objects access is granted on
  (see {ref}`cloud <cloud>`, {ref}`model <model>`,
  {ref}`offer <offer>`).
- **Credentials** are the *cloud* authentication material a user owns
  -- an entirely separate thing from the user's Juju login (see
  {ref}`credential <credential>`).
- **Secrets** a user creates are model-owned and managed by the user
  (see {ref}`secret <secret>`).
