(charm-taxonomy)=
# Charm taxonomy

Juju has been around for some time. As a result, charm writing has gone through multiple frameworks and patterns. Here we describe the resulting charm taxonomy.


Firstly, we can differentiate charms by the substrate they are intended to run on; either machines or containers on Kubernetes. For machine charms, we can further draw a line between principal and subordinate charms.
In Kubernetes charms, we can identify a few fuzzy but useful categories of charms, depending on the type of the workload they manage.

Finally, we can differentiate charms by the technology they are written with. Several generations of libraries have been used, the latest of which is `ops`.


## Charm types, by substrate

There are 'machine charms', meant to be deployed on VMs, and 'Kubernetes charms', meant to be deployed on Kubernetes.

### Machine


Juju’s beginnings were centered around simplifying the deployment of complex applications and services in a cloud-first world. At the time, many of those applications were run in virtual machines or on bare-metal servers, and deployments to these environments continue to enjoy first-class support. A machine charm can be deployed to a number of different underlying compute/storage resource providers:

- Bare-metal (using [MAAS](https://maas.io/))
- Virtual machine (using KVM in Openstack, EC2 in AWS, VMware Environment etc.)
- Container (using [LXD](https://linuxcontainers.org/lxd/introduction/) cluster)

Examples of machine charms include:
* [Ubuntu](https://charmhub.io/ubuntu)
* [Vault](https://charmhub.io/vault)
* [Rsyslog](https://charmhub.io/rsyslog)


#### Roles

A machine charm can have the `subordinate` role. When a machine charm is not subordinate, it is said to be `principal`. The basic difference is that a subordinate charm is deployed on the same unit as the principal charm it is attached to, while a principal charm is deployed on a new unit (i.e. its own machine).


##### Principal

All charms are principal charms, except those that are subordinate.

##### Subordinate


<!--
A *subordinate* charm augments the functionality of another regular charm, which in this context becomes known as the principal charm. When a subordinate charm is deployed no units are created. This happens only once a relation has been established between the principal and the subordinate.-->

Subordinate charms were created to enable the development of charms that could be deployed alongside existing charms -- also known as <a href="#heading--principal-charms">principal</a> charms -- to augment them with specific functionality. They are, in many ways, analogous to <a href="#heading--sidecar">sidecar</a> charms. 

A subordinate charm depends on the creation of a relationship to a principal charm, it is never deployed as a standalone application. This is best explained with an example:

Consider the deployment of a large web application scaled to `n` replicas. Each instance of the charm comprises a `unit`, in Juju parlance, and an `App` is the sum of all units of a given charm with the same name. When a large web application is scaled to `n` replicas, `n` `units` will be started by Juju. 


The administrators of the web application will likely want to collect logs from the application server. The developer of the application charm may not have included a mechanism for forwarding logs from the service that is compatible with the administrator’s specific environment. By using a subordinate charm such as [`rsyslog-forwarder`](https://jaas.ai/rsyslog-forwarder/trusty/1), the administrator can ensure that each deployed unit of their web application is automatically configured to forward logs using `rsyslog`. To do this, they must deploy the subordinate and `juju integrate` their web application to it.

Subordinate charms are written in the same way as other charms, with the addition of an extra `subordinate` flag in the charm’s [metadata](https://juju.is/docs/sdk/metadata-reference).

Examples of subordinate charms include:
- [Telegraf](https://charmhub.io/telegraf)
- [Canonical Livepatch](https://charmhub.io/canonical-livepatch)
- [Nrpe](https://charmhub.io/nrpe)

#### Patterns

##### Proxy

Proxy machine charms (also sometimes called "integrator charms") are the analogues of workload-less kubernetes charms. Historically they have been developed to act as stand-ins for other applications, hence the name.
These are charms that do nothing or little in terms of software installation on the host VM, and whose job consists solely in interacting with Juju constructs. 

Examples of proxy charms include:
- [AWS Integrator](https://charmhub.io/aws-integrator)
- [Openstack Integrator](https://charmhub.io/openstack-integrator)
- [GCP Integrator](https://charmhub.io/gcp-integrator)

#### Kubernetes

The first charms were machine charms.
More recently, [Juju](https://juju.is/docs/juju) introduced support for charms on Kubernetes. Juju can bring the same benefits to applications deployed on Kubernetes, by placing operators alongside workloads to manage them throughout their lifecycle. When a [model](https://juju.is/docs/models) is created with Juju, a corresponding Kubernetes [namespace](https://kubernetes.io/docs/concepts/overview/working-with-objects/namespaces/) is created. When a charm is deployed on Kubernetes, it is deployed as a [StatefulSet](https://kubernetes.io/docs/concepts/workloads/controllers/statefulset/) that manages a set of [Pods](https://kubernetes.io/docs/concepts/workloads/pods/) running the specified application containers alongside a sidecar container containing the charm code.

Examples of Kubernetes charms include:
* [Discourse K8s](https://charmhub.io/discourse-k8s)
* [Zinc K8s](https://charmhub.io/zinc-k8s)
* [Postgresql K8s](https://charmhub.io/postgresql-k8s)
* [Node-RED K8s](https://github.com/juanmanuel-tirado/nodered-charm-k8s)


#### Patterns

Over the time, some patterns have emerged in Kubernetes charms. 
A charm is an operator, so a natural categorization of charms is based on "what is that thing which the charm operates".
Some charms operate workloads running on a container; some charms operate Kubernetes resources, while some others operate `juju` resources. These are the `sidecar`, `workload-less` and `podspec` charm categories we describe in this section.

```{note}
 This categorization is fuzzy, since a charm can in principle do multiple of these things at once, or even "other" things. 
```

```{note}
 
Except for `podspec` (`pod` is a k8s construct), similar patterns could be identified in the machine charm space. 

```


##### Sidecar


By definition, the [sidecar pattern](https://www.learncloudnative.com/blog/2020-09-30-sidecar-container) is designed to allow the augmentation of workloads with additional capabilities and features. The pattern is implemented in this case by a small auxiliary container, located in the same Pod as your application, that provides operations functionality - this is exactly how Juju operates in other environments, and a well-established pattern in the Kubernetes community.

By utilising this pattern, we ensure that there is always an operator right next to every unit of the workload, irrespective of how the application is scaled. The operator will always have direct access to shared memory, the same network namespace and the ability to manipulate the workload as required to keep it running smoothly. This approach simplifies the operation of upstream or third-party application images, enabling administrators to make changes at runtime to suit their environment should they wish, without the requirement to maintain a fleet of bespoke container images.

For charm developers, these benefits are realised by utilising [Pebble](https://github.com/canonical/pebble) to manage workloads. Pebble is a lightweight, API-driven process supervisor designed for use with charms. Pebble enables charm developers to define how they want workloads to run, and provides an API that enables operations code to manage the workloads throughout their life.


Examples of sidecar charms include:
- [Traefik k8s](https://charmhub.io/traefik-k8s)


##### Workload-less

Charms are processes that exchange data. Sometimes you want to manipulate that data without having to rewrite or reconfigure the charms themselves; in that case you can implement a "charm-in-the-middle" pattern that sits in between and manipulates the relation data acting like a filter/proxy of some kind.
Other times you want to write a charm that manages a stack of other charmed applications (danger zone). Welcome to workload-less charms!

Examples of workload-less charms include:
- [Traefik Route k8s](https://charmhub.io/traefik-route-k8s)


##### Podspec


```{caution}

Beginning with `juju` v.3.0, podspec charms are deprecated.

```
Podspec charms create and manage Kubernetes resources that are used by other charms or applications running on the cloud.

This pattern is discouraged because it is more difficult to implement correctly, as the resources one creates and manages via podspec charms practically avoid juju's control, and therefore sidestep its model. While one of the core tenets of juju is that juju could suddenly disappear and all of the cloud services should continue to work as they did before (all that juju does is help operating them, not keeping them alive), the idea is that charms "cooperate" with juju by not doing anything without going through juju first, to prevent juju's model from desyncing.


Examples of podspec charms include:
- [Redis k8s](https://charmhub.io/redis-k8s)


## Charm types, by generation

Charm development has been going on for years, so naturally many attempts have been made at  making the development easier.
The 'raw' API juju exposes can be interacted with directly, but most people will want to use (at least) the bash scripts that come by default with every charm deployment, called 'hook commands' (or 'hook tools'). If your charm only uses those, then you're writing a "bare" charm.

If you fancy using a higher-level, object-oriented Python library to interact with the juju model, then you should be using `ops`.

There exists another python framework that also wraps the hook tools but offers a different (less OOP, less idiomatic) interface, called `reactive`. This framework is deprecated and no longer maintained, mentioned here only for historical reasons.


### Ops

These are charms developed using the {ref}`Ops <ops>` framework. They represent  the current recommended standard, which also integrates best with {ref}`Charmcraft <charmcraft>`.

Examples of Ops-based charms include:
- [LXD](https://charmhub.io/lxd)
- [Discourse K8s](https://charmhub.io/discourse-k8s)
- [Zinc K8s](https://charmhub.io/zinc-k8s)
- [Postgresql K8s](https://charmhub.io/postgresql-k8s)
- [Node-RED K8s](https://github.com/juanmanuel-tirado/nodered-charm-k8s)


### Reactive

These are charms developed using the [reactive](https://charmsreactive.readthedocs.io/en/latest/) framework. 

Note that this is a framework that has now been superseded by Ops.

```{note}

Please do not use reactive. Use Ops.

```

Examples of reactive-based charms include:
- [Containers Kubernetes Worker](https://charmhub.io/containers-kubernetes-worker)
- [Prometheus2](https://charmhub.io/prometheus2)
- [Telegraf](https://charmhub.io/telegraf)
- [Postgresql](https://charmhub.io/postgresql)
- [Node-RED](https://github.com/juanmanuel-tirado/nodered-charm)
- [Canonical Livepatch](https://charmhub.io/canonical-livepatch)


### Bare

These are charms developed without the help of a framework, with all the hook invocations being coded manually.

```{important}

For this reason these charms are also called 'hooks-based', or 'hooks-only', charms.

```

Examples of bare charms include:
- [this tiny bash charm](https://charmhub.io/tiny-bash), ideal for educational purposes
- [Mediawiki](https://charmhub.io/mediawiki)
- [Nrpe](https://charmhub.io/nrpe)


(12-factor-app-charm)=
## 12-Factor app charm

A **12-Factor app charm** is a {ref}`charm <charm>` that has been created using certain coordinated pairs of {ref}`Rockcraft <rockcraft>` and {ref}`Charmcraft <charmcraft>` profiles designed to give you most of the content you will need to generate a [rock^](https://documentation.ubuntu.com/rockcraft/en/latest/explanation/rocks/) for a charm, and then the charm itself, for a particular type of workload (e.g., an application developed with Flask). 

```{tip}

**Did you know?** The OCI images produced by the 12-Factor-app-geared Rockcraft extension are designed to work standalone and are also well integrated with the rest of the Flask framework tooling.

```

When you initialise a rock with a 12-Factor-app-charm-geared profile, the initialisation will generate all the basic structure and content you'll need for the rock, including a  [`rockcraft.yaml`^](https://canonical-rockcraft.readthedocs-hosted.com/en/latest/reference/rockcraft.yaml/#) prepopulated with an extension matching the profile. Similarly, when you initialise a charm with a 12-Factor-app-charm-geared profile, that will generate all the basic structure content you'll need for the charm, including a `charmcraft.yaml` pre-populated with an extension matching the profile as well as a `src/charm.py` pre-loaded with a library (`paas_charm`) with constructs matching the profile and the extension.


At present, there are four pairs of profiles: 
- `flask-framework` 
- `django-framework`
- `fastapi-framework`
- `go-framework` 

