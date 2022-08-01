# Overview

Defines a subordinate for the LXD profile.

# Usaage

juju deploy lxd-profile
juju deploy lxd-profile-subordinate
juju add-relation lxd-profile-subordinate lxd-profile

## Known Limitations and Issues

It doesn't do much, but it does get you a machine you can play with.  
If not deployed to an LXD container or cloud, that functionality is a no-op

# Configuration

None
