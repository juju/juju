---
myst:
  html_meta:
    description: "Juju cloud substrate reference: AWS, Azure, GCP, Kubernetes, OpenStack, MAAS, LXD, and other supported cloud platforms."
---

(cloud)=
# Cloud (substrate)
```{ibnote}
See also: {ref}`manage-clouds`
```

```{toctree}
:hidden:

cloud/list-of-supported-clouds/index
cloud/kubernetes-clouds-and-juju

```

To Juju, a **cloud** (or backing cloud) is any entity that has an API that can provide compute, networking, and optionally storage resources in order for application units to be deployed on them. This includes public clouds such as Amazon Web Services, Google Compute Engine, Microsoft Azure and Kubernetes as well as private OpenStack-based clouds. Juju can also make use of environments which are not clouds per se, but which Juju can nonetheless treat as a cloud. MAAS and LXD fit into this last category. Because of this, in Juju a cloud is sometimes also called, more generally, a **substrate**.

## Supported clouds

See {ref}`list-of-supported-clouds`.

(cloud-differences)=
## Cloud differences

While Juju aims to make all clouds feel the same, some differences still persist depending on whether the cloud is a machine cloud or a Kubernetes cloud or a specific cloud as opposed to another.

(machine-clouds-vs-kubernetes-clouds)=
### Machine clouds vs. Kubernetes clouds

Juju makes a fundamental distinction between **'machine' clouds** -- that is, clouds based on bare metal machines (BMs; e.g., MAAS), virtual machines (VMs; e.g., AWS EC2), or system containers (e.g., LXD) -- and **'Kubernetes' clouds** -- that is, based on containers (e.g., AWS EKS).

While the user experience is still mostly the same -- bootstrap a Juju controller into the cloud, add a model, deploy charms, scale, upgrade, etc. -- this difference affects:
 - the required system requirements (e.g., for a Juju controller, 4GB vs. 6GB memory)
- the way you connect the cloud to Juju (`add-cloud` + `add-credentials` vs. `add-k8s`)
- what charms you can deploy ('machine' charms vs. 'Kubernetes' charms)

and, occasionally

- what operations you may perform, e.g.,
    - `enable-ha` is currently supported just for machine controllers
    - scaling an application is done via `add-unit` on machines and via `scale-application` on Kubernetes).

Juju's vision is to eventually make this distinction irrelevant.

(cloud-foo-vs-cloud-bar)=
### Cloud foo vs. cloud bar

As a Juju user you will sometimes also notice small differences tied to a cloud's specific identity, beyond the machine-Kubernetes divide.

This usually affects the setup phase (the information you have to supply to Juju to connect Juju to your cloud, and whether Juju can retrieve any of that automatically for you) and, later on, the customisations you can make to your deployment (e.g., small differences in configurations, constraints, placement directives, subnets, spaces, storage, etc., depending on the features available / supported for a given cloud).

```{ibnote}
See more: {ref}`list-of-supported-clouds` > `<cloud name>`
```

However, note that all Kubernetes clouds are fundamentally the same.

```{ibnote}
See more: {ref}`kubernetes-clouds-and-juju`
```

(cloud-definition)=
## Cloud definition

In Juju, cloud definitions can be provided either interactively or via a YAML file or (depending on the cloud) environment variables.

Regardless of the method, they are saved in a file called `public-clouds.yaml` (for public clouds; on Linux, typically: `~/.local/share/juju/public-clouds.yaml`) or `clouds.yaml` (for user-defined clouds, including Kubernetes; on Linux, the default location is: `~/.local/share/juju/clouds.yaml`).

These files both follow the same basic schema.

````{dropdown} Expand to view an example 'clouds.yaml' file with a definition for LXD and Amazon EKS

```text
clouds:
  lxd:
    type: lxd
    auth-types:
      - certificate
    endpoint: <endpoint>
    regions:
      default:
        endpoint: <endpoint>
    config:
      apt-http-proxy: <endpoint>
  eks:
    type: kubernetes
    host-cloud-region: ec2/eu-north-1
    auth-types:
      - userpass
      - oauth2
      - clientcertificate
    endpoint: <endpoint>
    regions:
      eu-north-1:
        endpoint: <endpoint>
    config:
      operator-storage: gp2
      workload-storage: gp2
    ca-certificates: <certificates>

```

