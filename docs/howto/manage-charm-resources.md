(manage-charm-resources)=
# How to manage charm resources

> See also: {ref}`charm-resource`

When you deploy / update an application from a charm, that automatically deploys / updates any charm resources, using the defaults specified by the charm author. However, you can also specify resources manually (e.g., to try a resource released only to `edge` or to specify a non-Charmhub resource). This document shows you how.


## Find out the resources available for a charm

To find out what resources are available for a charm on Charmhub, run the `charm-resources` command followed by the name of the charm:

```text
juju charm-resources <charm>
```


````{dropdown} Expand to view a sample output for the postgresql-k8s charm

```text
$ juju charm-resources postgresql-k8s
Resource          Revision
postgresql-image  68
```
````

The command has flags that allow you to specify a charm channel, an output format, an output file, etc.

> See more: {ref}`command-juju-charm-resources`

Alternatively, you can also consider a resource available somewhere else online (e.g., a link to an OCI image) or in your local filesystem.


## Specify the resources to be deployed with a charm

How you specify a resource to deploy with a charm depends on whether you want to do this during deployment or, as an update, post-deployment.


- To specify a resource during deployment, run the `deploy` command with the `--resources` flag followed by a key-value pair consisting of the resource name and the resource:

```text
juju deploy <charm name> --resources <resource name>=<resource>
```

> See more: {ref}`command-juju-deploy`

- To specify a  resource after deployment, run the `attach-resource` command followed by the name of the deployed charm (= {ref}`application <application>`) and a key-value pair consisting of the resource name and the resource revision number of the local path to the resource file:

```text
juju attach-resource <charm name> <resource name>=<resource>
```

Regardless of the case, the resource name is always as defined by the charm author (see the Resources tab of the charm homepage on Charmhub or the `resources` map in the `metadata.yaml` file of the charm) and the resource is the resource revision number, a path to a local file, or a link to a public OCI image (only for OCI-image type resources).


````{dropdown} Expand to view an example where the resource is specified post-deployment by revision number

```text
juju attach-resource  juju-qa-test foo-file=3
```
````

- To update a resource's revision, run the `refresh` command with the `--resource` flag followed by a key=value pair denoting the name of the resource and its revision number or the local path to the resource file.

> See more: {ref}`command-juju-deploy` > `--resources`, {ref}`command-juju-attach-resource`, {ref}`command-juju-refresh` > `--resources`


## View the resources deployed with a charm


To view the resources that have been deployed with a charm, run the `resources` command followed by the name of the corresponding application / ID of one of the application's units.

```text
juju resources <application name> / <unit ID>
```

> See more: {ref}`command-juju-resources`

