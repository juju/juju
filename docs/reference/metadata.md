(metadata)=
# Simplestreams metadata
> See also: {ref}`manage-metadata`

When Juju bootstraps, it needs two critical pieces of information:
1. The uuid of the image to use when starting new compute instances
2. The URL from which to download the correct version of an agent binary tarball

The necessary information is stored in a json metadata format called simplestreams.
The simplestreams format is used to describe related items in a structural fashion.
See the Launchpad project lp:simplestreams for more details.

For supported public clouds like Amazon, etc., no action is required by the
end user so the following information is more for those interested in what happens
under the covers. Those setting up a private cloud, or who want to change how things
work (eg use a different Ubuntu image), need to pay closer attention.

## Basic workflow

Whether images or agent binaries, Juju uses a search path to try and find suitable metadata.
The path components (in order of lookup) are:

1. For a running model, the Juju controller database.
2. User supplied location (specified by agent-metadata-url or image-metadata-url config settings)
3. Provider specific locations (eg keystone endpoint if on Openstack)
4. A web location with metadata for supported public clouds (https://streams.canonical.com/juju)

Metadata may be inline signed, or unsigned. We indicate a metadata file is signed by using
a '.sjson' extension. Each location in the path is first searched for signed metadata, and
if none is found, unsigned metadata is attempted before moving onto the next path location.

Juju ships with public keys used to validate the integrity of image and agent metadata obtained
from https://streams.canonical.com/juju. So out of the box, Juju will "Just Work" with any supported
public cloud, using signed metadata. Setting up metadata for a private (eg Openstack) cloud requires
metadata to be generated using tools which ship with Juju (more below).

## Image metadata contents

Image metadata uses a simplestreams content type of "image-ids".
The product id is formed as follows:
"com.ubuntu.cloud:server:<series_version>:<arch>"
eg
"com.ubuntu.cloud:server:13.10:amd64"

Non-released images (eg beta, daily etc) have product ids like:
"com.ubuntu.cloud.daily:server:13.10:amd64"

The metadata index and product files are required to be in the following directory tree
(relative to the URL associated with each path component):

<path_url>
  |-streams
      |-v1
         |-index.(s)json
         |-product-foo.(s)json
         |-product-bar.(s)json

The index file must be called "index.(s)json" (sjson for signed). The various product files are
named according to the Path values contained in the index file.

## Tools Metadata Contents

Tools metadata uses a simplestreams content type of "content-download".
The product id is formed as follows:
"com.ubuntu.juju:<os_type>:<arch>"
eg
"com.ubuntu.juju:ubuntu:amd64"

The metadata index and product files are required to be in the following directory tree
(relative to the URL associated with each path component). In addition, agent binaries tarballs
which Juju needs to download are also expected. The tarballs reside in a directory named
after the stream to which they belong.

<path_url>
  |-streams
  |   |-v1
  |      |-index.(s)json
  |      |-product-foo.(s)json
  |      |-product-bar.(s)json
  |
  |-<stream>
      |-tools-abc.tar.gz
      |-tools-def.tar.gz
      |-tools-xyz.tar.gz

The index file must be called "index.(s)json" (sjson for signed). The product file and
agent binary tarball name(s) match whatever is in the index/product files.

## Configuration

For supported public clouds, no extra configuration is required; things work out-of-the-box.
However, for testing purposes, or for non-supported cloud deployments, Juju needs to know
where to find the agent binaries and which image to run. Even for supported public clouds where all
required metadata is available, the user can put their own metadata in the search path to
override what is provided by the cloud.

1. Bootstrap metadata

The first location in the agent binaries search path is the Juju controller database. When Juju bootstraps,
it can optionally read agent binaries and image metadata from a local directory and upload those to the database so
that the agent binaries are cached and available when needed.

The --metadata-source bootstrap argument specifies a top level directory, the structure of which is as follows:

<metadata-source-directory>
  |-tools
  |   |-streams
  |       |-v1
  |   |-released
  |   |-proposed
  |   |-testing
  |   |-devel
  |
  |-images
      |-streams
          |-v1

Of course, if only custom image metadata is required, the tools directory will not be required,
and vice versa.

2. User specified URLs

These are initially specified in the environments.yaml file (and then subsequently copied to the
jenv file when the model is bootstrapped). For images, use "image-metadata-url"; for agent binaries,
use "agent-metadata-url". The URLs can point to a world readable container/bucket in the cloud,
an address served by a http server, or even a shared directory accessible by all node instances
running in the cloud.

eg assume an Apache http server with base URL https://juju-metadata, providing access to
information at <base>/images and <base>/tools. The Juju model yaml file could have
the following entries (one or both):

agent-metadata-url: https://juju-metadata/tools
image-metadata-url: https://juju-metadata/images

The required files in each location is as per the directory layout described earlier.
For a shared directory, use a URL of the form "file:///sharedpath".

3. Provider specific storage

Providers may allow additional locations to search for metadata and agent binaries. For Openstack, keystone
endpoints may be created by the cloud administrator. These are defined as follows:

juju-tools      : the <path_url> value as described above in Tools Metadata Contents
product-streams : the <path_url> value as described above in Image Metadata Contents

4. Central web location (https://streams.canonical.com/juju)

This is the default location used to search for image and agent metadata and is used if no matches
are found earlier in any of the above locations. No user configuration is required.

## Deploying private clouds

There are two main issues when deploying a private cloud:
1. Images ids will be specific to the cloud
2. Often, outside internet access is blocked

Issue 1 means that image id metadata needs to be generated and made available.
Issue 2 means that agent binaries need to be mirrored locally to make them accessible.

Juju tools exist to help with generating and validating image and agent metadata.
For agent binaries, it is often easiest to just mirror https://streams.canonical.com/juju/tools.
However image metadata cannot be simply mirrored because the image ids are taken
from the cloud storage provider, so this need to be generated and validated using
the commands described below.

The available Juju metadata for agent binaries can be seen by using the help command:
  juju help metadata

A summary of the overall workflow is (more detail next):
- create a local directory in which to store image and agent metadata
- generate image metadata to local directory
- optionally download agent binaries to local directory/tools
Then either
- juju bootstrap --metadata-source <local_directory>
or
- optionally, copy image metadata to somewhere in the metadata search path
- optionally, mirror agent binaries to somewhere in the metadata search path
- optionally, configure agent-metadata-url and/or image-metadata-url

If the bootstrap --metadata-source directory option is used, any image metadata and agent binaries found
in the specified directory will be uploaded automatically to the cloud storage for that deployment.
This is useful for situations where image and agent metadata do not need to be shared amongst several
users, since each Juju model will upload its own separate copy of the required files.

Using the image-metadata-url and agent-metadata-url to point to publicly accessible locations is useful
when several Juju models are to be deployed on a private cloud and the metadata should be shared.

1. Image metadata

Generate image metadata using
  juju metadata generate-image -d <metadata_dir>

As a minimum, the above command needs to know the image id to use and a directory in which to write the files.
Other required parameters like region, series, architecture etc are taken from the current Juju model (or
a model specified with the -m option). These parameters can also be overridden on the command line.

The image metadata command can be run multiple times with different regions, series, architecture, and
it will keep adding to the metadata files. Once all required image ids have been added, the index and product
json files are ready to use. These can be uploaded to a location in the Juju metadata search path or the bootstrap
--metadata-source option may be used.

Examples:
1. image-metadata-url
 - upload contents of <metadata_dir> to http://somelocation
 - set image-metadata-url to http://somelocation/images
2. bootstrap option
 - juju bootstrap --metadata-source <metadata_dir>

To ensure that the image metadata has been generated and uploaded correctly, use the validation command to
ensure an image id can be discovered for a given scenario (region series, arch):
  juju metadata validate-images

If run without parameters, the validation command will take all required details from the current Juju
model (or as specified by -m) and print the image id it would use to spin up an instance.
Alternatively, series, region, architecture etc can be specified on the command line to override values
in the model config.

2. Tools metadata

Generally, agent binaries and related metadata are mirrored from https://streams.canonical.com/juju/tools. However,
it is possible to manually generate metadata for a custom built agent binary tarball using:
  juju generate-agent-binaries -d <metadata_dir> --stream <stream>

where the required agent binary tarballs are first placed in a directory <metadata_dir>/tools/<stream>.
If unspecified, <stream> defaults to "released".
Then, the contents of <metadata_dir> can be uploaded to a location in the Juju metadata search path or the
bootstrap --metadata-source option may be used.

Examples:
1. agent-metadata-url
 - upload contents of <metadata_dir> to http://somelocation
 - set agent-metadata-url to http://somelocation/tools
2. bootstrap option
 - juju bootstrap --metadata-source <tools_dir>

Note that image and agent metadata are generally written into the same local directory and the bootstrap
--metadata-source option will upload both types of metadata.

As with image metadata, the validation command is used to ensure tools are available for Juju to use:
  juju metadata validate-agent-binaries

The same comments apply. Run the validation tool without parameters to use details from the Juju
model, or override values as required on the command line. See juju help metadata validate-agent-binaries
for more details.
