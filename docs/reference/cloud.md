---
myst:
  html_meta:
    description: "Juju cloud reference: what a cloud is, the data the controller keeps about it, its types, and the calls that list, add, update, and remove clouds."
---

(cloud)=
# Cloud

```{ibnote}
See also: {ref}`Juju | Manage clouds <manage-clouds>`, {ref}`Terraform Provider for Juju | Manage clouds <tfjuju:manage-clouds>`
```

```{toctree}
:hidden:

cloud/list-of-supported-clouds/index
```

A **cloud** (also called a **substrate**) is anything with an API that can give Juju the compute, networking, and, optionally, storage that the {ref}`units <unit>` of an {ref}`application <application>` run on: a public cloud such as Amazon Web Services, Google Compute Engine, Microsoft Azure, or Kubernetes, a private cloud such as OpenStack, or an environment that Juju treats as a cloud, such as MAAS or LXD (see the {ref}`list of supported clouds <list-of-supported-clouds>`). A {ref}`controller <controller>` knows many clouds. Each {ref}`model <model>` is deployed into one of them.

(cloud-database-schema)=
## Cloud database schema

```{ggarch}
:file: ../juju.ggarch
:view: Cloud database schema
:caption: Read the cloud record first: its type decides how a model on the cloud runs, a machine cloud or Kubernetes, and models and credentials point at it. Regions, CA certificates, and admitted authentication types are lists that belong to the cloud alone. The model and the credential are other pages' records, shown for context.
:alt: Seven boxes of the same size, each with a filled table mark in the corner, and two of them (model and credential) also with an outlined arrow mark for records that other pages describe. Cloud is in the middle. Above it, a cloud type has many clouds. To its left, a cloud has many regions. To its right, a cloud has many models. Below it, a cloud has many CA certificates, many admitted authentication types, and many credentials.
```

The cloud database schema is the set of records that the {ref}`controller database <database>` keeps for the clouds the controller knows: one record per cloud, and records for its regions, CA certificates, and admitted authentication types.

The records, with the rules that apply to each, are:

- **Cloud:** The cloud's name, its type, its API endpoint, its identity and storage endpoints, and whether Juju skips [TLS](https://en.wikipedia.org/wiki/Transport_Layer_Security) verification (the check of the server's certificate) when it talks to the cloud.
  - *Rules:*
    - A cloud's name is unique.
- **Cloud type:** The type is one of `kubernetes`, `lxd`, `maas`, `unmanaged`, `azure`, `ec2`, `gce`, `oci`, `openstack`, and `vsphere`. The types fall into two kinds. The kind decides how a model on the cloud runs:

  (machine-cloud)=
  - **Machine clouds** are clouds that provide machine-level infrastructure. They are every type except `kubernetes`. Juju uses the cloud's API to provision or allocate {ref}`machines <machine>` (bare metal, virtual machines, or system containers, which are containers that run an operating system, such as LXD containers), plus the networking and storage that those machines need. A {ref}`model <model>` on a machine cloud runs its {ref}`applications <application>` on those machines. See the {ref}`list of supported machine clouds <list-of-supported-machine-clouds>`.

  (kubernetes-cloud)=
  - **Kubernetes clouds** are clouds backed by an existing Kubernetes cluster, which is the type `kubernetes`. Juju uses the Kubernetes API to deploy and manage applications in that cluster, and does not provision machine-level infrastructure. A {ref}`model <model>` on a Kubernetes cloud runs its {ref}`applications <application>` in the cluster. See the {ref}`list of supported Kubernetes clouds <list-of-supported-kubernetes-clouds>`.
