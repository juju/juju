#!/usr/bin/env python3
from __future__ import print_function
from argparse import ArgumentParser
import re
import sys
from textwrap import dedent

from utility import (
    add_basic_testing_arguments,
    configure_logging,
)
from deploy_stack import (
    BootstrapManager,
)
from assess_container_networking import (
    cleaned_bootstrap_context,
    ssh,
)

__metaclass__ = type


def parse_args(argv=None):
    """Parse all arguments."""

    description = dedent("""\
    Test container address allocation.
    For LXC and KVM, create machines of each type and test the network
    between LXC <--> LXC, KVM <--> KVM and LXC <--> KVM. Also test machine
    to outside world, DNS and that these tests still pass after a reboot. In
    case of failure pull logs and configuration files from the machine that
    we detected a problem on for later analysis.
    """)
    parser = add_basic_testing_arguments(
        ArgumentParser(description=description),
        existing=False)
    parser.add_argument(
        '--clean-environment', action='store_true', help=dedent("""\
        Attempts to re-use an existing environment rather than destroying it
        and creating a new one.

        On launch, if an environment exists, clean out services and machines
        from it rather than destroying it. If an environment doesn't exist,
        create one and use it.

        At termination, clean out services and machines from the environment
        rather than destroying it."""))
    return parser.parse_args(argv)


def assess_spaces_subnets(client):
    """Check that space and subnet functionality works as expected
    :param client: ModelClient
    """
    network_config = {
        'default': ['subnet-0fb97566', 'subnet-d27d91a9'],
        'dmz': ['subnet-604dcd09', 'subnet-882d8cf3'],
        'apps': ['subnet-c13fbfa8', 'subnet-53da7a28'],
        'backend': ['subnet-5e4dcd37', 'subnet-7c2c8d07'],
    }

    charms_to_space = {
        'haproxy': {'space': 'dmz'},
        'mediawiki': {'space': 'apps'},
        'memcached': {'space': 'apps'},
        'mysql': {'space': 'backend'},
        'mysql-slave': {
            'space': 'backend',
            'charm': 'mysql',
        },
    }

    _assess_spaces_subnets(client, network_config, charms_to_space)


def _assess_spaces_subnets(client, network_config, charms_to_space):
    """Check that space and subnet functionality works as expected
    :param client: ModelClient
    :param network_config: Map of 'space name' to ['subnet', 'list']
    :param charms_to_space: Map of 'unit name' to
           {'space': 'space name', 'charm': 'charm name (if not same as unit)}
    :return: None. Raises exception on failure.
    """
    for space in sorted(network_config.keys()):
        client.add_space(space)
        for subnet in network_config[space]:
            client.add_subnet(subnet, space)

    for name in sorted(charms_to_space.keys()):
        if 'charm' not in charms_to_space[name]:
            charms_to_space[name]['charm'] = name
        charm = charms_to_space[name]['charm']
        space = charms_to_space[name]['space']
        client.juju('deploy',
                    (charm, name, '--constraints', 'spaces=' + space))

    # Scale up. We don't specify constraints, but they should still be honored
    # per charm.
    client.juju('add-unit', 'mysql-slave')
    client.juju('add-unit', 'mediawiki')
    status = client.wait_for_started()

    spaces = client.list_space()

    unit_priv_address = {}
    units_found = 0
    for service in sorted(status.status['services'].values()):
        for unit_name, unit in service.get('units', {}).items():
            units_found += 1
            addrs = ssh(client, unit['machine'], 'ip -o addr')
            for addr in re.findall(r'^\d+:\s+(\w+)\s+inet\s+(\S+)',
                                   addrs, re.MULTILINE):
                if addr[0] != 'lo':
                    unit_priv_address[unit_name] = addr[1]

    cidrs_in_space = {}
    for name, attrs in spaces['spaces'].iteritems():
        cidrs_in_space[name] = []
        for cidr in attrs:
            cidrs_in_space[name].append(cidr)

    units_checked = 0
    for space, cidrs in cidrs_in_space.iteritems():
        for cidr in cidrs:
            for unit, address in unit_priv_address.iteritems():
                if ipv4_in_cidr(address, cidr):
                    units_checked += 1
                    charm = unit.split('/')[0]
                    if charms_to_space[charm]['space'] != space:
                        raise ValueError("Found {} in {}, expected {}".format(
                            unit, space, charms_to_space[charm]['space']))

    if units_found != units_checked:
        raise ValueError("Could not find spaces for all units")

    return units_checked


def ipv4_to_int(ipv4):
    """Convert an IPv4 dotted decimal address to an integer"""
    b = [int(b) for b in ipv4.split('.')]
    return b[0] << 24 | b[1] << 16 | b[2] << 8 | b[3]


def ipv4_in_cidr(ipv4, cidr):
    """Returns True if the given address is in the given CIDR"""
    if '/' in ipv4:
        ipv4, _ = ipv4.split('/')
    ipv4 = ipv4_to_int(ipv4)
    value, bits = cidr.split('/')
    subnet = ipv4_to_int(value)
    mask = 0xFFFFFFFF & (0xFFFFFFFF << (32 - int(bits)))
    return (ipv4 & mask) == subnet


def main(argv=None):
    args = parse_args(argv)
    configure_logging(args.verbose)
    bs_manager = BootstrapManager.from_args(args)
    bs_manager.client.enable_feature('address-allocation')
    with cleaned_bootstrap_context(bs_manager, args) as ctx:
        assess_spaces_subnets(bs_manager.client)
    return ctx.return_code


if __name__ == '__main__':
    sys.exit(main())
