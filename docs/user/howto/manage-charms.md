(manage-charms)=
# How to manage charms or bundles

This document shows various ways in which you may interact with a charm or a bundle.


## Query Charmhub for available charms / bundles


To query Charmhub for the charms / bundles that deliver a given application, run the `find` command followed by a suitable keyword. For example, to find out the charms and/or bundles that deliver WordPress:

```text
juju find wordpress
```


> See more: {ref}`command-juju-find`


## View details about a Charmhub charm / bundle


To view details about a particular Charmhub charm / bundle, run the `info` command followed by the name of the charm / bundle. For example:

```text
juju info postgresql
```
> See more: {ref}`command-juju-info`

```{caution}

For comprehensive information about the charm, including charm documentation, it is always best to see the charm's page on Charmhub.

```

## Find out the resources available for a charm

> See more: {ref}`manage-charm-resources`


## Download a Charmhub charm

```{important}

This is relevant for air-gapped deployments.

```

To download a Charmhub charm, run the `download` command followed by the name of the charm. For example:

```text
juju download postgresql
```

> See more: {ref}`command-juju-download`


(deploy-a-charm)=
## Deploy a charm / bundle


To deploy a charm / bundle from [Charmhub](https://charmhub.io/) / your local filesystem, use the `deploy` command followed by the name of the charm / bundle / the path to the local `<charm>.charm` / `<bundle>.yaml` file:


``` text
juju deploy <charm | charm bundle> | <path to the local charm or bundle>
```

````{dropdown} Example: Deploy a Charmhub charm

```text
juju deploy mysql
```

````



````{dropdown} Example: Deploy a Charmhub bundle

```{tip}

To get a summary of the deployment steps (without actually deploying), add the `--dry-run` flag. Note: This flag is only supported for bundles, not charms.

```

```text
juju deploy kubeflow
```

````

````{dropdown} Example: Deploy a local charm


```text
juju deploy ./mini_ubuntu-20.04-amd64.charm
```

````


````{dropdown} Example: Deploy a local charm with a resource


If your charm's `metadata.yaml` specifies a {ref}`resource <resource-charm>`, you must also explicitly pass the resource. For example:

```text
juju deploy ./demo-api-charm_ubuntu-22.04-amd64.charm --resource \
     demo-server-image=ghcr.io/beliaev-maksim/api_demo_server:0.0.9
```

````

````{dropdown} Example: Deploy a local bundle

```text
juju deploy ./mediawiki-model-bundle.yaml
```

````

`````{dropdown} Example: Deploy a local bundle as an overlay


To deploy a local bundle as an overlay, run the `deploy` command with the `--overlay` flag followed by the path to the overlay. To add an overlay to a model later, export the contents of the model to a bundle and deploy that bundle with the overlay.

````{dropdown} Generic example:

Suppose you want to deploy `mediawiki` and also apply an overlay bundle called `custom-wikimedia.yaml`. Run the `deploy` command followed by `mediawiki` and the `--overlay` flag followed by the local path to your overlay bundle `yaml`:

```text
juju deploy mediawiki \
  --overlay ./custom-mediawiki.yaml
``` 

Suppose now that have a model where you've already deployed `mediawiki`. You've also made some other changes in your model. Finally, you'd like to apply an overlay bundle `custom-mediawiki.yaml`. In that case:

1. Export the contents of your model to a bundle (below, `mediawiki-bundle.yaml`):

```text
juju export-bundle --filename mediawiki-model-bundle.yaml
```

2. Deploy the new bundle and during deploy apply the overlay:

```text
juju deploy ./mediawiki-model-bundle.yaml \
  --overlay ./custom-mediawiki.yaml
```


````

````{dropdown} OpenStack example


Suppose you want to deploy an OpenStack cloud. This is done by deploying a base bundle defining the cloud with an overlay bundle, to make the bundle deployable within the local environment, and -- optionally -- any other number of bundles, to override / add parameters in / to the existing bundle, e.g., storage or constraints. Run the `deploy` command followed by the base bundle and then repeat the `--overlay` flag followed by the path to the overlay for as many overlays as you want. For example, below we deploy an OpenStack Yoga cloud running on Focal nodes (our base bundle), ensure it can run in a MAAS environment (the first, mandatory, overlay) and that it has Shared filesystem services (the second overlay):

``` text
juju deploy ./bundle-focal-yoga.yaml \
   --overlay ./overlay-focal-yoga-mymaas.yaml
   --overlay ./overlay-focal-yoga-mymaas-shared-filesystem.yaml 
```

Suppose now that have a model where you've already deployed all of the above. You've maybe also made some other changes in your model. And you'd like to add manual zone Swift services by applying another overlay. 

1. Export the contents of your model to a bundle (below, `exported-bundle-focal-yoga-2022-06-07.yaml`):

``` text
juju export-bundle --filename exported-bundle-focal-yoga-2022-06-07.yaml
```

2. Deploy the new bundle and during deploy apply the overlay:

``` text
juju deploy ./exported-bundle-focal-yoga-2022-06-07.yaml \
   --overlay ./overlay-focal-yoga-mymaas-manual-swift.yaml
```

````
`````

````{dropdown} Example: Deploy a bundle to existing machines

To have a bundle use a model's existing machines, as opposed to creating new machines, the `--map-machines=existing` option is used. In addition, to specify particular machines for the mapping, comma-separated values of the form 'bundle-id=existing-id' can be passed where the bundle-id and the existing-id refer to top level machine IDs.

For example, consider a bundle whose YAML file is configured with machines 1, 2, 3, and 4, and a model containing machines 1, 2, 3, 4, and 5. The following deployment would use existing machines 1 and 2 for bundle machines 1 and 2 but use existing machine 4 for bundle machine 3 and existing machine 5 for bundle machine 4:

```text
juju deploy some-bundle --map-machines=existing,3=4,4=5
```

````

Depending on the cloud substrate that your controller is running on, the above command will allocate a machine (physical,  virtual, LXD container) or a Kubernetes pod and then proceed to deploy the contents of the charm / bundle.

```{note}

Depending on your use case, you may alternatively opt to provision a set of machines in advance via the `juju add-machine` command. 

In this case, when running the above `juju deploy` command, Juju will detect that the model contains machines with no applications assigned to them and automatically deploy the application to one of those machines instead of spinning up a new machine.

```

The command also allows you to add another argument to specify a custom name (alias) for your deployed application (charms only). You can also take advantage of the rich set of flags to specify a charm channel or revision, a machine base, a machine constraint (e.g., availability zone), the number of application units you want (clusterised), a space binding, a placement directive (e.g., to deploy to a LXD container), a specific storage instance, a specific machine, etc., and even to trust the application with the current credential -- in case the application requires access to the backing cloud in order to fulfil its purpose (e.g., stojrage-related tasks). 

```{note}

When deploying, if Juju fails to provision a subset of machines for some reason (e.g. machine quota limits on the cloud provider) the command {ref}`command-juju-retry-provisioning` can be used to retry the provisioning of specific machine numbers.

```

````{dropdown} Expand to view examples of using a placement directive to deploy to specific targets

> See also: {ref}`placement-directive`

```text
# Deploy to a new lxd-type container on new machine:
juju deploy mariadb --to lxd

# Deploy to a new container on existing machine 25:
juju deploy mongodb --to lxd:25

# Deploy to existing lxd-type container 3 on existing machine 24:
juju deploy nginx --to 24/lxd/3

# Deploy to zone us-east-1a on AWS:
juju deploy mysql --to zone=us-east-1a

# Deploy to a specific machine on MAAS:
juju deploy mediawiki --to node1.maas

# Deploy to a specific machine on LXD:
juju deploy mariadb --to node1.lxd

```

For a Kubernetes-backed cloud, a Kubernetes node can be targeted based on matching labels. The label can be either built-in or one that is user-defined and added to the node. For example:

```text
# Deploy to a specific Kubernetes node (using either a built-in or a user-defined label):
juju deploy mariadb-k8s --to kubernetes.io/hostname=somehost

```

````

> See more: {ref}`command-juju-deploy`

(update-a-charm)=
## Update a charm

Updating a charm to the latest revision always involves the `refresh` command, but the exact way to use it differs a little bit depending on whether you are dealing with a Charmhub charm or rather a local charm.


### Update a Charmhub charm

```{important}
 
Because of the way charm channels work, 'updating' doesn't have to mean 'upgrading' -- you can switch to any charm revision, no matter if it's newer or older. The instructions below reflect this.

However, as newer versions typically contain improvements, Juju will notify you if a new version exists: Juju polls Charmhub once a day to check for updates and, if an update is found, the poll will cause `juju status` to indicate that a newer charm version is available. 

```


1. **If you don't know your current channel:** Run `status` and check the App > Channel column.

1. **If you don't know which channel you want to update to / would like to find out all the available channels:** Run `info` followed by the charm name.

1. Run `refresh` followed by the charm name and the desired new `channel`.



````{dropdown} Expand to view an example featuring the machine charm for PostgreSQL

```text
# Find out the current channel (see App > Channel):
$  juju status
Model        Controller  Cloud/Region         Version  SLA          Timestamp
welcome-lxd  lxd         localhost/localhost  3.1.6    unsupported  14:58:37+01:00

App          Version        Status   Scale  Charm           Channel    Rev  Exposed  Message
postgresql                  waiting    0/1  postgresql      14/stable  351  no       agent initialising

Unit            Workload  Agent       Machine  Public address  Ports  Message
postgresql/0*   waiting   allocating  2        10.122.219.3           agent initialising

Machine  State    Address         Inst id        Base          AZ  Message
2        started  10.122.219.3    juju-f25b73-2  ubuntu@22.04      Running

# Find out all the available channels:
$ juju info postgresql
name: postgresql
publisher: Canonical Data Platform
summary: Charmed PostgreSQL VM operator
description: |
  Charm to operate the PostgreSQL database on machines.
store-url: https://charmhub.io/postgresql
charm-id: ChgcZB3RhaDOnhkAv9cgRg52LhjBbDt8
supports: ubuntu@22.04
tags: databases
subordinate: false
relations:
  provides:
    cos-agent: cos_agent
    database: postgresql_client
    db: pgsql
    db-admin: pgsql
  requires:
    certificates: tls-certificates
    s3-parameters: s3
channels: |
  14/stable:         351                            2024-01-03  (351)  29MB  amd64  ubuntu@22.04
  14/candidate:      363                            2024-01-31  (363)  33MB  amd64  ubuntu@22.04
  14/beta:           363                            2024-01-31  (363)  33MB  amd64  ubuntu@22.04
  14/edge:           365                            2024-02-02  (365)  33MB  amd64  ubuntu@22.04
  latest/stable:     initial-reactive-278-ge3f064a  2023-11-09  (345)  7MB   amd64  ubuntu@16.04, ubuntu@18.04, ubuntu@20.04, ubuntu@22.04
  latest/candidate:  ↑
  latest/beta:       ↑
  latest/edge:       ↑

# Update the charm to revision `365` by switching to the `14/edge` channel:
$ juju refresh postgresql --channel 14/edge
Added charm-hub charm "postgresql", revision 365 in channel 14/edge, to the model
no change to endpoints in space "alpha": certificates, cos-agent, database, database-peers, db, db-admin, restart, s3-parameters, upgrade

# Verify that the charm has been updated (see App > Channel):

$ juju status
Model        Controller  Cloud/Region         Version  SLA          Timestamp
welcome-lxd  lxd         localhost/localhost  3.1.6    unsupported  15:05:16+01:00

App          Version        Status  Scale  Charm           Channel  Rev  Exposed  Message
postgresql   14.9           active      1  postgresql      14/edge  365  no       

Unit            Workload  Agent      Machine  Public address  Ports     Message
postgresql/0*   active    executing  2        10.122.219.3    5432/tcp  (config-changed) 

Machine  State    Address         Inst id        Base          AZ  Message
2        started  10.122.219.3    juju-f25b73-2  ubuntu@22.04      Running

```


````

> See more: {ref}`command-juju-status`, {ref}`command-juju-info`, {ref}`command-juju-refresh`


### Update a local charm

To updae a local charm, run the `refresh` command followed by the name of the charm and the local path to the charm: 


```text
juju refresh juju-test --path ./path/to/juju-test
```

The command offers many other options, for example, the possibility to replace a charm completely with another charm by using the `--switch` option followed by a different path (a process known as 'crossgrading'). (Note: `--path` and `--switch` are mutually exclusive. Use `--switch` if you want to replace your existing charm with  a completely new charm.)

> See more: {ref}`command-juju-refresh`


## Remove a charm / bundle

As a charm / bundle is just the *means* by which (an) application(s) are deployed, there is no way to remove the *charm* / *bundle*. What you *can* do, however, is remove the *application* / *model*.

> See more: {ref}`manage-applications`, {ref}`manage-models`

