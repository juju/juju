---
myst:
  html_meta:
    description: "Juju secret backend reference: what a secret backend is, the records the controller keeps about backends, the changes that move secrets between backends, and the operations that add, list, set, update, and remove them."
---

(secret-backend)=
# Secret backend

```{ibnote}
See also: {ref}`Juju | Manage secret backends <manage-secret-backends>`
```

A **secret backend** is a service that stores the content of {ref}`secrets <secret>`, such as a Vault server or a Kubernetes namespace, registered on a {ref}`controller <controller>` under a name. Every {ref}`model <model>` uses exactly one backend for the secrets it creates, and the controller keeps no content of its own for a secret that lives in an external backend: it keeps only which backend holds each secret revision. Two backends exist on every controller: `internal`, which keeps content in the database, and `kubernetes`, which keeps content in the model's namespace on a Kubernetes cloud. A new model uses `internal` on a machine cloud and `kubernetes` on a Kubernetes cloud.

(secret-backend-data-model)=
## Secret backend data model

```{ggarch}
:file: ../juju.ggarch
:view: Secret backend data model
:caption: The controller database keeps one record for each secret backend. A backend has one backend type (a type has many backends), many settings (a setting belongs to one backend), and at most one token rotation record (a rotation belongs to one backend). A model has at most one model backend record, which names the backend the model uses (a backend can serve many models), and the database keeps one secret reference for each secret revision of a model that a backend holds (a backend and a model each have many references). The model is described on its own page and the figure shows it only for context: all boxes are records, and the one with a second, outlined arrow mark in the corner is the record that another page describes.
:alt: Seven boxes of the same size, each with a filled table mark in the corner. Six are inside a group named Controller database, and Model is outside it with an outlined arrow mark for a record that another page describes. Secret backend is in the middle. Backend type is to its right (m to 1), Backend setting is below it (1 to m), Token rotation is below and to the left (1 to 1), Secret reference is above it (1 to m), and Model backend is to its left (m to 1). Model is to the left of Model backend (1 to 1) and also points at Secret reference through a line above (1 to m).
```

The secret backend data model is the set of records that the {ref}`controller database <database>` keeps about the backends of a controller and about which models and secrets use them. The records, with the rules that apply to each, are:

- **Secret backend:** A backend's name, its type, and the interval at which its access token is replaced. The two built-in backends, `internal` and `kubernetes`, are marked as built in.
  - *Rules:*
    - A name is not empty and appears once.
    - A built-in backend cannot be changed or deleted.
- **Backend type:** One of `controller`, `kubernetes`, and `vault`. The built-in `internal` backend has the type `controller`.
  - *Rules:*
    - A type appears once, and its name is not empty.
- **Backend setting:** One configuration key of a backend and its value, such as the `endpoint`.
  - *Rules:*
    - A backend has each key once, and neither the key nor the value is empty.
- **Token rotation:** The time at which the backend's access token is next replaced.
  - *Rules:*
    - A backend has at most one.
- **Model backend:** The backend that a model uses for the secrets it creates.
  - *Rules:*
    - A model has at most one.
- **Secret reference:** A record that one secret revision of one model has its content in one backend, which lets the controller count the secrets of a backend and refuse to remove a backend that holds some.
  - *Rules:*
    - A backend holds a secret revision of a model once.
- **Model:** The record at the far end of the model backend and the secret reference. Another page owns it: see {ref}`model <model>`.

A secret backend is a row in the table `secret_backend`, its type comes from the lookup table `secret_backend_type`, and its settings are rows in `secret_backend_config`. A token rotation is a row in `secret_backend_rotation`, a model's backend is a row in `model_secret_backend`, and a secret reference is a row in `secret_backend_reference`. {ref}`juju-db-repl` shows the definition of any of them with `.ddl <name>`.

(secret-backend-change-streams)=
## Secret backend change streams

Secret backend change streams are the changes that the workers of a controller and the agents of a model follow, so that nothing has to poll. A change stream is delivered through a watcher (an API object that tells its holder when something it follows has changed).

