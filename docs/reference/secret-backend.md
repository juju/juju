---
myst:
  html_meta:
    description: "Juju secret backend reference: what a secret backend is, the records the controller keeps about backends, the changes that move secrets between backends, and the calls that add, list, set, update, and remove them."
---

(secret-backend)=
# Secret backend

```{ibnote}
See also: {ref}`Juju | Manage secret backends <manage-secret-backends>`
```

A **secret backend** is a service that stores the content of {ref}`secrets <secret>`, such as a Vault server (HashiCorp's secret store) or a Kubernetes namespace (a named group of Kubernetes resources), registered on a {ref}`controller <controller>` under a name. Every {ref}`model <model>` uses one backend for the secrets it creates.

(secret-backend-database-schema)=
## Secret backend database schema

```{ggarch}
:file: ../juju.ggarch
:view: Secret backend database schema
:caption: The controller database keeps one record for each secret backend. A backend has one backend type (a type has many backends), many settings (a setting belongs to one backend), and at most one token rotation record (a rotation belongs to one backend). A model has at most one model backend record, which names the backend the model uses (a backend can serve many models), and the database keeps one secret reference for each secret revision of a model that a backend holds (a backend and a model each have many references). The model is described on its own page and the figure shows it only for context: all boxes are records, and the one with a second, outlined arrow mark in the corner is the record that another page describes.
:alt: Seven boxes of the same size, each with a filled table mark in the corner. Six are inside a group named Controller database, and Model is outside it with an outlined arrow mark for a record that another page describes. Secret backend is in the middle. Backend type is to its right (m to 1), Backend setting is below it (1 to m), Token rotation is below and to the left (1 to at most 1), Secret reference is above it (1 to m), and Model backend is to its left (m to 1). Model is to the left of Model backend (1 to at most 1) and also points at Secret reference through a line above (1 to m).
```

The secret backend database schema is the set of records that the {ref}`controller database <database>` keeps about the backends of a controller and about which models and secrets use them. The records, with the rules that apply to each, are:

- **Secret backend:** A backend's name, its type, and the interval at which its access token is replaced. Every controller has two built-in backends, marked as built in: `internal`, which keeps content in the database, and `kubernetes`, which keeps it in the model's namespace on a Kubernetes cloud.
  - *Rules:*
    - A name is not empty and appears once.
    - A built-in backend cannot be changed or deleted.
- **Backend type:** One of `controller`, `kubernetes`, and `vault`. The built-in `internal` backend has the type `controller`.
  - *Rules:*
    - A type appears once. Its name is not empty.
- **Backend setting:** One configuration key of a backend and its value, such as the `endpoint`.
  - *Rules:*
    - A backend has each key once, and neither the key nor the value is empty.
- **Token rotation:** The time at which the backend's access token is next replaced.
  - *Rules:*
    - A backend has at most one.
- **Model backend:** The backend that a model uses for the secrets it creates.
  - *Rules:*
    - A model has at most one.
- **Secret reference:** A record that one secret revision of one model has its content in one backend.
  - *Rules:*
    - A backend holds a secret revision of a model once.
- **Model:** The record at the far end of the model backend and the secret reference. Another page owns it: see {ref}`model <model>`.

(secret-backend-change-streams)=
## Secret backend change streams

Secret backend change streams are the changes that the workers (background processes) of a controller and the {ref}`agents <agent>` of a model follow, so that nothing has to poll. A change stream is delivered through a {ref}`watcher <watcher>`. The controller follows the token rotation records of the backends, while the agents of a model follow the model's choice of backend:

- **Token rotation:** The primary controller (the one controller that runs the controller's single-instance workers, see {ref}`controller workers <controller-workers>`) follows the token rotation records through the watcher `WatchSecretBackendRotationChanges`. It sets one timer for the earliest due time, and when the timer fires it replaces the token of each due backend with a new one that the backend issues.
- **Model backend:** The unit agents and the model's own workers follow the model's backend record. When it changes, they move the content of the secrets they own from the old backend to the new one, as {ref}`Setting and getting model secret backends <secret-backend-setting>` describes. A unit agent moves the secrets of its unit and, on the leader, those of its application, and the model's workers move the user secrets.

(secret-backend-calls)=
## Secret backend calls

Secret backend calls are the calls that a {ref}`client <client>` can take on secret backends through the controller API (the set of calls that clients and agents make to the controller): add, list, update, remove, set a model's backend, and get a model's backend. Calls on the secrets themselves are described on the {ref}`secret <secret>` page.

(secret-backend-adding)=
### Adding secret backends

```{ibnote}
See also: {ref}`Juju | Add a secret backend <add-a-secret-backend>`
```

Adding secret backends registers external backends on the controller. A backend type is one of the registered types: `controller`, `kubernetes`, and `vault`. The controller checks the backend's configuration and that the backend answers before it stores anything.

The caller, the rules, and the errors are:

- *Caller:* A client, for a user with {ref}`superuser access <user-access-controller-superuser>` to the controller.
- *Rules:*
  - The name cannot be `auto`, `internal`, or `kubernetes`, and cannot end in `-local`.
  - The configuration must contain the keys that the type requires and no key that the type does not know. The keys of a `vault` backend are:
    - `endpoint` (required): the address of the Vault service, a URL.
    - `token`: the access token.
    - `namespace`: the namespace in which to store secrets.
    - `mount-path`: the mount path of the secret store.
    - `ca-cert`: the certificate authority (CA) certificate with which to verify the Vault server.
    - `client-cert` and `client-key`: the client certificate and its key, which must come together.
    - `tls-server-name`: the server name for Transport Layer Security (TLS).
  - The keys of a `kubernetes` backend are:
    - `endpoint` (required): the address of the Kubernetes API server, a URL.
    - `namespace` (required): the namespace in which to store secrets, which must already exist.
    - `ca-cert` and `ca-certs`: a CA certificate, or a list of them for a certificate chain.
    - `token`, `username`, and `password`: the credentials.
    - `service-account`: the service account from which the controller requests a new token. The account must be bound to a cluster role that grants the access the controller needs.
    - `client-cert` and `client-key`: the client certificate and its key, which must come together.
    - `skip-tls-verify`: do not verify the server's certificate.
    - `prefer-incluster-address`: prefer the address from inside the cluster.
  - The controller contacts the backend and refuses the request if the backend does not answer.
  - A token rotation interval makes the controller replace the backend's token before it expires. The interval must be at least one hour, the type must support replacing tokens (`vault` and `kubernetes` do), and a `kubernetes` backend also needs a `service-account`. The controller replaces the token after three quarters of the interval, and at most 24 hours after the last replacement.
  - A request that names the ID of an existing external backend updates that backend instead of failing. The ID of a built-in backend is refused.
  - The call takes a list of backends. Each backend in the list gets its own result.
- *Errors:* The client receives each as its own error code with this message, where `<name>` is the backend's name:
  - **`secret backend "<name>" already exists`**: *Trigger:* A backend with that name exists. The client receives it with the code `secret backend already exists`. *Remediation:* Choose another name, or update the existing backend.
  - **`cannot add secret backend "<name>": secret backend not valid: reserved name "<name>"`**: *Trigger:* The name is reserved. The client receives it with the code `secret backend not valid`. *Remediation:* Choose a name that is not reserved.
  - **`cannot add secret backend "<name>": secret backend not valid: config for provider "<type>": <detail>`**: *Trigger:* A required key is missing, a key is unknown, a certificate and key are not paired, or a `kubernetes` backend with a rotation interval has no `service-account`. The detail names the key. The client receives it with the code `secret backend not valid`. *Remediation:* Supply the missing key, or remove the unknown one.
  - **`cannot add secret backend "<name>": secret backend not valid: <detail>`**: *Trigger:* The name is empty, or a key or a value of the configuration is empty. The client receives it with the code `secret backend not valid`. *Remediation:* Give a non-empty value.
  - **`cannot add secret backend "<name>": secret backend not supported: token refresh on secret backend of type "<type>"`**: *Trigger:* A rotation interval on a type that cannot replace tokens. The client receives it with the code `secret backend not supported`. *Remediation:* Leave the interval out.
  - **`token rotate interval "<interval>" less than 1h not valid`**: *Trigger:* The interval is shorter than one hour. The client receives it with the code `not valid`. *Remediation:* Use an interval of one hour or more.
  - **`secret backend "<id>" is immutable`**: *Trigger:* The request names the ID of a built-in backend. The client receives it with no error code. *Remediation:* Leave out the ID, or use the ID of a backend you added.
  - **`backend not reachable: <reason>`** and other messages from the backend (such as `vault is sealed` or `auth token invalid: permission denied`): *Trigger:* The backend does not answer, or refuses the credentials. The client receives them with no error code. *Remediation:* Make the backend reachable, and correct the credentials.
  - **`permission denied`**: *Trigger:* The caller is not a superuser. The client receives it with the code `unauthorized access`. *Remediation:* Ask a superuser.

(secret-backend-listing)=
### Listing secret backends

```{ibnote}
See also: {ref}`Juju | View all the secret backends available on a controller <view-all-the-secret-backends-available-on-a-controller>`, {ref}`Juju | View details about a secret backend <view-details-about-a-secret-backend>`
```

Listing secret backends returns the backends of the controller, each with its type, its token rotation interval, its configuration, the number of secrets it holds, and its status. A request can name the backends it wants.

The caller and the rules are:

- *Caller:* A client, for a user that is logged in. A request that asks for secret values to be shown needs a user with {ref}`superuser access <user-access-controller-superuser>` to the controller. A user without it receives `permission denied`.
- *Rules:*
  - A backend's status is `active`, except for a backend of the type `vault`, which the controller contacts: if it does not answer, the status is `error` and the message is the reason.
  - The configuration leaves out the secret keys (`token` and `client-key` for `vault`, and `token`, `password`, and `client-key` for `kubernetes`) unless the request asks to show them.
  - The list does not show the built-in `kubernetes` backend under that name. It shows one backend named `<model>-local` for each Kubernetes model that holds secrets, with the configuration of the model's cloud and credential.

(secret-backend-setting)=
### Setting and getting model secret backends

```{ibnote}
See also: {ref}`Juju | Set or get the secret backend for a model <set-or-get-the-secret-backend-for-a-model>`
```

Setting and getting model secret backends chooses, or reports, the backend that a model uses for the secrets it creates. Setting a backend does not move the secrets that already exist: the agents of the model do that, as {ref}`secret backend change streams <secret-backend-change-streams>` describe.

The caller, the rules, and the errors are:

- *Caller:* A client, for a user with {ref}`read access <user-access-model-read>` to the model to get the backend, and with {ref}`write access <user-access-model-write>` to set it.
- *Rules:*
  - The name `auto` chooses `internal` on a machine model and `kubernetes` on a Kubernetes model, and getting the backend reports those two as `auto`.
  - The names `internal` and `kubernetes`, and any name that ends in `-local`, are refused: use `auto` to return to the default.
- *Errors:*
  - **`secret backend "<name>" not found`**: *Trigger:* No backend has that name. The client receives it with the code `secret backend not found`. *Remediation:* {ref}`Add the backend <secret-backend-adding>` first, or name an existing one.
  - **`secret backend "<name>" not valid`**: *Trigger:* The name is `internal`, `kubernetes`, or ends in `-local`. The client receives it with the code `secret backend not valid`. *Remediation:* Use `auto`.
  - **`missing backend name`**: *Trigger:* The request names no backend. The client receives it with no error code. *Remediation:* Name a backend, or `auto`.
  - **`setting model secret backend for "<model UUID>": cannot set secret backend "<name>" for model "<model UUID>": model not found`**: *Trigger:* The model does not exist. The client receives it with the code `model not found`. *Remediation:* Use the UUID of an existing model.
  - **`permission denied`**: *Trigger:* The caller lacks the access above. *Remediation:* Ask a model administrator.

(secret-backend-updating)=
### Updating secret backends

```{ibnote}
See also: {ref}`Juju | Update a secret backend <update-a-secret-backend>`
```

Updating secret backends renames a backend, changes its token rotation interval, and changes or removes keys of its configuration. A backend that models already use keeps serving them.

The caller, the rules, and the errors are:

- *Caller:* A client, for a user with {ref}`superuser access <user-access-controller-superuser>` to the controller.
- *Rules:*
  - A request names the backend by its current name.
  - The new configuration is merged into the old: supplied keys replace existing ones, and keys that the request lists for removal are deleted.
  - The same rules as for adding apply to the new name, the merged configuration, and the interval.
  - The `endpoint`, `namespace`, and `mount-path` of a `vault` backend, and the `endpoint` and `namespace` of a `kubernetes` backend, cannot change.
  - The controller contacts the backend and refuses the request if the backend does not answer, unless the request forces the update.
  - The built-in backends cannot be changed.
- *Errors:* The client receives each as its own error code with this message:
  - **`secret backend "<name>" not found`**: *Trigger:* No backend has that name. The client receives it with the code `secret backend not found`. *Remediation:* {ref}`List the backends <secret-backend-listing>` and use an existing name.
  - **`cannot rename secret backend "<name>": secret backend already exists: name "<new name>"`**: *Trigger:* Another backend has the new name. The client receives it with the code `secret backend already exists`. *Remediation:* Choose another name.
  - **`cannot update secret backend "<name>": secret backend "<id>" is immutable`**: *Trigger:* The backend is built in. The client receives it with the code `secret backend forbidden`. *Remediation:* Add a backend of your own instead.
  - **`cannot update secret backend "<name>": secret backend not valid: <detail>`**: *Trigger:* The new name is reserved or empty, a value is empty, or the configuration fails validation, including a change to a key that cannot change (`cannot change immutable field "<key>"`). The client receives it with the code `secret backend not valid`. *Remediation:* Correct the request, or add a new backend and set it on the models.
  - **`token rotate interval "<interval>" less than 1h not valid`**: *Trigger:* The interval is shorter than one hour. The client receives it with the code `not valid`. *Remediation:* Use an interval of one hour or more.
  - **`token refresh on secret backend of type "<type>" not supported`**: *Trigger:* The type cannot replace tokens. The client receives it with the code `not supported`. *Remediation:* Leave the interval out.
  - **`backend not reachable: <reason>`** and other messages from the backend: *Trigger:* The backend does not answer. The client receives them with no error code. *Remediation:* Make the backend reachable, or force the update.
  - **`permission denied`**: *Trigger:* The caller is not a superuser. *Remediation:* Ask a superuser.

(secret-backend-removing)=
### Removing secret backends

```{ibnote}
See also: {ref}`Juju | Remove a secret backend <remove-a-secret-backend>`
```

Removing secret backends deletes backends from the controller and returns the models that used them to their default backend.

The caller, the rules, and the errors are:

- *Caller:* A client, for a user with {ref}`superuser access <user-access-controller-superuser>` to the controller.
- *Rules:*
  - A backend that holds the content of any secret revision cannot be removed, unless the request forces the removal. A backend that models use but that holds no content can be removed.
  - A removal deletes the backend's settings and token rotation, resets each model that used the backend to its default backend, which starts the move of those models' secrets (see {ref}`secret backend change streams <secret-backend-change-streams>`), and deletes the backend's secret references. A forced removal deletes the backend's secret references, so the controller no longer tracks the content that remains in the backend.
  - The built-in backends cannot be removed.
- *Errors:* The client receives each as its own error code with this message:
  - **`deleting in use secret backend not supported`**: *Trigger:* The backend holds secret content, or the backend is built in. The client receives it with the code `not supported`. *Remediation:* For a backend that holds content, set another backend on the models that use it, wait until their secrets have moved, and retry, or force the removal. For a built-in backend, leave it in place.
  - **`secret backend "<name>" not found`**: *Trigger:* No backend has that name. The client receives it with the code `secret backend not found`. *Remediation:* Use the name of an existing backend, and {ref}`list the backends <secret-backend-listing>` first.
  - **`cannot remove secret backend "": secret backend not valid: both ID and name are missing`**: *Trigger:* The request names no backend. The client receives it with the code `secret backend not valid`. *Remediation:* Name a backend.
  - **`permission denied`**: *Trigger:* The caller is not a superuser. *Remediation:* Ask a superuser.

## Secret backends and model migration

A {ref}`migration <model-migrating>` does not move secret backends. The target controller must already have a backend with the name of the model's backend. A migration to a controller that lacks one fails its prechecks (the checks before a migration starts) with `model's secret backend "<name>" not found on target controller`. The model's secret references are created again on the target, each pointing at the target's backend of that name.
