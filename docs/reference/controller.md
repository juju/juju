---
myst:
  html_meta:
    description: "Juju controller reference: control plane architecture, database, high availability, and multi-cloud management capabilities. The controller record, its data model, operations, and watchers."
---

(controller)=
# Controller

```{ibnote}
See also: {ref}`manage-controllers`
```

In software design, a **controller** is an architectural component responsible for managing the flow of data and interactions within a system, and for mediating between different parts of the system. In Juju, it is defined in the same way, with the mention that:

- It is set up via the boostrap process.
- It refers to the initial controller {ref}`unit <unit>` as well as any units added later on (for machine clouds, for the purpose of {ref}`high-availability <high-availability>`) -- each of which includes
    - a {ref}`unit agent <unit-agent>`,
    - [`juju-controller`](https://charmhub.io/juju-controller) charm code, and
    - a {ref}`controller agent <controller-agent>` running, among other things, the Juju API server and an in-process embedded [Dqlite](https://canonical.com/dqlite) {ref}`database <database>`. <p>
- It is responsible for implementing all the changes defined by a Juju {ref}`user <user>` via a Juju client post-bootstrap.
- It stores state in the internal Dqlite {ref}`database <database>`.

(the-controller-record)=
## The controller record

A controller is a **singleton row** in the controller database -- the
schema enforces that exactly one exists: its UUID, the UUID of its
{ref}`controller model <the-controller-model>`, its target agent
version, its API port, its certificate and CA material, and its system
identity. Two satellite records complete it: the controller
configuration (key/value) and the **controller nodes** -- one record
per Dqlite node in the {ref}`high-availability <high-availability>`
cluster, carrying the node's Dqlite identity and bind address.

(types-of-controller)=
## Types of controller

A controller has no subtypes: one deployment, one controller.
{ref}`High availability <high-availability>` does not make a second
controller -- it makes more **controller nodes** running the same
controller's database and API.

(the-controller-in-the-data-model)=
## The controller in the data model

The controller database is the controller's own record set: the
controller row, its configuration, its nodes, and everything that
lives controller-side rather than per-model -- {ref}`users <user>`,
their {ref}`access levels <user-access-levels>`,
{ref}`clouds <cloud>` and {ref}`credentials <credential>`,
{ref}`SSH keys <ssh-key>`, {ref}`secret backends <secret-backend>`,
leases, and the {ref}`migration <the-model-migration>` bookkeeping.
The per-model databases are the other half: one Dqlite database per
model (see {ref}`the full spine <data-model-full-spine>`).

### Controller storage

The controller has two persistent stores: the Dqlite databases and
blob storage. By default, Juju will use the filesystem of the controller's supporting infrastructure for both.

However, either during bootstrap or later, you can (and, in a production-setting, should!) specify any S3-compatible object store you want (e.g., AWS S3, MicroCeph, MinIO, etc.) using the object-store-related controller configuration keys ({ref}`controller-config-object-store-type`, {ref}`controller-config-object-store-s3-endpoint`, {ref}`controller-config-object-store-s3-static-key`, {ref}`controller-config-object-store-s3-static-secret`, {ref}`controller-config-object-store-s3-static-session`, {ref}`controller-config-object-store-s3-region`).

Also, Juju will apply default S3 policy permissions, but you are free to change them, so long as they satisfy the following as a minimum (at least, during model creation):

```text
{
   "Version" : "2012-10-17",
   "Statement" : [
      {
         "Effect" : "Allow",
         "Action" : [
            "s3:CreateBucket",
            "s3:PutBucketPolicy",
            "s3:PutBucketTagging",
            "s3:PutBucketVersioning",
            "s3:PutBucketObjectLockConfiguration"
         ],
         "Resource" : "arn:aws:s3:::*"
      },
      {
         "Effect" : "Allow",
         "Action" : [
            "s3:ListBucket",
            "s3:ListBucketVersions",
            "s3:ListAllMyBuckets",
            "s3:GetBucketLocation",
            "s3:GetBucketPolicy",
            "s3:GetBucketTagging",
            "s3:GetBucketVersioning",
            "s3:GetBucketObjectLockConfiguration",
            "s3:GetObject",
            "s3:GetObjectLegalHold",
            "s3:GetObjectRetention",
            "s3:PutObject",
            "s3:PutObjectLegalHold",
            "s3:BypassGovernanceRetention",
            "s3:PutObjectRetention",
            "s3:DeleteObject"
         ],
         "Resource" : "arn:aws:s3:::*"
      }
   ]
}
```

(the-controller-states)=
## Controller states

A controller has no state machine: there is no life column and no
status vocabulary -- the controller is up, and its nodes' liveness is
the Dqlite cluster's business (see
{ref}`high availability <high-availability>`).

(the-controller-operations)=
## Controller operations

(controller-bootstrap)=
### Controller bootstrap

A controller comes into being through the {ref}`bootstrap <bootstrap-a-controller>` process: `juju bootstrap` turns an empty cloud into a running control plane. The mechanism and the state it leaves:

```{ggarch}
:file: ../juju.ggarch
:slides: Bootstrap machine | Bootstrap machine result
:caption: Bootstrapping a controller on a machine cloud: the mechanism and the state it leaves.
:slide-captions: Sequence diagram: The mechanism: the CLI authenticates against the cloud, provisions a virtual machine, installs jujud, and waits; the controller machine starts its controller agent, API server, and database, then reports the API ready. | Topology: The result: one controller, one model, no applications -- the controller machine running jujud, the API server and Dqlite in-process.
:alt: User invokes juju bootstrap. CLI authenticates with Cloud and provisions a VM. CLI installs jujud on the Controller machine. Controller machine starts the controller agent, API server, and database. Controller machine reports API ready. CLI reports Bootstrap complete to User. The resulting state is the controller machine alone: one controller, one model, no applications yet.
```

Bootstrap is also the controller record's creator: the initialised
database gets the controller row, the `admin`
{ref}`user <user>` and its superuser access, the controller model, and
the bootstrapped {ref}`cloud <cloud>` and
{ref}`credential <credential>` records.

### Controller configuration

The controller configuration is the controller's key/value record,
read and written through the controller config service -- and its
changes are watched (see
{ref}`controller watchers <the-controller-watchers>`). See
{ref}`the list of controller configuration keys <list-of-controller-configuration-keys>` for the
key list.

```{ibnote}
See more: {ref}`manage-controllers`
```

(the-controller-watchers)=
## Controller watchers

The controller side exposes these watch surfaces:

- **Controller configuration** -- the controller config record and
  the controller row; the controller's own workers reconcile on it.
- **Controller nodes** -- the HA cluster's membership.
- **The API addresses** -- the addresses the controller's nodes serve
  on, so agents can re-orient as the cluster changes.

Every watcher fires once immediately when it is created -- the
initial query is the baseline snapshot -- and again on each qualifying
change: database triggers feed the change stream, the watcher wakes,
and the consumer fetches the current state and reconciles.

(the-controller-rules-and-errors)=
## Controller rules and errors

- the controller database holds exactly one controller row -- the
  schema's singleton index enforces it;
- the controller model is marked, not derived: bootstrap writes the
  controller-model flag on the one model that runs Juju (see
  {ref}`the controller model <the-controller-model>`);
- the controller's cloud cannot be removed while it still has
  {ref}`models <model>`.

(related-entities-controller)=
## Related entities

- **The controller model** is the model that hosts the controller
  application and its workers (see {ref}`model <model>`).
- **Machines** host the controller's units on machine clouds; each
  runs a controller node (see {ref}`machine <machine>`,
  {ref}`high availability <high-availability>`).
- **Models** are the controller's tenants -- one Dqlite database each
  (see {ref}`model <model>`, {ref}`database <database>`).
- **Users, clouds, credentials and SSH keys** live in the controller
  database (see {ref}`user <user>`, {ref}`cloud <cloud>`,
  {ref}`credential <credential>`, {ref}`ssh key <ssh-key>`).
- **The controller agent** is the controller's process -- the worker
  tree that *is* the running controller (see
  {ref}`controller agent <controller-agent>`).
