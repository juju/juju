---
myst:
  html_meta:
    description: "How Juju identifies users, grants access to individual resources, and creates, activates, disables, and removes controller accounts."
---

(user)=
# User

```{ibnote}
See also: {ref}`Juju CLI | Manage users <manage-users>`; {ref}`Terraform Provider for Juju | Manage users <tfjuju:manage-users>`
```

```{ggarch}
:file: ../juju.ggarch
:view: user-account
:caption: A user belongs to a controller. The account saved on a client identifies which user it signs in as on that controller.
:alt: A client contains a saved account that refers to a user inside a controller.
```

In Juju, a **user** is an identity a {ref}`controller <controller>` recognizes. The account saved on your {ref}`client <cli-accounts-by-controller>` identifies which user you sign in as on that controller. Your {ref}`access on each resource <user-access-levels>` determines what you may do there. {ref}`Signing out <sign-out-as-a-user>` leaves the controller user in place; {ref}`removing the user <removing-a-user>` is permanent.

(user-names)=
## User names

A username identifies one user on a particular controller. Juju gives the user a unique identifier of its own; an optional display name, such as `Alex Morgan`, is for people reading the user directory. Use the username for lookups and grants.

A local username has no domain. Juju recognises `alex@local` as `alex`. An external username includes its identity domain, as in `alex@example.com`. The username and any domain must each contain at least two characters and start and end with an ASCII letter or digit. Inside either part, Juju also accepts `.`, `+` and `-`.

(types-of-users)=
## Types of users

- **Local user**: The controller authenticates this user with Juju sign-in details. {ref}`Bootstrapping a controller <bootstrap-a-controller>` creates the local user `admin` and grants it {ref}`controller superuser access <user-access-controller-superuser>` on that controller. Bootstrap also creates `juju-metrics` with controller login access. An administrator can {ref}`add other local users <adding-a-local-user>`.
- **External user**: An identity provider authenticates this user. When the controller accepts an external user's first sign-in, it records that identity if it doesn't already have a record. An administrator can also establish the record by {ref}`granting access to a named external user <users-eligible-for-an-access-grant>`. The provider manages the user's sign-in details.

A user's sign-in details identify the user to Juju. A {ref}`cloud credential <credential>` serves a different purpose: Juju uses it to act on a cloud on behalf of a model.

(user-access-levels)=
## User access levels

```{ibnote}
See also: {ref}`Juju CLI | Manage user access <manage-user-access>`; [Terraform Provider for Juju | Manage user access](https://canonical.com/juju/docs/terraform-provider-juju/latest/howto/manage-users/#manage-user-access)
```

```{ggarch}
:file: ../juju.ggarch
:view: user-grants
:caption: A user's access level applies to the resource named in the grant. The same user can administer one model and read another.
:alt: One user has two separate grants, admin on one model and read on another.
```

A **grant** assigns an access level to one user on one target: a controller, a cloud on that controller, a model or an application offer. Each target has its own levels.

(users-eligible-for-an-access-grant)=
### Users eligible for an access grant

A local user must already exist on the controller. For a controller, cloud or model, a successful grant to a new external username creates a Juju user record for that name. The identity provider manages the external identity. {ref}`Offer grants <changing-access-to-an-offer>` require an enabled user record on the offering controller.

(shared-access-for-external-users)=
### Shared access for external users

`everyone@external` is a special recipient for grants to external users. When Juju checks grants stored on the controller, it uses the higher of the external user's individual level and the level assigned to `everyone@external` **on the same target**. The shared level can apply before the individual user has a record. Local users don't inherit it, and a disabled or removed external user doesn't regain access through it.

For example, suppose an enabled external user has a direct `write` grant on a model and `everyone@external` has `read` on that model. The user's effective level is `write`. If the administrator removes the user's direct grant, the user still has `read`. Changing the shared grant affects other external users too.

(changes-to-a-users-access-grant)=
### Changes to a user's access grant

Granting a higher level raises the direct grant. Granting a level the user already holds or exceeds leaves the grant unchanged.

Before revoking, check the user's direct grant and choose the level they hold or a lower one. Revocation leaves the level immediately below the one you revoke. Revoking the lowest level removes the direct grant. Check the {ref}`model administrator requirement <keeping-an-active-model-administrator>` and the {ref}`effect on offer relations <changing-access-to-an-offer>` before changing those grants.

(valid-access-levels-for-controllers)=
### Valid access levels for controllers

The {ref}`controller access levels <controller-access-levels>` govern access to one controller, including sign-in to its controller and model endpoints.

(user-access-controller-login)=
#### `login`

A user with `login` may sign in to the controller and read the port and public host key for its SSH jump server. That information alone does not authorize a {ref}`target session through the jump server <access-a-machine-via-ssh>`. The user must be a {ref}`model admin <user-access-model-admin>` on the destination model or a {ref}`controller superuser <user-access-controller-superuser>`, and present a key registered for that user on that model.

(user-access-controller-superuser)=
#### `superuser`

A controller superuser can administer this controller, including {ref}`adding users <adding-a-local-user>` and {ref}`changing controller access <changing-controller-access>`.

