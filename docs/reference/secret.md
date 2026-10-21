---
myst:
  html_meta:
    description: "Juju secret reference: what a secret is, the records the model keeps about its revisions, owners, consumers, and grants, the changes that reach units, and the operations that add, list, read, grant, update, rotate, and remove secrets."
---

(secret)=
# Secret

```{ibnote}
See also: {ref}`Juju | Manage secrets <manage-secrets>`
```

A **secret** is a set of named values (such as a password, a token, or a certificate) that Juju stores in the {ref}`secret backend <secret-backend>` of a {ref}`model <model>` and discloses only to the entities that the secret's owner grants access to. A user or a {ref}`charm <charm>` adds a secret to one model. Its owner updates and removes it.

(secret-database-schema)=
## Secret database schema

```{ggarch}
:file: ../juju.ggarch
:view: Secret database schema
:caption: The model database keeps one record for each secret. A secret has at most one rotation record and at most one model owner record. It has any number of application owner records and unit owner records, and many revisions, consumers, and grants (each of those belongs to one secret). An application has many application owner records, a unit has many unit owner records, and a unit has many consumer records. A revision has many content items, and at most one backend reference, one expiry, and one obsolete mark. A consumer record joins one unit to one secret, so a unit can consume many secrets and a secret can have many consuming units. The application and the unit are described on their own pages. The figure shows them only for context. All boxes are records, and the two with a second, outlined arrow mark in the corner are the records that other pages describe.
:alt: Fourteen boxes of the same size inside a group named Model database, each with a filled table mark in the corner. Secret is in the middle. Rotation is above it (1 to at most 1) and Grant is below it (1 to m). Model owner is to its left (1 to at most 1). Application owner is above Model owner and Unit owner is below it (each 1 to m). Application is to the left of Application owner (1 to m), and Unit is to the left of Unit owner (1 to m). Consumer is below Unit (Unit to Consumer 1 to m) and is joined to Secret by a long line (1 to m). Revision is to the right of Secret (1 to m). Content item is above Revision (1 to m). Backend reference is below Revision, and Revision expiry is to the right of Revision (each 1 to at most 1). Obsolete mark is to the right of Backend reference and is joined to Revision by a bent line (1 to at most 1). Application and Unit have an outlined arrow mark for records that other pages describe.
```

The secret database schema is the set of records that the {ref}`model database <database>` keeps about secrets, their revisions, owners, consumers, and grants. The records, with the rules that apply to each, are:

- **Secret:** A secret's description, its rotation policy, whether Juju removes the revisions that no unit tracks (auto-prune), the checksum of its latest revision, and its creation and update times.
  - *Rules:*
    - The rotation policy is one of `never`, `hourly`, `daily`, `weekly`, `monthly`, `quarterly`, and `yearly`.
    - Auto-prune is off by default.
  - A secret has three identifiers:

    (secret-uri)=
    - **URI:** The identifier that Juju assigns when it creates the secret, in the form `secret:<id>`, where the ID has 20 characters. The first 19 are from `0` to `9` and `a` to `v`. The last is `0` or `g`. The URI of a secret that another model shares has that model's UUID before the ID, in the form `secret://<model UUID>/<id>`.

    (secret-name)=
    - **Name:** The label of a user secret.

    (secret-label)=
    - **Label:** A string that an owner gives its secret, or that a consuming unit gives a secret it reads, so that a charm can refer to the secret without its URI.
- **Owner:** The entity that manages a secret, with the label that the owner gave it. The kind of owner makes the kind of secret:

  (user-secret)=
  - **User secrets** are secrets that a user adds to a model. The model owns them, so the record is a model owner.

  (charm-secret)=
  - **Charm secrets** are secrets that a charm adds. An application or a unit owns them:

    (application-secret)=
    - **Application secrets** are owned by the application and managed by its {ref}`leader unit <leader-unit>`. When the leader changes, the new leader manages them.

    (unit-secret)=
    - **Unit secrets** are owned by the unit that added them. Only that unit manages them.
  - *Rules:*
    - A secret has at most one model owner record.
    - A label that is not empty appears once for a given application, once for a given unit, and once for the model.
