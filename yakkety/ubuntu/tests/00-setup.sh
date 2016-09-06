#!/bin/sh

# Install Amulet testing harness as it is not on the 
# default cloud image.  Amulet should pull in its dependencies.
sudo add-apt-repository -y ppa:juju/stable
sudo apt-get update 
sudo apt-get install -y amulet
