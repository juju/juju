(manage-metadata)=
# How to manage Simplestreams metadata
> See also: {ref}`metadata`
<!--
Using this to replace https://juju.is/docs/juju/cloud-image-metadata (https://discourse.charmhub.io/t/how-to-configure-machine-image-metadata-for-your-openstack-cloud/1137), because the recipe here is actually generic -- not specific to OpenStack.

Still, revisit that doc to see if there's anything else we'd like to bring here to make the story clearer.

Also consider this section in our Ref vsphere doc: https://discourse.charmhub.io/t/vmware-vsphere-and-juju/1099#heading--using-templates

-->

When Juju creates a controller it needs two critical pieces of information:

- *(For machine clouds:)* **Metadata regarding the LXD container image and LXD VM image to use:**  The unique identifier of the image to use when spawning a new machine (instance).
- *(For all clouds:)* **Metadata regarding the agent binaries:** The URL from which to download the correct Juju agent.


This metadata is stored in a JSON format called 'Simplestreams'. The image metadata is available by default for all the public clouds that Juju supports but needs to be generated if you're setting up your own private cloud. The agent binary metadata is available by default for all clouds but developers may want to generate for testing (though an alternative is `juju sync-agent-binaries`).

This document shows how to manage this metadata in Juju.

## Generate metadata

**For cloud images.** To generate metadata for cloud images, use the `metadata` plugin with the `generate-image` subcommand. This is useful for creating metadata for custom images. The metadata is stored in *SimpleStreams*, a data format designed to provide a standardized way to represent and discover metadata about cloud resources.
```text
juju metadata generate-image
```

The cloud specification comes from the current Juju model, but it is possible to override certain cloud attributes, including the region, endpoint, and charm base using the command arguments. While "amd64" serves as the default setting for the architecture, this option can also be adjusted to accommodate different architectural requirements.

> See more: {ref}`plugin-juju-metadata` > `generate-image`

The generated metadata image can then be used to speed up bootstrap and deployment.

> See more: {ref}`command-juju-bootstrap`, {ref}`cloud-vsphere`

**For agent binaries.** To create metadata for Juju agent binaries, use the `metadata` plugin with the `generate-agent-binaries` subcommand. This generates simplestreams metadata for agent binaries, facilitating their discovery and use.

```text
juju metadata generate-agent-binaries -d <workingdir>
```

The simplestream stream for which metadata is generated is specified using the `--stream`
parameter (default is "released"). Metadata can be generated for any supported
stream - released, proposed, testing, devel.

```text
juju metadata generate-agent-binaries -d <workingdir> --stream proposed
```

Newly generated metadata will be merged with any existing metadata that is already there. To first remove metadata for the specified stream before generating new metadata,
 use the `--clean` option.

```text
juju metadata generate-agent-binaries -d <workingdir> --stream proposed --clean
```

> See more: {ref}`plugin-juju-metadata` > `generate-agent-binaries`

## Validate metadata

**For images.** To validate image metadata and ensure the specified image or images exist for a model, use the `metadata` plugin with the `validate-images` subcommand.

```text
juju metadata validate-images
```

The key model attributes may be overridden using command arguments, so
that the validation may be performed on arbitrary metadata.

<!-- A key use case is to validate newly generated metadata prior to deployment to production. In this case, the metadata is placed in a local directory, a cloud provider type is specified (ec2, openstack etc), and the validation is performed for each supported region and base. -->

> See more: {ref}`plugin-juju-metadata` > `validate-images`

**For agent binaries.** To ensure that the compressed tar archives (.tgz) for the Juju agent binaries are available and correct, use the `metadata` plugin with the `validate-agent-binaries` subcommand. For example:

```text
juju metadata validate-agent-binaries
```

It is also possible to indicate the os type for which to validate, the cloud provider, region as well as the endpoint. It is possible to specify a local directory containing agent metadata, in which case cloud attributes like provider type, region etc are optional.

> See more: {ref}`plugin-juju-metadata` > `validate-agent-binaries`

## Add metadata

**For images.** To add custom image metadata to your model, use the `metadata` plugin with the `add-image` subcommand followed by the unique image identifier and specifying the base. This is useful when you have specific cloud images that you want Juju to use for creating instances. For example:

```text
juju metadata add-image <image-id> --base <base>
```
It is also possible to pass various options to specify the image architecture, choose a model to operate in, and the cloud region where this image exists, etc.

> See more: {ref}`plugin-juju-metadata` > `add-image`

## Sign metadata

If you need to sign your simplestreams metadata for security purposes, use the `metadata` plugin with the `sign` subcommand.

```text
juju metadata sign -d <directory> -k <key>
```

The specified keyring file is expected to contain an amored private key. If the key
is encrypted, then a passphrase should be specified using the command option `--passphrase` to decrypt the key.

> See more: {ref}`plugin-juju-metadata` > `sign`

## View all the known metadata

**For images.** To view a list of cloud image metadata currently used by Juju, use the `metadata` plugin with the `images` (or its alias `list-images` ) subcommand. This shows the images Juju considers when choosing an image to start.

```text
juju metadata images
```
```text
juju metadata list-images --format yaml
```

The result list can be filtered in order to show specific images for a region, architecture or a set of bases using the OS name and the version. For example:

```text
juju metadata images --bases ubuntu@22.04 --region eu-west-1 --model mymodel
```
> See more: {ref}`plugin-juju-metadata` > `images`

## Delete metadata

**For images.** To remove previously added image metadata from a Juju environment, use the `metadata` plugin with the `delete-image` subcommand followed by the id of the image.

```text
juju metadata delete-image <image-id>
```
The command also allows you to specify whether this operation should show a verbose output or no informational output at all. The `--model` option can be set in order to specify the model to operate in.

> See more: {ref}`plugin-juju-metadata` > `delete-image`
