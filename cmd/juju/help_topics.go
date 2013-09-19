// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

const helpBasics = `
Juju -- devops distilled
https://juju.ubuntu.com/

Juju provides easy, intelligent service orchestration on top of environments
such as OpenStack, Amazon AWS, bare metal, or your own local machine.

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
  juju help local       use on this computer
  juju help aws         use on AWS
  juju help openstack   use on OpenStack
  juju help hpcloud     use on HP Cloud
`

const helpLocalProvider = `
First install Juju and some dependencies it needs. Keep in mind that LXC and
mongodb are needed for the local provider to work.

    sudo add-apt-repository ppa:juju/stable
    sudo apt-get update
    sudo apt-get install juju-core lxc mongodb-server

After that you might get error for SSH authorized/public key not found. ERROR
SSH authorized/public key not found.

    ssh-keygen -t rsa

First configure your environment local environment, if you've not set up Juju
before do a:

    juju init -w

This will write out an example config file that will work. Then you need to
tell Juju to use the local provider and then bootstrap:

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

 - Source: Question on Ask Ubuntu [1]
 - [Documentation][2]

  [1]: http://askubuntu.com/questions/65359/how-do-i-configure-juju-for-local-usage
  [2]: https://juju.ubuntu.com/docs/getting-started.html
`

const helpOpenstackProvider = `

First off you need juju and charm-tools, ensure you have the latest stable
juju:

    sudo add-apt-repository ppa:juju/stable
    sudo apt-get update && sudo apt-get install juju-core charm-tools

Do a 'juju generate-config -w' to generate a config for OpenStack that you can
customize for your needs.

Here's an example OpenStack configuration for '~/.juju/environments.yaml',
including the commented out sections:

      openstack:
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
        # default-series: precise
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

 - Source: Question on Ask Ubuntu [1]
 - Official Docs [2]

  [1]: http://askubuntu.com/questions/132411/how-can-i-configure-juju-for-deployment-on-openstack
  [2]: http://juju.ubuntu.com/docs/provider-configuration-openstack.html

Other OpenStack Based Clouds:

This answer is for generic upstream OpenStack support, if you're using an
OpenStack-based provider check these questions out for provider-specific
information:

 - http://askubuntu.com/questions/116174/how-can-i-configure-juju-for-deployment-to-the-hp-cloud
 - http://askubuntu.com/questions/166102/how-do-i-configure-juju-for-deployment-on-rackspace-cloud

`

const helpEC2Provider = `
First install Juju:

    sudo add-apt-repository ppa:juju/stable
    sudo apt-get update && sudo apt-get -y install juju-core

Do a 'juju generate-config -w' to generate a config for AWS that you can
customize for your needs. This will create the file
'~/.juju/environments.yaml'.

Which is a sample environment configured to run with EC2 machines and S3
permanent storage.

To make this environment actually useful, you will need to tell juju about an
AWS access key and secret key. To do this, you can either set the
'AWS_ACCESS_KEY_ID' and 'AWS_SECRET_ACCESS_KEY' [environment variables][1] (as
usual for other EC2 tools) or you can add access-key and secret-key options to
your environments.yaml. These are already in place in the generated config,
you just need to uncomment them out. For example:

    default: sample
    environments:
      sample:
        type: ec2
        access-key: YOUR-ACCESS-KEY-GOES-HERE
        secret-key: YOUR-SECRET-KEY-GOES-HERE
        control-bucket: juju-faefb490d69a41f0a3616a4808e0766b
        admin-secret: 81a1e7429e6847c4941fda7591246594
        default-series: precise
        ssl-hostname-verification: true

See the [EC2 provider documentation][2] for more options. The S3 bucket does
not need to exist already.

Note If you already have an AWS account, you can determine your access key by
visiting [your account page][3], clicking "Security Credentials" and then
clicking "Access Credentials". You'll be taken to a table that lists your
access keys and has a "show" link for each access key that will reveal the
associated secret key.

And that's it, you're ready to go!

 - https://juju.ubuntu.com/docs/getting-started.html
 - https://juju.ubuntu.com/docs/provider-configuration-ec2.html

References:

 - Source: Question on Ask Ubuntu [4]

  [1]: http://askubuntu.com/questions/730/how-do-i-set-environment-variables
  [2]: https://juju.ubuntu.com/docs/provider-configuration-ec2.html
  [3]: http://aws.amazon.com/account
  [4]: http://askubuntu.com/questions/225513/how-do-i-configure-juju-to-use-amazon-web-services-aws
`

