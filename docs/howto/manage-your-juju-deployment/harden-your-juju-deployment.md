(harden-your-deployment)=
# Harden your Juju deployment

> See also: {ref}`juju-security`

Juju ships with sensible security defaults. However, security doesn't stop there.

## Harden the cloud

Use a private cloud.

> See more: {ref}`list-of-supported-clouds`

If you want to go one step further, take your cloud (and the entire deployment) offline.

> See more: {ref}`take-your-deployment-offline`

## Harden the client and the agent binaries

When you install Juju (= the `juju` CLI client + the Juju agent binaries) on Linux, you're installing it from a strictly confined snap. Make sure to keep this snap up to date.

> See more: [Snapcraft | Snap confinement](https://snapcraft.io/docs/snap-confinement), {ref}`manage-juju`, {ref}`juju-roadmap-and-releases`

## Harden the controller(s)

In a typical Juju workflow you allow your client to read your locally stored cloud credentials, then copy them to the controller, so that the controller can use them to authenticate with the cloud. However, for some clouds Juju now supports a workflow where your (client and) controller doesn't need to know your credentials directly -- you can just supply an instance profile (AWS) or a managed identity (Azure). One way to harden your controller is to take advantage of this workflow.

> See more: {ref}`bootstrap-a-controller`, {ref}`cloud-ec2`, {ref}`cloud-azure`

(Like all the cloud resources provisioned through Juju,) the cloud resource(s) (machines or containers) that a controller is deployed on by default run the latest Ubuntu LTS.  This Ubuntu is *not* CIS- and DISA-STIG-compliant (see more: [Ubuntu | The Ubuntu Security Guide](https://ubuntu.com/security/certifications/docs/usg)). However, it is by default behind a firewall, inside a VPC, with only the following three ports opened -- as well as hardened (through security groups) -- by default:

- (always:) `17070`, to allow access from clients and agents;
- (in high-availability scenarios): mongo
- (In high-availability scenarios): `controller-api-port`, which can be turned off (see {ref}`controller-config-api-port`).

When a controller deploys a charm, all the traffic between the controller and the resulting application unit agent(s) is [TLS](https://en.wikipedia.org/wiki/Transport_Layer_Security)-encrypted (each agent starts out with a CA certificate from the controller and, when they connect to the controller, they get another certificate that is then signed by the preshared CA certificate). In addition to that, every unit agent authenticates itself with the controller using a password.

> See more: [Wikipedia | TLS](https://en.wikipedia.org/wiki/Transport_Layer_Security)



<!--
```{caution}

On a MAAS cloud there is no MAAS-based firewall. In that case it is better to have your controller

```
-->

## Harden the user(s)

When you bootstrap a controller into a cloud, you automatically become a user with controller admin access. Make sure to change your password, and choose a strong password.

Also, when you create other users (whether human or for an application), take advantage of Juju's granular access levels to grant access to clouds, controllers, models, or application offers only as needed. Revoke or remove any users that are no longer needed.

> See more: {ref}`user`, {ref}`user-access-levels`, {ref}`manage-users`

## Harden the model(s)

Within a single controller, living on a particular cloud, you can have multiple users, each of which can have different models (i.e., workspaces or namespaces), each of which can be associated with a different credential for a different cloud. Juju thus supports multi-tenancy.

You can also restrict user access to a model and also restrict the commands that any user can perform on a given model.

> See more: {ref}`manage-models`

## Harden the applications

When you deploy (an) application(s) from a charm or a bundle, choose the charm / bundle carefully:

- Choose charms / bundles that show up in the Charmhub search – that means they’ve passed formal review – and which have frequent releases -- that means they're actively maintained.

- Choose charms that don’t require deployment with `--trust` (i.e., access to the cloud credentials). If not possible, make sure to audit those charms.

- Choose charms whose `charmcraft.yaml > containers > uid` and `gid` are not 0 (do not require root access). If not possible, make sure to audit those charms.

- *Starting with Juju 3.6:* Choose charms whose `charmcraft.yaml > charm-user` field set to `non-root`. If not possible, make sure to audit those charms.

- Choose charms that support secrets (see more:  {ref}`secret`).

(Like all the cloud resources provisioned through Juju,) the cloud resource(s) (machines or containers) that an application is deployed on by default run the latest Ubuntu LTS.  This Ubuntu is *not* CIS- and DISA-STIG-compliant (see more: [Ubuntu | The Ubuntu Security Guide](https://ubuntu.com/security/certifications/docs/usg)). However, it is by default behind a firewall, inside a VPC. Just make sure to expose application or application offer endpoints only as needed.

Keep an application's charm up to date.

> See more: {ref}`manage-charms`,  {ref}`manage-applications`

## Audit and observe

Juju generates agent logs that can help administrators perform auditing for troubleshooting, security maintenance, or compliance.

> See more: {ref}`log`

You can also easily collect metrics about or generally monitor and observe your deployment by deploying and integrating with the Canonical Observability Stack.

> See more: {ref}`collect-metrics-about-a-controller` (the same recipe -- integration with the [Canonical Observability Stack](https://charmhub.io/topics/canonical-observability-stack) bundle -- can be used to observe applications other than the controller)