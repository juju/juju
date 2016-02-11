// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package helptopics

const (
	OpenstackProvider = helpProviderStart + helpOpenstackProvider + helpProviderEnd
	EC2Provider       = helpProviderStart + helpEC2Provider + helpProviderEnd
	HPCloud           = helpProviderStart + helpHPCloud + helpProviderEnd
	AzureProvider     = helpProviderStart + helpAzureProvider + helpProviderEnd
	MAASProvider      = helpProviderStart + helpMAASProvider + helpProviderEnd
)

const helpProviderStart = `
Start by generating a generic configuration file for Juju, using the command:

  juju init

This will create the $JUJU_DATA directory, if set, otherwise it will try
$XDG_DATA_HOME/juju if said variable is set or finally default to
~/.local/share/juju if it doesn't already exist and generate a file, 
'environments.yaml' in that directory.
`
const helpProviderEnd = `
See Also:

  juju help init
  juju help bootstrap

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

If you have set the described OS_* model variables, you only need "type:".
References:

  http://juju.ubuntu.com/docs/provider-configuration-openstack.html
  http://askubuntu.com/questions/132411/how-can-i-configure-juju-for-deployment-on-openstack

Placement directives:

  OpenStack models support the following placement directives for use
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
Configuring the EC2 model requires telling Juju about your AWS access key
and secret key. To do this, you can either set the 'AWS_ACCESS_KEY_ID' and
'AWS_SECRET_ACCESS_KEY' model variables[1] (as usual for other EC2 tools)
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

  EC2 models support the following placement directives for use with
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
model type for Juju, which would look something like this:

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
A generic Windows Azure model looks like this:

  sample_azure:
    type: azure

    # Location for instances, e.g. West US, North Europe.
    location: West US

    # application-id is the ID of an application you create in Azure Active
    # Directory for Juju to use. For instructions on how to do this, see:
    #   https://azure.microsoft.com/en-us/documentation/articles/resource-group-authenticate-service-principal
    application-id: 00000000-0000-0000-0000-000000000000

    # application-password is the password specified when creating the
    # application in Azure Active Directory.
    application-password: XXX

    # subscription-id defines the Azure account subscription ID to
    # manage resources in. You can list your account subscriptions
    # with the Azure CLI's "account list" action: "azure account list".
    # The ID associated with each account is the subscription ID.
    subscription-id: 00000000-0000-0000-0000-000000000000

    # tenant-id is the ID of the Azure tenant, which identifies the Azure
    # Active Directory instance. You can obtain this ID by using the Azure
    # CLI's "account show" action. First list your accounts with
    # "azure account list", and then feed the account ID to
    # "azure account show" to obtain the properties of the account, including
    # the tenant ID.
    tenant-id: 00000000-0000-0000-0000-000000000000

    # storage-account-type specifies the type of the storage account,
    # which defines the replication strategy and support for different
    # disk types.
    storage-account-type: Standard_LRS

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
You will need to set application-id, application-password, subscription-id,
and tenant-id.

Note: Other than location, the defaults are recommended, but can be updated to
your preference.

See the online help for more information:

  https://juju.ubuntu.com/docs/config-azure.html
`

const helpMAASProvider = `
A generic MAAS model looks like this:

  sample_maas:
    type: maas
    maas-server: 'http://<my-maas-server>:80/MAAS'
    maas-oauth: 'MAAS-API-KEY'

The API key can be obtained from the preferences page in the MAAS web UI.

Placement directives:

  MAAS models support the following placement directives for use with
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