````

 The rest of this section gives details about this schema.

> [Source](https://github.com/juju/juju/blob/ecd609d9e8700e87f630b6fb8c8b6690f211092d/cloud/clouds.go)

```{note}

The most important keys are `clouds`, `.<cloud name>`, `..type`, `..auth-types`, and `..endpoint`.

```

### `clouds`

**Status:** Required.

**Purpose:** To define different clouds.

**Value:** Mapping. Keys are cloud names.

### `clouds.<cloud>`

**Status:** Required.

**Purpose:** To define a cloud.

**Name:** String = the name of the cloud. For built-in clouds and for public clouds, set by Juju; see {ref}`list-of-supported-clouds` > `<cloud name>`. For user-defined clouds, set by the user.

**Value:** Mapping. Keys are strings = cloud properties.

### `clouds.<cloud>.auth-types`

**Status:** Required.

**Purpose:** To define the authentication types supported by the clouds.

**Value:** Sequence. Items are strings = authentication types supported by the cloud given its cloud type. See more: {ref}`list-of-supported-clouds` > `<cloud name>`.

### `clouds.<cloud>.ca-certificates`

**Status:** Optional.

**Purpose:** To define the Certificate Authority certificates to be used to validate certificates of cloud infrastructure components.

**Value:** Sequence. Items are strings = base64-encoded x.509 certs.

### `clouds.<cloud>.config`

**Status:** Optional.

**Purpose:** To define a model configuration to use when bootstrapping Juju in the cloud. The configuration will be combined with Juju-generated, and user-supplied values; user-supplied values take precedence.

**Value:** Mapping. Keys are model configuration keys (either generic or cloud-specific).  See more: {ref}`list-of-model-configuration-keys` and/or {ref}`list-of-supported-clouds` > `<cloud name>`.

### `clouds.<cloud>.description`

**Status:** Optional.

**Purpose:** To describe the cloud.

**Value:** String = the cloud description.

### `clouds.<cloud>.endpoint`

**Status:** TBA

**Purpose:** To define the default endpoint for the cloud regions. Note: It may be overridden by a region.

**Value:** String = the endpoint URL or, for manual clouds, the SSH URI (e.g., `ubuntu@1.2.3.4`).

### `clouds.<cloud>.host-cloud-region`

**Status:** TBA

**Purpose:** To define the Kubernetes host cloud region.

**Value:** String = the Kubernetes host cloud region, in the following format: `<cloudType>/<region>`.

### `clouds.<cloud>.identity-endpoint`

**Status:** TBA

**Purpose:** To define the default identity endpoint for the cloud regions. Note: It may be overridden by a region.

**Value:** String = the default identity endpoint for the cloud regions.

### `clouds.<cloud>.region-config`

**Status:** Optional.

**Purpose:**  To define a cloud-specific configuration to use when bootstrapping Juju in a specific cloud region. The configuration will be combined with Juju-generated and user supplied values;  user supplied values take precedence.

**Value:** TBA

### `clouds.<cloud>.regions`

**Status:** Optional.

**Purpose:** To define the regions available in the cloud.

**Value:** Mapping. Keys are strings = region names. Cloud-specific.  See more: {ref}`list-of-supported-clouds` > `<cloud name>`.

### `clouds.<cloud>.regions.<region>`

**Value:** String = the name of the region.

### `clouds.<cloud>.regions.<region>.endpoint`

**Value:** String = the region's primary endpoint URL.

### `clouds.<cloud>.regions.<region>.identity-endpoint`

The region's identity endpoint URL.  If the cloud/region does not have an identity-specific endpoint URL, this will be empty.

### `clouds.<cloud>.regions.<region>.storage-endpoint`

The region's storage endpoint URL.  If the cloud/region does not have an storage-specific endpoint URL, this will be empty.

### `clouds.<cloud>.storage-endpoint`

**Status:** Optional.

**Purpose:** To define the default storage endpoint for the cloud regions. Note: It may be overridden by a region.

**Value:** String = the storage endpoint.

### `clouds.<cloud>.type`

**Status:** Required.

**Purpose:** To define the type of cloud in Juju.

**Value:** String = the cloud type. See more: {ref}`list-of-supported-clouds` > `<cloud name>`.
