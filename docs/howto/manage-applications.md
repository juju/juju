(manage-applications)=
# How to manage applications

> See also: {ref}`application`

This document shows how to manage applications with Juju.

## Deploy an application

To deploy an application, find and deploy a charm / bundle that delivers it.

> See more: {ref}`deploy-a-charm`

````{note}

- **Machines:**

Deploy on machines consists of the following steps: Provision resources/a machine M from the relevant cloud, via cloud-init maybe network config, download the `jujud` binaries from the controller, start `jujud`.

For failure at any point, retry the `deploy` command with the `--debug` and `--verbose` flags:

```text
juju deploy <charm> --debug --verbose
```

If it still fails,  connect to the machine and examine the logs.

> See more: {ref}`manage-logs`, {ref}`troubleshoot-your-deployment`

- **Kubernetes:**

Deploy on Kubernetes includes creating a Kubernetes pod and in it charm and workload containers. To troubleshoot, inspect these containers with `kubectl`:

```text

kubectl exec <pod> -itc <container> -n <namespace> -- bash
```


````


(view-details-about-an-application)=
## View details about an application

To view more information about a deployed application, run the `show-application` command followed by the application name or alias:

```text
juju show-application <application name or alias >

```

By specifying various flags you can also specify a model, or an output format or file.

> See more: {ref}`command-juju-deploy`


## Set the machine base for an application
> Only for machine clouds.

You can set the base for the machines provisioned by Juju for your application's units either during deployment or after.

**Set the machine base during deployment.** To set the machine base during deployment, run the `deploy` command with the `--base` flag followed by the desired compatible base. For example:


```text
juju deploy ubuntu --base ubuntu@20.04
```

**Set the machine base after deployment.** (*starting with Juju 4.0, this is no longer possible*) To set the machine base after deployment (i.e., for machines provisioned for future units of the application, if any), run the `set-application-base` command followed by the name of the application and the desired compatible base. (This will affect any future units added to the application.) For example:

```text
juju set-application-base ubuntu ubuntu@20.04
```

Note that the charm's current revision must support the base you want to switch to.

> See more: {ref}`command-juju-set-application-base`

(trust-an-application-with-a-credential)=
## Trust an application with a credential


Some applications may require access to the backing cloud in order to fulfil their purpose (e.g., storage-related tasks). In such cases, the remote credential associated with the current model would need to be shared with the application. When the Juju administrator allows this to occur the application is said to be *trusted*.

An application can be trusted during deployment or after deployment.

**Trust an application during deployment.** To trust an application during deployment, run the `deploy` command with the `--trust` flag. E.g., below we trust

```text
juju deploy --trust ...
```

> See more: {ref}`command-juju-deploy`

**Trust an application after deployment.** To trust an application after deployment, use the `trust` command:

```text
juju trust <application>
```

By specifying various flags, you can also use this command to remove trust from an application, or to give an application deployed on a Kubernetes model access to the full Kubernetes cluster, etc.

> See more: {ref}`command-juju-trust`


## Run an application action

> See also: {ref}`action`
>
> See more: {ref}`manage-actions`

(configure-an-application)=
## Configure an application
> See also: {ref}`application-configuration`


<!-- Application configuration here = configuration of the application, which comes from the charm. Not to be confused with application configuration = something like trust (juju deploy --trust, juju trust) and a couple of things for podspec charms (not important since podspec charms are functionally deprecated).-->

<!--When deploying an application, the charm you use will often support or even require specific configuration options to be set.-->

Most charms ship with a sensible default configuration out of the box. However, for some use cases, it may be desirable or necessary to override the default application configuration options.

**Get values.** The way to view the existing configuration for an application depends on whether the application has been deployed or not.

- To view the configuration options of an application that you have not yet deployed,
    - if its charm is on Charmhub, inspect its Configurations page; for example: https://charmhub.io/mediawiki/configure.
    - if its charm is local on your machine, inspect its `config.yaml` file.

- To view the configuration values for a deployed application, run the `config` command followed by the name of the application. For example:

``` text
juju config mediawiki
```


````{dropdown} Expand to view a sample output

