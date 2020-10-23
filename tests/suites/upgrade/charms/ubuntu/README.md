# Overview

This charm provides a blank [Ubuntu](http://ubuntu.com) image. It does not provide any services other than a blank cloud image for you to manage manually, it is intended for testing and development. 

# Usage

Step by step instructions on using this charm:

    juju deploy ubuntu

You can then ssh to the instance with:

    juju ssh ubuntu/0

## Scale out Usage

This charm is not designed to be used at scale since it does not have any relationships, however you can bulk add machines with `add-unit`:

    juju add-unit ubuntu      # Add one more
    juju add-unit -n5 ubuntu  # Add 5 at a time


You can also alias names in order to organize a bunch of empty instances:

    juju deploy ubuntu mytestmachine1
    juju deploy ubuntu mytestmachine2

and so on. 

## Known Limitations and Issues

This charm does not provide anything other than a blank server, so it does not relate to other charms.

# Configuration

This charm has no configuration options.

# Contact Information


## Upstream Project Name

- [Ubuntu](http://ubuntu.com)
- [Bug tracker](http://bugs.launchpad.net/ubuntu)
- [Ubuntu Server Mailing list](https://lists.ubuntu.com/archives/ubuntu-server/)

## Charm Contact Information

- Author: Juju Charm Community
- Report bugs at: [http://bugs.launchpad.net/charms/+source/ubuntu](http://bugs.launchpad.net/charms/+source/ubuntu)
- Location: [http://jujucharms.com/charms/precise/ubuntu](http://jujucharms.com/charms/precise/ubuntu)

