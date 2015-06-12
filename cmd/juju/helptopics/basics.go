// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package helptopics

const Basics = `
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

  juju help juju        what is juju?
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
