#!/usr/bin/env python
from __future__ import print_function
from argparse import ArgumentParser
import contextlib
from copy import (
    copy,
    deepcopy,
)
import logging
import re
import os
import subprocess
import sys
import tempfile
from textwrap import dedent
import time

from deploy_stack import (
    BootstrapManager,
    get_random_string,
    update_env,
)
from jujupy import (
    KVM_MACHINE,
    LXC_MACHINE,
    LXD_MACHINE,
    )
from utility import (
    add_basic_testing_arguments,
    configure_logging,
    wait_for_port,
)



__metaclass__ = type


log = logging.getLogger("assess_container_networking")


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
        '--machine-type',
        help='Which virtual machine/container type to test. Defaults to all.',
        choices=[KVM_MACHINE, LXC_MACHINE, LXD_MACHINE])
    parser.add_argument(
        '--clean-environment', action='store_true', help=dedent("""\
        Attempts to re-use an existing environment rather than destroying it
        and creating a new one.

        On launch, if an environment exists, clean out services and machines
        from it rather than destroying it. If an environment doesn't exist,
        create one and use it.

        At termination, clean out services and machines from the environment
        rather than destroying it."""))
    args = parser.parse_args(argv)
    # Passing --clean-environment implies --keep-env
    if args.clean_environment:
        args.keep_env = True
    return args


def ssh(client, machine, cmd):
    """Convenience function: run a juju ssh command and get back the output
    :param client: A Juju client
    :param machine: ID of the machine on which to run a command
    :param cmd: the command to run
    :return: text output of the command
    """
    back_off = 2
    attempts = 4
    for attempt in range(attempts):
        try:
            return client.get_juju_output('ssh', machine, cmd)
        except subprocess.CalledProcessError as e:
            # If the connection to the host failed, try again in a couple of
            # seconds. This is usually due to heavy load.
            if(attempt < attempts-1 and
               re.search('ssh_exchange_identification: '
                         'Connection closed by remote host', e.stderr)):
                time.sleep(back_off)
                back_off *= 2
            else:
                raise


def clean_environment(client, services_only=False):
    """Remove all the services and, optionally, machines from an environment.

    Use as an alternative to destroying an environment and creating a new one
    to save a bit of time.

    :param client: a Juju client
    """
    # A short timeout is used for get_status here because if we don't get a
    # response from  get_status quickly then the environment almost
    # certainly doesn't exist or needs recreating.
    try:
        status = client.get_status(5)
    except Exception as e:
        # TODO(gz): get_status should return a more specific error type.
        log.info("Could not clean existing env: %s", e)
        return False

    for service in status.status.get('services', {}):
        client.juju('remove-service', service)

    if not services_only:
        # First remove all containers; we can't remove a machine that is
        # hosting containers.
        for m, _ in status.iter_machines(containers=True, machines=False):
            client.juju('remove-machine', m)

        client.wait_for('containers', 'none')

        for m, _ in status.iter_machines(containers=False, machines=True):
            if m != '0':
                try:
                    client.juju('remove-machine', m)
                except subprocess.CalledProcessError:
                    # Sometimes this fails because while we have asked Juju
                    # to remove a container and it says that it has, when we
                    # ask it to remove the host Juju thinks it still has
                    # containers on it. Normally a small pause and trying
                    # again is all that is needed to resolve this issue.
                    time.sleep(2)
                    client.wait_for_started()
                    client.juju('remove-machine', m)

        client.wait_for('machines-not-0', 'none')

    client.wait_for_started()
    return True


def make_machines(client, container_types):
    """Make a test environment consisting of:
       Two host machines.
       Two of each container_type on one host machine.
       One of each container_type on one host machine.
    :param client: An EnvJujuClient
    :param container_types: list of containers to create
    :return: hosts (list), containers {container_type}{host}[containers]
    """
    # Find existing host machines
    old_hosts = client.get_status().status['machines']
    machines_to_add = 2 - len(old_hosts)

    # Allocate more hosts as needed
    if machines_to_add > 0:
        client.juju('add-machine', ('-n', str(machines_to_add)))
    status = client.wait_for_started()
    hosts = sorted(status.status['machines'].keys())[:2]

    # Find existing containers
    required = dict(zip(hosts, [copy(container_types) for h in hosts]))
    required[hosts[0]] += container_types
    for c in status.iter_machines(containers=True, machines=False):
        host, type, id = c[0].split('/')
        if type in required[host]:
            required[host].remove(type)

    # Start any new containers we need
    for host, containers in required.iteritems():
        for container in containers:
            client.juju('add-machine', ('{}:{}'.format(container, host)))

    status = client.wait_for_started()

    # Build a list of containers, now they have all started
    tmp = dict(zip(hosts, [[] for h in hosts]))
    containers = dict(zip(container_types,
                          [deepcopy(tmp) for t in container_types]))
    for c in status.iter_machines(containers=True, machines=False):
        host, type, id = c[0].split('/')
        if type in containers and host in containers[type]:
            containers[type][host].append(c[0])

    return hosts, containers


