---
myst:
  html_meta:
    description: "Juju user reference: what a user is, the records the controller keeps about users and their access, the access levels, and the operations that add, list, enable, disable, remove, and register users, change passwords, log in, and grant or revoke access."
---

(user)=
# User

```{ibnote}
See also: {ref}`manage-users`
```

A **user** is an identity that logs in to a {ref}`controller <controller>` and holds access levels on the controller, its {ref}`clouds <cloud>`, {ref}`models <model>`, and {ref}`offers <offer>`. A local user has a name and a password that the controller stores. An external user has a name of the form `<name>@<domain>`, is authenticated by an external identity provider (see {ref}`identity-url <controller-config-identity-url>`), and is recorded by the controller at its first login.

```{note}
A user is not an account on the machine of the {ref}`client <client>`. A user's password authenticates against the controller. A {ref}`credential <credential>` authenticates against a cloud.
```

(user-database-schema)=
## User database schema

```{ggarch}
:file: ../juju.ggarch
:view: User database schema
:caption: The controller database keeps one record for each user. A user has at most one authentication record, which says whether the user is disabled. An authentication record has at most one password and at most one activation key (a one-time secret that lets a new user set a first password). A user has many permissions, and a permission belongs to one user. A user has many last-login records, one for each model the user has logged in to. A last-login record belongs to one user and one model, and a model has many last-login records. The model is described on its own page and the figure shows it only for context: all boxes are records, and the one with a second, outlined arrow mark in the corner is the record that another page describes. Every user also has a creator, which is a user. The figure leaves that link out.
:alt: Seven boxes of the same size inside a group named Controller database, each with a filled table mark in the corner. User is in the middle of the top row. Last login is to its left (many for each user) and Model is further left (many last logins for each model). Permission is to the right of User (many for each user). Authentication is below User (at most one for each user). Password is to the left of Authentication and Activation key is to the right of it (at most one of each for each authentication record). Model has an outlined arrow mark for a record that another page describes.
```

The user database schema is the set of records that the {ref}`controller database <database>` keeps about users and their access. Authentication details are kept apart from the user. Each access of a user is a record of its own. The records, with the rules that apply to each, are:

- **User:** A user's name, display name, whether it is external, whether it is removed, its creator, and its creation time.
  - *Rules:*
    - A name belongs to at most one user that is not removed. A removed user's name can be used again.
    - Every user has a creator, which is a user. The user `admin` is its own creator.
- **Authentication:** Whether the user is disabled.
  - *Rules:*
    - A user has at most one authentication record.
- **Password:** The salted hash of a local user's password (a hash of the password mixed with a random value, so that equal passwords have different hashes).
  - *Rules:*
    - An authentication record has at most one password.
- **Activation key:** The one-time secret key of a user who has not yet set a password.
  - *Rules:*
    - An authentication record has at most one activation key.
- **Permission:** One access level of one user on one object. The object is a controller, a cloud, a model, or an offer.
  - *Rules:*
    - A user holds at most one permission on an object.
    - The pair of object type and access level must be one of the pairs in {ref}`user access levels <user-access-levels>`.
- **Last login:** When a user last logged in to a model.
  - *Rules:*
    - A user has at most one last-login record for a model.
- **Model:** The record at the far end of a last login. Another page owns it: see {ref}`model <model>`.

A user is a row in the table `user`. Its authentication is a row in `user_authentication`, its password a row in `user_password`, and its activation key a row in `user_activation_key`. A permission is a row in `permission`, which names its object in the column `grant_on` (the name of a cloud, or the UUID of a controller, model, or offer). The allowed pairs of object type and access level come from the lookup tables `permission_object_type`, `permission_access_type`, and `permission_object_access`. A last login is a row in `model_last_login`. The view `v_user_auth` joins a user with its disabled flag, and the view `v_user_last_login` gives the latest login of each user. The views `v_permission_controller`, `v_permission_cloud`, and `v_permission_model` list only the permissions whose object exists. {ref}`juju-db-repl` shows the definition of any of them with `.ddl <name>`.

(user-access-levels)=
## User access levels

An access level is what a user may do on one object. The levels that exist for each type of object, from the lowest to the highest, are:

- **Controller:** {ref}`login <user-access-controller-login>`, {ref}`superuser <user-access-controller-superuser>`.
- **Cloud:** {ref}`add-model <user-access-cloud-add-model>`, {ref}`admin <user-access-cloud-admin>`.
- **Model:** {ref}`read <user-access-model-read>`, {ref}`write <user-access-model-write>`, {ref}`admin <user-access-model-admin>`.
- **Offer:** {ref}`read <user-access-offer-read>`, {ref}`consume <user-access-offer-consume>`, {ref}`admin <user-access-offer-admin>`.

