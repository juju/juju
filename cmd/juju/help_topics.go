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