def find_network(client, machine, addr):
    """Find a connected subnet containing the given address.

    When using this to find the subnet of a container, don't use the container
    as the machine to run the ip route show command on ("machine"), use a real
    box because lxc will just send everything to its host machine, so it is on
    a subnet containing itself. Not much use.
    :param client: A Juju client
    :param machine: ID of the machine on which to run a command
    :param addr: find the connected subnet containing this address
    :return: CIDR containing the address if found, else, None
    """
    ip_cmd = ' '.join(['ip', 'route', 'show', 'to', 'match', addr])
    routes = ssh(client, machine, ip_cmd)

    for route in re.findall(r'^(\S+).*[\d\.]+/\d+', routes, re.MULTILINE):
        if route != 'default':
            return route

    raise ValueError("Unable to find route to %r" % addr)


def assess_network_traffic(client, targets):
    """Test that all containers in target can talk to target[0]
    :param client: Juju client
    :param targets: machine IDs of machines to test
    :return: None;
    """
    log.info('Waiting for the bootstrap machine agent to start.')
    status = client.wait_for_started().status
    source = targets[0]
    dests = targets[1:]

    with tempfile.NamedTemporaryFile(delete=False) as f:
        f.write('tmux new-session -d -s test "nc -l 6778 > nc_listen.out"')
    client.juju('scp', (f.name, source + ':/home/ubuntu/listen.sh'))
    os.remove(f.name)

    # Containers are named 'x/type/y' where x is the host of the container. We
    host = source.split('/')[0]
    address = status['machines'][host]['containers'][source]['dns-name']

    for dest in dests:
        msg = get_random_string()
        ssh(client, source, 'rm nc_listen.out; bash ./listen.sh')
        ssh(client, dest,
            'echo "{msg}" | nc {addr} 6778'.format(msg=msg, addr=address))
        result = ssh(client, source, 'more nc_listen.out')
        if result.rstrip() != msg:
            raise ValueError("Wrong or missing message: %r" % result.rstrip())


def private_address(client, host):
    default_route = ssh(client, host, 'ip -4 -o route list 0/0')
    device = re.search(r'(\w+)\s*$', default_route).group(1)
    device_ip = ssh(client, host, 'ip -4 -o addr show {}'.format(device))
    return re.search(r'inet\s+(\S+)/\d+\s', device_ip).group(1)


def assess_address_range(client, targets):
    """Test that two containers are in the same subnet as their host
    :param client: Juju client
    :param targets: machine IDs of machines to test
    :return: None; raises ValueError on failure
    """
    status = client.wait_for_started().status

    host_subnet_cache = {}

    for target in targets:
        host = target.split('/')[0]

        if host in host_subnet_cache:
            host_subnet = host_subnet_cache[host]
        else:
            host_address = private_address(client, host)
            host_subnet = find_network(client, host, host_address)
            host_subnet_cache[host] = host_subnet

        addr = status['machines'][host]['containers'][target]['dns-name']
        subnet = find_network(client, host, addr)
        if host_subnet != subnet:
            raise ValueError(
                '{} ({}) not on the same subnet as {} ({})'.format(
                    target, subnet, host, host_subnet))


def assess_internet_connection(client, targets):
    """Test that targets can ping their default route
    :param client: Juju client
    :param targets: machine IDs of machines to test
    :return: None; raises ValueError on failure
    """

    for target in targets:
        routes = ssh(client, target, 'ip route show')

        d = re.search(r'^default\s+via\s+([\d\.]+)\s+', routes, re.MULTILINE)
        if d:
            rc = client.juju('ssh', (target, 'ping -c1 -q ' + d.group(1)),
                             check=False)
            if rc != 0:
                raise ValueError('%s unable to ping default route' % target)
        else:
            raise ValueError("Default route not found")