- **Token rotation:** The `secret-backend-rotate` worker of the primary controller (the one controller that runs the controller's single-instance workers, see {ref}`controller workers <controller-workers>`) follows the token rotation records. It sets one timer for the earliest due time, and when the timer fires it replaces the token of each due backend with a new one that the backend issues.
- **Model backend:** The unit agents and the model's own workers follow the model's backend record. When it changes, they move the content of the secrets they own from the old backend to the new one, as {ref}`Setting and getting model secret backends <secret-backend-setting>` describes. A unit agent moves the secrets of its unit and, on the leader, those of its application, and the model's workers move the user secrets.

(secret-backend-operations)=
## Secret backend operations

Secret backend operations are the actions a {ref}`client <client>` can take on secret backends through the controller API (the set of calls that clients and {ref}`agents <agent>` make to the controller): add, list, set and get a model's backend, update, and remove. Operations on the secrets themselves are described on the {ref}`secret <secret>` page. No operation on a secret backend checks a block.

(secret-backend-adding)=
### Adding secret backends

```{ibnote}
See also: {ref}`Juju | Add a secret backend <add-a-secret-backend>`
```

Adding secret backends registers external backends on the controller. A backend type is `vault` or `kubernetes`, and the controller checks the backend's configuration and that the backend answers before it stores anything.

The operation's required access, rules, and errors are:

- *Required access:* {ref}`Superuser access <user-access-controller-superuser>` to the controller.
- *Rules:*
  - The name cannot be `auto`, `internal`, or `kubernetes`, and cannot end in `-local`.
  - The configuration must contain the keys that the type requires and no key that the type does not know. The keys of a `vault` backend are:
    - `endpoint` (required): the address of the Vault service, a URL.
    - `token`: the access token.
    - `namespace`: the namespace in which to store secrets.
    - `mount-path`: the mount path of the secret store.
    - `ca-cert`: the CA certificate with which to verify the Vault server.
    - `client-cert` and `client-key`: the client certificate and its key, which must come together.
    - `tls-server-name`: the server name for TLS.
  - The keys of a `kubernetes` backend are:
    - `endpoint` (required): the address of the Kubernetes API server, a URL.
    - `namespace` (required): the namespace in which to store secrets, which must already exist.
    - `ca-cert` and `ca-certs`: a CA certificate, or a list of them for a certificate chain.
    - `token`, or `username` and `password`: the credentials.
    - `service-account`: the service account from which the controller requests a new token. The account needs a cluster role that allows access to namespaces, cluster roles, cluster role bindings, secrets, service accounts, and service account tokens.
    - `client-cert` and `client-key`: the client certificate and its key, which must come together.
    - `skip-tls-verify`: do not verify the server's certificate, for testing only.
    - `prefer-incluster-address`: prefer the address from inside the cluster.
  - The controller contacts the backend and refuses the request if the backend does not answer.
  - A token rotation interval makes the controller replace the backend's token before it expires. The interval must be at least one hour, the type must support replacing tokens (`vault` and `kubernetes` do), and a `kubernetes` backend also needs a `service-account`. The controller replaces the token after three quarters of the interval, and at most 24 hours after the last replacement.
  - A request that names the ID of an existing external backend updates that backend instead of failing. The ID of a built-in backend is refused.
  - The request can carry a batch of backends, and the answer has one result for each.
- *Errors:* The client receives each as its own error code with this message, where `<name>` is the backend's name:
  - **`secret backend "<name>" already exists`**: *Trigger:* A backend with that name exists. The client receives it with the code `secret backend already exists`. *Remediation:* Choose another name, or update the existing backend.
  - **`cannot add secret backend "<name>": secret backend not valid: reserved name "<name>"`**: *Trigger:* The name is reserved. The client receives it with the code `secret backend not valid`. *Remediation:* Choose a name that is not reserved.
  - **`cannot add secret backend "<name>": secret backend not valid: config for provider "<type>": <detail>`**: *Trigger:* A required key is missing, a key is unknown, a certificate and key are not paired, or a `kubernetes` backend with a rotation interval has no `service-account`. The detail names the key. The client receives it with the code `secret backend not valid`. *Remediation:* Supply the missing key, or remove the unknown one.
  - **`cannot add secret backend "<name>": secret backend not valid: <detail>`**: *Trigger:* The name, the type, or a value of the configuration is empty. The client receives it with the code `secret backend not valid`. *Remediation:* Give a non-empty value.
  - **`cannot add secret backend "<name>": secret backend not supported: token refresh on secret backend of type "<type>"`**: *Trigger:* A rotation interval on a type that cannot replace tokens. The client receives it with the code `secret backend not supported`. *Remediation:* Leave the interval out.
  - **`token rotate interval "<interval>" less than 1h not valid`**: *Trigger:* The interval is shorter than one hour. *Remediation:* Use an interval of one hour or more.
  - **`backend not reachable: <reason>`** and other messages from the backend (such as `vault is sealed` or `auth token invalid: permission denied`): *Trigger:* The backend does not answer, or refuses the credentials. The client receives them with no error code. *Remediation:* Make the backend reachable, and correct the credentials.
  - **`permission denied`**: *Trigger:* The caller is not a superuser. The client receives it with the code `unauthorized access`. *Remediation:* Ask a superuser.

(secret-backend-listing)=
### Listing secret backends

```{ibnote}
See also: {ref}`Juju | View all the secret backends available on a controller <view-all-the-secret-backends-available-on-a-controller>`, {ref}`Juju | View details about a secret backend <view-details-about-a-secret-backend>`
```

Listing secret backends returns the backends of the controller, each with its type, its token rotation interval, its configuration, the number of secrets it holds, and its status. A request can name the backends it wants.

The operation's required access and rules are:

- *Required access:* Any logged-in user. A request that asks for secret values to be shown needs {ref}`superuser access <user-access-controller-superuser>` to the controller, and a caller without it receives `permission denied`.
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

The operation's required access, rules, and errors are:

- *Required access:* {ref}`Read access <user-access-model-read>` to the model to get the backend, and {ref}`write access <user-access-model-write>` to set it.
- *Rules:*
  - The name `auto` chooses `internal` on a machine model and `kubernetes` on a Kubernetes model, and getting the backend reports those two as `auto`.
  - The names `internal` and `kubernetes`, and any name that ends in `-local`, are refused: use `auto` to return to the default.
  - The controller checks neither that the backend answers nor that its type fits the model.
- *Errors:*
  - **`secret backend "<name>" not found`**: *Trigger:* No backend has that name. The client receives it with the code `secret backend not found`. *Remediation:* {ref}`Add the backend <secret-backend-adding>` first, or name an existing one.
  - **`secret backend "<name>" not valid`**: *Trigger:* The name is `internal`, `kubernetes`, or ends in `-local`. The client receives it with the code `secret backend not valid`. *Remediation:* Use `auto`.
  - **`missing backend name`**: *Trigger:* The request names no backend. The client receives it with no error code. *Remediation:* Name a backend, or `auto`.
  - **`model not found`**: *Trigger:* The model does not exist. *Remediation:* Check the model.
  - **`permission denied`**: *Trigger:* The caller lacks the access above. *Remediation:* Ask a model administrator.

(secret-backend-updating)=
### Updating secret backends

```{ibnote}
See also: {ref}`Juju | Update a secret backend <update-a-secret-backend>`
```

Updating secret backends renames a backend, changes or removes keys of its configuration, and changes its token rotation interval. A backend that models already use keeps serving them.

The operation's required access, rules, and errors are:

- *Required access:* {ref}`Superuser access <user-access-controller-superuser>` to the controller.
- *Rules:*
  - A request names the backend by its current name, and the new configuration is merged into the old: supplied keys replace existing ones, and keys that the request lists for removal are deleted.
  - The same rules as for adding apply to the new name, the merged configuration, and the interval.
  - The `endpoint`, `namespace`, and `mount-path` of a `vault` backend, and the `endpoint` and `namespace` of a `kubernetes` backend, cannot change.
  - The controller contacts the backend and refuses the request if the backend does not answer, unless the request forces the update.
  - The built-in backends cannot be changed.
- *Errors:* The client receives each as its own error code with this message:
  - **`secret backend "<name>" not found`**: *Trigger:* No backend has that name. The client receives it with the code `secret backend not found`. *Remediation:* {ref}`List the backends <secret-backend-listing>` and use an existing name.
  - **`cannot rename secret backend "<name>": secret backend already exists: name "<new name>"`**: *Trigger:* Another backend has the new name. The client receives it with the code `secret backend already exists`. *Remediation:* Choose another name.
  - **`cannot update secret backend "<name>": secret backend "<id>" is immutable`**: *Trigger:* The backend is built in. The client receives it with the code `secret backend forbidden`. *Remediation:* Add a backend of your own instead.
  - **`cannot update secret backend "<name>": secret backend not valid: <detail>`**: *Trigger:* The new name is reserved or empty, a value is empty, or the configuration fails validation, including a change to a key that cannot change (`cannot change immutable field "<key>"`). The client receives it with the code `secret backend not valid`. *Remediation:* Correct the request, or add a new backend and set it on the models.
  - **`token rotate interval "<interval>" less than 1h not valid`** and **`token refresh on secret backend of type "<type>" not supported`**: *Trigger:* The interval is shorter than one hour, or the type cannot replace tokens. *Remediation:* Use an interval of one hour or more on a type that supports it.
  - **`backend not reachable: <reason>`** and other messages from the backend: *Trigger:* The backend does not answer. The client receives them with no error code. *Remediation:* Make the backend reachable, or force the update.
  - **`permission denied`**: *Trigger:* The caller is not a superuser. *Remediation:* Ask a superuser.

(secret-backend-removing)=
### Removing secret backends

```{ibnote}
See also: {ref}`Juju | Remove a secret backend <remove-a-secret-backend>`
```

Removing secret backends deletes backends from the controller and returns the models that used them to their default backend.

The operation's required access, rules, and errors are:

- *Required access:* {ref}`Superuser access <user-access-controller-superuser>` to the controller.
- *Rules:*
  - A backend that holds the content of any secret revision cannot be removed, unless the request forces the removal. A backend that models use but that holds no content can be removed.
  - A removal deletes the backend's settings and token rotation, resets each model that used the backend to its default backend, which starts the move of those models' secrets (see {ref}`secret backend change streams <secret-backend-change-streams>`), and deletes the backend's secret references. The secret content that a forced removal leaves in the backend is not moved by the controller.
  - The built-in backends cannot be removed.
- *Errors:* The client receives each as its own error code with this message:
  - **`deleting in use secret backend not supported`**: *Trigger:* The backend holds secret content, or the backend is built in. The client receives it with the code `secret backend not supported`. *Remediation:* For a backend that holds content, set another backend on the models that use it, wait until their secrets have moved, and retry, or force the removal. A built-in backend cannot be removed.
  - **`secret backend "<name>" not found`**: *Trigger:* No backend has that name. The client receives it with the code `secret backend not found`. *Remediation:* Check the name.
  - **`cannot remove secret backend "": secret backend not valid: both ID and name are missing`**: *Trigger:* The request names no backend. The client receives it with the code `secret backend not valid`. *Remediation:* Name a backend.
  - **`permission denied`**: *Trigger:* The caller is not a superuser. *Remediation:* Ask a superuser.

## Secret backends and model migration

A migration does not move secret backends. The target controller must already have a backend with the name of the model's backend, and a migration to a controller that lacks one fails its prechecks with `model's secret backend "<name>" not found on target controller`. The model's secret references are created again on the target, each pointing at the target's backend of that name.