- **Region:** A part of the cloud, with its own name and, optionally, its own endpoints.
- **CA certificate:** A certificate of a certificate authority (CA, an organization that vouches for a server's identity) that Juju trusts when it connects to the cloud.
- **Admitted authentication type:** One of the ways a credential can authenticate against the cloud. The possible values are `access-key`, `instance-role`, `userpass`, `oauth1`, `oauth2`, `jsonfile`, `clientcertificate`, `httpsig`, `interactive`, `empty`, `certificate`, `oauth2withcert`, `service-principal-secret`, `managed-identity`, and `service-account`.
- **Model and credential:** The records that point at a cloud. Other pages own them: see {ref}`model <model>` and {ref}`credential <credential>`. The figure shows them for context.

(cloud-change-streams)=
## Cloud change streams

Cloud change streams are the changes that the agent of a model on a Kubernetes cloud follows: the model's cloud spec. The cloud spec is the cloud's definition and region together with the model's {ref}`credential <credential>`. The agent follows it through the `WatchCloudSpecsChanges` method of the {ref}`CAAS agent facade <agent-facades>` (a group of related API methods) and applies a change without restarting, when the cloud type supports that.

(cloud-calls)=
## Cloud calls

Cloud calls are the calls a {ref}`client <client>` can take on clouds through the controller API (the set of calls that clients and agents make to the controller): add, list, show, update, and remove.

(cloud-adding)=
### Adding clouds

```{ibnote}
See also: {ref}`Juju | Add a cloud <add-a-cloud>`
```

Adding clouds registers a new cloud with the {ref}`controller <controller>`. The controller refuses a machine cloud that is not compatible with the cloud it was bootstrapped on, unless the caller forces it.

The caller, the rules, and the errors are:

- *Caller:* A client, for a user with {ref}`superuser access <user-access-controller-superuser>` to the controller.
- *Rules:*
  - A Kubernetes cloud can always be added. Any other cloud type must be allowed for the type of cloud that the controller was {ref}`bootstrapped <bootstrap-a-controller>` on, as the table below shows, or the caller must force the addition:

    | The controller's cloud | Cloud types that can be added |
    |---|---|
    | `kubernetes` | `lxd`, `maas`, `openstack` |
    | `lxd` | `lxd`, `maas`, `openstack` |
    | `maas` | `maas`, `openstack` |
    | `openstack` | `openstack` |
    | Any other type | None, unless forced |

  - When no region is given, the controller creates a region named `default`.
  - When no authentication type is given and the cloud type allows the `empty` type, the controller admits `empty`; otherwise it refuses the cloud with the error below.
  - The controller gives the caller administrator access to the new cloud (see {ref}`cloud administrator access <user-access-cloud-admin>`).
- *Errors:*
  - **`cloud type "<type>" is not whitelisted for controller cloud type "<type>", current whitelist: [<types>]`** or **`controller cloud type "<type>" is not whitelisted, current whitelist: …`**: *Trigger:* Adding a cloud whose type the controller's cloud type does not allow. The client receives it with the code `incompatible clouds`. *Remediation:* Add a cloud of an allowed type, or force the addition.
  - **`cloud "<name>" already exists`**: *Trigger:* A cloud with that name is already known. The client receives it with the code `already exists`. *Remediation:* Use another name.
  - **`empty auth-types not valid`**: *Trigger:* No authentication type is given and the cloud type does not allow `empty`. The client receives it with the code `not valid`. *Remediation:* Give at least one authentication type.
  - **`permission denied`**: *Trigger:* The caller lacks superuser access. The client receives it with the code `unauthorized access`. *Remediation:* Ask a controller superuser to add the cloud.

(cloud-listing)=
### Listing and showing clouds

```{ibnote}
See also: {ref}`Juju | View all the known clouds <view-all-the-known-clouds>`, {ref}`Juju | View details about a cloud <view-details-about-a-cloud>`
```

Listing clouds returns the clouds that the caller can use, and showing a cloud returns one cloud's definition and the users who have access to it.

The caller, the rules, and the errors are:

- *Caller:* A client, for a logged-in user. A user with {ref}`superuser access <user-access-controller-superuser>` sees every cloud. Any other user sees the clouds on which they hold at least {ref}`add-model access <user-access-cloud-add-model>`.
- *Rules:*
  - A cloud the caller cannot use is left out of a list. Asking for the definition of such a cloud returns `cloud "<name>" not found`.
  - Showing a cloud lists its users only to a controller superuser or a cloud administrator. Any other caller sees only their own access.
  - Listing clouds with details also returns the caller's access level on each cloud. A superuser who asks for all clouds sees every cloud with administrator access.
- *Errors:*
  - **`cloud "<name>" not found`**: *Trigger:* Asking for the details of a cloud that does not exist, or for the definition of one that the caller cannot use. The client receives it with the code `not found`. *Remediation:* Check the name, and ask a cloud administrator for access.
  - **`permission denied`**: *Trigger:* Showing the details of a cloud on which the caller holds no access. The client receives it with the code `unauthorized access`. *Remediation:* Ask a cloud administrator for access.

(cloud-updating)=
### Updating clouds

```{ibnote}
See also: {ref}`Juju | Update a cloud <update-a-cloud>`
```

Updating clouds replaces the definition of clouds the controller already knows.

The caller, the rules, and the errors are:

- *Caller:* A client, for a user with {ref}`superuser access <user-access-controller-superuser>` to the controller.
- *Rules:*
  - A cloud's type cannot change.
  - The default region and the `empty` authentication type are filled in as when adding.
- *Errors:*
  - **`cannot change cloud "<name>" type from "<type>" to "<type>"`**: *Trigger:* An update that names a different type. *Remediation:* Keep the type, or remove the cloud and add it again.
  - **`cloud not found "<name>"`**: *Trigger:* Updating a cloud the controller does not know. *Remediation:* Check the name, or add the cloud.
  - **`permission denied`**: *Trigger:* The caller lacks superuser access. The client receives it with the code `unauthorized access`. *Remediation:* Ask a controller superuser to update the cloud.

(cloud-removing)=
### Removing clouds

```{ibnote}
See also: {ref}`Juju | Remove a cloud <remove-a-cloud>`
```

Removing clouds deletes clouds the controller no longer needs.

The caller, the rules, and the errors are:

- *Caller:* A client, for a user with {ref}`superuser access <user-access-controller-superuser>` to the controller, or with {ref}`administrator access <user-access-cloud-admin>` to the cloud.
- *Rules:*
  - A cloud that any {ref}`model <model>` is deployed into, including the controller's own model, cannot be removed.
  - Removing a cloud also removes its {ref}`credentials <credential>`, regions, CA certificates, admitted authentication types, and every user's access to it.
- *Errors:*
  - **`cannot delete cloud "<name>" as it is still referenced by models`**: *Trigger:* Removing a cloud with models. *Remediation:* Remove the models first.
  - **`permission denied`**: *Trigger:* Removing a cloud on which the caller is not an administrator. The client receives it with the code `unauthorized access`. *Remediation:* Ask a cloud administrator or a superuser.