A level includes the levels below it. {ref}`Granting and revoking access <user-granting-access>` changes who holds a level.

### Valid access levels for controllers

(user-access-controller-login)=
#### `login`

The holder can log in to the controller (see {ref}`logging in <user-logging-in>`) and list its nodes (see {ref}`listing controller nodes <controller-listing-nodes>`). A new user starts with this level.

(user-access-controller-superuser)=
#### `superuser`

The holder can do everything that the operations of the controller, cloud, and model pages list superuser access for, and manages users. The controller gives it to the user `admin` when it is {ref}`bootstrapped <bootstrap-a-controller>`. External users inherit the higher of their own level and the level of the special user `everyone@external`, which acts as a group for every external user.

### Valid access levels for clouds

A controller can manage models on many clouds. A user holds a separate level on each cloud.

(user-access-cloud-add-model)=
#### `add-model`

The holder can see the cloud (see {ref}`listing clouds <cloud-listing>`) and add a model on it (see {ref}`adding models <model-adding>`).

(user-access-cloud-admin)=
#### `admin`

The holder can remove the cloud (see {ref}`removing clouds <cloud-removing>`) and grant or revoke cloud access to others. The creator of a cloud holds it (see {ref}`adding clouds <cloud-adding>`).

### Valid access levels for models

(user-access-model-read)=
#### `read`

The holder can see the model and its contents (see {ref}`showing models <model-showing>`).

(user-access-model-write)=
#### `write`

The holder can change the model's configuration and its machines (see {ref}`configuring models <model-configuring>`).

(user-access-model-admin)=
#### `admin`

The holder can destroy the model (see {ref}`destroying models <model-destroying>`) and grant or revoke model access to others. The creator of a model holds it.

### Valid access levels for application offers

(user-access-offer-read)=
#### `read`

The holder can see the offer in a search. It is the lowest offer level.

(user-access-offer-consume)=
#### `consume`

The holder can relate an application to the offer. Revoking this level suspends the holder's relations to the offer, unless the holder is a controller superuser or admin of the offer's model.

(user-access-offer-admin)=
#### `admin`

The holder can grant or revoke offer access to others.

(user-operations)=
## User operations

User operations are the actions a {ref}`client <client>` can take on users through the controller API (the set of calls that clients and {ref}`agents <agent>` make to the controller): add, list, enable, disable, remove, change a password, register, log in, and grant or revoke access. Most calls take a list. Each entry in the list gets its own result. Logging in and registering handle one user for each request. A user is named by a tag of the form `user-<name>`.

(user-change-block)=
The **change block** is an operator setting that makes the controller refuse operations that change the model. A refused operation fails with an "operation blocked" error that carries the operator's message. The calls below that name the change block check the controller model's block.

(user-adding)=
### Adding users

```{ibnote}
See also: {ref}`Juju | Add a user <add-a-user>`
```

Adding users (`AddUser`) creates local users with {ref}`login access <user-access-controller-login>` to the controller. A user created without a password gets a secret key that the new user exchanges for a first password (see {ref}`registering users <user-registering>`).

The operation's required access, rules, and errors are:

- *Required access:* {ref}`Superuser access <user-access-controller-superuser>` to the controller. The {ref}`change block <user-change-block>` must not be on.
- *Rules:*
  - The name starts and ends with a letter or digit, has only letters, digits, `.`, `+`, and `-` between them, and has at least two characters.
  - The name must be local: a name with a domain is refused.
  - A password, when given, is not empty and is at most 1024 bytes. The controller stores a salted hash of it (see the {ref}`database schema <user-database-schema>`).
  - A request without a password makes the controller generate a random 32-byte secret key and return it. The key stays valid until the user sets a password.
  - The new user is enabled, its creator is the caller, and the optional display name is stored.
