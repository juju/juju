// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package main

const helpBasics = `
Juju -- devops distilled
https://juju.ubuntu.com/

Juju provides easy, intelligent service orchestration on top of environments
such as OpenStack, Amazon AWS, or bare metal.

Basic commands:
  juju init             generate boilerplate configuration for juju environments
  juju bootstrap        start up an environment from scratch

  juju deploy           deploy a new service
  juju add-relation     add a relation between two services
  juju expose           expose a service

  juju help bootstrap   more help on e.g. bootstrap command
  juju help commands    list all commands
  juju help topics      list all help topics
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
