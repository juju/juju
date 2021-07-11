[![Juju logo](doc/juju-logo.png?raw=true)](https://juju.is/)

Juju is an Open Source Charmed Operator Framework. It helps you move from configuration management to application management and has two main components:

* **Charmed Operator Lifecycle Manager (OLM)** - a hybrid-cloud application management and orchestration system that helps you from Day 0 to Day 2. Deploy, configure, scale, **integrate**, maintain and manage Kubernetes native, container-native and VM-native applications -- and the relations between them.
   * **Charmed Operators, packaged as “Charms”**, are software that encapsulate a single application and all the code and know-how it takes to operate it on Kubernetes or machines.

* **Charmed Operator SDK** - a guide to help you build Charmed Operators for use with the Charmed OLM.

## Why Juju

A Kubernetes operator is [a container that drives the config and operation
of a workload](https://charmhub.io/about). By encapsulating ops code as a
reusable container, the operator pattern moves [beyond traditional config
management](https://juju.is/beyond-configuration-management) to allow much
more agile operations for complex cloud workloads.

Shared, open source operators **take infra as code to the next level** with
community-driven ops and integration code. Reuse of ops code [improves
quality](https://juju.is/ops-code-quality) and encourages wider community
engagement and contribution. Operators also improve security through
consistent automation. Juju operators are a [community-driven
devsecops](https://juju.is/devsecops) approach to open source operations.

Juju implements the Kubernetes operator pattern, but is also a **machine
OLM** that extends the operator pattern to traditional applications (without
Kubernetes) on Linux and Windows. Such machine operators can work on bare
metal, virtual machines or cloud instances, enabling [multi cloud and hybrid
cloud operations](https://juju.is/multi-cloud-operations). Juju allows you
to embrace the operator pattern on [both container and legacy
estate](https://juju.is/universal-operators). An operator for machine-based
environments can share 95% of its code with a Kubernetes operator for the
same app.

**Juju excels at application integration**. Instead of simply focusing on
lifecycle management, the Juju OLM provides a [rich application graph
model](https://juju.is/model-driven-operations) that tells operators how to
integrate with one another. This dramatically simplifies the operations of
large deployments.

A key focus for Juju is to **simplify operator design, development and
usage**.  Instead of making very complex operators for specific scenarios,
Juju encourages devops to make composable operators, each of which drives a
single Docker image, and which can be reused in different settings.
[Composable operators](https://juju.is/integration) enable very rich
scenarios to be constructed out of simpler **operators that do one thing and
do it well**.

The OLM provides a central mechanism for operator instantiation,
configuration, upgrades, integration and administration. The OLM provides a
range of [operator lifecycle services](https://juju.is/operator-services)
including leader election and persistent state. Instead of manually
deploying and configuring operators, the OLM manages all the operators in a
model at the direction of the administrator.

## Charmed Operator Collection

The world's [largest collection of operators](https://charmhub.io/) all use
Juju as their OLM. The Charmhub community emphasizes quality, collaboration
and consistency. Publish your own operator and share integration code for
other operators to connect to your application.

The [Open Operator Manifesto](https://charmhub.io/manifesto) outlines the
values of the community and describe the ideal behaviour of operators, to
shape contributions and discussions.

## Multi cloud and hybrid operations across ARM and x86 infrastructure

The Juju OLM supports AWS, Azure, Google, Oracle, OpenStack, VMware and bare
metal machines, as well as any conformant Kubernetes cluster. Integrate
operators across clouds, and across machines and containers, just as easily.
A single scenario can include applications on Kubernetes, as well as
applications on a range of clouds and bare metal instances, all integrated
automatically.

Juju operators support multiple CPU architectures. Connect applications on
ARM with applications on x86 and take advantage of silicon-specific
optimisations. It is good practice for operators to adapt to their
environment and accelerate workloads accordingly.

## Pure Python operators

The [Charmed Operator Framework](https://pythonoperatorframework.io/) makes
it easy to write an operator. The framework handles all the details of
communication between integrated operators, so you can focus on your own
[application lifecycle
management](https://juju.is/operator-lifecycle-manager).

Code sharing between operator publishers is simplified making it much faster
to collaborate on distributed systems involving components from many
different publishers and upstreams. Your operator is a Python event handler.
Lifecycle management, configuration and integration are all events delivered
to your charmed operator by the framework.

## Architecture

The Juju [client, server and agent](https://juju.is/architecture) are all
written in Golang. The standard Juju packaging includes an embedded database
for centralised logging and persistence, but there is no need to manage that
database separately.

Operators can be written in any language but we do encourage new authors to
use the Charmed Operator Framework for ease of contribution, support and
community participation.

## Production grade

The Juju server has built-in support for [high
availability](https://juju.is/high-availability-enterprise-olm) when scaled
out to three instances. It can monitor itself and grow additional instances
in the event of failure, within predetermined limits. Juju supports backup,
restore, and rolling upgrade operations appropriate for large-scale
centralised enterprise grade management and operations systems.

## Get started

Our community hangs out at the [Charmhub
discourse](https://discourse.juju.is/) which serves as a combination mailing
list and web forum. Keep up with the news and get a feel for operator
engineering and usage there. Get  the Juju CLI on Windows, macOS or Linux
with the [install instructions](https://juju.is/docs/installing) and [try
the tutorials](https://juju.is/docs/tutorials). All you need is a small K8s
cluster, or an Ubuntu machine or VM to run MicroK8s.

Read the [documentation](https://juju.is/docs) for a comprehensive reference
of commands and usage.

## Contributing

Follow our [code and contribution guidelines](CONTRIBUTING.md) to learn how
to make code changes. File bugs in
[Launchpad](https://bugs.launchpad.net/juju/+filebug) or ask questions on
our [Mattermost channel](https://chat.charmhub.io/).