- *Errors:* A refused access or block fails the whole call. The client receives the others in the result for the user, with no error code unless stated:
  - **`permission denied`**: *Trigger:* The caller does not hold superuser access. The client receives it with the code `unauthorized access`. *Remediation:* Ask a superuser.
  - **The operator's block message**, or `the operation has been blocked` when the operator gave none (code `operation is blocked`): *Trigger:* The change block is on. *Remediation:* Ask the operator to lift the block.
  - **`creating user: user name "<name>" not valid`**: *Trigger:* The name does not follow the rule above. The client receives it with the code `not valid`. *Remediation:* Use a valid name.
  - **`creating user: cannot add external user "<name>": username not valid`**: *Trigger:* The name has a domain. *Remediation:* Use a name without a domain.
  - **`creating user: password not valid, size must be less then 1024 bytes`**: *Trigger:* The password is longer than 1024 bytes. *Remediation:* Use a shorter password.
  - **`creating user: adding user "<name>": user already exists`**, or `creating user: adding user with uuid "<uuid>": adding user "<name>": user already exists` when the request had a password: *Trigger:* A user that is not removed already has the name. *Remediation:* {ref}`List the users <user-listing>` and choose another name.

(user-listing)=
### Listing users

```{ibnote}
See also: {ref}`Juju | Manage users <manage-users>`
```

Listing users (`UserInfo`) returns a user's name, display name, creator, creation time, last connection, whether it is disabled, and its access to the controller. A second call (`ModelUserInfo`) returns the users of a model with their model access and last connection to it.

The operation's required access, rules, and errors are:

- *Required access:* A caller logged in as a user. To see another user, {ref}`superuser access <user-access-controller-superuser>` to the controller. For the users of a model, {ref}`read access <user-access-model-read>` to the model. No block applies.
- *Rules:*
  - A request without names returns every user that is not removed when the caller is a superuser, and only the caller otherwise. Disabled users are left out unless the request asks for them.
  - A disabled user shows an empty access level.
  - The last connection is the latest login of the user to any model. It is empty for a user who has never logged in.
  - The users of a model come with their access level on the model. A superuser or an {ref}`admin <user-access-model-admin>` of the model sees every user of the model. Any other caller sees only their own entry.
- *Errors:* The client receives each of the first three in the result for the user. The last two fail the whole call:
  - **`permission denied`**: *Trigger:* A caller who is not a superuser asked about another user. The client receives it with the code `unauthorized access`. *Remediation:* Ask for your own user, or ask a superuser.
  - **`user "<name>" not found`**: *Trigger:* No user has the name. The client receives it with the code `user not found`. *Remediation:* Check the name.
  - **`"<tag>" is not a valid tag`** and **`"<tag>" is not a valid user tag`**: *Trigger:* The tag is not of the form `user-<name>`. The client receives them with no error code. *Remediation:* Use that form.
  - **`"<tag>" is not a valid tag`** and **`"<tag>" is not a valid model tag`**: *Trigger:* A tag for the users of a model is not of the form `model-<UUID>`. *Remediation:* Use that form.
  - **`permission denied`**: *Trigger:* The caller does not hold read access to the model. The client receives it with the code `unauthorized access`. *Remediation:* Ask an admin of the model.

(user-controller-access)=
### Reading controller access

Reading controller access (`GetControllerAccess` of the controller) returns the controller access level of each user in the request.

The operation's required access and errors are:

- *Required access:* A superuser can read any user's level, and any other caller can read only their own. No block applies.
- *Errors:* The client receives each in the result for the user, with no error code unless stated:
  - **`permission denied`**: *Trigger:* A caller who is not a superuser asked about another user. The client receives it with the code `unauthorized access`. *Remediation:* Ask for your own level, or ask a superuser.
  - **`access not found for "<name>" on "<controller UUID>"`**: *Trigger:* The user holds no controller access, or is disabled or removed. *Remediation:* {ref}`Grant access <user-granting-access>` or {ref}`enable the user <user-enabling>`.
  - The tag errors of {ref}`listing users <user-listing>`.

(user-enabling)=
### Enabling users

```{ibnote}
See also: {ref}`Juju | Manage users <manage-users>`
```

Enabling users (`EnableUser`) lets disabled users log in again. Enabling a user who is already enabled succeeds.

The operation's required access and errors are:

- *Required access:* {ref}`Superuser access <user-access-controller-superuser>` to the controller. The {ref}`change block <user-change-block>` must not be on.
- *Errors:* A refused access or block fails the whole call with the errors of {ref}`adding users <user-adding>`. The client receives the others in the result for the user, with no error code:
  - **`failed to enable user: enabling user with uuid "<name>": active user "<name>": user not found`**: *Trigger:* No user that is not removed has the name. *Remediation:* {ref}`List the users <user-listing>` and use an existing name.
  - The tag errors of {ref}`listing users <user-listing>`.

