---
myst:
  html_meta:
    description: "Juju external controller reference: what a record of another controller is, the records the controller keeps about it, the change stream that follows them, and how they come and go."
---

(external-controller)=
# External controller

```{ibnote}
See also: {ref}`Juju | Integrate with an offer <integrate-with-an-offer>`
```

An **external controller** is a {ref}`controller <controller>` that this controller keeps a record of in order to reach it: to use the {ref}`offers <offer>` that it hosts, and to follow a {ref}`model <model>` that moved to it. The {ref}`controller database <database>` holds one record for each external controller, with the addresses of its API servers, the certificate that verifies them, and the models on it that models of this controller use. A record is written when a client consumes an offer and when a model migrates. No operation removes it.

(external-controller-database-schema)=
## External controller database schema

```{ggarch}
:file: ../juju.ggarch
:view: External controller database schema
:caption: One record per external controller, one address per API server, one row per model on it that this controller's models use. The addresses say how to reach it; the model rows say which of its models are in use here.
:alt: Three boxes of the same size inside a group named Controller database, each with a filled table mark in the corner. External controller is on the left, External controller address is to its right (1 to m), and External model is below the address (1 to m).
```

The external controller database schema is the set of records that the controller database keeps about external controllers. The records, with the rules that apply to each, are:

- **External controller:** The other controller's UUID, an alias (a name that a person can read), and the CA certificate that verifies the TLS certificate of the external controller's API servers.
  - *Rules:*
    - The certificate is always present.
    - The controller keeps one record per external controller.
- **External controller address:** One address, in the form `host:port`, of an API server of the external controller.
  - *Rules:*
    - The record of a controller holds each of its addresses once.
- **External model:** The UUID of a model that lives on the external controller and that a model of this controller reaches over a {ref}`cross-model relation <cross-model-relation>` — normally the model that hosts an offer that a model of this controller consumes.
  - *Rules:*
    - A model appears once. It belongs to one external controller.

(external-controller-change-streams)=
## External controller change streams

External controller change streams are the changes to external controller records that the workers of the controller follow. The worker that keeps the records current holds the watcher: when the stream names a controller that the worker does not follow yet, the worker starts one follower for that controller; when the stream names a controller that it already follows, the worker stops that follower (see {ref}`refreshing external controller addresses <external-controller-refreshing>`). A worker that calls a remote relation reads the record of the controller that hosts the model it calls, at the moment it connects.

(external-controller-calls)=
## External controller calls

External controller calls are the calls that write and read the records. A client reaches the records through {ref}`consuming an offer <offer-consuming>` and through asking for the API addresses of a model ({ref}`reading a model's controller <external-controller-reading>`). A worker of the controller keeps them current. A migration writes them again.

(external-controller-recording)=
### Recording external controllers

Recording external controllers saves the controller that hosts an offer, when a client consumes the offer. The record replaces the alias, the certificate, and the whole address list of the controller, and adds the model that hosts the offer to its models.

The caller, the rules, and the errors are:

- *Caller:* A client, for a user with the same access as for {ref}`consuming an offer <offer-consuming>`: {ref}`write access <user-access-model-write>` to the consuming model, with no block on changes.
- *Rules:*
  - A consume that carries no controller info writes no record.
  - An offer hosted by this controller leaves no record: consuming it changes nothing here.
  - The models of a controller accumulate: a consume adds the model that hosts the offer. No operation removes a model record.
  - An address that the new list does not name is removed; the new list can be empty.
- *Errors:* The client receives each in the result for the offer:
  - **`saving external controller info: updating external controller "<uuid>": <reason>`**: *Trigger:* The record cannot be stored. The client receives it with no error code. *Remediation:* Read the reason in the controller's log and fix its cause.
  - The tag errors of {ref}`consuming an offer <offer-consuming>`: a controller tag of another kind, or one that cannot be parsed.

(external-controller-reading)=
### Reading a model's controller

Reading a model's controller returns the API addresses and the certificate of the controller that hosts a model. A client or an agent calls `ControllerAPIInfoForModels` for it.

The caller and the rules are:

- *Caller:* Any logged-in client or agent. No model access level applies.
- *Rules:*
  - A model on this controller returns this controller's addresses.
  - A model on an external controller returns the addresses and the certificate from its record.
  - A model that moved to another controller and has no record makes the controller follow the model's redirection, record the controller the model moved to, and return its addresses.

(external-controller-refreshing)=
### Refreshing external controller addresses

Refreshing external controller addresses keeps the address list of each record current. A worker of the controller does it: for each external controller it connects to the controller as an anonymous client, calls `WatchControllerInfo` on it, and saves the new address list when the controller reports a change.

The caller and the rules are:

- *Caller:* The controller itself. No client or agent calls it.
- *Rules:*
  - The worker keeps the alias and the certificate of the record, and replaces only the addresses.
  - A controller that cannot report its own addresses, such as one behind a load balancer, is left as it is.

(external-controller-and-model-migration)=
## External controllers and model migration

A migration writes the records at both ends. The controller that receives a model records the controller the model came from, together with the controllers that host the offers the model consumes; it refuses to overwrite a record that holds a different alias, certificate, or address list, and refuses to move a model that another controller already hosts, which fails the import. The controller that a model leaves records the controller the model moves to when the migration starts. It keeps the records working for the consumers that stay: the workers that call the migrated model follow its redirection and record the controller it moved to. See {ref}`Migrating models <model-migrating>`.
