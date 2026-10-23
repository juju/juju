---
myst:
  html_meta:
    description: "Juju user reference: authentication, access levels, permissions, and user management for controllers, clouds, models, and offers. User declaration, persistence (the record, access levels, kinds), execution, and rules."
---

(user)=
# User

In Juju, a **user** is any person able to log in to a Juju {ref}`controller <controller>`.

```{note}
Juju users are not related in any way to the client system users.
```

```{note}

A user's username and password are entirely different from the {ref}`cloud credentials <credential>` a client manages; those are about access to a cloud, whereas these are about access to a Juju controller.

```

Users sit at the centre of Juju's access model: they log in to a {ref}`controller <controller>`, own {ref}`credentials <credential>` for clouds, and are granted access levels on {ref}`clouds <cloud>`, {ref}`models <model>`, and {ref}`application offers <offer>`; the {ref}`secrets <secret>` they create record them as the owner.

(the-user-declaration-rules)=
## The user in the declaration layer

How clients add users and manage what they may do.

- **Adding:** Adding a user requires controller {ref}`superuser access <user-access-controller-superuser>`.
  - *Rule:* The user name must be a valid user name: lowercase letters, or the `name@qualifier` form for external identities.
  - *Related errors:*
    - **`user already exists`**: *Trigger:* Creating a user whose name is already taken by an active user. *Remediation:* Choose an unused name.
    - **`username not valid`**: *Trigger:* A user name with illegal characters or length. *Remediation:* Use lowercase letters, or the `name@qualifier` form for external identities.
- **Login details:** Registering the controller sets a new user's first password; passwords are set or reset; authentication is disabled or re-enabled; access is granted or revoked.
  - *Related error:*
    - **`user authentication disabled`**: *Trigger:* Setting a password or an activation key, or granting access, for a user whose authentication is disabled. *Remediation:* Re-enable the user's authentication first.
- **Disabling and removing:** Disabling a user locks them out without removing them; removing a user retires the name.
  - *Rule:* The last model admin cannot be disabled or removed; every model keeps an administrator.
  - *Related error:*
    - **`user is the last model admin for 1 or more models`**: *Trigger:* Disabling or removing a user who is the last admin on one or more models. *Remediation:* Grant another user admin on those models first.
- **The bootstrap admin:** The user bootstrap creates gets the username `admin` and is prompted to create a password on first login; a user added explicitly sets their login details when they register the controller with their client.
- **Access grants:** A user's abilities come from the access levels they are granted on clouds, models, and offers (see {ref}`the user access levels <user-access-levels>` below).

```{ibnote}
See also: {ref}`Juju | Manage users <manage-users>`, {ref}`Terraform Provider for Juju | Manage users <tfjuju:manage-users>`
```

(user-access-levels)=
### User access levels

A Juju user may have different abilities, according to the access level they have been granted. This section is the definitional home of those levels; the grant itself (the {ref}`access <access>` record and the commands that grant and revoke it) is described in the access reference.

#### Valid access levels for controllers

(user-access-controller-login)=
##### `login`

Granted: Via `juju register`.

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

(the-users-persistence)=
(the-user-persistence-rules)=
## The user in the persistence layer

In the {ref}`controller database <database>`, a user is a record and its satellites:

- **User record:** The name (one active user per name), the display name, the external flag, the creator, and the removed flag. The `admin` user is seeded implicitly at bootstrap; every other user is added explicitly.
  - *Rule:* A user's record carries two toggles, the authentication-disabled flag and the removed flag. There is no alive, dying, dead life cycle for users: disable and remove are direct writes.
  - *Rule:* A removed user's name is retired: the records are kept for audit and the name cannot be re-created while the removed record is the active one.
  - *Related error:*
    - **`user not found`**: *Trigger:* A lookup of a user that does not exist. *Remediation:* Check the user name.
- **Authentication records:** The salted password hash, the activation key a new user sets their password with, and the disabled-authentication flag.
- **Permission records:** One record per grant, naming who is granted what access level on what kind of object: a {ref}`cloud <cloud>`, the {ref}`controller <controller>`, a {ref}`model <model>`, or an {ref}`application offer <offer>`. The allowed level-per-object combinations are their own lookup: `login` and `superuser` for controllers, `add-model` and `admin` for clouds, `read`, `write`, and `admin` for models, `read`, `consume`, and `admin` for offers. The {ref}`access <access>` reference describes the grant record and its errors.

**Writers:** The user service writes the record and its authentication records (adding issues an activation key; a password set or reset writes a new hash, a reset issuing a fresh key; disabling locks the user out without removing them; re-enabling restores access); the permission service creates, updates, and deletes the grant records.

(types-of-user)=
### Types of user

The user record carries one discriminator: the **external** flag.

- **Local user:** Created in Juju; authenticates against Juju's stored password records. The `admin` user bootstrap creates is local, with the controller `superuser` level.
- **External user:** Comes from an identity provider outside Juju; the name carries the provider's qualifier (`alice@external`); imported in bulk or ensured on first use; the same record shape with external authentication.
- **Starting levels:** A user added explicitly starts with the controller `login` level: they can register the controller and log in, and nothing more, until granted a higher level.

## The user in the execution layer

A user has no machinery of their own: what acts on the records is the controller itself, the authentication at login and the permission check on every request. By the time the setting call returns, the record exists; no login has happened and no permission has been checked yet.

- **Authentication at login:** The controller looks the user up by name and compares the supplied password with the stored hash.
  - *Related error:*
    - **`user unauthorized`**: *Trigger:* The supplied password does not match the stored one, or the user has no password set. *Remediation:* Supply the right password, or set one (register the controller or have it reset).

(the-user-watchers)=
### User watchers

The user domain exposes no watch surfaces: user and permission changes are read on demand (the client lists access when it needs it), not watched.
