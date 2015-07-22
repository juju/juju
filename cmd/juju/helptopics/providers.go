// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package helptopics

const (
	LocalProvider     = helpProviderStart + helpLocalProvider + helpProviderEnd
	OpenstackProvider = helpProviderStart + helpOpenstackProvider + helpProviderEnd
	EC2Provider       = helpProviderStart + helpEC2Provider + helpProviderEnd
	HPCloud           = helpProviderStart + helpHPCloud + helpProviderEnd
	AzureProvider     = helpProviderStart + helpAzureProvider + helpProviderEnd
	MAASProvider      = helpProviderStart + helpMAASProvider + helpProviderEnd
)

const helpProviderStart = `
Start by generating a generic configuration file for Juju, using the command:

  juju init

This will create the '~/.juju/' directory (or $JUJU_HOME, if set) if it doesn't
already exist and generate a file, 'environments.yaml' in that directory.
`
const helpProviderEnd = `
See Also:

  juju help init
  juju help bootstrap

`

const helpLocalProvider = `
The local provider is a Linux-only Juju environment that uses LXC containers as
a virtual cloud on the local machine.  Because of this, lxc and mongodb are
required for the local provider to work. All of these dependencies are tracked
in the 'juju-local' package. You can install that with:

  sudo apt-get update
  sudo apt-get install juju-local

After that you might get error for SSH authorised/public key not found. ERROR
SSH authorised/public key not found.

  ssh-keygen -t rsa

Now you need to tell Juju to use the local provider and then bootstrap:

  juju switch local
  juju bootstrap

The first time this runs it might take a bit, as it's doing a netinstall for
the container, it's around a 300 megabyte download. Subsequent bootstraps
should be much quicker. You'll be asked for your 'sudo' password, which is
needed because only root can create LXC containers. When you need to destroy
the environment, do 'juju destroy-environment local' and you could be asked
for your 'sudo' password again.

You deploy charms from the charm store using the following commands:

  juju deploy mysql
  juju deploy wordpress
  juju add-relation wordpress mysql

For Ubuntu deployments, the local provider will prefer to use lxc-clone to create
the machines for the trusty OS series and later.
A 'template' container is created with the name
  juju-<series>-template
where <series> is the OS series, for example 'juju-trusty-template'.
You can override the use of clone by specifying
  lxc-clone: true
or
  lxc-clone: false
in the configuration for your local provider.  If you have the main container
directory mounted on a btrfs partition, then the clone will be using btrfs
snapshots to create the containers. This means that clones use up much
less disk space.  If you do not have btrfs, lxc will attempt to use aufs
(an overlay type filesystem). You can explicitly ask Juju to create
full containers and not overlays by specifying the following in the provider
configuration:
  lxc-clone-aufs: false


References:

  http://askubuntu.com/questions/65359/how-do-i-configure-juju-for-local-usage
  https://juju.ubuntu.com/docs/getting-started.html
`

const helpOpenstackProvider = `
Here's an example OpenStack configuration:

  sample_openstack:
    type: openstack

    # Specifies whether the use of a floating IP address is required to
    # give the nodes a public IP address. Some installations assign public
    # IP addresses by default without requiring a floating IP address.
    # use-floating-ip: false

    # Specifies whether new machine instances should have the "default"
    # Openstack security group assigned.
    # use-default-secgroup: false

    # Usually set via the env variable OS_AUTH_URL, but can be specified here
    # auth-url: https://yourkeystoneurl:443/v2.0/

    # The following are used for userpass authentication (the default)
    # auth-mode: userpass

    # Usually set via the env variable OS_USERNAME, but can be specified here
    # username: <your username>

    # Usually set via the env variable OS_PASSWORD, but can be specified here
    # password: <secret>

    # Usually set via the env variable OS_TENANT_NAME, but can be specified here
    # tenant-name: <your tenant name>

    # Usually set via the env variable OS_REGION_NAME, but can be specified here
    # region: <your region>

If you have set the described OS_* environment variables, you only need "type:".
References:

  http://juju.ubuntu.com/docs/provider-configuration-openstack.html
  http://askubuntu.com/questions/132411/how-can-i-configure-juju-for-deployment-on-openstack

Placement directives:

  OpenStack environments support the following placement directives for use
  with "juju bootstrap" and "juju add-machine":

    zone=<availability-zone-name>
      The "zone" placement directive instructs the OpenStack provider to
      allocate a machine in the specified availability zone. If the zone
      does not exist, or a machine cannot be allocated within it, then
      the machine addition will fail.

Other OpenStack Based Clouds:

This answer is for generic OpenStack support, if you're using an OpenStack-based
provider check these questions out for provider-specific information:

  https://juju.ubuntu.com/docs/config-hpcloud.html

`

