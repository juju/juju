---
myst:
  html_meta:
    description: "Juju cloud reference: what a cloud is, the data the controller keeps about it, its types, and the operations that list, add, update and remove clouds."
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

A **cloud** (also called a **substrate**) is anything with an API that can give Juju the compute, networking and, optionally, storage that the {ref}`units <unit>` of an {ref}`application <application>` run on: a public cloud such as Amazon Web Services, Google Compute Engine, Microsoft Azure, or Kubernetes, a private cloud such as OpenStack, or an environment that Juju treats as a cloud, such as MAAS or LXD (see the {ref}`list of supported clouds <list-of-supported-clouds>`). A {ref}`controller <controller>` knows many clouds, and each {ref}`model <model>` is deployed into one of them.

(cloud-data-model)=
## Cloud data model

```{ggarch}
:file: ../juju.ggarch
:view: Cloud data model
:caption: The controller keeps one record for each cloud it knows. The record holds the cloud's name, which is unique, and the endpoints Juju talks to, and it names one cloud type from a fixed list. A cloud has many regions and many CA certificates, and it admits one or more authentication types, each as its own small record. Models and credentials point at a cloud: a cloud has many models deployed into it, and many credentials that authenticate against it. The model and the credential are described on their own pages, and the figure shows them only for context: all boxes are records, and the two with a second, outlined arrow mark in the corner are the records that other pages describe.
:alt: Seven boxes of the same size, each with a filled table mark in the corner, and two of them (model and credential) also with an outlined arrow mark for records that other pages describe. Cloud is in the middle. Above it, cloud type has many clouds (1 to m). To its left, a cloud has many regions (1 to m). To its right, a cloud has many models (1 to m). Below it, a cloud has many CA certificates, many admitted authentication types, and many credentials (each 1 to m).
```

The cloud data model is the set of records that the {ref}`controller database <database>` keeps for the clouds the controller knows: one record per cloud, and records for its regions, CA certificates and admitted authentication types.

The records, with the rules that apply to each, are:

- **Cloud:** The cloud's name, its type, its API endpoint, its identity and storage endpoints, and whether Juju skips [TLS](https://en.wikipedia.org/wiki/Transport_Layer_Security) verification (the check of the server's certificate) when it talks to the cloud.
  - *Rules:*
    - A cloud's name is unique.
- **Cloud type:** The type is one of `kubernetes`, `lxd`, `maas`, `unmanaged`, `azure`, `ec2`, `gce`, `oci`, `openstack`, and `vsphere`. The types fall into two kinds, and the kind decides how a model on the cloud runs:

  (machine-cloud)=
  - **Machine clouds** are clouds that provide machine-level infrastructure, and they are every type except `kubernetes`. Juju uses the cloud's API to provision or allocate machines (bare metal, virtual machines, or system containers), plus the networking and storage that those machines need, and a model on a machine cloud runs its applications on those machines. See the {ref}`list of supported machine clouds <list-of-supported-machine-clouds>`.

  (kubernetes-cloud)=
  - **Kubernetes clouds** are clouds backed by an existing Kubernetes cluster, which is the type `kubernetes`. Juju uses the Kubernetes API to deploy and manage applications in that cluster, and does not provision machine-level infrastructure. A model on a Kubernetes cloud runs its applications in the cluster. See the {ref}`list of supported Kubernetes clouds <list-of-supported-kubernetes-clouds>`.
- **Region:** A part of the cloud, with its own name and, optionally, its own endpoints. A model can name the region of its cloud that it is deployed into.
- **CA certificate:** A certificate of a certificate authority (CA, an organization that vouches for a server's identity) that Juju trusts when it connects to the cloud.
- **Admitted authentication type:** One of the ways a credential can authenticate against the cloud. The possible values are `access-key`, `instance-role`, `userpass`, `oauth1`, `oauth2`, `jsonfile`, `clientcertificate`, `httpsig`, `interactive`, `empty`, `certificate`, `oauth2withcert`, `service-principal-secret`, `managed-identity`, and `service-account`.
- **Model and credential:** The records that point at a cloud. Other pages own them: see {ref}`model <model>` and {ref}`credential <credential>`. The figure shows them for context.

A cloud is a row in the table `cloud`. Its regions, CA certificates, and admitted authentication types are rows in `cloud_region`, `cloud_ca_cert` and `cloud_auth_type`, and the allowed cloud types and authentication types come from the lookup tables `cloud_type` and `auth_type`. Models and credentials point at a cloud from the tables `model` and `cloud_credential`. {ref}`juju-db-repl` shows the definition of any of them with `.ddl <name>`.

(cloud-change-streams)=
## Cloud change streams

Cloud change streams are the notifications that tell a worker that a cloud's definition has changed, delivered through a watcher (an API object that tells its holder when something it follows has changed, so that nothing has to poll). On a Kubernetes cloud, the broker worker of a model (the worker that manages the model's connection to the cluster) watches the model's cloud spec (the cloud's definition and region together with the model's credential) through the CAAS agent facade (a group of related API methods), and applies a change without restarting, when the cloud type supports that. A client has no watcher for clouds.

