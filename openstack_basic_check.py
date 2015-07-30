#!/usr/bin/env python
from argparse import ArgumentParser
import os

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


def sniff_openstack():
    run_cmds(ceilometer_cmds)
    run_cmds(cinder_cmds)
    run_cmds(glance_cmds)
    run_cmds(heat_cmds)
    run_cmds(keystone_cmds)
    run_cmds(neutron_cmds)
    run_cmds(nova_cmds)
    run_cmds(swift_cmds)


def set_environ(args):
    os.environ['OS_USERNAME'] = args.user
    os.environ['OS_PASSWORD'] = args.password
    os.environ['OS_TENANT_NAME'] = args.tenant
    os.environ['OS_REGION_NAME'] = args.region
    os.environ['OS_AUTH_URL'] = args.auth_url


def get_args(argv=None):
    parser = ArgumentParser()
    parser.add_argument('--user', default=os.environ.get('OS_USERNAME'),
                        help='OpenStack user name.')
    parser.add_argument('--password', default=os.environ.get('OS_PASSWORD'),
                        help='OpenStack admin password.')
    parser.add_argument('--tenant', default=os.environ.get('OS_TENANT_NAME'),
                        help='OpenStack tenant.')
    parser.add_argument('--region', default=os.environ.get('OS_REGION_NAME'),
                        help='OpenStack region.')
    parser.add_argument('--auth-url', default=os.environ.get('OS_AUTH_URL'),
                        help='URL to Keystone.')
    args = parser.parse_args(argv)
    if not args.user:
        raise Exception(
            "User must be provided; pass --user or set OS_USERNAME.")
    if not args.password:
        raise Exception(
            "Password must be provided; pass --password or set OS_PASSWORD.")
    if not args.tenant:
        raise Exception(
            "Tenant must be provided; pass --tenant or set OS_TENANT_NAME.")
    if not args.region:
        raise Exception(
            "Region must be provided; pass --region or set OS_REGION_NAME.")
    if not args.auth_url:
        raise Exception(
            "auth-url must be provided; pass --auth-url or set OS_AUTH_URL.")
    return args


def main():
    args = get_args()
    set_environ(args)
    sniff_openstack()


if __name__ == '__main__':
    """Perform basic Openstack command line checks."""
    main()