def _assessment_iteration(client, containers):
    """Run the network tests on this collection of machines and containers
    :param client: Juju client
    :param hosts: list of hosts of containers
    :param containers: list of containers to run tests between
    :return: None
    """
    assess_internet_connection(client, containers)
    assess_address_range(client, containers)
    assess_network_traffic(client, containers)


def _assess_container_networking(client, types, hosts, containers):
    """Run _assessment_iteration on all useful combinations of containers
    :param client: Juju client
    :param args: Parsed command line arguments
    :return: None
    """
    for container_type in types:
        # Test with two containers on the same host
        _assessment_iteration(client, containers[container_type][hosts[0]])

        # Now test with two containers on two different hosts
        test_containers = [
            containers[container_type][hosts[0]][0],
            containers[container_type][hosts[1]][0],
            ]
        _assessment_iteration(client, test_containers)

    if KVM_MACHINE in types and LXC_MACHINE in types:
        test_containers = [
            containers[LXC_MACHINE][hosts[0]][0],
            containers[KVM_MACHINE][hosts[0]][0],
            ]
        _assessment_iteration(client, test_containers)

        # Test with an LXC and a KVM on different machines
        test_containers = [
            containers[LXC_MACHINE][hosts[0]][0],
            containers[KVM_MACHINE][hosts[1]][0],
            ]
        _assessment_iteration(client, test_containers)


def assess_container_networking(client, types):
    """Runs _assess_address_allocation, reboots hosts, repeat.
    :param client: Juju client
    :param types: Container types to test
    :return: None
    """
    hosts, containers = make_machines(client, types)
    status = client.wait_for_started().status

    _assess_container_networking(client, types, hosts, containers)

    # Reboot all hosts apart from machine 0 because we use machine 0 to jump
    # through for some hosts.
    for host in hosts[1:]:
        ssh(client, host, 'sudo reboot')

    # Finally reboot machine 0
    ssh(client, hosts[0], 'sudo reboot')

    # Wait for the state server to shut down. This prevents us from calling
    # wait_for_started before machine 0 has shut down, which can cause us
    # to think that we have finished rebooting before we actually have.
    hostname = status['machines'][hosts[0]]['dns-name']
    wait_for_port(hostname, 22, closed=True)

    client.wait_for_started()

    # Once Juju is up it can take a little while before ssh responds.
    for host in hosts:
        hostname = status['machines'][host]['dns-name']
        wait_for_port(hostname, 22, timeout=240)

    _assess_container_networking(client, types, hosts, containers)


class _CleanedContext:

    def __init__(self, client):
        self.client = client
        self.return_code = None


@contextlib.contextmanager
def cleaned_bootstrap_context(bs_manager, args):
    ctx = _CleanedContext(bs_manager.client)
    client = ctx.client
    # TODO(gz): Having to manipulate client env state here to get the temp env
    #           is ugly, would ideally be captured in an explicit scope.
    update_env(client.env, bs_manager.temp_env_name, series=bs_manager.series,
               agent_url=bs_manager.agent_url,
               agent_stream=bs_manager.agent_stream, region=bs_manager.region)
    with bs_manager.top_context() as machines:
        bootstrap_required = True
        if args.clean_environment and clean_environment(client):
            bootstrap_required = False
        if bootstrap_required:
            with bs_manager.bootstrap_context(machines):
                client.bootstrap(args.upload_tools)
        with bs_manager.runtime_context(machines):
            yield ctx
        ctx.return_code = 0
        if args.clean_environment and not clean_environment(client):
            ctx.return_code = 1


def _get_container_types(client, machine_type):
    """
    Give list of container types to run testing against.

    If a machine_type was explictly specified, only test against those kind
    of containers. Otherwise, test all possible containers for the given
    juju version.
    """
    if machine_type:
        if machine_type not in client.supported_container_types:
            raise Exception(
                "no {} support on juju {}".format(machine_type,
                                                  client.version))
        return [machine_type]
    # TODO(gz): Only include LXC for 1.X clients
    types = list(client.supported_container_types)
    types.sort()
    return types


def main(argv=None):
    args = parse_args(argv)
    configure_logging(args.verbose)
    bs_manager = BootstrapManager.from_args(args)
    client = bs_manager.client
    client.enable_feature('address-allocation')
    machine_types = _get_container_types(client, args.machine_type)
    import pdb; pdb.set_trace()
    with cleaned_bootstrap_context(bs_manager, args) as ctx:
        assess_container_networking(bs_manager.client, machine_types)
    return ctx.return_code


if __name__ == '__main__':
    sys.exit(main())