- **Consumer:** A unit that reads a secret, the revision that it tracks, and the label that it gave the secret. The record also names the model that the secret comes from, which is another model for a secret that a {ref}`cross-model relation <cross-model-relation>` shares.
  - *Rules:*
    - A unit consumes a secret once.
    - A label that is not empty appears once for a given unit.
- **Grant:** The access that a subject has to a secret: the role, the subject, and the scope. The subject is a unit, an application, or the model. The scope is a unit, an application, the model, or a {ref}`relation <relation>`. The subject and the scope are IDs, with no foreign key to those entities.
  - *Rules:*
    - A role is `none`, `view`, or `manage`.
    - A secret has one grant for a subject.
    - A subject ID and a scope ID are not empty.
    - An update of a grant cannot change its subject type, its scope type, or its scope ID.
- **Revision:** One version of a secret's content, with the times of its creation and last update. A secret keeps its earlier content as numbered revisions.
  - *Rules:*
    - A revision number appears once for a secret.
- **Content item:** One key and value of a revision whose content the model database holds.
  - *Rules:*
    - A revision has each key once.
    - A key is not empty.
- **Backend reference:** The backend and the ID that locate the content of a revision that a {ref}`secret backend <secret-backend>` holds.
  - *Rules:*
    - A revision has at most one backend reference.
- **Revision expiry:** The time at which a revision expires.
  - *Rules:*
    - A revision has at most one expiry.
- **Obsolete mark:** Whether a revision is obsolete and whether it is waiting for deletion.
  - *Rules:*
    - A revision has at most one obsolete mark.
- **Rotation:** The time at which a secret is next due for rotation.
  - *Rules:*
    - A secret has at most one rotation.
- **Application and unit:** The records that own or consume secrets. Other pages describe them: see {ref}`application <application>` and {ref}`unit <unit>`.

The model database also keeps a record that holds only the secret's ID, and lookups that hold the allowed values of the rotation policy, the role, the grant subject type, and the grant scope type. It keeps four more tables that the figure leaves out: `secret_reference` for secrets that other models share, `secret_deleted_value_ref` for deleted revisions whose content a backend still holds, `secret_remote_unit_consumer` for units of other models that consume a secret, and `secret_reservation` for URIs that a unit has reserved and not yet used.

(secret-change-streams)=
## Secret change streams

Secret change streams are the notifications that tell the unit {ref}`agents <agent>` of a model what happened to the secrets they consume or own. They also tell the {ref}`controller <controller>` which revisions to remove. A change stream is delivered through a watcher (an API object that tells its holder when something it follows has changed). The streams carry:

- **Consumed secrets:** A unit agent calls `WatchConsumedSecretsChanges` to hear that a secret its unit consumes has a revision newer than the one the unit tracks. The agent then runs the {ref}`secret-changed <hook-secret-changed>` hook.
- **Rotation:** A unit agent calls `WatchSecretsRotationChanges` to hear that a secret it owns is due for rotation. The agent then runs the {ref}`secret-rotate <hook-secret-rotate>` hook.
- **Expiry:** A unit agent calls `WatchSecretRevisionsExpiryChanges` to hear that a revision of a secret it owns has expired. The agent then runs the {ref}`secret-expired <hook-secret-expired>` hook.
- **Obsolete revisions:** A unit agent calls `WatchObsolete` to hear that no consumer tracks a revision of a secret it owns any more. The agent then runs the {ref}`secret-remove <hook-secret-remove>` hook.
- **Deleted secrets:** A unit agent calls `WatchDeleted` to hear that a secret it owns, or a revision of one, was deleted.
- **Revisions to prune:** The controller calls `WatchRevisionsToPrune` to hear that a user secret has revisions that it can remove.

A unit agent can ask about any unit of its own application. It can ask about the application itself only when its unit is the leader.

(secret-operations)=
## Secret operations

Secret operations are the actions on secrets: add, list, read, grant, revoke, update, rotate, and remove. A {ref}`client <client>` takes them on the user secrets of a model through the controller API (the set of calls that clients and agents make to the controller). A charm takes them on its own secrets through {ref}`hook commands <hook-command>`. A client manages the user secrets of a model. A charm manages the secrets that it owns. Juju applies the changes of a hook when the hook succeeds. A **unit agent** is the {ref}`agent <agent>` process that runs a unit's charm. It logs in as its unit. The facades that serve it accept unit agents only, so no model access level and no block applies to it.

