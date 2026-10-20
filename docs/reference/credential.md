---
myst:
  html_meta:
    description: "Juju credential reference: what a cloud credential is, the records the controller keeps about it, the changes that follow it, and the operations that add, list, show, update, and remove credentials."
---

(credential)=
# Credential

```{ibnote}
See also: {ref}`Juju | Manage credentials <manage-credentials>`
```

A **credential** is the authentication material (such as a user name and password, or a client ID and secret key) that lets a {ref}`user <user>` of a {ref}`controller <controller>` act on one {ref}`cloud <cloud>`. A credential belongs to one user and one cloud, and always authenticates against a cloud, never against Juju. Every {ref}`model <model>` is created with a cloud and, for most clouds, one of the user's credentials, because the model's workers need it to create resources on the cloud.

(credential-client-and-controller)=
## Client and controller credentials

A credential can be held by a {ref}`client <client>`, by a controller, or by both. A **client credential** is one that the client keeps and the controller does not know. A **controller credential** is one stored on a controller, in its database. When a user bootstraps a controller with a client credential, the client uploads the credential, which makes it a controller credential too. The two sets can end up the same, but they do not have to. The sections below describe controller credentials: what a client stores is up to the client.

(credential-data-model)=
## Credential data model

```{ggarch}
:file: ../juju.ggarch
:view: Credential data model
:caption: The controller database keeps one record for each credential. A credential belongs to one user and one cloud (a user and a cloud each have many credentials), has one authentication type (a type has many credentials), and has many attributes (an attribute belongs to one credential). A model uses at most one credential, and a credential can serve many models. The user, cloud, and model are described on their own pages and the figure shows them only for context: all boxes are records, and the three with a second, outlined arrow mark in the corner are the records that other pages describe.
:alt: Six boxes of the same size inside a group named Controller database, each with a filled table mark in the corner. Credential is in the middle. User is above it (1 to m), Cloud is to its left (1 to m), Authentication type is below it (m to 1), and Credential attribute is to its right (1 to m). Model is below and to the right of Credential, joined through a dashed line because a model's credential is optional (1 to m). User, Cloud, and Model have an outlined arrow mark for records that other pages describe.
```

The credential data model is the set of records that the {ref}`controller database <database>` keeps about credentials. The records, with the rules that apply to each, are:

- **Credential:** A credential's name, its owner, its cloud, its authentication type, whether it is invalid and why, and whether it is revoked.
  - *Rules:*
    - A credential's name is not empty.
    - A cloud can hold one credential of a given name for each owner, so two owners can each have a credential with the same name on one cloud.
- **Authentication type:** How the credential authenticates against the cloud. The types that exist are those listed on the {ref}`cloud <cloud-data-model>` page.
- **Credential attribute:** One key and value of a credential, such as `username` or `access-key`. Which keys a credential needs depends on the cloud type and on the authentication type, and the {ref}`page of each cloud <list-of-supported-clouds>` lists them.
  - *Rules:*
    - A credential has each key once, and a key is not empty.
- **User, cloud, and model:** The records at the far ends of a credential. Other pages own them: see {ref}`user <user>`, {ref}`cloud <cloud>`, and {ref}`model <model>`.

A credential is a row in the table `cloud_credential`, and its attributes are rows in `cloud_credential_attribute`. The allowed authentication types come from the lookup table `auth_type`. {ref}`juju-db-repl` shows the definition of any of them with `.ddl <name>`.

(credential-change-streams)=
## Credential change streams

Credential change streams are the notifications that tell the workers of a model that the model's credential has changed, so that nothing has to poll. A change stream is delivered through a watcher (an API object that tells its holder when something it follows has changed). A client has no watcher for credentials. The agents of a model follow which credential the model uses and whether it is valid, and the model's provider tracker (the worker that opens the cloud for the model) reloads the credential and the cloud's definition when either changes.

(credential-operations)=
## Credential operations

Credential operations are the actions a {ref}`client <client>` can take on credentials through the controller API (the set of calls that clients and {ref}`agents <agent>` make to the controller): add, list, show, update, and remove. Choosing the credential of a model is described on the {ref}`model <model-configuring>` page. A credential belongs to its owner, and the access rule is the same for every operation: the owner can act on the credential, and so can a controller superuser, except for the contents of a credential, which only the owner sees.

(credential-adding)=
### Adding credentials

```{ibnote}
See also: {ref}`Juju | Add a credential <add-a-credential>`
```

Adding credentials stores credentials for a user on the controller. Adding a credential whose name the owner already uses on that cloud replaces it.

The operation's required access, rules, and errors are:

- *Required access:* The credential's owner, or {ref}`superuser access <user-access-controller-superuser>` to the controller. No block applies.
- *Rules:*
  - The credential's owner must exist, and the cloud must be one that the controller knows.
  - The authentication type must be one that the cloud admits.
  - A credential cannot be added as invalid.
  - The request can carry a batch of credentials, and the answer has one result for each.