const helpHPCloud = `

You should start by generating a generic configuration file for Juju, using
the command:

   'juju generate-config -w'

This will generate a file, 'environments.yaml', which will live in your
'~/.juju/' directory (and will create the directory if it doesn't already
exist).

The essential configuration sections for HP Cloud look like this:

      hpcloud:
        type: openstack
        admin-secret: 6638bebf0c54ffff1007e0247d4dae98
        control-bucket: juju-bc66a4a4adbee50b2ceeee70436528e5
        tenant-name: "juju-project1"
        auth-url: https://region-a.geo-1.identity.hpcloudsvc.com:35357/v2.0
        auth-mode: userpass
        username: "xxxyour-hpcloud-usernamexxx"
        password: "xxxpasswordxxx"
        region: az-1.region-a.geo-1
        tools-url: https://region-a.geo-1.objects.hpcloudsvc.com:443/v1/60502529753910/juju-dist/tools

Please refer to the question on Ask Ubuntu [1] for details on how to get
the relevant information to finish configuring your hpcloud environment.

Official docs:

 - [Documentation][2]
 - General OpenStack configuration: [3]

References:

 - Source: Question on Ask Ubuntu [1]

  [1]: http://askubuntu.com/questions/116174/how-can-i-configure-juju-for-deployment-on-hp-cloud
  [2]: https://juju.ubuntu.com/docs/provider-configuration-openstack.html#openstack-configuration
  [3]: http://askubuntu.com/questions/132411/how-can-i-configure-juju-for-deployment-on-openstack
`

const helpGlossary = `
Bootstrap
   To boostrap an environment means initializing it so that Services may be
   deployed on it.

Charm
   A Charm provides the definition of the service, including its metadata,
   dependencies to other services, packages necessary, as well as the logic
   for management of the application. It is the layer that integrates an
   external application component like Postgres or WordPress into juju. A juju
   Service may generally be seen as the composition of its juju Charm and the
   upstream application (traditionally made available through its package).

Charm URL
   A Charm URL is a resource locator for a charm, with the following format
   and restrictions:

       <schema>:[~<user>/]<collection>/<name>[-<revision>]

   schema must be either "cs", for a charm from the Juju charm store, or
   "local", for a charm from a local repository.

   user is only valid in charm store URLs, and allows you to source charms
   from individual users (rather than from the main charm store); it must be a
   valid Launchpad user name.

   collection denotes a charm's purpose and status, and is derived from the
   Ubuntu series targeted by its contained charms: examples include "precise",
   "quantal", "oneiric-universe".

   name is just the name of the charm; it must start and end with lowercase
   (ascii) letters, and can otherwise contain any combination of lowercase
   letters, digits, and "-"s.

   revision, if specified, points to a specific revision of the charm pointed
   to by the rest of the URL. It must be a non-negative integer.

Endpoint
   The combination of a service name and a relation name.

Environment
   An Environment is a configured location where Services can be deployed
   onto. An Environment typically has a name, which can usually be omitted
   when there's a single Environment configured, or when a default is
   explicitly defined. Depending on the type of Environment, it may have to be
   bootstrapped before interactions with it may take place (e.g. EC2). The
   local environment configuration is defined in the ~/.juju/environments.yaml
   file.

Machine Agent
   Software which runs inside each machine that is part of an Environment, and
   is able to handle the needs of deploying and managing Service Units in this
   machine.

Provisioning Agent
   Software responsible for automatically allocating and terminating machines
   in an Environment, as necessary for the requested configuration.

Relation
   Relations are the way in which juju enables Services to communicate to each
   other, and the way in which the topology of Services is assembled. The
   Charm defines which Relations a given Service may establish, and what kind
   of interface these Relations require.

   In many cases, the establishment of a Relation will result into an actual
   TCP connection being created between the Service Units, but that's not
   necessarily the case. Relations may also be established to inform Services
   of configuration parameters, to request monitoring information, or any
   other details which the Charm author has chosen to make available.

Repository
   A location where multiple charms are stored. Repositories may be as simple
   as a directory structure on a local disk, or as complex as a rich smart
   server supporting remote searching and so on.

Service
   juju operates in terms of services. A service is any application (or set of
   applications) that is integrated into the framework as an individual
   component which should generally be joined with other components to perform
   a more complex goal.

   As an example, WordPress could be deployed as a service and, to perform its
   tasks properly, might communicate with a database service and a load
   balancer service.

Service Configuration
   There are many different settings in a juju deployment, but the term
   Service Configuration refers to the settings which a user can define to
   customize the behavior of a Service.

   The behavior of a Service when its Service Configuration changes is
   entirely defined by its Charm.

Service Unit
   A running instance of a given juju Service. Simple Services may be deployed
   with a single Service Unit, but it is possible for an individual Service to
   have multiple Service Units running in independent machines. All Service
   Units for a given Service will share the same Charm, the same relations,
   and the same user-provided configuration.

   For instance, one may deploy a single MongoDB Service, and specify that it
   should run 3 Units, so that the replica set is resilient to
   failures. Internally, even though the replica set shares the same
   user-provided configuration, each Unit may be performing different roles
   within the replica set, as defined by the Charm.

Service Unit Agent
   Software which manages all the lifecycle of a single Service Unit.

`