const helpEC2Provider = `
Configuring the EC2 environment requires telling Juju about your AWS access key
and secret key. To do this, you can either set the 'AWS_ACCESS_KEY_ID' and
'AWS_SECRET_ACCESS_KEY' environment variables[1] (as usual for other EC2 tools)
or you can add access-key and secret-key options to your environments.yaml.
These are already in place in the generated config, you just need to uncomment
them out. For example:

  sample_ec2:
    type: ec2
    # access-key: YOUR-ACCESS-KEY-GOES-HERE
    # secret-key: YOUR-SECRET-KEY-GOES-HERE

See the EC2 provider documentation[2] for more options.

Note If you already have an AWS account, you can determine your access key by
visiting your account page[3], clicking "Security Credentials" and then clicking
"Access Credentials". You'll be taken to a table that lists your access keys and
has a "show" link for each access key that will reveal the associated secret
key.

And that's it, you're ready to go!

Placement directives:

  EC2 environments support the following placement directives for use with
  "juju bootstrap" and "juju add-machine":

    zone=<availability-zone-name>
      The "zone" placement directive instructs the EC2 provider to
      allocate a machine in the specified availability zone. If the zone
      does not exist, or a machine cannot be allocated within it, then
      the machine addition will fail.

References:

  [1]: http://askubuntu.com/questions/730/how-do-i-set-environment-variables
  [2]: https://juju.ubuntu.com/docs/provider-configuration-ec2.html
  [3]: http://aws.amazon.com/account

More information:

  https://juju.ubuntu.com/docs/getting-started.html
  https://juju.ubuntu.com/docs/provider-configuration-ec2.html
  http://askubuntu.com/questions/225513/how-do-i-configure-juju-to-use-amazon-web-services-aws
`

const helpHPCloud = `
HP Cloud is an Openstack cloud provider.  To deploy to it, use an openstack
environment type for Juju, which would look something like this:

  sample_hpcloud:
    type: openstack
    tenant-name: "juju-project1"
    auth-url: https://region-a.geo-1.identity.hpcloudsvc.com:35357/v2.0/
    auth-mode: userpass
    username: "xxxyour-hpcloud-usernamexxx"
    password: "xxxpasswordxxx"
    region: az-1.region-a.geo-1

See the online help for more information:

  https://juju.ubuntu.com/docs/config-hpcloud.html
`

const helpAzureProvider = `
A generic Windows Azure environment looks like this:

  sample_azure:
    type: azure

    # Location for instances, e.g. West US, North Europe.
    location: West US

    # http://msdn.microsoft.com/en-us/library/windowsazure
    # Windows Azure Management info.
    management-subscription-id: 886413e1-3b8a-5382-9b90-0c9aee199e5d
    management-certificate-path: /home/me/azure.pem

    # Windows Azure Storage info.
    storage-account-name: juju0useast0

    # Override OS image selection with a fixed image for all deployments.
    # Most useful for developers.
    # force-image-name: b39f27a8b8c64d52b05eac6a62ebad85__Ubuntu-13_10-amd64-server-DEVELOPMENT-20130713-Juju_ALPHA-en-us-30GB

    # image-stream chooses a simplestreams stream from which to select
    # OS images, for example daily or released images (or any other stream
    # available on simplestreams).
    #
    # image-stream: "released"

    # agent-stream chooses a simplestreams stream from which to select tools,
    # for example released or proposed tools (or any other stream available
    # on simplestreams).
    #
    # agent-stream: "released"

This is the environments.yaml configuration file needed to run on Windows Azure.
You will need to set the management-subscription-id, management-certificate-
path, and storage-account-name.

Note: Other than location, the defaults are recommended, but can be updated to
your preference.

See the online help for more information:

  https://juju.ubuntu.com/docs/config-azure.html
`

const helpMAASProvider = `
A generic MAAS environment looks like this:

  sample_maas:
    type: maas
    maas-server: 'http://<my-maas-server>:80/MAAS'
    maas-oauth: 'MAAS-API-KEY'

The API key can be obtained from the preferences page in the MAAS web UI.

Placement directives:

  MAAS environments support the following placement directives for use with
  "juju bootstrap" and "juju add-machine":

    zone=<physical-zone-name>
      The "zone" placement directive instructs the MAAS provider to
      allocate a machine in the specified availability zone. If the zone
      does not exist, or a machine cannot be allocated within it, then
      the machine addition will fail.

    <hostname>
      If the placement directive does not contain an "=" symbol, then
      it is assumed to be the hostname of a node in MAAS. MAAS will attempt
      to acquire that node and will fail if it cannot.

See the online help for more information:

  https://juju.ubuntu.com/docs/config-maas.html
`