- *Errors:* The client receives each in the result for the credential:
  - **`permission denied`**: *Trigger:* The caller is neither the owner nor a superuser. The client receives it with the code `unauthorized access`. *Remediation:* Ask the owner, or a superuser.
  - **`"<tag>" is not a valid tag`**: *Trigger:* The request's credential tag is not of the form `cloudcred-<cloud>_<owner>_<name>`, where the name starts with a letter and has only letters, digits, and `.@_+-`. The client receives it with no error code. *Remediation:* Use that form.
  - **`validating credential "<name>" owned by "<owner>" for cloud "<cloud>": supported auth-types [<types>], "<type>" not supported`**: *Trigger:* The cloud does not admit the authentication type. The client receives it with the code `not supported`. *Remediation:* Use one of the listed types.
  - **`cloud "<cloud>" for credential not found`**: *Trigger:* The controller does not know the cloud. The client receives it with the code `not found`. *Remediation:* {ref}`Add the cloud <cloud-adding>` first.
  - **`credential name cannot be empty`**: *Trigger:* The credential's name is empty, in a request that passes the tag check. The client receives it with the code `not valid`. *Remediation:* Give the credential a name.
  - **`getting user uuid for credential owner "<owner>": <reason>`**: *Trigger:* The credential's owner does not exist. *Remediation:* Name an existing user.

(credential-listing)=
### Listing credentials

```{ibnote}
See also: {ref}`Juju | View all the known credentials <view-all-the-known-credentials>`, {ref}`Juju | View details about a credential <view-details-about-a-credential>`
```

Listing credentials returns the credentials of a user on a cloud, and shows the contents of one credential, or of all of the caller's credentials, with the models that use each.

The operation's required access and rules are:

- *Required access:* To list the names of a user's credentials on a cloud, or to show one credential without its secrets, the owner or {ref}`superuser access <user-access-controller-superuser>` to the controller. To show the contents of credentials with the models that use them, the owner only: a superuser has no special right here.
- *Rules:*
  - Showing a credential without secrets leaves out the attributes that the cloud's credential schema marks as hidden, and names them as redacted.
  - The contents of credentials include whether each is valid, and a model is named with its access for the owner. A request can ask for secrets to be included.
  - A credential that does not exist gives `credential "<name>" not found` for one named credential, or `cloud credential "<id>" not found` for contents, each with the code `not found`.

(credential-updating)=
### Updating credentials

```{ibnote}
See also: {ref}`Juju | Update a credential <update-a-credential>`
```

Updating credentials replaces the contents of credentials, after checking that the models that use them can still reach their clouds. A credential that is invalid becomes valid again when it is replaced.

The operation's required access, rules, and errors are:

- *Required access:* The credential's owner, or {ref}`superuser access <user-access-controller-superuser>` to the controller. Forcing the update needs superuser access. No block applies.
- *Rules:*
  - The same rules as for {ref}`adding <credential-adding>` apply to the owner, the cloud, and the authentication type.
  - A request can ask to check a credential without storing it, which returns for each model that uses the credential the errors that the check found.
  - The update is refused if the check finds an error for any model that uses the credential, unless a superuser forces it. The answer lists the models and their errors either way. In 4.1 the check of a model finds none.
- *Errors:* The client receives each in the result for the credential:
  - **`unexpected force specified: invalid request`**: *Trigger:* A caller who is not a superuser asked to force the update. *Remediation:* Do not force it, or ask a superuser.
  - **`credential is not valid for one or more models`**: *Trigger:* The check found errors for a model that uses the credential, and the update is not forced. *Remediation:* Fix the errors that the answer lists for each model, or ask a superuser to force the update.
  - **`unknown cloud "<name>" for credential "<cloud>"`**: *Trigger:* The controller does not know the credential's cloud. *Remediation:* {ref}`Add the cloud <cloud-adding>` first.
  - The errors of {ref}`adding <credential-adding>` for the owner, the cloud, and the authentication type.

(credential-removing)=
### Removing credentials

```{ibnote}
See also: {ref}`Juju | Remove a credential <remove-a-credential>`
```

Removing credentials deletes credentials from the controller, and takes them off the models that use them.

The operation's required access, rules, and errors are:

- *Required access:* The credential's owner, or {ref}`superuser access <user-access-controller-superuser>` to the controller. No block applies.
- *Rules:*
  - A credential that a model uses cannot be removed unless the request forces the removal. A forced removal leaves each of those models with no credential.
  - A removal deletes the credential's attributes and the credential.
- *Errors:* The client receives each in the result for the credential:
  - **`cannot revoke credential <cloud>/<owner>/<name>: it is still used by <n> model(s)`**: *Trigger:* Models use the credential and the request does not force the removal. *Remediation:* {ref}`Change the credential of those models <model-configuring>` first, or force the removal.
  - **`credential <tag> not found`**: *Trigger:* The credential does not exist, where `<tag>` is the credential's tag. The client receives it with the code `not found`. *Remediation:* {ref}`List the credentials <credential-listing>` and use an existing one.
  - **`permission denied`**: *Trigger:* The caller is neither the owner nor a superuser. *Remediation:* Ask the owner, or a superuser.

## Credentials and model migration

A migration does not move credentials. The target controller must already hold a credential with the cloud, owner, and name that the model uses, and a migration to a controller without it fails with `setting cloud credential for model "<uuid>": not found cloud credential "<key>"`. See {ref}`Migrating models <model-migrating>`.
