#!/bin/sh
set -ex

# Do the manual steps a user has to run on a fresh system to get an lxd
# bridge so the juju lxd provider can function. Taken from changes made
# to cloud-init to do approximately this.

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