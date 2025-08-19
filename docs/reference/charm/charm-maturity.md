(charm-maturity)=
# Charm maturity

Charms are built for reuse. Having reuse in mind, the design and implementation of a charm needs to  be independent from particular use cases or domains. But how can you ensure reuse?

The best way to enable reuse is to start an open source project. Open source brings experts together; they can participate in the development and contribute their knowledge from their different backgrounds. In addition, the open source approach offers transparency and lets users and developers freely use, modify, analyse and redistribute the software.

## Two stages of maturity

An open source project is the suitable foundation for reuse. However, providing a reusable charm is also a matter of maturity: high-quality software development and relevant capabilities for operating applications. Accordingly, the development of a charm follows a two-stage approach:

1. {ref}`Stage 1: Important qualities <stage-1-important-qualities>`: A quality open source project, implements state-the-art documentation, testing and automation - this is the foundation for sharing and effective collaboration.

2. {ref}`Stage 2: Important capabilities <stage-2-important-capabilities>` are about implementing the most relevant capabilities to ensure effective operations.

(stage-1-important-qualities)=
### Stage 1: Important qualities

<!-- THIS IS MAYBE TOO MUCH DETAIL HERE.
Publishing charms refers to two elements:

1. Publishing the charm to Charmhub. If the charm is on Charmhub, Juju can automatically fetch the charm for deployments. Please note that Juju can deploy charms from the local filesystem as well.
2. Publishing the software project which produces the charm as an open source project.

Publishing a charm to Charmhub makes it available for a wider audience - thus, two things are essential:

Either way, to

1. A charm must provide sound functionality and works reliably.
2. The provided charm is approachable for interested users, both for using, testing and/or contributing to its development.