(valid-access-levels-for-clouds)=
### Valid access levels for clouds

The {ref}`cloud access levels <cloud-access-levels>` apply to the named cloud as registered on this controller.

(user-access-cloud-add-model)=
#### `add-model`

The user may {ref}`create a model on this cloud <add-a-model>`. The grant supplies permission, not a {ref}`cloud credential <credential>` or authority to grant access on an existing model.

(user-access-cloud-admin)=
#### `admin`

The user may {ref}`change other users' access to this cloud <changing-cloud-access>`.

(valid-access-levels-for-models)=
### Valid access levels for models

The {ref}`model access levels <model-access-levels>` apply to the named model.

(user-access-model-read)=
#### `read`

The user may {ref}`view the model's status <view-the-status-of-a-model>` and {ref}`list visible users of the model <listing-users-with-model-access>`.

(user-access-model-write)=
#### `write`

The user may change workloads in this model, for example by {ref}`adding units to an application <command-juju-add-unit>`.

(user-access-model-admin)=
#### `admin`

The user may {ref}`change other users' access to this model <changing-model-access>`.

(valid-access-levels-for-application-offers)=
### Valid access levels for application offers

The {ref}`offer access levels <offer-access-levels>` apply to one offer in the model that publishes it.

(user-access-offer-read)=
#### `read`

The user may {ref}`find the offer <find-an-offer-to-use>` and view its details.

(user-access-offer-consume)=
#### `consume`

The user may obtain the offering controller's details to {ref}`consume the offer <integrate-with-an-offer>`. Adding the offered application to the consuming model also requires at least {ref}`model write access <user-access-model-write>` on that model.

(user-access-offer-admin)=
#### `admin`

The user may {ref}`change other users' access to this offer <changing-access-to-an-offer>`.

(adding-a-local-user)=
## Adding a local user

```{ibnote}
See also: {ref}`Juju CLI | Add a user <add-a-user>`; [Terraform Provider for Juju | Add a user](https://canonical.com/juju/docs/terraform-provider-juju/latest/howto/manage-users/#add-a-user)
```

Adding a local user creates a controller account with {ref}`controller login access <user-access-controller-login>`. The administrator may supply a password, or leave it to the new user: in that case the controller returns an activation key for an invitation. Neither form grants access to a model.

**Who can do this:** A {ref}`controller superuser <user-access-controller-superuser>`.

**Errors:** Adding a local user can fail with these errors:

- *Error message:* ends with `user already exists`. *Trigger:* An active or disabled user has that name. *What to do:* Look up the existing user before choosing whether to enable it, reset its password or use a different name.
- *Error message:* ends with `creating user: user name "<name>" not valid`. *Trigger:* The name doesn't meet the controller's {ref}`username requirements <user-names>`. *What to do:* Choose a valid local username.
- *Error message:* ends with `cannot add external user "<name>": username not valid`. *Trigger:* The administrator supplied an external name when adding a local user. *What to do:* Use the external identity provider for sign-in and grant that identity access to the intended resource.

(activating-a-local-user)=
## Activating a local user

```{ibnote}
See also: {ref}`Juju CLI | Activate a local user <activate-a-local-user>`
```

```{ggarch}
:file: ../juju.ggarch
:view: user-registration
:caption: A local user invitation lets its recipient choose a password. The administrator passes the invitation privately; the controller accepts the password before the recipient's client saves the account.
:alt: An administrator hands an invitation to the user. The user chooses a password, the controller consumes the activation key, and the user's client saves the account.
```

Activation lets the invited user set a password on the controller. The invitation contains a one-use activation key. The controller consumes that key when it accepts the recipient's password, even if saving the account on the client fails later.

**Who can do this:** The recipient of a valid invitation for that local user.

**Errors:** Activation can fail with these errors from the controller:

- *Error message:* `activation key not found`. *Trigger:* No current key exists for this user. *What to do:* If the controller already accepted your password, sign in with it. Otherwise, ask a controller superuser to reset the password and issue a fresh invitation.
- *Error message:* `activation key not valid`. *Trigger:* The activation data doesn't match the current key. *What to do:* Use the latest invitation for the intended controller and user.
- *Error message:* ends with `user authentication disabled`. *Trigger:* The account is disabled. *What to do:* Ask a controller superuser whether the account should be re-enabled before retrying.

(listing-users-on-a-controller)=
## Listing users on a controller

```{ibnote}
See also: {ref}`Juju CLI | List users on a controller <list-users-on-a-controller>`
```

A {ref}`controller superuser <user-access-controller-superuser>` receives the directory of enabled users and can include disabled users on request. Other callers see their own record. See {ref}`listing users with model access <listing-users-with-model-access>` for model grants.

**Who can do this:** Any signed-in user. The caller's controller access determines which records the controller returns.

(looking-up-a-controller-user)=
## Looking up a controller user

```{ibnote}
See also: {ref}`Juju CLI | View details about a user <view-details-about-a-user>`
```

Looking up a known username returns its record even if the user is disabled. The controller reports the username, display name, creator, creation time, disabled status and controller access. A disabled record has no current controller access. The last-connection time, when available, is the latest recorded connection to a model on this controller; a model-specific read reports the time for that model.