``` text
application: mediawiki
charm: mediawiki
settings:
  admins:
    description: Admin users to create, user:pass
    is_default: true
    type: string
    value: ""
  debug:
    description: turn on debugging features of mediawiki
    is_default: true
    type: boolean
    value: false
  logo:
    description: URL to fetch logo from
    is_default: true
    type: string
    value: ""
  name:
    description: The name, or Title of the Wiki
    is_default: true
    type: string
    value: Please set name of wiki
  server_address:
    description: The server url to set "$wgServer". Useful for reverse proxies
    is_default: true
    type: string
    value: ""
  skin:
    description: skin for the Wiki
    is_default: true
    type: string
    value: vector
  use_suffix:
    description: If we should put '/mediawiki' suffix on the url
    is_default: true
    type: boolean
    value: true
```

````

> See more: {ref}`command-juju-config`

**Set values.** You can set configuration values for an application during deployment or later.

- To set configuration values for an application during deployment, run the `deploy` command with the `--config` flag followed by the relevant key=value pair. For example, [the `mediawiki` charm allows users to configure the `name` of the deployed application](https://charmhub.io/mediawiki/configure); below, we set it to `my media wiki`:

```text
juju deploy mediawiki --config name='my media wiki'
```

To pass multiple values, you can repeat the flag or store the values into a config file and pass that as an argument.

> See more: {ref}`command-juju-deploy`


- To set configuration values for an application post deployment, run the `config` command followed by the name of the application and the relevant (list of space-separated) key=value pair(s). For example, [the `mediawiki` charm provides a `name` and a `skin` configuration key](https://charmhub.io/mediawiki/configure); below we set both:

``` text
juju config mediawiki name='Juju Wiki'  skin=monoblock
```

By exploring various options you can also use this command to pass the pairs from a YAML file or to reset the keys to their default values.

> See more: {ref}`command-juju-config`

(scale-an-application)=
## Scale an application

> See also: {ref}`scaling`

(scale-an-application-vertically)=
### Scale an application vertically

To scale an application vertically, set constraints for the resources that the application's units will be deployed on.

> See more: {ref}`manage-constraints-for-an-application`

(scale-an-application-horizontally)=
### Scale an application horizontally

To scale an application horizontally, control the number of units.

> See more: {ref}`control-the-number-of-units`

(make-an-application-highly-available)=
## Make an application highly available
> See also: {ref}`high-availability`

1. Find out if the charm delivering the application supports high availability natively or not. If the latter, find out what you need to do. This could mean integrating with a load balancing reverse proxy, configuring storage etc.

> See more: [Charmhub > `<your charm of interest`](https://charmhub.io/)

2. Scale up horizontally as usual.

> See more: {ref}`scale-an-application-horizontally`


````{dropdown} Expand to view an example featuring the machine charm for Wordpress

The `wordpress` charm supports high availability natively, so we can proceed to scale up horizontally:

```text
juju add-unit wordpress
```

````

````{dropdown} Expand to view an example featuring the machine charm for Mediawiki

The `mediawiki` charm needs to be placed behind a load balancing reverse proxy. We can do that by deploying the `haproxy` charm, integrating the `haproxy` application with `wordpress`, and then scaling the `wordpress` application up horizontally:

``` text
# Suppose you have a deployment with mediawiki and mysql and you want to scale mediawiki.
juju deploy mediawiki
juju deploy mysql

# Deploy haproxy and integrate it with your existing deployment, then expose haproxy:
juju deploy haproxy
juju integrate mediawiki:db mysql
juju integrate mediawiki haproxy
juju expose haproxy

# Get the proxy's IP address:
juju status haproxy

# Finally, scale mediawiki up horizontally
# (since it's a machine charm, use 'add-unit')
# by adding a few more units:
juju add-unit -n 5 mediawiki

```

````



Every time a unit is added to an application, Juju will spread out that application's units, distributing them evenly as supported by the provider (e.g., across multiple availability zones) to best ensure high availability. So long as a cloud's availability zones don't all fail at once, and the charm and the charm's workload are well-written (changing leaders, coordinating across units, etc.), you can rest assured that cloud downtime will not affect your application.

> See more: [Charmhub | `wordpress`](https://charmhub.io/wordpress), [Charmhub | `mediawiki`](https://charmhub.io/mediawiki), [Charmhub | `haproxy`](https://charmhub.io/haproxy)

## Integrate an application with another application

> See more: {ref}`manage-relations`


## Manage an application’s public availability over the network

**Expose an application.** By default, once an application is deployed, it is _only_ reachable by other applications in the _same_ Juju model. However, if the particular deployment use case requires for the application to be reachable by Internet traffic (e.g. a web server, Wordpress installation etc.), Juju needs to tweak the backing cloud's firewall rules to allow Internet traffic to reach the application. This is done with the `juju expose` command.

```{note}

After running a `juju expose` command, any ports opened by the application's charmed operator  will become accessible by **any** public or private IP address.

```

Assuming the `wordpress` application has been deployed (and a relation has been made to the deployed database `mariadb`), the following command can be used to expose the application outside the Juju model:

```text
juju expose wordpress
```

When running `juju status`, its output will not only indicate whether an application is exposed or not, but also the public address that can be used to access each exposed application:

```text
App        Version  Status  Scale  Charm      Rev  Exposed  Message
mariadb    10.1.36  active      1  mariadb      7  no
wordpress           active      1  wordpress    5  yes      exposed

Unit          Workload  Agent  Machine  Public address  Ports   Message
mariadb/0*    active    idle   1        54.147.127.19           ready
wordpress/0*  active    idle   0        54.224.246.234  80/tcp
```

The command also has flags that allow you to expose just specific endpoints of the application, or to make the application available to only specific CIDRs or spaces. For example:

```text
juju expose percona-cluster --endpoints db-admin --to-cidrs 10.0.0.0/24

```

```{important}

To override an initial `expose` command, run the command again with the new desired specifications.

```


> See more: {ref}`command-juju-expose`

**Inspect exposure.**

To view details of how the application has been exposed, run the `show-application` command. Sample session:

```text
$ juju show-application percona-cluster
percona-cluster:
  ...
  exposed: true
  exposed-endpoints:
    "":
      expose-to-cidrs:
      - 0.0.0.0/0
      - ::/0
    db-admin:
      expose-to-cidrs:
      - 192.168.0.0/24
      - 192.168.1.0/24
  ...
```

> See more: {ref}`command-juju-show-application`


**Unexpose an application.** The `juju unexpose` command can be used to undo the firewall changes and once again only allow the application to be accessed by applications in the same Juju model:

``` text
juju unexpose wordpress
```

You can again choose to unexpose just certain endpoints of the application. For example, running `juju unexpose percona-cluster --endpoints db-admin` will block access to any port ranges opened for the `db-admin` endpoint but still allow access to ports opened for all other endpoints:

```text
juju unexpose percona-cluster --endpoints db-admin
```

> See more: {ref}`command-juju-unexpose`

(manage-constraints-for-an-application)=
## Manage constraints for an application

> See also: {ref}`constraint`

**Set values.** You can set constraints for an application during deployment or later.

- To set constraints for an application during deployment, run the `deploy` command with the `--constraints` flag followed by the relevant key-value pair or a quotes-enclosed list of key-value pairs. For example, to deploy MySQL on a resource that has at least 6 GiB of memory and 2 CPUs:

``` text
juju deploy mysql --constraints "mem=6G cores=2"
```

````{dropdown} Expand to see more examples


Assuming a LXD cloud, to deploy PostgreSQL with a specific amount of CPUs and memory, you can use a combination of the `instance-type` and `mem` constraints, as below -- `instance-type=c5.large` maps to 2 CPUs and 4 GiB, but `mem` overrides the latter, such that the result is a machine with 2 CPUs and *3.5* GiB of memory.

``` text
juju deploy postgresql --constraints "instance-type=c5.large mem=3.5G"
```

To deploy Zookeeper to a new LXD container (on a new machine) limited to 5 GiB of memory and 2 CPUs, execute:

``` text
juju deploy zookeeper --constraints "mem=5G cores=2" --to lxd
```

To deploy two units of Redis across two AWS availability zones, run:

``` text
juju deploy redis -n 2 --constraints zones=us-east-1a,us-east-1d
```

````


> See more: {ref}`command-juju-deploy`

```{caution}

**If you want to use the `image-id` constraint with `juju deploy`:** <br>
You must also use the `--base` flag of the command. The base specified via `--base` will be used to determine the charm revision deployed on the resource created with the `image-id` constraint.

```

> See more: {ref}`command-juju-deploy`


<!--CLARIFY:
--base on its own does two things: (1) it determines the OS to be used on the provisioned machines; (2) it determines the charm revision to be deployed on the provisioned machines. In conjunction with `image-id`, though, it only does (2) -- part (1) is overridden by the (unknown) OS specified in the image chosen via `image-id`.
-->

- To set constraints for an application after deployment, run the `set-constraints` command followed by the desired ("-enclosed list of) key-value pair(s), as below. This will affect any future units you may add to the application.

``` text
juju set-constraints mariadb cores=2
```

```{tip}

To reset a constraint key to its default value, run the command with the value part empty (e.g., `juju deploy apache2 --constraints mem= `).

```

> See more: {ref}`command-juju-set-constraints`

**Get values.** To view an application's current constraints, use the `constraints` command:

``` text
juju constraints mariadb
```

> See more: {ref}`command-juju-constraints`


## Change space bindings for an application

You can set space bindings for an application during deployment or post-deployment. In both cases you can set either a default space for the entire application or a specific space for one or more individual application endpoints or both.

- To change space bindings for an application during deployment, use the `deploy` command with the `bind` flag followed by the name of the application and the name of the default space and/or key-value pairs consisting of specific application endpoints and the name of the space that you want to bind them to. For example (where `public` is the name of the space that will be used as a default):

```text
juju deploy <application> --bind "public db=db db-client=db admin-api=public"
```

> See more: {ref}`command-juju-deploy`

- To change space bindings for an application after deployment, use the `bind` command followed by the name of the application and the name of the default space and/or key-value pairs consisting of specific application endpoints and the name of the space that you want to bind them to. For example:

```text
juju bind <application> new-default endpoint-1=space-1
```

> See more: {ref}`command-juju-bind`

<!-- Feels better suited for the upstream. As a matter of policy, we should only document charm solutions when pertaining to juju core material, e.g., the controller charm or the juju-dashboard charm.
(observe-an-application)=
## Observe an application

To observe an application, on a separate Kubernetes model deploy the Canonical Observability Stack, then set up all the necessary cross-model relations. Alternatively, you can deploy only the charms pieces in the stack that you need immediately.


> See more: [Charmhub | Canonical Observability Stack](https://charmhub.io/topics/canonical-observability-stack)
-->

(migrate-an-application)=
## Migrate an application

To migrate an application from one controller to another, migrate the model that it has been deployed to.

> See more: {ref}`migrate-a-model`

(upgrade-an-application)=
## Upgrade an application

To upgrade an application, update its charm.

> See more: {ref}`update-a-charm`

(remove-an-application)=
## Remove an application
> See also: {ref}`removing-things`


To remove an application, run the `remove-application` command followed by the name of the application. For example:

```{caution}
Removing an application which has relations with another application will terminate that relation. This may adversely affect the other application.
```


```text
juju remove-application kafka
```

This will issue a warning with a list of all the pieces to be removed and a request to confirm removal; once you've confirmed, this will remove all of the application's units.

All associated resources will also be removed, provided they are not hosting containers or another application's units.

If persistent storage is in use by the application it will be detached and left in the model; however, if you wish to destroy that as well, you can use the `--destroy-storage` option.



```{note}
It it normal for application removal to take a while (you can inspect progress in the usual with `juju status`). However, if it gets stuck in an error state, it will require manual intervention. In that case, please run `juju resolved --no-retry <unit>` for each one of the application's units (e.g., `juju resolved --no-retry kafka/0`).
```


> See more: {ref}`command-juju-remove-application`


````{note} Troubleshooting:


````{dropdown} One or more units are stuck in error state


If the status of one or more of the units being removed is error, Juju will not proceed until the error has been resolved or the remove applications command has been run again with the force flag.

 > See more: {ref}`mark-unit-errors-as-resolved`


```

````

Behind the scenes, the application removal consists of multiple different stages. If something goes wrong, it can be useful to determine in which step it happened. The steps are the following:
- The client tells the controller to remove the application.
- The controller signals to the application (charm) that it is going to be
  destroyed.
- The charm breaks any relations to its application by calling
  relationship-broken and relationship-departed.
- The charm calls its ‘stop hook’ which should:
  - Stop the application
  - Remove any files/configuration created during the application lifecycle
  - Prepare any backup(s) of the application that are required for restore
    purposes.
- The application and all its units are then removed.
- In the case that this leaves machines with no running applications, the machines are also removed.