(secret-adding)=
### Adding secrets

```{ibnote}
See also: {ref}`Juju | Manage secrets <manage-secrets>`
```

Adding secrets creates a secret with its first revision. A client adds user secrets, which the model owns. A charm adds application secrets and unit secrets with the hook command {ref}`secret-add <hook-command-secret-add>`.

The operation's required access, rules, and errors are:

- *Required access:* For a user secret, {ref}`write access <user-access-model-write>` to the model. For a charm secret, a unit agent, for its own unit. For an application secret, that unit is the {ref}`leader unit <leader-unit>` of the application.
- *Rules:*
  - The call takes a list of secrets. Each secret in the list gets its own result.
  - Juju assigns the URI and returns it. The secret starts at revision 1. Its owner has the role `manage`.
  - A secret has content: a set of keys with values. A key has at least three characters. It starts with a lowercase letter, continues with lowercase letters, digits, and single dashes, and ends with a letter or a digit. The hook commands and the `juju` client apply this rule before they send the request. They encode each value in base64 unless its key has the suffix `#base64`. An encoded value has at most 1,000,000 bytes. The encoded values of a secret have at most 1,000,000 bytes together.
  - A user secret takes a description, a name (stored as its label), and content. The request can leave the owner empty or name the model by its UUID.
  - A charm secret takes a description, a label, a rotation policy, an expiry time, and content. The application owns it unless the request names the unit as the owner. A rotation policy other than `never` sets the first rotation one interval after the creation.
  - A label that is not empty is not yet in use by the owner. The label of a charm secret also differs from the labels of the secrets that the application and its units own.
  - The content goes to the model's backend. The backend `internal` keeps the content items in the model database. A backend that keeps the content elsewhere leaves a backend reference.
  - A charm secret takes effect when the hook that added it succeeds. The unit asks for the URI first. The controller reserves it for that unit until the hook commits.
- *Errors:* The client receives each in the result for the secret, except `permission denied`, which fails the whole call:
  - **`permission denied`**: *Trigger:* The caller lacks write access to the model. The client receives it with the code `unauthorized access`. *Remediation:* Ask a model administrator.
  - **`owner tag "<tag>" not valid`**: *Trigger:* The request names an owner other than the model. The client receives it with the code `not valid`. *Remediation:* Leave the owner out.
  - **`empty secret value not valid`**: *Trigger:* The request has no content. The client receives it with the code `not valid`. *Remediation:* Give at least one key and value.
  - **`secret with name "<name>" already exists`**: *Trigger:* Another user secret has the label. The client receives it with the code `already exists`. *Remediation:* Choose another name.
  - **`saving secret content to backend: <reason>`**: *Trigger:* The model's backend refused the content. The client receives it with no error code. *Remediation:* Fix the backend as the reason says, or {ref}`set another backend <secret-backend-setting>`.
  - **`this unit is not the leader`**: *Trigger:* A unit that is not the leader adds an application secret. The hook command fails. *Remediation:* Run it on the leader, or add the secret as a unit secret.
  - **`missing secret value or filename`** and **`empty secret value not valid`**: *Trigger:* The hook command has no content. The hook command fails. *Remediation:* Give at least one key and value.
  - **`key "<key>" not valid`**: *Trigger:* A key breaks the rule above. The hook command fails. *Remediation:* Rename the key.
  - **`key value "<argument>" not valid`**: *Trigger:* An argument is not of the form `key=value`. The hook command fails. *Remediation:* Write each argument as `key=value`.
  - **`base64 encoded secret content for key "<key>" too large: <n> bytes`** and **`base64 encoded secret content too large: <n> bytes`**: *Trigger:* A value, or the values together, exceed 1,000,000 bytes once encoded. The hook command fails. *Remediation:* Shorten the content.
  - **`secret with label "<label>" already exists`**: *Trigger:* The hook already added a secret with that label. The hook command fails. *Remediation:* Choose another label.
  - **`rotate policy "<policy>" not valid`**, **`secret owner "<owner>" not valid`**, **`expire time or duration "<value>" not valid`**, and **`negative expire duration "<value>" not valid`**: *Trigger:* The hook command has a policy that is not in the list above, an owner that is not `application` or `unit`, or an expiry that is not a time or a positive duration. The hook command fails. *Remediation:* Correct the value.

