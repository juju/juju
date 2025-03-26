(define-cloud-resource-tags-in-a-cloud)=
# How to define resource tags in a cloud

Juju now tags instances and volumes created in supported clouds with the Juju model UUID, and related Juju entities.  This document describes the default instance naming and tagging scheme and then shows you how you can define your own tags.

## The default naming and tagging scheme

Instances and volumes are now named consistently across EC2 and OpenStack, using the scheme:

``` text
juju-<model>-<resource-type>-<resource-ID>
```

...where `<model>` is the given name of the model; `<resource-type>` is the type of the resource ("machine" or "volume") and `<resource-ID>` is the numeric ID of the Juju machine or volume corresponding to the IaaS resource.

Tagging also follows a scheme: Instances will be tagged with the names of units initially assigned to the machine. Volumes will be tagged with the storage-instance name, and the owner (unit or service) of said storage.

For example, names in Amazon AWS appear like this: ![named instances in Amazon](https://assets.ubuntu.com/v1/0261cc58-config-tagging-named.png) 
...and tags like this: 
![tagged instances in Amazon](https://assets.ubuntu.com/v1/f480625d-config-tagging-tagged.png)

## How to define your own tags


Juju also adds any user-specified tags set via the "resource-tags" model setting to instances and volumes. The format of this setting is a space-separated list of key=value pairs.

``` text
resource-tags: key1=value1 [key2=value2 ...]
```

Alternatively, you can change the tags allocated to new machines in a bootstrapped model by using the `juju model-config` command

```text
juju model-config resource-tags="origin=v2 owner=Canonical"
```

![user tagged instances in Amazon](https://assets.ubuntu.com/v1/1fac4427-config-tagging-user.png)

You can change the tags back by running the above command again with different values. Changes will not be made to existing machines, but the new tags will apply to any future machines created.

> See more: {ref}`list-of-model-configuration-keys` > `resource-tags`

These tags may be used, for example, to set up chargeback accounting.

Any tags that Juju manages will be prefixed with "juju-"; users must avoid modifying these, and for safety, it is recommended none of your own tags start with "juju".

