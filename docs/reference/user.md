---
myst:
  html_meta:
    description: "Juju user reference: authentication, access levels, permissions, and user management for controllers, clouds, models, and offers."
---

(user)=
# User

```{ibnote}
See also: {ref}`manage-users`
```

In Juju, a **user** is any person able to log in to a Juju {ref}`controller <controller>`.

```{note}
Juju users are not related in any way to the client system users.
```

Users can be created in two ways: Implicitly by bootstrapping a controller into a cloud or explicitly by adding a user to a controller (`juju add-user`).

A user logs in to a Juju controller using a username and a password. The user created implicitly gets the username `admin` and  is prompted to create a password the first time they attempt to log out. A user created explicitly gets the username assigned to them when being added (via `juju add-user`) and is prompted to create login details when they register the new controller with their Juju client.

```{note}
A user's username and password are entirely different from the credentials referenced in `juju` commands such as `add-credential` -- those are about access to a cloud, whereas these are about access to a Juju controller.
```

```{important}
Multiple users can be accommodated by the same Juju client. However, there can only be one user logged in at a time.
```

Every user is associated with an {ref}`access level <user-access-level>`.

(user-access-level)=
## User access level

In Juju, **user access** (or **access level**) is a property assigned to a user for a specific resource (controller, cloud, model, or offer) that determines what abilities the user has on that resource.

(list-of-user-access-levels)=
## List of user access levels

Access levels are defined per resource type. A user may have different access levels on different resources of the same type -- for example, `admin` access on one model but only `read` access on another model.

```{note}
Access levels follow the resource hierarchy: granting access at a higher-level resource automatically grants access to all lower-level resources within its scope. For example, a user with controller `superuser` access has full access to all clouds, models, and offers managed by that controller.
```

(list-of-user-access-levels-for-controllers)=
### List of user access levels for controllers

(user-access-controller-login)=
#### `login`

Granted: Via `juju register`.

Abilities:
- Log in to the controller.
- View your own user information.

Cannot:
- View models, clouds, or other controller resources.
- Perform any operations beyond authentication.

```{note}
This is the default access level for users created with `juju add-user`. Users must be explicitly granted additional access to clouds, models, or offers to perform useful work.
```

(user-access-controller-superuser)=
#### `superuser`

Granted: Automatically by bootstrapping a controller.

Abilities:
- Full access to all models, clouds, and offers managed by this controller (overrides all resource-level permissions).
- Add, remove, and manage users.
- Create and destroy models on any cloud.
- Enable controller high availability.
- Destroy the controller.
- Manage controller configuration.
- Grant and revoke access at all levels (controller, cloud, model, offer).

```{note}
The `admin` user created at controller bootstrap automatically has superuser access. This is the only way to receive superuser access -- it cannot be granted to other users.
```

```{note}
A person logged into the `jaas` controller automatically has the login access level. This is automatically granted via ` juju grant login everyone@external`.
```

```{note}
Since multiple controllers -- and therefore multiple controller administrators -- are possible, there is no such thing as an overarching 'Juju administrator'. Nevertheless, a user with the superuser access level is usually what people refer to as 'Juju admin'.
```

(list-of-user-access-levels-for-clouds)=
### List of user access levels for clouds

A controller can manage models on many clouds. With cloud-level access you can give a user permission to access one cloud but not another related to that controller.

(user-access-cloud-add-model)=
#### `add-model`

Granted: Via {ref}`command-juju-grant-cloud`.

Abilities:
- Create new models on this cloud (`juju add-model`).
- Automatic `admin` access to models you create.
- Grant model-level access to other users for models you create.

Cannot:
- View or manage models you didn't create (unless separately granted access).
- Modify cloud credentials or configuration.
- Manage the cloud itself.

(user-access-cloud-admin)=
#### `admin`

Granted: Via {ref}`command-juju-grant-cloud`.

Abilities:
- View and manage all models on this cloud (overrides model-level permissions).
- Manage cloud credentials and configuration.
- Grant and revoke cloud-level access.

(list-of-user-access-levels-for-models)=
### List of user access levels for models

(user-access-model-read)=
#### `read`

Granted: Via {ref}`command-juju-grant`.

Abilities:
- View model status and details (`juju status`, `juju show-model`).
- View application configurations and constraints.
- View relations between applications.
- List actions, operations, and view their status.
- View annotations, storage details, and network information.
- View secrets (metadata only).
- View model configuration.
- SSH access to units (for debugging).

Cannot:
- Deploy, configure, or remove applications.
- Execute actions or run commands.
- Modify any resource or configuration.
- Grant or revoke access to other users.

(user-access-model-write)=
#### `write`

Granted: Via {ref}`command-juju-grant`.

Abilities (includes all `read` operations, plus):
- Deploy and remove applications (`juju deploy`, `juju remove-application`).
- Update and configure applications (`juju refresh`, `juju config`).
- Scale applications (add/remove units).
- Create and manage relations (`juju integrate`, `juju remove-relation`).
- Execute actions on units (`juju run-action`).
- Manage storage (create, detach filesystems and volumes).
- Set annotations and constraints.
- Expose and unexpose applications.
- Upgrade the model to a new Juju version.

Cannot:
- Destroy the model.
- Execute commands directly on machines (`juju exec`).
- Grant or revoke model access to other users.
- Export model state or database dumps.

(user-access-model-admin)=
#### `admin`

Granted: Via {ref}`command-juju-grant`.

Abilities (includes all `write` operations, plus):
- Destroy the model (`juju destroy-model`).
- Execute commands directly on machines and units (`juju exec`).
- Grant and revoke model access to other users (`juju grant`, `juju revoke`).
- Export model state and database dumps.
- Manage model generations.

(list-of-user-access-levels-for-offers)=
### List of user access levels for application offers

(user-access-offer-read)=
#### `read`

Granted: Via {ref}`command-juju-grant`.

Abilities:
- View offers in searches (`juju find-offers`).
- View offer details and specifications.

Cannot:
- Consume (integrate with) the offer.
- Modify or remove the offer.
- Grant access to other users.

(user-access-offer-consume)=
#### `consume`

Granted: Via {ref}`command-juju-grant`.

Abilities (includes all `read` operations, plus):
- Consume the offer (integrate applications with it via `juju integrate`).
- View consumed applications in your model.
- Remove consumed applications from your model.

Cannot:
- Modify the offer itself.
- Destroy the offer.
- Grant or revoke offer access to other users.

(user-access-offer-admin)=
#### `admin`

Granted: Via {ref}`command-juju-grant`.

Abilities (includes all `consume` operations, plus):
- Create application offers (`juju offer`).
- Destroy offers (`juju remove-offer`).
- Grant and revoke offer access to other users (`juju grant`, `juju revoke`).