(secret-listing)=
### Listing secrets

```{ibnote}
See also: {ref}`Juju | Manage secrets <manage-secrets>`
```

Listing secrets returns the secrets of a model with their metadata, revisions, and grants, and can return their content. A charm lists the secrets that it owns with the hook commands {ref}`secret-ids <hook-command-secret-ids>` and {ref}`secret-info-get <hook-command-secret-info-get>`.

The operation's required access, rules, and errors are:

- *Required access:* {ref}`Read access <user-access-model-read>` to the model. A request for the content needs {ref}`admin access <user-access-model-admin>` to the model or {ref}`superuser access <user-access-controller-superuser>` to the controller. For a charm secret, a unit agent, for its own unit. No block applies to a client.
- *Rules:*
  - The answer for a secret holds:
    - The URI, version, owner, description, label, and rotation policy.
    - The next rotation time.
    - The latest revision, its checksum, and its expiry time.
    - The creation and update times.
    - The grants that have the role `view`.
  - The answer for a revision holds its number, its times, its expiry time, and the name of the backend that holds it. The backend `kubernetes` appears as `<model name>-local`.
  - A request can filter by URI, by label, by revision, or by owner. An owner filter names an application or a unit, and returns only the charm secrets of that owner. Without an owner filter, the list holds user secrets and charm secrets.
  - A request for the content returns the latest revision, or the revision that the filter names. A backend that fails leaves an error in the result of that secret.
  - A charm lists the secrets of its unit. The leader also lists those of its application.
- *Errors:* The last appears in the result for the secret. The others fail the whole call:
  - **`permission denied`**: *Trigger:* The caller lacks the access above. The client receives it with the code `unauthorized access`. *Remediation:* Ask a model administrator.
  - **`secret owner tag kind "<kind>" not valid`**: *Trigger:* The owner filter is not an application or a unit. The client receives it with the code `not valid`. *Remediation:* Name an application or a unit.
  - **`secret URI "<value>" not valid`**: *Trigger:* The URI is not of the form `secret:<id>`. The client receives it with the code `not valid`. *Remediation:* Use the URI that {ref}`listing <secret-listing>` returns.
  - **`getting secret by URI "<id>": <reason>`**: *Trigger:* The URI names no secret, or the revision filter names no revision of it. The client receives it with no error code. *Remediation:* List the secrets without a URI filter and use one of their URIs.
  - **`cannot specify both URI and labels`**: *Trigger:* The request has a URI filter and a label filter. The client receives it with no error code. *Remediation:* Use one of them.
  - **`cannot specify revision without URI or labels`**: *Trigger:* The request has a revision filter and neither a URI nor a label. The client receives it with no error code. *Remediation:* Add the URI or the label.
  - **`getting content for secret "<URI>": <reason>`**: *Trigger:* The revision does not exist (code `secret revision not found`), or the controller does not know the backend that holds it (code `secret backend not found`). *Remediation:* List the secret without its content and use a revision it has, or {ref}`add the backend <secret-backend-adding>`.

(secret-reading)=
### Reading secret content

```{ibnote}
See also: {ref}`Juju | Manage secrets <manage-secrets>`
```

Reading secret content returns the content of one revision of a secret to a unit. A charm reads a secret with the hook command {ref}`secret-get <hook-command-secret-get>`. A unit that reads a secret becomes its consumer and tracks a revision of it.

The operation's required access, rules, and errors are:

- *Required access:* A unit agent, for its own unit. The unit or its application has the role `view` or `manage` on the secret. Every unit of an application can read a secret that the application owns.
- *Rules:*
  - The call names the secret by its URI or by a label. An owner finds its own secret by the owner label. A consumer finds a secret by the label that it gave it.
  - The first read of a secret by a unit tracks the latest revision. Later reads return the tracked revision.
  - A request to refresh returns the latest revision and tracks it from then on. A request to peek returns the latest revision once and keeps the tracked revision. A request cannot ask for both.
  - A request that names both a URI and a label gives the secret that label for the unit, so that the unit can use the label from then on.
  - A refresh makes the revision that the unit stopped tracking obsolete when no other unit tracks it and a newer revision exists.
  - The answer holds the content, or a backend reference with the backend's configuration so that the unit can read the content from the backend.
