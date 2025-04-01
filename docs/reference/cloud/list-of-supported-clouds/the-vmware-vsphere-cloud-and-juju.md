(cloud-vsphere)=
# The VMware vSphere cloud and Juju


This document describes details specific to using your existing VMware vSphere cloud with Juju. 

> See more: [VMware vSphere](https://docs.vmware.com/) 

When using this cloud with Juju, it is important to keep in mind that it is a (1) machine cloud and (2) not some other cloud.

> See more: {ref}`cloud-differences`

As the differences related to (1) are already documented generically in the rest of the docs, here we record just those that follow from (2).


## Requirements

In order to add a vSphere cloud you will need to have an existing vSphere installation which supports, or has access to, the following: <p> - VMware Hardware Version 8 (or greater) <p> - ESXi 5.0 (or greater) <p> - Internet access <p> - DNS and DHCP <p> Juju supports both high-availability vSAN deployments as well and standard deployments.

## Notes on `juju add-cloud`

Type in Juju: `vsphere`

Name in Juju: User-defined.

## Notes on `juju add-credential`


### Authentication types



#### `userpass`
Attributes:
- user: The username to authenticate with. (required)
- password: The password to authenticate with. (required)
- vmfolder: The folder to add VMs from the model. (optional)


## Notes on `juju bootstrap`

Recommended: Bootstrap with the following cloud-specific model-configuration keys: `datastore` and `primary-network`. See more below. <p> **Pro tip:** When creating a controller with vSphere, a cloud image is downloaded to the client and then uploaded to the ESX host. This depends on your network connection and can take a while.  Using templates can speed up bootstrap and machine deployment.

## Cloud-specific model configuration keys

### `datastore`
The datastore in which to create VMs. If this is not specified, the process will abort unless there is only one datastore available.

| | |
|-|-|
| type | string |
| default value | schema.omit{} |
| immutable | false |
| mandatory | false |

### `primary-network`
The primary network that VMs will be connected to. If this is not specified, Juju will look for a network named "VM Network".

| | |
|-|-|
| type | string |
| default value | schema.omit{} |
| immutable | false |
| mandatory | false |

### `force-vm-hardware-version`
The HW compatibility version to use when cloning a VM template to create a VM. The version must be supported by the remote compute resource, and greater or equal to the template’s version.

| | |
|-|-|
| type | int |
| default value | 0 |
| immutable | false |
| mandatory | false |

### `enable-disk-uuid`
Expose consistent disk UUIDs to the VM, equivalent to disk.EnableUUID. The default is True.

| | |
|-|-|
| type | bool |
| default value | true |
| immutable | false |
| mandatory | false |

### `disk-provisioning-type`
Specify how the disk should be provisioned when cloning the VM template. Allowed values are: thickEagerZero (default), thick and thin.

| | |
|-|-|
| type | string |
| default value | "thick" |
| immutable | false |
| mandatory | false |

### `external-network`
An external network that VMs will be connected to. The resulting IP address for a VM will be used as its public address.

| | |
|-|-|
| type | string |
| default value | "" |
| immutable | false |
| mandatory | false |


## Supported constraints

| {ref}`CONSTRAINT <constraint>`         |                                                                                                                                                                                                                                                                                                                                      |
|----------------------------------------|--------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| conflicting:                           |                                                                                                                                                                                                                                                                                                                                      |
| supported?                             |                                                                                                                                                                                                                                                                                                                                      |
| - {ref}`constraint-allocate-public-ip` | &#10005;                                                                                                                                                                                                                                                                                                                             |
| - {ref}`constraint-arch`               | &#10003; <br> Valid values: `{ref}`amd64]`.                                                                                                                                                                                                                                                                                |
| - {ref}`constraint-container`          | &#10003;                                                                                                                                                                                                                                                                                                                    |
| - {ref}`constraint-cores`              | &#10003;                                                                                                                                                                                                                                                                                                                    |
| - {ref}`constraint-cpu-power`          | &#10003;                                                                                                                                                                                                                                                                                                                    |
| - {ref}`constraint-image-id`           | &#10005;                                                                                                                                                                                                                                                                                                                             |
| - {ref}`constraint-instance-role`      | &#10005;                                                                                                                                                                                                                                                                                                                             |
| - {ref}`constraint-instance-type`      | &#10003;                                                                                                                                                                                                                                                                                                                    |
| - {ref}`constraint-mem`                | &#10003;                                                                                                                                                                                                                                                                                                                    |
| - {ref}`constraint-root-disk`          | &#10003;                                                                                                                                                                                                                                                                                                                    |
| - {ref}`constraint-root-disk-source`   | &#10003;  <br> `root-disk-source` is the datastore for the root disk                                                                                                                                                                                                                                                        |
| - {ref}`constraint-spaces`             | &#10005;                                                                                                                                                                                                                                                                                                                             |
| - {ref}`constraint-tags`               | &#10005;                                                                                                                                                                                                                                                                                                                             |
| - {ref}`constraint-virt-type`          | &#10005;                                                                                                                                                                                                                                                                                                                             |
| - {ref}`constraint-zones`              | &#10003;  <p> Use to specify  resurce pools within a host or cluster, e.g. <p> `juju deploy myapp --constraints zones=myhost` <p> `juju deploy myapp --constraints zones=myfolder/myhost`<p> `juju deploy myapp --constraints zones=mycluster/mypool` <p> `juju deploy myapp --constraints zones=mycluster/myparent/mypool` |


## Supported placement directives

| {ref}`PLACEMENT DIRECTIVE <placement-directive>` |   |
|--------------------------------------------------|---|
| {ref}`placement-directive-machine`               | &#10003;                                                                                                                                                                                                                                                                                                                                                          |
| {ref}`placement-directive-subnet`                | &#10005;                                                                                                                                                                                                                                                                                                                                                                   |
| {ref}`placement-directive-system-id`             | &#10005;                                                                                                                                                                                                                                                                                                                                                                   |
| {ref}`placement-directive-zone`                  | &#10003;  <br> Valid values: `<cluster\|host>`. <p> :warning:  If your topology has a cluster without a host, Juju will see this as an availability zone and may fail silently. To solve this, either make sure the host is within the cluster, or use a placement directive: `juju bootstrap vsphere/<datacenter> <controllername> --to zone=<cluster\|host>`.   |





## Other notes

### Using templates

To speed up bootstrap and deploy, you can use VM templates, already created in your vSphere.  Templates can be created by hand on your vSphere, or created from an existing VM.  

Examples assume that the templates are in directory $DATA_STORE/templates.

Via simplestreams:
```text
mkdir -p $HOME/simplestreams
juju-metadata generate-image -d $HOME/simplestreams/ -i "templates/juju-focal-template" --base ubuntu@22.04 -r $DATA_STORE -u $CLOUD_ENDPOINT
juju-metadata generate-image -d $HOME/simplestreams/ -i "templates/juju-noble-template" --base ubuntu@24.04 -r $DATA_STORE -u $CLOUD_ENDPOINT
juju bootstrap --metadata-source $HOME/image-streams vsphere
```

Bootstrap juju with the controller on a VM running focal:
```text
juju bootstrap vsphere --bootstrap-image="templates/focal-test-template"  --bootstrap-base ubuntu@22.04 --bootstrap-constraints "arch=amd64"
```

Using [add-image](https://discourse.charmhub.io/t/new-feature-in-juju-2-8-add-custom-machine-images-with-the-juju-metadata-command/3171):
```text
juju metadata add-image templates/bionic-test-template --base ubuntu@22.04
```
