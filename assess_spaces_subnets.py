#!/usr/bin/env python
from __future__ import print_function
import logging
import re
import yaml
from textwrap import dedent
from argparse import ArgumentParser

from jujuconfig import (
    get_juju_home,
)
from jujupy import (
    parse_new_state_server_from_error,
    temp_bootstrap_env,
    SimpleEnvironment,
    EnvJujuClient,
)
from utility import (
    print_now,
    add_basic_testing_arguments,
)
from deploy_stack import (
    update_env,
    dump_env_logs
)
from assess_container_networking import (
    clean_environment,
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
    parser = add_basic_testing_arguments(ArgumentParser(
        description=description
    ))
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
    client.juju('space create', 'public')
    client.juju('space create', 'dmz')
    client.juju('subnet add', ('subnet-d27d91a9', 'public'))
    client.juju('subnet add', ('subnet-0fb97566', 'dmz'))
    client.juju('deploy', ('ubuntu', '--constraints', 'spaces=dmz'))
    client.juju('deploy', ('ubuntu', 'ubuntu-public', '--constraints',
                           'spaces=public'))
    status = client.wait_for_started()
    spaces = yaml.load(client.get_juju_output('space list'))

    unit_priv_address = {}
    for service in sorted(status.status['services'].values()):
        for unit_name, unit in service.get('units', {}).items():

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

    expect = {
        'ubuntu/0': 'dmz',
        'ubuntu-public/0': 'public',
    }

    units_checked = 0
    for space, cidrs in cidrs_in_space.iteritems():
        for cidr in cidrs:
            for unit, address in unit_priv_address.iteritems():
                if ipv4_in_cidr(address, cidr):
                    units_checked += 1
                    if expect[unit] != space:
                        raise ValueError("Found {} in {}, expected {}".format(
                            unit, space, expect[unit]))

    return units_checked


def ipv4_to_int(ipv4):
    b = [int(b) for b in ipv4.split('.')]
    return b[0] << 24 | b[1] << 16 | b[2] << 8 | b[3]


def ipv4_in_cidr(ipv4, cidr):
    if '/' in ipv4:
        ipv4, _ = ipv4.split('/')
    ipv4 = ipv4_to_int(ipv4)
    value, bits = cidr.split('/')
    subnet = ipv4_to_int(value)
    mask = 0xFFFFFFFF & (0xFFFFFFFF << (32-int(bits)))
    return (ipv4 & mask) == subnet


def get_client(args):
    client = EnvJujuClient.by_version(
        SimpleEnvironment.from_config(args.env),
        args.juju_bin, args.debug
    )
    client.enable_container_address_allocation()
    update_env(client.env, args.temp_env_name)
    return client


def main():
    args = parse_args()
    client = get_client(args)
    juju_home = get_juju_home()
    bootstrap_host = None
    try:
        if args.clean_environment:
            try:
                if not clean_environment(client):
                    with temp_bootstrap_env(juju_home, client):
                        client.bootstrap(args.upload_tools)
            except Exception as e:
                logging.exception(e)
                client.destroy_environment()
                client = get_client(args)
                with temp_bootstrap_env(juju_home, client):
                    client.bootstrap(args.upload_tools)
        else:
            client.destroy_environment()
            client = get_client(args)
            with temp_bootstrap_env(juju_home, client):
                client.bootstrap(args.upload_tools)

        logging.info('Waiting for the bootstrap machine agent to start.')
        status = client.wait_for_started()
        mid, data = list(status.iter_machines())[0]
        bootstrap_host = data['dns-name']

        assess_spaces_subnets(client)

    except Exception as e:
        logging.exception(e)
        try:
            if bootstrap_host is None:
                bootstrap_host = parse_new_state_server_from_error(e)
        except Exception as e:
            print_now('exception while dumping logs:\n')
            logging.exception(e)
        exit(1)
    finally:
        if bootstrap_host is not None:
            dump_env_logs(client, bootstrap_host, args.logs)
        if args.clean_environment:
            clean_environment(client)
        else:
            client.destroy_environment()


if __name__ == '__main__':
    main()
