#!/usr/bin/env python
from __future__ import print_function

from utility import run_command


ceilometer_cmds = [
    ('ceilometer', 'resource-list'),
]


cinder_cmds = [
    ('cinder', 'list'),
    ('cinder', 'list-extensions'),
    ('cinder', 'snapshot-list'),
    ('cinder', 'transfer-list'),
    ('cinder', 'type-list'),
]


glance_cmds = [
    ('glance', 'image-list'),
]


heat_cmds = [
    ('heat', 'stack-list'),
]


keystone_cmds = [
    ('keystone', 'ec2-credentials-list'),
    ('keystone', 'endpoint-list'),
    ('keystone', 'role-list'),
    ('keystone', 'service-list'),
    ('keystone', 'tenant-list'),
    ('keystone', 'token-get'),
    ('keystone', 'user-list'),
    ('keystone', 'user-role-list'),
]


neutron_cmds = [
    ('neutron', 'net-list'),
    ('neutron', 'router-list'),
    ('neutron', 'subnet-list'),
]


nova_cmds = [
    ('nova', 'flavor-list'),
    ('nova', 'floating-ip-list'),
    ('nova', 'floating-ip-pool-list'),
    ('nova', 'keypair-list'),
    ('nova', 'host-list'),
    ('nova', 'hypervisor-list'),
    ('nova', 'image-list'),
    ('nova', 'network-list'),
    ('nova', 'secgroup-list'),
    ('nova', 'service-list'),
]


swift_cmds = [
    ('swift', 'list'),
]


def run_cmds(commands):
    for cmd in commands:
        run_command(cmd, verbose=True)


def main():
    run_cmds(ceilometer_cmds)
    run_cmds(cinder_cmds)
    run_cmds(glance_cmds)
    run_cmds(heat_cmds)
    run_cmds(keystone_cmds)
    run_cmds(neutron_cmds)
    run_cmds(nova_cmds)
    run_cmds(swift_cmds)


if __name__ == '__main__':
    """Perform basic Openstack command line checks."""
    main()