- *Errors:* The client receives each in the result for the secret:
  - **`cannot read secret "<URI or label>": "<unit>" is not allowed to read this secret`**: *Trigger:* Neither the unit nor its application has a grant. The client receives it with the code `unauthorized access`. *Remediation:* {ref}`Grant access <secret-granting>` to the application or the unit.
  - **`getting content for secret "<URI or label>": <reason>`**: *Trigger:* The secret (code `secret not found`), the revision (code `secret revision not found`), or the backend (code `secret backend not found`) does not exist. *Remediation:* Use the URI that the owner gave, or ask the owner for the new one.
  - **`require either a secret URI or label`** and **`specify one of --peek or --refresh but not both`**: *Trigger:* The hook command names no secret, or asks to peek and to refresh. The hook command fails. *Remediation:* Name the secret, and use one option.

(secret-granting)=
### Granting and revoking access to secrets

```{ibnote}
See also: {ref}`Juju | Manage secrets <manage-secrets>`
```

Granting and revoking access to secrets gives applications and units the right to read a secret, and takes it away. A client grants and revokes access to a user secret for applications. A charm grants and revokes access to its own secrets with the hook commands {ref}`secret-grant <hook-command-secret-grant>` and {ref}`secret-revoke <hook-command-secret-revoke>`.

The operation's required access, rules, and errors are:

- *Required access:* For a user secret, {ref}`write access <user-access-model-write>` to the model. For a charm secret, a unit agent, for its own unit, which is the owning unit or the leader of the owning application and has the role `manage` on the secret. No block applies to a client.
- *Rules:*
  - A grant gives the role `view`.
  - A client's call names the secret by URI or by name, and lists the applications. Each application in the list gets its own result. The grant has the model as its scope, so it lasts until a revoke or the removal of the secret.
  - A charm names a relation, and optionally a unit of the related application. The grant has the relation as its scope, so removing the relation removes the grant. A revoke names an application or a unit, or a relation, which stands for the related application.
  - A unit that a grant names belongs to the related application.
  - A grant to an application covers its units, so a grant to a unit of an application that has the role in that scope changes nothing. A grant to an application is refused while one of its units has a grant in that scope.
  - A grant keeps its subject type, scope type, and scope ID. A repeated grant for the same subject and scope changes nothing.
  - A charm's grants and revokes apply when the hook succeeds. The controller leaves out the grants and revokes of secrets that no longer exist, and refuses those of a unit that cannot manage the secret.
- *Errors:* The first four fail the whole call. The client receives the others in the result for the application:
  - **`permission denied`**: *Trigger:* The caller lacks write access to the model. The client receives it with the code `unauthorized access`. *Remediation:* Ask a model administrator.
  - **`must specify either URI or name`**: *Trigger:* The call names neither. The client receives it with no error code. *Remediation:* Name the secret.
  - **`getting user secret for label "<name>": <reason>`**: *Trigger:* No user secret has the name. The client receives it with the code `secret not found`. *Remediation:* Use the name of an existing user secret.
  - **`secret URI "<value>" not valid`**: *Trigger:* The URI is not of the form `secret:<id>`. The client receives it with the code `not valid`. *Remediation:* Use the URI that {ref}`listing <secret-listing>` returns.
  - **`secret "<URI>" not found`**: *Trigger:* The secret does not exist. The client receives it with the code `secret not found`. *Remediation:* {ref}`List the secrets <secret-listing>` and use an existing one.
  - **`cannot change access to "<URI>" for "<application>": "<model UUID>" is not allowed to manage this secret`**: *Trigger:* The secret is a charm secret. The client receives it with the code `unauthorized access`. *Remediation:* Grant access from the charm.
  - **`cannot change access to "<URI>" for "<application>": application "<application>" not found`**: *Trigger:* The model has no such application. The client receives it with no error code. *Remediation:* Name an application of the model.
  - **`cannot change access to "<URI>" for "<application>": cannot change a secret permission scope or subject type`**: *Trigger:* The application has a grant with another scope, such as a grant from a charm. The client receives it with no error code. *Remediation:* Revoke that grant first.
  - **`this unit is not the leader`**: *Trigger:* A unit that is not the leader grants or revokes access to an application secret. The hook command fails. *Remediation:* Run it on the leader.
  - **`secret "<id>" not found`**: *Trigger:* A hook grants a secret that the unit does not own. The hook command fails. *Remediation:* Use the URI of a secret the unit owns.
  - **`no relation id specified`**: *Trigger:* The hook command names no relation. The hook command fails. *Remediation:* Name the relation.
  - **`cannot specify unit "<unit>" in relation to application "<application>"`**: *Trigger:* The unit is not in the related application. The hook command fails. *Remediation:* Name a unit of the related application.
  - **`any unit level grants need to be revoked before granting access to the corresponding application`**: *Trigger:* A hook grants an application whose unit already has a grant in that scope. The hook command fails. *Remediation:* Revoke the unit grants first.