(user-disabling)=
### Disabling users

```{ibnote}
See also: {ref}`Juju | Manage users <manage-users>`
```

Disabling users (`DisableUser`) stops users from logging in. Disabling a user who is already disabled succeeds.

The operation's required access, rules, and errors are:

- *Required access:* The same as {ref}`enabling users <user-enabling>`.
- *Rules:*
  - A disabled user fails every login with the same error as a wrong password, and shows no access to the controller.
  - A disabled user's password, activation key, and permissions stay stored. The user cannot be granted access and cannot have a password set.
  - A user is refused when a model would be left without an enabled admin other than this user.
- *Errors:* The client receives each in the result for the user, with no error code:
  - **`failed to disable user: <n> model(s) would be left without an admin user`**: *Trigger:* After the change, `<n>` models have no enabled admin. *Remediation:* {ref}`Grant admin access <user-granting-access>` on those models to another user first.
  - **`failed to disable user: active user "<name>": user not found`**: *Trigger:* No user that is not removed has the name. *Remediation:* {ref}`List the users <user-listing>` and use an existing name.
  - The errors of {ref}`enabling users <user-enabling>` for the access, the block, and the tag.

(user-removing)=
### Removing users

```{ibnote}
See also: {ref}`Juju | Manage users <manage-users>`
```

Removing users (`RemoveUser`) marks users as removed for good. The controller keeps the user's record for auditing.

The operation's required access, rules, and errors are:

- *Required access:* {ref}`Superuser access <user-access-controller-superuser>` to the controller. The {ref}`change block <user-change-block>` must not be on.
- *Rules:*
  - Removing a user deletes the user's SSH keys with their model authorizations (see {ref}`SSH key <ssh-key>`), its password, its activation key, and all its permissions.
  - The user `admin` cannot be removed.
  - A user is refused when a model would be left without an enabled admin other than this user.
- *Errors:* A refused access or block fails the whole call with the errors of {ref}`adding users <user-adding>`. The client receives the others in the result for the user:
  - **`cannot delete controller owner "admin"`**: *Trigger:* The request names `admin`. The client receives it with no error code. *Remediation:* Remove another user.
  - **`failed to delete user "<name>": <n> model(s) would be left without an admin user`**: *Trigger:* After the removal, `<n>` models have no enabled admin. The client receives it with no error code. *Remediation:* {ref}`Grant admin access <user-granting-access>` on those models to another user first.
  - **`failed to delete user "<name>": active user "<name>": user not found`**: *Trigger:* No user that is not removed has the name. The client receives it with no error code. *Remediation:* {ref}`List the users <user-listing>` and use an existing name.
  - The tag errors of {ref}`listing users <user-listing>`.

(user-passwords)=
### Changing passwords

