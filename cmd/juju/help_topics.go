// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

const helpBasics = `
Juju -- devops distilled
https://juju.ubuntu.com/

Juju provides easy, intelligent service orchestration on top of environments
such as Amazon AWS, HP Cloud, OpenStack, MaaS, or your own local machine.

Basic commands:
  juju init             generate boilerplate configuration for juju environments
  juju bootstrap        start up an environment from scratch

  juju deploy           deploy a new service
  juju add-relation     add a relation between two services
  juju expose           expose a service

  juju help bootstrap   more help on e.g. bootstrap command
  juju help commands    list all commands
  juju help glossary    glossary of terms
  juju help topics      list all help topics

Provider information:
  juju help azure       use on Windows Azure
  juju help ec2         use on Amazon EC2
  juju help hpcloud     use on HP Cloud
  juju help local       use on this computer
  juju help openstack   use on OpenStack
`

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
The local provider is a linux-only Juju environment that uses LXC containers as
a virtual cloud on the local machine.  Because of this, lxc and mongodb are
required for the local provider to work.  If you don't already have lxc and
mongodb installed, run the following commands:

  sudo apt-get update
  sudo apt-get install lxc mongodb-server

After that you might get error for SSH authorized/public key not found. ERROR
SSH authorized/public key not found.

  ssh-keygen -t rsa

Now you need to tell Juju to use the local provider and then bootstrap:

  juju switch local
  sudo juju bootstrap

The first time this runs it might take a bit, as it's doing a netinstall for
the container, it's around a 300 megabyte download. Subsequent bootstraps
should be much quicker. 'sudo' is needed because only root can create LXC
containers. After the initial bootstrap, you do not need 'sudo' anymore,
except to 'sudo juju destroy-environment' when you want to tear everything
down.

You deploy charms from the charm store using the following commands:

  juju deploy mysql
  juju deploy wordpress
  juju add-relation wordpress mysql

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
    admin-secret: 13850d1b9786065cadd0f477e8c97cd3
    # Globally unique swift bucket name
    control-bucket: juju-fd6ab8d02393af742bfbe8b9629707ee
    # Usually set via the env variable OS_AUTH_URL, but can be specified here
    # auth-url: https://yourkeystoneurl:443/v2.0/
    # override if your workstation is running a different series to which
    # you are deploying
    # The following are used for userpass authentication (the default)
    auth-mode: userpass
    # Usually set via the env variable OS_USERNAME, but can be specified here
    # username: <your username>
    # Usually set via the env variable OS_PASSWORD, but can be specified here
    # password: <secret>
    # Usually set via the env variable OS_TENANT_NAME, but can be specified here
    # tenant-name: <your tenant name>
    # Usually set via the env variable OS_REGION_NAME, but can be specified here
    # region: <your region>

References:

  http://juju.ubuntu.com/docs/provider-configuration-openstack.html
  http://askubuntu.com/questions/132411/how-can-i-configure-juju-for-deployment-on-openstack

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
    control-bucket: juju-faefb490d69a41f0a3616a4808e0766b
    admin-secret: 81a1e7429e6847c4941fda7591246594

See the EC2 provider documentation[2] for more options. The S3 bucket does not
need to exist already.

Note If you already have an AWS account, you can determine your access key by
visiting your account page[3], clicking "Security Credentials" and then clicking
"Access Credentials". You'll be taken to a table that lists your access keys and
has a "show" link for each access key that will reveal the associated secret
key.

And that's it, you're ready to go!

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
HP Cloud is an Openstack cloud provider, so to deploy to it, use an openstack
environment type for Juju, which would look something like this:

  sample_hpcloud:
    type: openstack
    admin-secret: 6638bebf0c54ffff1007e0247d4dae98
    control-bucket: juju-bc66a4a4adbee50b2ceeee70436528e5
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
    admin-secret: 35d65be36c72da940933dd02f8d7cef0
    # Location for instances, e.g. West US, North Europe.
    location: West US
    # http://msdn.microsoft.com/en-us/library/windowsazure
    # Windows Azure Management info.
    management-subscription-id: 886413e1-3b8a-5382-9b90-0c9aee199e5d
    management-certificate-path: /home/me/azure.pem
    # Windows Azure Storage info.
    storage-account-name: juju0useast0
    # Public Storage info (account name and container name) denoting a public
    # container holding the juju tools.
    # public-storage-account-name: jujutools
    # public-storage-container-name: juju-tools
    # Override OS image selection with a fixed image for all deployments.
    # Most useful for developers.
    # force-image-name: b39f27a8b8c64d52b05eac6a62ebad85__Ubuntu-13_10-amd64-server-DEVELOPMENT-20130713-Juju_ALPHA-en-us-30GB
    # Pick a simplestreams stream to select OS images from: daily or released
    # images, or any other stream available on simplestreams.  Leave blank for
    # released images.
    # image-stream: ""