(secret-updating)=
### Updating secrets

```{ibnote}
See also: {ref}`Juju | Manage secrets <manage-secrets>`
```

Updating secrets changes the content or the metadata of a secret. New content adds a revision. A client updates user secrets. A charm updates its own secrets with the hook command {ref}`secret-set <hook-command-secret-set>`.

The operation's required access, rules, and errors are:

- *Required access:* For a user secret, {ref}`write access <user-access-model-write>` to the model. For a charm secret, a unit agent, for its own unit, which is the owning unit or the leader of the owning application and has the role `manage` on the secret. No block applies to a client.
- *Rules:*
  - The call takes a list of updates. Each update in the list gets its own result.
  - An update names the secret by its URI or by its label, not by both, and changes at least one attribute.
  - A client can change the description, the label, the content, and auto-prune. A charm can change the description, the label, the content, the rotation policy, and the expiry time.
  - Content whose checksum differs from the latest revision's adds a revision. The same content adds none.
  - An expiry time applies to the latest revision.
  - An update makes each revision obsolete that no unit tracks and that is not the latest. Auto-prune on a user secret lets the controller remove the obsolete revisions.
  - A rotation policy that is more frequent than the current one schedules the next rotation one interval after the update. The policy `never` removes the schedule.
  - A charm's update applies when the hook succeeds. The controller skips an update of a secret that no longer exists.
  - The rules for {ref}`adding <secret-adding>` apply to labels and content.
- *Errors:* The client receives each in the result for the update, except `permission denied`, which fails the whole call:
  - **`permission denied`**: *Trigger:* The caller lacks write access to the model. The client receives it with the code `unauthorized access`. *Remediation:* Ask a model administrator.
  - **`at least one attribute to update must be specified`**, **`must specify either URI or label`**, and **`must specify either URI or label but not both`**: *Trigger:* The update has nothing to change, or does not name the secret exactly once. The client receives them with no error code. *Remediation:* Name the secret once, and change an attribute.
  - **`secret URI "<value>" not valid`**: *Trigger:* The URI is not of the form `secret:<id>`. The client receives it with the code `not valid`. *Remediation:* Use the URI that {ref}`listing <secret-listing>` returns.
  - **`secret "<URI>" not found`**: *Trigger:* The secret does not exist. The client receives it with the code `secret not found`. *Remediation:* {ref}`List the secrets <secret-listing>` and use an existing one.
  - **`getting user secret for label "<label>": <reason>`**: *Trigger:* No user secret has the label. The client receives it with the code `secret not found`. *Remediation:* Use the name of an existing user secret.
  - **`cannot update secret "<URI>": "<model UUID>" is not allowed to manage this secret`**: *Trigger:* The secret is a charm secret. The client receives it with the code `unauthorized access`. *Remediation:* Update the secret from its charm.
  - **`secret with name "<name>" already exists`**: *Trigger:* Another user secret has the label. The client receives it with the code `already exists`. *Remediation:* Choose another name.
  - **`this unit is not the leader`**: *Trigger:* A unit that is not the leader updates an application secret. The hook command fails. *Remediation:* Run it on the leader.

(secret-rotating)=
### Rotating secrets

```{ibnote}
See also: {ref}`Juju | Manage secrets <manage-secrets>`
```

Rotating secrets asks the owner of a charm secret to add a revision when the secret's rotation is due. Expiry works alike: it asks the owner to remove a revision when the revision's expiry time has passed. Juju runs a hook on the owner for each: {ref}`secret-rotate <hook-secret-rotate>` and {ref}`secret-expired <hook-secret-expired>`.

The operation's required access, rules, and errors are:

