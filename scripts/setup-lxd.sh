#!/bin/bash
# Copyright 2016 Canonical Ltd.
# Licensed under the AGPLv3, see LICENCE file for details.
set -eux

# Do the manual steps a user has to run on a fresh system to get an lxd
# bridge so the juju lxd provider can function. Taken from changes made
# to cloud-init to do approximately this.

VERSION=$(lxd --version)

# LXD 2.3+ needs lxdbr0 setup via lxc.
if [[ "$VERSION" > "2.2" ]]; then
    if [[ ! $(lxc network list | grep lxdbr0) ]]; then
        # Configure a known address ranges for lxdbr0.
        lxc network create lxdbr0 \
            ipv4.address=10.0.8.1/24 ipv4.nat=true \
            ipv6.address=none ipv6.nat=false
    fi
    lxc network show lxdbr0
    exit 0
fi

# LXD 2.2 and earlier use debconf to create and configure the network.
debconf-communicate << EOF
set lxd/setup-bridge true
set lxd/bridge-domain lxd
set lxd/bridge-name lxdbr0
set lxd/bridge-ipv4 true
set lxd/bridge-ipv4-address 10.0.8.1
set lxd/bridge-ipv4-dhcp-first 10.0.8.2
set lxd/bridge-ipv4-dhcp-last 10.0.8.254
set lxd/bridge-ipv4-dhcp-leases 252
set lxd/bridge-ipv4-netmask 24
set lxd/bridge-ipv4-nat true
set lxd/bridge-ipv6 false
EOF

rm -rf /etc/default/lxd-bridge

dpkg-reconfigure lxd --frontend=noninteractive

# Must run a command for systemd socket activation to start the service
lxc finger
