// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

const helpBasics = `
Juju -- devops distilled
https://juju.ubuntu.com/

Juju provides easy, intelligent service orchestration on top of environments
such as Amazon EC2, HP Cloud, OpenStack, MaaS, or your own local machine.

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
  juju help azure-provider       use on Windows Azure
  juju help ec2-provider         use on Amazon EC2
  juju help hpcloud-provider     use on HP Cloud
  juju help local-provider       use on this computer
  juju help openstack-provider   use on OpenStack
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

const helpConstraints = `
Constraints constrain the possible instances that may be started by juju
commands. They are usually passed as a flag to commands that provision a new
machine (such as bootstrap, deploy, and add-machine).

Each constraint defines a minimum acceptable value for a characteristic of a
machine.  Juju will provision the least expensive machine that fulfills all the
constraints specified.  Note that these values are the minimum, and the actual
machine used may exceed these specifications if one that exactly matches does
not exist.

If a constraint is defined that cannot be fulfilled by any machine in the
environment, no machine will be provisioned, and an error will be printed in the
machine's entry in juju status.

Constraint defaults can be set on an environment or on specific services by
using the set-constraints command (see juju help set-constraints).  Constraints
set on the environment or on a service can be viewed by using the get-
constraints command.  In addition, you can specify constraints when executing a
command by using the --constraints flag (for commands that support it).

Constraints specified on the environment and service will be combined to
determine the full list of constraints on the machine(s) to be provisioned by
the command.  Service-specific constraints will override environment-specific
constraints, which override the juju default constraints.

Constraints are specified as key value pairs separated by an equals sign, with
multiple constraints delimited by a space.

Constraint Types:

arch
   Arch defines the CPU architecture that the machine must have.  Currently
   recognized architectures:
      amd64 (default)
      i386
      arm

cpu-cores
   Cpu-cores is a whole number that defines the number of effective cores the
   machine must have available.

mem
   Mem is a float with an optional suffix that defines the minimum amount of RAM
   that the machine must have.  The value is rounded up to the next whole
   megabyte.  The default units are megabytes, but you can use a size suffix to
   use other units:

      M megabytes (default)
      G gigabytes (1024 megabytes)
      T terabytes (1024 gigabytes)
      P petabytes (1024 terabytes)

root-disk
   Root-Disk is a float that defines the amount of space in megabytes that must
   be available in the machine's root partition.  For providers that have
   configurable root disk sizes (such as EC2) an instance with the specified
   amount of disk space in the root partition may be requested.  Root disk size
   defaults to megabytes and may be specified in the same manner as the mem
   constraint.

container
   Container defines that the machine must be a container of the specified type.
   A container of that type may be created by juju to fulfill the request.
   Currently supported containers:
      none - (default) no container
      lxc - an lxc container
      kvm - a kvm container

cpu-power
   Cpu-power is a whole number that defines the speed of the machine's CPU,
   where 100 CpuPower is considered to be equivalent to 1 Amazon ECU (or,
   roughly, a single 2007-era Xeon).  Cpu-power is currently only supported by
   the Amazon EC2 environment.

tags
   Tags defines the list of tags that the machine must have applied to it.
   Multiple tags must be delimited by a comma. Tags are currently only supported
   by the MaaS environment.

Example:

   juju add-machine --constraints "arch=amd64 mem=8G tags=foo,bar"

See Also:
   juju help set-constraints
   juju help get-constraints
   juju help deploy
   juju help add-unit
   juju help add-machine
   juju help bootstrap
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

const helpLogging = `
Juju has logging available for both client and server components. Most
users' exposure to the logging mechanism is through either the 'debug-log'
command, or through the log file stored on the bootstrap node at
/var/log/juju/all-machines.log.

All the agents have their own log files on the individual machines. So
for the bootstrap node, there is the machine agent log file at
/var/log/juju/machine-0.log.  When a unit is deployed on a machine,
a unit agent is started. This agent also logs to /var/log/juju and the
name of the log file is based on the id of the unit, so for wordpress/0
the log file is unit-wordpress-0.log.

Juju uses rsyslog to forward the content of all the log files on the machine
back to the bootstrap node, and they are accumulated into the all-machines.log
file.  Each line is prefixed with the source agent tag (also the same as
the filename without the extension).

Juju has a hierarchical logging system internally, and as a user you can
control how much information is logged out.

Output from the charm hook execution comes under the log name "unit".
By default Juju makes sure that this information is logged out at
the DEBUG level.  If you explicitly specify a value for unit, then
this is used instead.

Juju internal logging comes under the log name "juju".  Different areas
of the codebase have different anmes. For example:
  providers are under juju.provider
  workers are under juju.worker
  database parts are under juju.state

All the agents are started with all logging set to DEBUG. Which means you
see all the internal juju logging statements until the logging worker starts
and updates the logging configuration to be what is stored for the environment.

You can set the logging levels using a number of different mechanisms.

environments.yaml
 - all environments support 'logging-config' as a key
 - logging-config: ...
environment variable
 - export JUJU_LOGGING_CONFIG='...'
setting the logging-config at bootstrap time
 - juju bootstrap --logging-config='...'
juju set-environment logging-config='...'

Configuration values are separated by semicolons.

Examples:

  juju set-environment logging-config "juju=WARNING; unit=INFO"

Developers may well like:

  export JUJU_LOGGING_CONFIG='juju=INFO; juju.current.work.area=TRACE'

Valid logging levels:
  CRITICAL
  ERROR
  WARNING
  INFO
  DEBUG
  TRACE
`
