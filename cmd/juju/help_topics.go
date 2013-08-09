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