**Who can do this:** A signed-in user for their own record; a {ref}`controller superuser <user-access-controller-superuser>` for another user.

**Errors:** Looking up a controller user can fail with these errors:

- *Error message:* `permission denied`. *Trigger:* The caller asked for another user's record without controller superuser access. *What to do:* Look up your own name or ask a controller superuser to look up the other user.
- *Error message:* `user "<name>" not found`. *Trigger:* An authorized lookup found no current user by that name on this controller. *What to do:* Check the spelling, domain and controller. A removed user no longer has a visible record.

(changing-a-local-users-password)=
## Changing a local user's password

```{ibnote}
See also: {ref}`Juju CLI | Change a local user's password <change-a-local-users-password>`; [Terraform Provider for Juju | Change a user's password](https://canonical.com/juju/docs/terraform-provider-juju/latest/howto/manage-users/#manager-a-user-s-login-details)
```

Changing a password sets the chosen password for an enabled local user. The new password must contain a non-whitespace character and be at most 1,024 bytes. Setting it invalidates any outstanding activation key for that user.

**Who can do this:** The user changing their own password, or a {ref}`controller superuser <user-access-controller-superuser>` setting one for another local user.

**Errors:** Changing a local user's password can fail with these errors:

- *Error message:* `cannot use an empty password`. *Trigger:* The supplied password is empty or consists only of whitespace. *What to do:* Supply a password with at least one non-whitespace character.
- *Error message:* ends with `password not valid, size must be less then 1024 bytes`. *Trigger:* The password exceeds 1,024 bytes. *What to do:* Use at most 1,024 bytes.
- *Error message:* ends with `user authentication disabled`. *Trigger:* The target user is disabled. *What to do:* Ask a controller superuser to re-enable the user before setting its password.

(resetting-a-local-users-password)=
## Resetting a local user's password

```{ibnote}
See also: {ref}`Juju CLI | Reset another local user's password <reset-another-local-users-password>`
```

Resetting a local user's password discards the current password and replaces any outstanding activation key. The controller returns a new key so the administrator can invite the user to choose a password again.

**Who can do this:** A {ref}`controller superuser <user-access-controller-superuser>` acting for a different, enabled local user.

**Errors:** Resetting a local user's password can fail with these errors:

- *Error message:* `permission denied`. *Trigger:* The caller isn't a controller superuser or is trying to reset their own account. *What to do:* Change your own password, or ask a different controller superuser to reset it.
- *Error message:* ends with `user authentication disabled`. *Trigger:* The target user is disabled. *What to do:* Decide whether to re-enable the account before issuing a new invitation.

(disabling-and-re-enabling-a-user)=
## Disabling and re-enabling a user

```{ibnote}
See also: {ref}`Juju CLI | Disable a user <disable-a-user>`; {ref}`Juju CLI | Re-enable a user <re-enable-a-user>`
```

Disabling a user stops new sign-ins to this controller and preserves the account and its grants. Re-enabling the user allows sign-in again with the grants that remain. Disabling a user is subject to the {ref}`model administrator requirement <keeping-an-active-model-administrator>`.

**Who can do this:** A {ref}`controller superuser <user-access-controller-superuser>`.

**Errors:** Changing a user's enabled status can fail with these errors:

- *Error message:* ends with `model(s) would be left without an admin user`. *Trigger:* Disabling the user would leave a model without an {ref}`enabled model administrator <keeping-an-active-model-administrator>`. *What to do:* Grant another enabled user model `admin` on each affected model before retrying.
- *Error message:* ends with `user not found`. *Trigger:* The user does not exist on this controller or was removed. *What to do:* Verify the controller and username. A removed account cannot be re-enabled.

(removing-a-user)=
## Removing a user

```{ibnote}
See also: {ref}`Juju CLI | Remove a user <remove-a-user>`; [Terraform Provider for Juju | Remove a user](https://canonical.com/juju/docs/terraform-provider-juju/latest/howto/manage-users/#remove-a-user)
```

Removal permanently retires the user's identity on this controller. The controller clears the user's sign-in material, direct grants and registered {ref}`SSH keys <ssh-key>`, including their model associations. Removal neither revokes cloud credentials nor switches models to replacements. If the departing user's credentials are to be retired, {ref}`arrange the handover before removing the account <remove-a-user>`. Adding the same username later creates a new identity without the old grants or keys.

**Who can do this:** A {ref}`controller superuser <user-access-controller-superuser>`.

**Errors:** Removing a user can fail with these errors:

- *Error message:* `cannot delete controller owner "admin"`. *Trigger:* The target is the built-in controller owner. *What to do:* Keep that account. To withdraw a grant on a particular resource, change access to that resource instead.
- *Error message:* ends with `model(s) would be left without an admin user`. *Trigger:* Removing the user would leave a model without an {ref}`enabled model administrator <keeping-an-active-model-administrator>`. *What to do:* Grant another enabled user model `admin` on each affected model before retrying.
- *Error message:* ends with `user not found`. *Trigger:* No current user matches the name. *What to do:* Verify the controller and username before treating removal as successful.