(cloud-operations)=
## Cloud operations

Cloud operations are the actions a {ref}`client <client>` can take on clouds through the controller API (the set of calls that clients and agents make to the controller): add, list, show, update, and remove.

(cloud-adding)=
### Adding clouds

```{ibnote}
See also: {ref}`Juju | Add a cloud <add-a-cloud>`
```

Adding clouds registers a new cloud with the {ref}`controller <controller>`. The controller refuses a machine cloud that is not compatible with the cloud it was bootstrapped on, unless the caller forces it.

The operation's required access, rules, and errors are:

- *Required access:* {ref}`Superuser access <user-access-controller-superuser>` to the controller.
- *Rules:*
  - A Kubernetes cloud can always be added. Any other cloud type must be allowed for the type of cloud that the controller was {ref}`bootstrapped <bootstrap-a-controller>` on, as the table below shows, or the caller must force the addition:

    | The controller's cloud | Cloud types that can be added |
    |---|---|
    | `kubernetes` | `lxd`, `maas`, `openstack` |
    | `lxd` | `lxd`, `maas`, `openstack` |
    | `maas` | `maas`, `openstack` |
    | `openstack` | `openstack` |
    | Any other type | None, unless forced |

- *Rules:*
  - When no region is given, the controller creates a region named `default`.
  - When no authentication type is given and the cloud type allows the `empty` type, the controller admits `empty`; otherwise it refuses the cloud.
  - The controller gives the caller administrator access to the new cloud (see {ref}`cloud administrator access <user-access-cloud-admin>`).
- *Errors:*
  - **`cloud type "<type>" is not whitelisted for controller cloud type "<type>", current whitelist: [<types>]`** or **`controller cloud type "<type>" is not whitelisted, current whitelist: …`**: *Trigger:* Adding a cloud whose type the controller's cloud type does not allow. The client receives it with the code for incompatible clouds. *Remediation:* Add a cloud of an allowed type, or force the addition.
  - **`cloud "<name>" already exists`**: *Trigger:* A cloud with that name is already known. *Remediation:* Use another name.

(cloud-listing)=
### Listing and showing clouds

```{ibnote}
See also: {ref}`Juju | View all the known clouds <view-all-the-known-clouds>`, {ref}`Juju | View details about a cloud <view-details-about-a-cloud>`
```

Listing clouds returns the clouds that the caller can use, and showing a cloud returns one cloud's definition and the users who have access to it.

The operation's required access, rules, and errors are:

- *Required access:* None beyond being logged in. A caller with {ref}`superuser access <user-access-controller-superuser>` sees every cloud. Any other caller sees the clouds on which they hold at least {ref}`add-model access <user-access-cloud-add-model>`.
- *Rules:*
  - A cloud the caller cannot use is left out of a list. Asking for the definition of such a cloud is answered as if it did not exist.
  - Showing a cloud lists its users only to a controller superuser or a cloud administrator. Any other caller sees only their own access.
- *Errors:*
  - **`cloud "<name>" not found`**: *Trigger:* Asking for the details of a cloud that does not exist, or for the definition of one that the caller cannot use; the controller does not say which in the second case. *Remediation:* Check the name, and ask a cloud administrator for access.
  - **`permission denied`**: *Trigger:* Showing the details of a cloud on which the caller holds no access. *Remediation:* Ask a cloud administrator for access.

(cloud-updating)=
### Updating clouds

```{ibnote}
See also: {ref}`Juju | Update a cloud <update-a-cloud>`
```

Updating clouds replaces the definition of clouds the controller already knows.

The operation's required access, rules, and errors are:

- *Required access:* {ref}`Superuser access <user-access-controller-superuser>` to the controller.
- *Rules:*
  - A cloud's type cannot change.
  - The default region and the `empty` authentication type are filled in as when adding.
- *Errors:*
  - **`cannot change cloud "<name>" type from "<type>" to "<type>"`**: *Trigger:* An update that names a different type. *Remediation:* Keep the type, or remove the cloud and add it again.
  - **`cloud "<name>" not found`**: *Trigger:* Updating a cloud the controller does not know. *Remediation:* Check the name, or add the cloud.

(cloud-removing)=
### Removing clouds

```{ibnote}
See also: {ref}`Juju | Remove a cloud <remove-a-cloud>`
```

Removing clouds deletes clouds the controller no longer needs.

The operation's required access, rules, and errors are:

- *Required access:* {ref}`Superuser access <user-access-controller-superuser>` to the controller, or {ref}`administrator access <user-access-cloud-admin>` to the cloud.
- *Rules:*
  - A cloud that any model is deployed into cannot be removed. The controller's own model is deployed into the cloud the controller was {ref}`bootstrapped <bootstrap-a-controller>` on, so that cloud cannot be removed while the controller exists.
- *Errors:*
  - **`cannot delete cloud "<name>" as it is still referenced by models`**: *Trigger:* Removing a cloud with models. *Remediation:* Remove the models first.
  - **`permission denied`**: *Trigger:* Removing a cloud on which the caller is not an administrator. *Remediation:* Ask a cloud administrator or a superuser.