The following guidelines are crucial for ensuring reliable and approachable charms. Thus, we consider these guidelines for public listing on Charmhub. Please note that public listing refers to the listing of search results, which is a separate setting for charms on Charmhub. Published charms are always available for Juju controllers and can be found using their URL but they are not automatically listed. For more details on how to publish a charm, please consider [the documentation about the publication of charms](https://juju.is/docs/sdk/publishing).

The guideline lists essential goals to be covered. In addtion, it refers to the [best practice documentation](https://juju.is/docs/sdk/styleguide), the documentation about the technical implementation, and examples that serve as a template.

-->

<!-- packages expert knowledge about how to manage an application in the cloud in a way that makes i-->

The power of a charm lies in the fact that it packages expert knowledge in a way that is shareable and reusable. But, for this to work as intended, the charm must meet certain quality standards. This document outlines the first round of standards -- standards intended to ensure that your charm is ready to be shared with others.

```{important}

While every charm can be published on [Charmhub](https://charmhub.io/), only charms that meet this first set of standards will be *listed*, that is, be visible when a user browses or searches for content on Charmhub.

```


```{caution}

These standards keep evolving. Revisit this doc to get the latest updates.

```

#### The charm is reliable

A charm is no good if it does not work reliably as intended. To that end, make sure that your charm has unit testing and integration testing.

##### Unit testing


| Objectives  | Tips, examples, further reading |
| ----------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------- |
|The charm has appropriate unit tests. These tests cover all the actions of the charm and are executed as part of a CI/CD pipeline.  |  See the best practice notes in [Charmcraft](https://canonical-charmcraft.readthedocs-hosted.com/stable/) and [Ops](https://ops.readthedocs.io/en/latest/) docs.|

<!--Reasonable refers to covering the actions of the charm. It does not refer to reaching a specific code coverage metric.
Unit tests cover, for example, handling of events with a mocked application.
-->

#### Integration testing

| Objectives  | Tips, examples, further reading |
| ----------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------- |
| The charm has suitable integration tests. These tests cover installation and basic functionality and are executed automatically as part of a CI/CD pipeline.<p>The implementation of a basic integration test or a smoke test (“turn on and see if smoke comes out”) is not crucial, but the definition of basic or minimal functionality testing is required.<p>To make integration tests possible in the ecosystem, charm authors provide the following information:<p>&#8226; Definition of the project’s reference setup, such as substrate version and required settings. Testers need to understand the setup which developers have considered.<br/>&#8226; In addition to the reference setup, the test documentation lists anticipated substrates/platforms/setups to show the community opportunities for additional testing.<br/>&#8226; Description about the use and expected behaviour of relevant integration points subject to testing, e.g. API, service endpoints, relations.Integration tests should be executed automatically and visible to the community.| See the best practice notes in [Charmcraft](https://canonical-charmcraft.readthedocs-hosted.com/stable/) and [Ops](https://ops.readthedocs.io/en/latest/) docs.|


#### The charm is collaboration-ready

The power of a charm compounds every time someone else finds it, uses it, and contributes to it. As such, make sure that your charm has a good name and icon, is well documented, and has readable code.

##### Consistent naming

| Objectives  | Tips, examples, further reading |
| ----------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------- |
| The charm is named similarly to existing charms, in accordance with the naming guidelines.<p>The name of the publisher identifies the organisation responsible for publishing the charm.| |

##### Icon


| Objectives  | Tips, examples, further reading |
| ----------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------- |
| The charm has an appropriate and recognizable icon that can be displayed on Charmhub or the Juju dashboard as a symbol instead of the charm name. | The icon helps users identify the charm both when searching and selecting on Charmhub and when using the charm in models displayed in the juju dashboard. <p> The workload/application icon is considered for the charm in many cases. If the publisher of the charm and the publisher of the workload/application do not belong to the same legal entity, trademark rules may apply when using existing icons. <p>  See [Charmcraft | Manage charms > Add an icon](https://canonical-charmcraft.readthedocs-hosted.com/stable/howto/manage-charms/#add-an-icon).|

##### Documentation

| Objectives  | Tips, examples, further reading |
| ----------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------- |
|The charm has documentation that covers:<p>1. how to use the charm <p>2. how to modify the charm<p>3. how to contribute to the development<p> Usage documentation covers configuration, limitations, and deviations in behaviour from the “non-charmed” version of the application.<p>There is a concise `summary` field, with a more detailed description field in the `charmcraft.yaml`.| For contributions, many OSS projects have adopted the best practice of providing a CONTRIBUTING.md’ file at the project’s root level.|

##### Readable code

| Objectives  | Tips, examples, further reading |
| ----------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------- |
| The code favours a simple, readable approach. There is sufficient documentation for any integration points with the charm, such as libraries, to aid the forming of relations.| [PEP 8](https://peps.python.org/pep-0008/) for general code style:<p>[PEP 257](https://peps.python.org/pep-0257/) for in-code documentation style<p> See [Charmcraft | Manage charms > Add docs](https://canonical-charmcraft.readthedocs-hosted.com/stable/howto/manage-charms/#add-docs) for user-facing documentation.|


#### The charm is compliant

When publishing charms as open source, on Charmhub or other public places, the published content must comply with copyright and trademarks usage rights.

| Objectives  | Tips, examples, further reading |
| ----------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------- |
| If trademarks and/or logos are used, their use must comply with the permissions of the trademark holder. <p> If 3rd party software or content is used in the charm repository, it must be handled as such. <p> In the detailed view of the charm in Charmhub.io: The "by"- entry should identify the charm's author, not the application's author.| For trademarks: <p> &#8226; Use of trademarks in accordance with the trademark guidelines by the trademark owner. <p> For third-party content (source, images, texts, etc.): <p> &#8226; Use of the content only according to the licence of the copyright owner.

#### The charm stays up-to-date

Charms cover applications which need to be updated regularly. In today’s world of vulnerabilities and cyber security threats, efficient ways of updating software are crucial. Therefore, the automated production of charms is essential.

| Objectives  | Tips, examples, further reading |
| ----------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------- |
| The charm is up-to-date, that is, it has build, test and delivery automation. This automation is important to ensure the project can roll out updates quickly.<p>For this, you need CI/CD in place, including publishing edge and beta/candidate builds.<p>CI/CD is important to ensure that the most recent developments are also accessible to the community for testing.| &#8226; CI/CD builds are triggered ideally at a commit to the main line / master of the charm code.<p>&#8226; In the sense of “CD”, the charm is being published to its beta or edge channel on Charmhub<p>&#8226; See more: [Ops | Write integration tests for a charm](https://ops.readthedocs.io/en/latest/howto/write-integration-tests-for-a-charm.html) |

#### The charm maintainers are reachable

Users must know where to reach out to ask questions or find relevant information.


##### Contact and URLs


| Objectives  | Tips, examples, further reading |
| ----------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------- |
|The charm homepage provides links that are important to the user. Interested developers should be able to contact the charming project and directions on where and how to submit questions and issues must be provided.| URLs must be provided to enable collaboration and exchange on the charm, ideally as metadata on Charmhub.<br/>A best practice is to have an issue template configured for the issue tracker! (Example see, e.g., [alertmanager-k8s issue template](https://github.com/canonical/alertmanager-k8s-operator/issues/new/choose))<br/>The homepage should point to the source code repository, providing an entry point to charm development and contributions.<br/> :warning: Further support is coming for the distinct identification of the project homepage, source code repository and issue tracker in charm metadata, and on Charmhub.|

##### Community discussions

| Objectives  | Tips, examples, further reading |
| ----------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------- |
| A Discourse link or Mattermost channel must be available for discussion, announcements and the exchange of ideas, as well as anything else which would not fit into an issue.<p>For the application, links to the referring forums can also be provided. | Discourse is preferred because framework topics and other charms are also discussed there. It is the most popular place for the community of charms. Therefore, technical questions are most likely covered there.<p> Issues can also be discussed in the [public chat](https://matrix.to/#/#charmhub-juju:ubuntu.com).


(stage-2-important-capabilities)=
### Stage 2: Important capabilities

Your charm looks right, right enough for you to share it with the world. What next? Time to make sure it also works right! This document spells out the second round of standards that you should try to meet -- standards designed to ensure that your charm is good enough to be used in production, at least for some use cases.

```{tip}

Assuming you've already published your charm on [Charmhub](https://charmhub.io/), and passed the review to have it publicly listed, these standards are the way to ensure that your charm gains recognition as a noteworthy charm.

```


<!--That is, publishing a charm on Charmhub is just the beginning. Now that you've made it available to the broader open source developer community, you should plan to evolve it together with this community, so that it best meets everyone's needs and standards. But what are these needs and standards? Which operations implemented by a charm take priority? This document lists a core set of target capabilities for every software operator.
-->

<!--
evolve it so it meets this community's scrutiny, to address their needs and standards, in short, to make it *evaluation* ready! But how do you know what the community's needs and standards are? And which one should take priority? This document lists a core set of target capabilities for every software operator. We recommend you use it to make your charm evaluation-ready.

Publishing a charm on Charmhub is just the beginning. Now that you've made it available to the broader community, you should plan to evolve it together with this community so it best meets everyone's needs and standards. But what are these needs? Which ones tend to take priority? This document lists a core set of target capabilities for every software operator. We recommend you use it to make your charm evaluation-ready.
-->

<!--
[Publishing a charm](https://juju.is/docs/sdk/publishing) on Charmhub makes it available to a broader audience - the charm community. A charm that’s published on Charmhub must provide proper functionality and work reliably for the benefit of the community. We have [guidelines](https://juju.is/docs/sdk/charm-publication-checklist) to ensure all published charms meet our standards of quality and reliability.

After the first release of a charm, the development team will cover additional capabilities and functionalities. But which are the most relevant? Which should the development team prefer? This document lists and explains a core set of capabilities for every software operator. In addition, it contains links to the [best practice documentation](https://juju.is/docs/sdk/styleguide), the [technical documentation pages](https://juju.is/docs/sdk), and example [templates](https://github.com/canonical/template-operator) or [implementations](https://github.com/canonical/charming-actions).
-->

```{caution}

These standards keep evolving. Revisit this doc to get the latest updates.

```


#### The charm has sensible defaults

A user can deploy the charm with a sensible default configuration.

| Objectives  | Tips, examples, further reading |
| ----------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------- |
| The purpose is to provide a fast and reliable entry point for evaluation. Of course, optimised deployments will require configurations.  |  Often applications require initial passwords to be set, which should be auto-generated and retrievable using an action or {ref}`secrets <secret>`. <p> Hostnames and load balancer addresses are examples that often cannot be set with a sensible default. But they should be covered in the documentation and indicated clearly in the status messages on deployment when not properly set.|

#### The charm is compatible with the ecosystem

The charm can expose provides/requires interfaces for integration ready to be adopted by the ecosystem.

| Objectives  | Tips, examples, further reading |
| ----------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------- |
| Newly proposed relations have been reviewed and approved by experts to ensure:<p>&#8226; The relation is ready for adoption by other charmers from a development best practice point of view.<p>&#8226;  No conflicts with existing relations of published charms.<p>&#8226;  Relation naming and structuring are consistent with existing relations.<p>&#8226; Tests cover integration with the applications consuming the relations. | A [Github project](https://github.com/canonical/charm-relation-interfaces) structures and defines the implementation of relations.<p>No new relation should conflict with the ones covered by the relation integration set [published on Github](https://github.com/canonical/charm-relation-interfaces).<p>&#8226; See more: [Charmcraft | Manage relations](https://canonical-charmcraft.readthedocs-hosted.com/stable/howto/manage-charms/#manage-relations), [Ops | Manage relations](https://ops.readthedocs.io/en/latest/howto/manage-relations.html)|


#### The charm respects juju model-config

Most developers are keenly aware of their own charm's configs, without being aware that `juju model-config` is another point of administrative control.

| Objectives  | Tips, examples, further reading |
| ----------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------- |
| Avoid duplicating configuration options that are best controlled at a model level: <p>&#8226; `juju-http-proxy`, `juju-https-proxy`, `juju-no-proxy` should influence the charm's behavior when the charm or charm workload makes any HTTP request. | A [Github project](https://github.com/canonical/charms.proxylib) provides a library to help charms direct url requests and subprocess calls through the model-configured proxy environment. |

#### The charm upgrades the application safely

The charm supports upgrading the workload and the application. An upgrade task preserves data and settings of both.

| Objectives  | Tips, examples, further reading |
| ----------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------- |
| A best practice is to support upgrades sequentially, meaning that users of the charm can regularly apply upgrades in the sequence of released revisions. | &#8226; {ref}`upgrade-an-application`|

#### The charm supports scaling up and down
**If the application permits or supports it,** the charm does not only scale up but also supports scaling down.

| Objectives  | Tips, examples, further reading |
| ----------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------- |
| Scale-up and scale-down can involve the number of deployment units and the allocated resources (such as storage or computing). | <p>&#8226;  {ref}`scale-an-application` <p>Note that the cited links also point to how to deal with relations when instances are added or removed:<p>&#8226; See more: [Charmcraft | Manage relations](https://canonical-charmcraft.readthedocs-hosted.com/stable/howto/manage-charms/#manage-relations), [Ops | Manage relations](https://ops.readthedocs.io/en/latest/howto/manage-relations.html) |
<!--
<a href="#heading--backup"><h2 id="heading--backup">The charm supports backup and restore</h2></a>

**If the application supports it,** the charm should be recoverable to a working state after a unit is redeployed, migrated, or lost, and a backup copy of the workload's state is attached.

| Objectives  | Tips, examples, further reading |
| ----------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------- |
| As a best practice, charms…<p>&#8226; … are as stateless as possible (or just stateless), and<p>&#8226; … store in a storage that can be backed up.<p> If the application provides backup functionality already, the charm uses this functionality. | Consider [this example](https://...) as an example of backup operations to be covered. |
-->

#### The charm is integrated with observability

Engineers and administrators who operate an application at a production-grade level need to capture and interpret the application’s state.

| Objectives  | Tips, examples, further reading |
| ----------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------- |
| Integrating observability refers to providing:<p>&#8226; a metrics endpoint,<p>&#8226; alert rules,<p>&#8226; Grafana dashboards, and<p>&#8226; integration with a log sink (e.g. [Loki](https://charmhub.io/loki-k8s)).| Consider the [Canonical Observability Stack](https://charmhub.io/topics/canonical-observability-stack) (COS) for covering observability in charms. Several endpoints are available from the COS to integrate with charms:<p>&#8226; Provide metrics endpoints using the MetricsProviderEndpoint<p>&#8226; Provide alert rules to Prometheus<p>&#8226; Provide dashboards using the GrafanaDashboardProvider<p>&#8226; Require a logging endpoint using the LogProxyConsumer or LokiPushApiConsumer<p>More information is available on the [Canonical Observability Stack homepage](https://charmhub.io/topics/canonical-observability-stack).<p>Consider the Zinc charm implementation as [an example for integrations with Prometheus, Grafana and Loki](https://github.com/jnsgruk/zinc-k8s-operator/blob/main/charmcraft.yaml). |



## Requirements for public listing

Everyone can publish charms to [https://charmhub.io/](https://charmhub.io/). Then, the charm can be accessed for deployments using Juju or via a web browser by its URL. If a charm is published in Charmhub.io and included in search results, the charm entry needs to be switched into the listed mode. To bring your charm into the listing, [reach out to the community](https://discourse.charmhub.io/c/charmhub-requests/46) to announce your charm and ask for a review by an experienced community member.

The Stage 1- Important qualities reference is the requirement for switching a charm into the *listed* mode. The points listed in the first stage ensure a useful charm project is suitable for presentation on [https://charmhub.io/](https://charmhub.io/) and for testing by others.