```{ibnote}
See also: {ref}`Juju | Manage a user's login details <manage-a-users-login-details>`
```

Changing passwords sets a new password for users (`SetPassword`) or resets it (`ResetPassword`). Setting a password stores a new salted hash and discards any activation key. Resetting a password deletes the stored password and returns a new random secret key for the user, who exchanges it for a password (see {ref}`registering users <user-registering>`).

The operation's required access, rules, and errors are:

- *Required access:* To set a password, {ref}`superuser access <user-access-controller-superuser>` to the controller, or being the user whose password changes. To reset a password, superuser access. The {ref}`change block <user-change-block>` must not be on.
- *Rules:*
  - A new password is not empty after trimming spaces and is at most 1024 bytes.
  - A password cannot be set for a disabled user.
  - A superuser cannot reset their own password.
- *Errors:* A refused block fails the whole call with the block error of {ref}`adding users <user-adding>`. The client receives the others in the result for the user:
  - **`permission denied`**: *Trigger:* A caller who is not a superuser set another user's password. Or a caller reset their own password, or reset a password without superuser access. The client receives it with the code `unauthorized access`. *Remediation:* To set another user's password, ask a superuser. To reset your own password, ask another superuser.
  - **`failed to set password: password not valid, size must be less then 1024 bytes`**: *Trigger:* The new password is longer than 1024 bytes. The client receives it with no error code. *Remediation:* Use a shorter password.
  - **`cannot use an empty password`**: *Trigger:* The new password is empty or only spaces. The client receives it with no error code. *Remediation:* Send a password.
  - **`failed to set password: setting password for user "<name>": active user "<name>": user not found`**: *Trigger:* No user that is not removed has the name. The client receives it with no error code. *Remediation:* {ref}`List the users <user-listing>` and use an existing name.
  - **`failed to set password: setting password for user "<name>": setting password hash for user "<name>": "<name>": user authentication disabled`**: *Trigger:* The user is disabled. The client receives it with no error code. *Remediation:* {ref}`Enable the user <user-enabling>` first.
  - **`setting activation key for user "<name>": setting activation key for user "<name>": "<name>": user authentication disabled`**: *Trigger:* A reset names a disabled user. The client receives it with no error code. *Remediation:* {ref}`Enable the user <user-enabling>` first.
  - **`setting activation key for user "<name>": active user "<name>": user not found`**: *Trigger:* A reset names a user that does not exist. The client receives it with no error code. *Remediation:* {ref}`List the users <user-listing>` and use an existing name.
  - The tag errors of {ref}`listing users <user-listing>`.

(user-registering)=
### Registering users

```{ibnote}
See also: {ref}`Juju | Add a user <add-a-user>`
```

Registering users completes the setup of a user who holds a secret key (from {ref}`adding <user-adding>` or a password reset). A client sends a POST request to the controller's `/register` route with the user's tag, a nonce (a one-time value that the client chooses), and a message sealed with the secret key (encrypted and authenticated with it). The controller sets the password that the message holds and discards the activation key. It answers with the controller's CA certificate (the certificate of the authority that signed the controller's TLS certificate) and UUID, sealed with the same key. The answer can also hold the proxy settings that the client needs to reach the controller.

The operation's required access, rules, and errors are:

- *Required access:* Knowing the user's secret key. No login is needed.
- *Rules:*
  - A secret key works once, because setting the password discards it.
  - The answer sets a short-lived local-login macaroon (a bearer token that lets the client log in as the user) in a cookie.
- *Errors:* The client receives each as the answer to the request:
  - **`unsupported method: "<method>"`** (code `method not allowed`): *Trigger:* The request is not a POST. *Remediation:* Send a POST request.
  - **`activation key not found`** (code `not found`): *Trigger:* The user has no secret key, for example because it was used. *Remediation:* Ask a superuser to {ref}`reset the password <user-passwords>`.
  - **`activation key not valid`** (code `not valid`): *Trigger:* The sealed message cannot be opened with the user's secret key. *Remediation:* Use the key that the controller returned.
  - **`nonce not valid`** (code `not valid`): *Trigger:* The nonce is not 24 bytes long. *Remediation:* Send a nonce of 24 bytes.
  - **`setting new password: setting password for user "<name>": setting password hash for user "<name>": "<name>": user authentication disabled`**: *Trigger:* The user is disabled. The client receives it with no error code. *Remediation:* Ask a superuser to {ref}`enable the user <user-enabling>`.
  - **`"<tag>" is not a valid tag`** and **`"<tag>" is not a valid user tag`**: *Trigger:* The tag in the request is not of the form `user-<name>`. The client receives them with no error code. *Remediation:* Use that form.

(user-logging-in)=
### Logging in

```{ibnote}
See also: {ref}`Juju | Manage users <manage-users>`
```

Logging in (`Login` of the `Admin` facade) authenticates a user and opens a session: all later requests on the connection act as that user. The answer holds the user's tag, its access to the controller and to the model addressed, the controller tag, the server addresses, the server version, and the facades (named groups of API calls) the user may call.

The operation's required access, rules, and errors are:

- *Required access:* A local user logs in with the name and password, or with a macaroon. An external user logs in with a macaroon from the identity provider. A user who logs in to a controller needs {ref}`login access <user-access-controller-login>` to it. A user who logs in to a model needs access to the model, unless the user is a superuser.
- *Rules:*
  - A request without a password is authenticated with macaroons. When none is valid, the answer asks the client to discharge a macaroon (to have it confirmed by the third party named in it). The client then logs in again with the result.
  - Every authenticated login stores the time as the user's last login to the model.
  - An external user's record is created at the first login that passes the access checks.
- *Errors:* The client receives each as the answer to the request:
  - **`invalid entity name or password`** (code `unauthorized access`): *Trigger:* The user does not exist, the password is wrong, or the user is disabled. The answer is the same for all three. *Remediation:* Check the name and password, or ask a superuser to {ref}`enable the user <user-enabling>`.
  - **`login failed - maintenance in progress`**: *Trigger:* An upgrade, migration, or similar operation is in progress and the login failed. *Remediation:* Try again when the operation has finished.
  - **A redirection** (code `redirection required`): *Trigger:* The login names a model that migrated to another controller. The user is anonymous, is external, or held access to the model when it migrated. The answer holds the new controller's addresses and CA certificate. *Remediation:* Log in to the controller in the answer.
  - **`permission denied`** (code `unauthorized access`): *Trigger:* The user holds no access to the controller, or none to the model without being a superuser. *Remediation:* Ask a superuser to {ref}`grant access <user-granting-access>`.

(user-granting-access)=
### Granting and revoking access

```{ibnote}
See also: {ref}`Juju | Manage user access <manage-user-access>`
```

Granting and revoking access changes the {ref}`access level <user-access-levels>` of users on a controller (`ModifyControllerAccess`), a model (`ModifyModelAccess`), a cloud (`ModifyCloudAccess`), or an offer (`ModifyOfferAccess`). A change names the user, the object, the action (`grant` or `revoke`), and the level.

The operation's required access, rules, and errors are:

- *Required access:* For a controller, {ref}`superuser access <user-access-controller-superuser>` to it. For a model, {ref}`admin access <user-access-model-admin>` to it or superuser access. For a cloud, {ref}`admin access <user-access-cloud-admin>` to it or superuser access. For an offer, {ref}`admin access <user-access-offer-admin>` to it, admin access to its model, or superuser access. No block applies.
- *Rules:*
  - The level must be valid for the type of object (see {ref}`access levels <user-access-levels>`).
  - Granting a level to a user who holds it or a higher one is refused.
  - Granting to an external user that has no record creates the record.
  - Revoking a level sets the user's access to the level just below it, whatever the user holds now. Revoking the lowest level removes the permission.
  - Revoking on a model is refused when no other enabled user holds admin on it.
  - Revoking offer access below `consume` suspends the user's relations to the offer, unless the user is a controller superuser or admin of the offer's model.
  - A user cannot change their own cloud access.
- *Errors:* The client receives each in the result for the change, with no error code unless stated:
  - **`permission denied`**: *Trigger:* The caller lacks the access above. The client receives it with the code `unauthorized access`. *Remediation:* Ask a holder of the access above.
  - **`could not modify controller access: <tag error>`** and **`could not modify model access: <tag error>`**: *Trigger:* The user tag of a controller change, or the user or model tag of a model change, is malformed. A cloud or offer change returns the tag error alone. The client receives them with no error code. *Remediation:* Use tags of the forms `user-<name>` and `model-<UUID>`.
  - **`"<level>" <type> access not valid`**, where `<type>` is `controller`, `cloud`, `model`, or `offer`: *Trigger:* The level does not exist for the type. The client receives it with the code `not valid`. *Remediation:* Use a level from the list above.
  - **`change "<action>" not valid`**: *Trigger:* The action of a controller, model, or cloud change is neither `grant` nor `revoke`. The client receives it with the code `not valid`. *Remediation:* Use `grant` or `revoke`.
  - **`unknown action "<action>"`**: *Trigger:* The action of an offer change is neither `grant` nor `revoke`. The client receives it with no error code. *Remediation:* Use `grant` or `revoke`.
  - **`user "<name>" already has "<level>" access or greater`**: *Trigger:* The user already holds the level or a higher one. *Remediation:* Continue; nothing needs granting.
  - **`active users "<name>": user not found`**: *Trigger:* The user is local and does not exist, or is disabled. *Remediation:* {ref}`Add <user-adding>` or {ref}`enable <user-enabling>` the user first.
  - **`model would be left without an admin user`**: *Trigger:* A revocation on a model would leave it without an enabled admin. *Remediation:* Grant admin on the model to another user first.
  - **`cannot change your own cloud access`**: *Trigger:* A cloud change names the caller. *Remediation:* Ask another admin of the cloud.
  - **`offer "<url>" not found`** (code `not found`) and **`model "<qualifier>/<name>": not found`** (code `not found`): *Trigger:* An offer change names an offer or a model that does not exist. *Remediation:* Check the offer URL.
  - **`updating offer permission for "<name>" on "<offer UUID>": offer permission for "<name>" on "<offer UUID>" permission not found`**: *Trigger:* An offer revocation names a user who holds no access to the offer. *Remediation:* Continue; nothing needs revoking.
  - The tag errors of {ref}`listing users <user-listing>`.

## Users and model migration

A migration brings the model's users with their access and last connection to the model. The target controller creates the external users of the model that it does not know. Local users must already exist on the target. The migration checks that (see {ref}`Migrating models <model-migrating>`).
