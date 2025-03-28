(charm)=
# Charm

> See also: {ref}`manage-charms`

```{toctree}
:hidden:
charm/charm-development-best-practices
charm/charm-maturity
```

In Juju, a **charm** is an operator -- software that wraps an {ref}`application <application>` and that contains all of the instructions necessary for deploying, configuring, scaling, integrating, etc., the application on any {ref}`Juju-supported cloud <list-of-supported-clouds>`.

Charms are often published on [Charmhub](https://charmhub.io/).

(charm-taxonomy)=
## Charm taxonomy

(charm-taxonomy-by-substrate)=
### By substrate

(kubernetes-charm)=
#### Kubernetes

A **Kubernetes charm** is a charm designed to run on a resource from a Kubernetes cloud -- i.e., in a container in a pod.

Example Kubernetes charms:

- [Discourse K8s](https://charmhub.io/discourse-k8s)
- [Zinc K8s](https://charmhub.io/zinc-k8s)
- [Postgresql K8s](https://charmhub.io/postgresql-k8s)

(machine-charm)=
#### Machine

A **machine charm** is a charm designed to run on a resource from a machine cloud -- i.e., a bare metal machine, a virtual machine, or a system container.

Example machine charms:
- [Ubuntu](https://charmhub.io/ubuntu)
- [Vault](https://charmhub.io/vault)
- [Rsyslog](https://charmhub.io/rsyslog)

(infrastructure-agnostic-charm)=
#### Infrastructure-agnostic

While charms are still very much either for {ref}`Kubernetes <kubernetes-charm>` or {ref}`machines <machine-charm>`, some {ref}`workloadless <workloadless-charm>` charms are in fact infrastructure-agnostic and can be deployed on both.

```{note}
That is because most of the difference between a machine charm and a Kubernetes charm comes from how the charm handles the workload. So, if a charm does not have a workload, and its metadata does not stipulate Kubernetes, and the charm does not do anything that would only make sense on machines / Kubernetes, it can run perfectly fine on both machines and Kubernetes -- the details of the deployment will differ (the charm will be deployed on a machine vs. a container in a pod), but the deployment will be successful. Example workloadless charms that are cloud-agnostic: [Azure Storage Integrator](https://charmhub.io/azure-storage-integrator).
```


(charm-taxonomy-by-function)=
### By function

While charms are fundamentally about codifying operations for a given workload, some have a slightly different function.


(workloadless-charm)=
#### Workloadless

A **workloadless** charm is a charm that does not run any workload locally.

Because of their nature, workloadless charms are often {ref}`infrastructure-agnostic <infrastructure-agnostic-charm>`.

Examples:

- [Data Integrator](https://charmhub.io/data-integrator)

(configurator-charm)=
#### Configurator

A **configurator charm** is a {ref}`workloadless charm <workloadless-charm>` that configures another charm, once they're integrated.

Examples:

- [Canonical Observability Stack Proxy](https://charmhub.io/cos-proxy)
- [Prometheus Scrape Config (K8s)](https://charmhub.io/prometheus-scrape-config-k8s)

(proxy-charm)=
#### Proxy

A **proxy charm** is a {ref}`configurator charm <configurator-charm>` where the configuration is about how to interact with a non-charmed workload.

Examples:

- [Parca Scrape Target](https://charmhub.io/parca-scrape-target)
- [Prometheus Scrape Target](https://charmhub.io/prometheus-scrape-target)
- [Prometheus Scrape Config](https://charmhub.io/prometheus-scrape-config)

(integrator-charm)=
#### Integrator

An **integrator charm** is a {ref}`proxy charm <proxy-charm>` where the non-charmed workload is some {ref}`cloud <cloud>`-related functionality.

Examples:

- [AWS-Integrator](https://charmhub.io/aws-integrator)
- [VMware vSphere Integrator](https://charmhub.io/vsphere-integrator)

(charm-taxonomy-by-role)=
### By role

(principal-charm)=
#### Principal

In {ref}`machine charms <machine-charm>`, a **principal charm** is any charm that has a {ref}`subordinate <subordinate-charm>`.

(subordinate-charm)=
#### Subordinate

In {ref}`machine charms <machine-charm>`, a **subordinate charm** is a charm designed to be deployed adjacent to another charm and to augment the functionality of that charm, known as its {ref}`principal <principal-charm>`.

When a subordinate charm is deployed, no units are created; this happens only once a relation has been established between the principal and the subordinate.

Examples:
- [Telegraf](https://charmhub.io/telegraf)
- [Canonical Livepatch](https://charmhub.io/canonical-livepatch)
- [Nrpe](https://charmhub.io/nrpe)

<!--
Subordinate charms are written in the same way as other charms, with the addition of an extra `subordinate` flag in the charm’s [metadata](https://juju.is/docs/sdk/metadata-reference).
-->

(charm-taxonomy-by-architecture)=
### By architecture

(sidecar-charm)=
#### Sidecar

> This is the state-of-the-art way to develop Kubernetes charms. Both [Charmcraft](https://canonical-charmcraft.readthedocs-hosted.com/) and [Ops](https://ops.readthedocs.io/) are designed to produce *sidecar* Kubernetes charms, where the way these charms manage the workload across their respective container boundaries is through [Pebble](https://documentation.ubuntu.com/pebble/).

In {ref}`Kubernetes charms <kubernetes-charm>`, a **sidecar charm** is a {ref}`Kubernetes charm <kubernetes-charm>` designed to be placed in a container that is in the same pod as container where the workload is, following the [sidecar pattern](https://www.learncloudnative.com/blog/2020-09-30-sidecar-container). As in Juju a Kubernetes pod corresponds to a unit, that means that there is an operator inside each unit of the workload.

Examples:

- [Traefik k8s](https://charmhub.io/traefik-k8s)

(podspec-charm)
#### Podspec

> Superseded by {ref}`sidecar charms <sidecar-charm>`. Also deprecated in Juju 3+.

In {ref}`Kubernetes charms <kubernetes-charm>`, a **podspec** charm is a {ref}`Kubernetes charm <kubernetes-charm>` designed to create and manage Kubernetes resources that are used by other charms or applications running on the cloud. As this pattern was difficult to implement correctly and also sidestepped Juju's model (the resources created by a podspec charm were not under Juju's control), this pattern has been deprecated in favor of {ref}`sidecar charms <sidecar-charm>`.


(charm-taxonomy-by-generation)=
### By generation

Charm development has been going on for years, so naturally many attempts have been made at making the development easier. The 'raw' API Juju exposes can be interacted with directly, but most people will want to use (at least) the Bash scripts that come by default with every charm deployment, called {ref}`'hook commands' (or 'hook tools') <hook-command>`. If your charm only uses those, then you're writing a 'bare' charm. If you fancy using a higher-level, object-oriented Python library to interact with the juju model, then you should be using Ops. There exists another python framework that also wraps the hook tools but offers a different (less OOP, less idiomatic) interface, called `reactive`. This framework is deprecated and no longer maintained, mentioned here only for historical reasons.


(ops-charm)=
#### Ops

> This is the state-of-the-art way to develop a charm.

An **Ops charm** is a charm developed using the [Ops](https://ops.readthedocs.io/) (operator) framework.

Examples:
- [LXD](https://charmhub.io/lxd)
- [Discourse K8s](https://charmhub.io/discourse-k8s)
- [Zinc K8s](https://charmhub.io/zinc-k8s)
- [Postgresql K8s](https://charmhub.io/postgresql-k8s)


(12-factor-app-charm)=
#### 12-Factor app charm

A **12-Factor app charm** is a charm that has been created using certain coordinated pairs of [Rockcraft](https://documentation.ubuntu.com/rockcraft/en/latest/index.html) and [Charmcraft](https://canonical-charmcraft.readthedocs-hosted.com/en/stable/) profiles designed to give you most of the content you will need to generate a [rock^](https://documentation.ubuntu.com/rockcraft/en/latest/explanation/rocks/) for a charm, and then the charm itself, for a particular type of workload (e.g., an application developed with Flask).

When you initialise a rock with a 12-Factor-app-charm-geared profile, the initialisation will generate all the basic structure and content you'll need for the rock, including a [`rockcraft.yaml`^](https://canonical-rockcraft.readthedocs-hosted.com/en/latest/reference/rockcraft.yaml/#) prepopulated with an extension matching the profile. Similarly, when you initialise a charm with a 12-Factor-app-charm-geared profile, that will generate all the basic structure content you'll need for the charm, including a `charmcraft.yaml` pre-populated with an extension matching the profile as well as a `src/charm.py` pre-loaded with a library (`paas_charm`) with constructs matching the profile and the extension.

> See more:
> - {external+charmcraft:ref}`Charmcraft | Write your first Kubernetes charm for a Django app <write-your-first-kubernetes-charm-for-a-django-app>`
> - {external+charmcraft:ref}`Charmcraft | Write your first Kubernetes charm for a FastAPI app <write-your-first-kubernetes-charm-for-a-fastapi-app>`
> - [Charmcraft | Write your first Kubernetes charm for a Flask app](https://canonical-charmcraft.readthedocs-hosted.com/en/stable/tutorial/flask/)
> - {external+charmcraft:ref}`Charmcraft | Write your first Kubernetes charm for a Go app <write-your-first-kubernetes-charm-for-a-go-app>`

(reactive-charm)=
#### Reactive
> Superseded by {ref}`Ops <ops-charm>`.

A **Reactive charm** is a charm developed using the [Reactive](https://charmsreactive.readthedocs.io/en/latest/) framework.


Examples:
- [Prometheus2](https://charmhub.io/prometheus2) (obsolete; replaced by [Prometheus K8s](charmhub.io/prometheus-k8s))
- [Telegraf](https://charmhub.io/telegraf)
- [Canonical Livepatch](https://charmhub.io/canonical-livepatch) (no longer maintained)

(bare-charm)=
#### Bare
> Superseded by {ref}`Ops <ops-charm>`.

A **bare charm** is a charm developed without the help of a framework, with all the {ref}`hook <hook>` invocations being coded manually (which is why such charms are sometimes also called 'hooks-based' or 'hooks-only').

Examples:

- [this tiny bash charm](https://charmhub.io/tiny-bash), ideal for educational purposes
- [Mediawiki](https://charmhub.io/mediawiki)
- [Nrpe](https://charmhub.io/nrpe)

(charm-anatomy)=
## Charm anatomy

(charm-revision)=
### Charm revision

A **charm revision** is a number that uniquely identifies the version of the charm that a charm author has uploaded to Charmhub.

```{caution}
The revision increases with every new version of the charm being uploaded to Charmhub. This can lead to situations of mismatch between the semantic version of a charm and its revision number. That is, whether the changes are for a semantically newer or older version, the revision number always goes up.
```

A revision only becomes available for consumption once it's been released into a {ref}`channel <charm-channel>`. At that point, charm users will be able to see the revision at `charmhub.io/<charm/channel>` or access it via `juju info <charm>` or `juju deploy <charm> --channel`. And to inspect a specific revision of a charm, use the `--revision` flag. The syntax is `juju info <charm> --revision <revision>`.

(charm-channel)=
### Charm channel

A **charm channel** is a charm release identifier built on the pattern `<track>/<risk>/<branch>` (e.g., `juju deploy kafka --channel 3/stable`).

(charm-channel-track)=
#### Track

A `<track>` is a way to collect multiple supported releases of your charm under the same name.
When deploying a charm, specifying a track is optional; if none is specified, the default option is the `latest`.
To ensure consistency between tracks of the same charm, tracks must comply with a guardrail.

(charm-channel-track-guardrail)=
##### Track guardrail

A **track guardrail** is a regex generated by a Charmhub admin at the request of a charm author whose purpose is to ensure that any new track of the charm complies with the specific pattern selected by the charm author for the charm, usually in conformity with the pattern established by the upstream workload (e.g., no numbers, cf, e.g., [OpenStack](https://docs.openstack.org/charm-guide/latest/project/charm-delivery.html); numbers in the major.minor format; just integers; etc.)

<!--Their format is usually modeled on the upstream workload. For example, some don't use numbers (e.g., [tracks for the Charmed OpenStack project](https://docs.openstack.org/charm-guide/latest/project/charm-delivery.html)); others use numbers in the major.minor format; others use just integers; etc. To ensure consistency between tracks of the same charm, tracks must comply with a guardrail -- a regex that is generated by a Charmhub admin and which will enforce the track pattern you have chosen.-->

#### Risk
The `<risk>` refers to one of the following risk levels:
- **stable**: (default) This is the latest, tested, working stable version of the charm.- **candidate**: A release candidate. There is high confidence this will work fine, but there may be minor bugs.- **beta**: A beta testing milestone release.- **edge**: The very latest version - expect bugs!

#### Branch
Finally, the `<branch>` is an optional finer subdivision of a channel for a published charm that allows for the creation of short-lived sequences of charms (guaranteed for only 30 days without modification) that can be pushed on demand by charm authors to help with fixes or temporary experimentation. Note that, if you use `--channel` to specify a branch (e.g., during `juju deploy` or `juju refresh`), you must specify a track and a risk level as well.