This is the environments.yaml configuration file needed to run on Windows Azure.
You will need to set the management-subscription-id, management-certificate-
path, and storage-account-name.

Note: Other than location, the defaults are recommended, but can be updated to
your preference.

See the online help for more information:

  https://juju.ubuntu.com/docs/config-azure.html
`

const helpGlossary = `
Bootstrap
  To boostrap an environment means initializing it so that Services may be
  deployed on it.

Charm
  A Charm provides the definition of the service, including its metadata,
  dependencies to other services, packages necessary, as well as the logic for
  management of the application. It is the layer that integrates an external
  application component like Postgres or WordPress into Juju. A Juju Service may
  generally be seen as the composition of its Juju Charm and the upstream
  application (traditionally made available through its package).

Charm URL
  A Charm URL is a resource locator for a charm, with the following format and
  restrictions:

    <schema>:[~<user>/]<collection>/<name>[-<revision>]

  schema must be either "cs", for a charm from the Juju charm store, or "local",
  for a charm from a local repository.

  user is only valid in charm store URLs, and allows you to source charms from
  individual users (rather than from the main charm store); it must be a valid
  Launchpad user name.

  collection denotes a charm's purpose and status, and is derived from the
  Ubuntu series targeted by its contained charms: examples include "precise",
  "quantal", "oneiric-universe".

  name is just the name of the charm; it must start and end with lowercase
  (ascii) letters, and can otherwise contain any combination of lowercase
  letters, digits, and "-"s.

  revision, if specified, points to a specific revision of the charm pointed to
  by the rest of the URL. It must be a non-negative integer.

Endpoint
  The combination of a service name and a relation name.

Environment
  An Environment is a configured location where Services can be deployed onto.
  An Environment typically has a name, which can usually be omitted when there's
  a single Environment configured, or when a default is explicitly defined.
  Depending on the type of Environment, it may have to be bootstrapped before
  interactions with it may take place (e.g. EC2). The local environment
  configuration is defined in the ~/.juju/environments.yaml file.

Machine Agent
  Software which runs inside each machine that is part of an Environment, and is
  able to handle the needs of deploying and managing Service Units in this
  machine.

Provisioning Agent
  Software responsible for automatically allocating and terminating machines in
  an Environment, as necessary for the requested configuration.

Relation
  Relations are the way in which Juju enables Services to communicate to each
  other, and the way in which the topology of Services is assembled. The Charm
  defines which Relations a given Service may establish, and what kind of
  interface these Relations require.
 
  In many cases, the establishment of a Relation will result into an actual TCP
  connection being created between the Service Units, but that's not necessarily
  the case. Relations may also be established to inform Services of
  configuration parameters, to request monitoring information, or any other
  details which the Charm author has chosen to make available.

Repository
  A location where multiple charms are stored. Repositories may be as simple as
  a directory structure on a local disk, or as complex as a rich smart server
  supporting remote searching and so on.

Service
  Juju operates in terms of services. A service is any application (or set of
  applications) that is integrated into the framework as an individual component
  which should generally be joined with other components to perform a more
  complex goal.

  As an example, WordPress could be deployed as a service and, to perform its
  tasks properly, might communicate with a database service and a load balancer
  service.

Service Configuration
  There are many different settings in a Juju deployment, but the term Service
  Configuration refers to the settings which a user can define to customize the
  behavior of a Service.

  The behavior of a Service when its Service Configuration changes is entirely
  defined by its Charm.

Service Unit
  A running instance of a given Juju Service. Simple Services may be deployed
  with a single Service Unit, but it is possible for an individual Service to
  have multiple Service Units running in independent machines. All Service Units
  for a given Service will share the same Charm, the same relations, and the
  same user-provided configuration.

  For instance, one may deploy a single MongoDB Service, and specify that it
  should run 3 Units, so that the replica set is resilient to failures.
  Internally, even though the replica set shares the same user-provided
  configuration, each Unit may be performing different roles within the replica
  set, as defined by the Charm.

Service Unit Agent
  Software which manages all the lifecycle of a single Service Unit.

`
