#!/usr/bin/env python
from __future__ import print_function

from utility import run_command


ceilometer_cmds = [
    ('/usr/bin/ceilometer', 'resource-list'),
]


cinder_cmds = [
    ('/usr/bin/cinder', 'list'),
    ('/usr/bin/cinder', 'list-extensions'),
    ('/usr/bin/cinder', 'snapshot-list'),
    ('/usr/bin/cinder', 'transfer-list'),
    ('/usr/bin/cinder', 'type-list'),
]


glance_cmds = [
    ('/usr/bin/glance', 'image-list'),
]


heat_cmds = [
    ('/usr/bin/heat', 'stack-list'),
]


keystone_cmds = [
    ('/usr/bin/keystone', 'ec2-credentials-list'),
    ('/usr/bin/keystone', 'endpoint-list'),
    ('/usr/bin/keystone', 'role-list'),
    ('/usr/bin/keystone', 'service-list'),
    ('/usr/bin/keystone', 'tenant-list'),
    ('/usr/bin/keystone', 'token-get'),
    ('/usr/bin/keystone', 'user-list'),
    ('/usr/bin/keystone', 'user-role-list'),
]


neutron_cmds = [
    ('/usr/bin/neutron', 'net-list'),
    ('/usr/bin/neutron', 'router-list'),
    ('/usr/bin/neutron', 'subnet-list'),
]


nova_cmds = [
    ('/usr/bin/nova', 'flavor-list'),
    ('/usr/bin/nova', 'floating-ip-list'),
    ('/usr/bin/nova', 'floating-ip-pool-list'),
    ('/usr/bin/nova', 'keypair-list'),
    ('/usr/bin/nova', 'host-list'),
    ('/usr/bin/nova', 'hypervisor-list'),
    ('/usr/bin/nova', 'image-list'),
    ('/usr/bin/nova', 'network-list'),
    ('/usr/bin/nova', 'secgroup-list'),
    ('/usr/bin/nova', 'service-list'),
]


swift_cmds = [
    ('/usr/bin/swift', 'list'),
]


def run_cmds(commands):
    for cmd in commands:
        run_command(cmd, verbose=True)


if __name__ == '__main__':
    """Perform basic Openstack command line checks."""
    run_cmds(ceilometer_cmds)
    run_cmds(cinder_cmds)
    run_cmds(glance_cmds)
    run_cmds(heat_cmds)
    run_cmds(keystone_cmds)
    run_cmds(neutron_cmds)
    run_cmds(nova_cmds)
    run_cmds(swift_cmds)