- *Required access:* A unit agent, for its own unit. The secret is owned by the unit or, when the unit is the leader, by its application.
- *Rules:*
  - The unit agent reports a rotation with the call `SecretsRotated` of the `SecretsManager` facade, which sets the next rotation.
  - The next rotation is one interval of the policy after the one that was due. The policy `never` sets none.
  - A hook that adds no revision makes the next rotation five minutes after the one that was due, so that the hook runs again.
  - A latest revision that expires before the next rotation makes the next rotation five minutes after the one that was due, when that time is before the expiry. The owner can then add a revision before the expiry.
  - A revision that remains after the `secret-expired` hook makes Juju run the hook again five minutes after the expiry time.
- *Errors:* The unit agent receives each in the result for the secret:
  - **`secret "<URI>" not found`**: *Trigger:* The secret does not exist. The agent receives it with the code `secret not found`. *Remediation:* Stop rotating the secret.
  - **`cannot rotate secret "<URI>": "<unit>" is not allowed to manage this secret`**: *Trigger:* The unit does not own the secret, and is not the leader of the owning application. The agent receives it with the code `unauthorized access`. *Remediation:* Rotate the secret on its owner.

(secret-removing)=
### Removing secrets

```{ibnote}
See also: {ref}`Juju | Manage secrets <manage-secrets>`
```

Removing secrets deletes a secret with all its revisions, or deletes chosen revisions of it. The controller schedules the removal, so it finishes after the call returns. A client removes user secrets. A charm removes its own secrets and revisions with the hook command {ref}`secret-remove <hook-command-secret-remove>`.

The operation's required access, rules, and errors are:

- *Required access:* For a user secret, {ref}`write access <user-access-model-write>` to the model. For a charm secret, a unit agent, for its own unit, which is the owning unit or the leader of the owning application and has the role `manage` on the secret. No block applies to a client.
- *Rules:*
  - The call takes a list of secrets. Each secret in the list gets its own result. A call with an empty list does nothing.
  - A request names the secret by URI or by label, and can list the revisions to remove. Without revisions, the removal covers all of them.
  - Removing the last revision removes the secret with its owner, consumers, grants, and rotation.
  - A removal deletes the content from the secret backend that holds it. The controller logs a failure of the backend and goes on.
  - Removing an application or a unit removes the secrets it owns. Removing a relation removes the grants that have the relation as scope.
  - A charm's removal applies when the hook succeeds. The controller skips a secret that no longer exists. The removal cancels the other changes that the hook made to that secret.
  - The owner of a charm secret receives the {ref}`secret-remove <hook-secret-remove>` hook for each obsolete revision.
- *Errors:* The client receives each in the result for the secret, except `permission denied`, which fails the whole call:
  - **`permission denied`**: *Trigger:* The caller lacks write access to the model. The client receives it with the code `unauthorized access`. *Remediation:* Ask a model administrator.
  - **`secret URI "<value>" not valid`**: *Trigger:* The URI is not of the form `secret:<id>`. The client receives it with the code `not valid`. *Remediation:* Use the URI that {ref}`listing <secret-listing>` returns.
  - **`secret "<URI>" not found`**: *Trigger:* The secret does not exist. The client receives it with the code `secret not found`. *Remediation:* {ref}`List the secrets <secret-listing>` and use an existing one.
  - **`secret revision not found: secret:<id>/<revision>`**: *Trigger:* The secret has no such revision. The client receives it with no error code. *Remediation:* {ref}`List the secret <secret-listing>` and use a revision it has.
  - **`cannot remove secret "<URI>": "<model UUID>" is not allowed to manage this secret`**: *Trigger:* The secret is a charm secret. The client receives it with the code `unauthorized access`. *Remediation:* Remove the secret from its charm.
  - **`this unit is not the leader`**: *Trigger:* A unit that is not the leader removes an application secret. The hook command fails. *Remediation:* Run it on the leader.

## Secrets and model migration

A {ref}`migration <model-migrating>` moves the secrets of a model with their revisions, content, consumers, and grants. A revision whose content a secret backend holds needs that backend on the target controller. A migration to a controller without it fails with `target controller does not have all required secret backends set up, missing "<backend ID>"`. The built-in `kubernetes` backend of a model on a Kubernetes cloud becomes the target's built-in `kubernetes` backend.
