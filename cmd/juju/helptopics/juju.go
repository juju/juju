// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package helptopics

const Juju = `

What is Juju?

Juju is a state-of-the-art, open source, universal model for service oriented
architecture and service oriented deployments. Juju allows you to deploy,
configure, manage, maintain, and scale cloud services quickly and efficiently
on public clouds, as well as on physical servers, OpenStack, and containers.
You can use Juju from the command line or through its beautiful GUI.

What is service modelling?

In modern environments, services are rarely deployed in isolation. Even simple
applications may require several actual services in order to function - like a
database and a web server for example. For deploying a more complex system,
e.g. OpenStack, many more services need to be installed, configured and
connected to each other. Juju's service modelling provides tools to express
the intent of how to deploy such services and to subsequently scale and manage
them.

At the lowest level, traditional configuration management tools like Chef and
Puppet, or even general scripting languages such as Python or bash, automate
the configuration of machines to a particular specification. With Juju, you
create a model of the relationships between services that make up your
solution and you have a mapping of the parts of that model to machines. Juju
then applies the necessary configuration management scripts to each machine in
the model.

Application-specific knowledge such as dependencies, scale-out practices,
operational events like backups and updates, and integration options with
other pieces of software are encapsulated in Juju's 'charms'. This knowledge
can then be shared between team members, reused everywhere from laptops to
virtual machines and cloud, and shared with other organisations.

The charm defines everything you all collaboratively know about deploying that
particular service brilliantly. All you have to do is use any available charm
(or write your own), and the corresponding service will be deployed in
seconds, on any cloud or server or virtual machine.

See Also:
    juju help juju-systems
    juju help bootstrap
    juju help topics
    https://jujucharms.com/docs
`
