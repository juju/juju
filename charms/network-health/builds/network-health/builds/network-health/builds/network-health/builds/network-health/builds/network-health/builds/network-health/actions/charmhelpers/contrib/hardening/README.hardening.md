# Juju charm-helpers hardening library

## Description

This library provides multiple implementations of system and application
hardening that conform to the standards of http://hardening.io/.

Current implementations include:

 * OS
 * SSH
 * MySQL
 * Apache

## Requirements

* Juju Charms

## Usage

1. Synchronise this library into your charm and add the harden() decorator
   (from contrib.hardening.harden) to any functions or methods you want to use
   to trigger hardening of your application/system.

2. Add a config option called 'harden' to your charm config.yaml and set it to
   a space-delimited list of hardening modules you want to run e.g. "os ssh"

3. Override any config defaults (contrib.hardening.defaults) by adding a file
   called hardening.yaml to your charm root containing the name(s) of the
   modules whose settings you want override at root level and then any settings
   with overrides e.g.
   
   os:
       general:
            desktop_enable: True

4. Now just run your charm as usual and hardening will be applied each time the
   hook runs.
