#!/usr/bin/env python
from __future__ import print_function

__metaclass__ = type

import logging
import re
import tempfile
import os
import socket
from textwrap import dedent

from jujuconfig import (
    get_juju_home,
)
from jujupy import (
    make_client,
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
    dump_env_logs,
)

from argparse import ArgumentParser


KVM_MACHINE = 'kvm'
LXC_MACHINE = 'lxc'


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
        choices=[KVM_MACHINE, LXC_MACHINE])
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


def ssh(client, machine, cmd):
    """Convenience function: run a juju ssh command and get back the output
    :param client: A Juju client
    :param machine: ID of the machine on which to run a command
    :param cmd: the command to run
    :return: text output of the command
    """
    return client.get_juju_output('ssh', machine, cmd)


def clean_environment(client, services_only=False):
    """Remove all the services and, optionally, machines from an environment.

    Use as an alternative to destroying an environment and creating a new one
    to save a bit of time.

    :param client: a Juju client
    """
    status = client.get_status()
    for service in status.status['services']:
        client.juju('remove-service', service)

    if not services_only:
        # First remove all containers; we can't remove a machine that is
        # hosting containers.
        for m, _ in status.iter_machines(containers=True, machines=False):
            client.juju('remove-machine', m)

        client.wait_for('containers', 'none')

        for m, _ in status.iter_machines(containers=False, machines=True):
            if m != '0':
                client.juju('remove-machine', m)

        client.wait_for('machines-not-0', 'none')

    client.wait_for_started()


class MachineGetter:
    def __init__(self, client):
        """Get or allocate machines in this Juju environment
        :param client: Juju client
        """
        self.client = client
        self.count = None
        self.requested_machines = []
        self.allocated_machines = []

    def get(self, container_type=None, machine=None, container=None, count=1):
        """Get one or more machines or containers that match the given spec.
        :param container_type: machine type ('machine', KVM_MACHINE,
                LXC_MACHINE)
        :param machine: obtain a specific machine if available
        :param container: obtain a specific container on the given machine if
                available
        :param count: number of machines to allocate/create
        :return: list of machine IDs
        """

        # Allow user to send integers for ease of calling
        if machine is not None:
            machine = str(machine)
        if container is not None:
            container = str(container)

        self.count = count
        self.requested_machines = []
        self.allocated_machines = []

        status = self.client.get_status().status

        for s in status['services'].values():
            for u in s['units'].values():
                self.allocated_machines.append(u['machine'])

        if container_type in [KVM_MACHINE, LXC_MACHINE]:
            self._get_container(status, machine, container, container_type)
        elif container_type is None:
            self._get_machine(status, machine)
        else:
            raise ValueError("Unrecognised container type %r" % container_type)

        return self.requested_machines

    def _add_new_machines(self, machine_spec):
        req_machines = [machine_spec for _ in range(self._new_machine_count())]
        self._add_machines_worker(req_machines)
        self.client.wait_for_started(60 * 10)

    def _add_machines_worker(self, machines):
        if len(machines) > 0:
            with self.client.juju_async('add-machine', machines[0]):
                if len(machines) > 1:
                    self._add_machines_worker(machines[1:])

    def _new_machine_count(self):
        return self.count - len(self.requested_machines)

    def _get_machine(self, status, machine):
        # If we have the requested machine, return it if it exists
        if machine:
            if machine in status['machines']:
                self.requested_machines.append(machine)
            return

        self._allocate_free_machines()
        self._add_new_machines('')
        self._allocate_free_machines()

    def _get_container(self, status, machine, container, container_type):
        if machine is not None and machine in status['machines']:
            hosts = [machine]

            if container is not None:
                name = '/'.join([machine, container_type, container])
                m = status['machines'][machine]
                if 'containers' in m and name in m['containers']:
                    self.requested_machines.append(name)
                return
        else:
            hosts = sorted(status['machines'].keys())

        self._allocate_free_containers(hosts, container_type)
        req = container_type + ':' + (machine or '0')
        self._add_new_machines(req)
        self._allocate_free_containers(hosts, container_type)

    def _allocate_free_containers(self, hosts, container_type):
        status = self.client.get_status().status
        for m in hosts:
            if 'containers' in status['machines'][m]:
                for c in status['machines'][m]['containers']:
                    self._try_allocation(c, container_type)

    def _allocate_free_machines(self):
        status = self.client.get_status().status
        for m in status['machines']:
            self._try_allocation(m)

    def _try_allocation(self, machine, container_type=None):
        if (machine not in self.requested_machines and
                machine not in self.allocated_machines):
            if container_type is not None:
                if container_type != machine.split('/')[1]:
                    return

            self.requested_machines.append(machine)
            if self._new_machine_count() == 0:
                return


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
        ssh(client, source, 'rm nc_listen.out; bash ./listen.sh')
        ssh(client, dest, 'echo "hello" | nc ' + address + ' 6778')
        result = ssh(client, source, 'more nc_listen.out')
        if result.rstrip() != 'hello':
            raise ValueError("Wrong or missing message: %r" % result.rstrip())


def assess_address_range(client, targets):
    """Test that two containers are in the same subnet as their host
    :param client: Juju client
    :param targets: machine IDs of machines to test
    :return: None; will assert failures
    """
    status = client.wait_for_started().status

    host = targets[0].split('/')[0]
    host_address = socket.gethostbyname(status['machines'][host]['dns-name'])
    host_subnet = find_network(client, host, host_address)

    for target in targets:
        vm_host = target.split('/')[0]
        addr = status['machines'][vm_host]['containers'][target]['dns-name']
        subnet = find_network(client, host, addr)
        assert host_subnet == subnet, \
            '{} ({}) not on the same subnet as {} ({})'.format(
                target, subnet, host, host_subnet)


def assess_internet_connection(client, targets):
    """Test that targets can ping Google's DNS server, google.com
    :param client: Juju client
    :param targets: machine IDs of machines to test
    :return: None; will assert failures
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


def _assess_container_networking(client, args):
    """Run _assessment_iteration on all useful combinations of containers
    :param client: Juju client
    :param args: Parsed command line arguments
    :return: None
    """
    # Only test the containers we were asked to test
    if args.machine_type:
        types = [args.machine_type]
    else:
        types = [KVM_MACHINE, LXC_MACHINE]

    mg = MachineGetter(client)
    hosts = mg.get(count=2)

    for container_type in types:
        # Test with two containers on the same host
        containers = mg.get(container_type, count=2)
        _assessment_iteration(client, containers)

        # Now test with two containers on two different hosts
        containers = [mg.get(container_type, machine=hosts[0])[0],
                      mg.get(container_type, machine=hosts[1])[0]]
        _assessment_iteration(client, containers)

    if args.machine_type is None:
        # Test with an LXC and a KVM on the same machine
        containers = [mg.get(LXC_MACHINE, machine=hosts[0])[0],
                      mg.get(KVM_MACHINE, machine=hosts[0])[0]]
        _assessment_iteration(client, containers)

        # Test with an LXC and a KVM on different machines
        containers = [mg.get(LXC_MACHINE, machine=hosts[0])[0],
                      mg.get(KVM_MACHINE, machine=hosts[1])[0]]
        _assessment_iteration(client, containers)


def assess_container_networking(client, args):
    """Runs _assess_address_allocation, reboots hosts, repeat.
    :param client: Juju client
    :param args: Parsed command line arguments
    :return: None
    """
    _assess_container_networking(client, args)
    mg = MachineGetter(client)
    hosts = mg.get(count=2)
    for host in hosts:
        ssh(client, host, 'sudo reboot')
    client.wait_for_started()
    _assess_container_networking(client, args)


def main():
    args = parse_args()
    client = EnvJujuClient.by_version(
        SimpleEnvironment.from_config(args.env),
        os.path.join(args.juju_bin, 'juju'), args.debug)
    client.enable_container_address_allocation()
    update_env(client.env, args.temp_env_name)
    juju_home = get_juju_home()
    bootstrap_host = None
    try:
        if args.clean_environment:
            try:
                clean_environment(client, services_only=True)
            except Exception as e:
                logging.exception(e)
                with temp_bootstrap_env(juju_home, client):
                    client.bootstrap(args.upload_tools)
        else:
            client.destroy_environment()
            client = make_client(
                args.juju_bin, args.debug, args.env, args.temp_env_name)
            with temp_bootstrap_env(juju_home, client):
                client.bootstrap(args.upload_tools)

        logging.info('Waiting for the bootstrap machine agent to start.')
        status = client.wait_for_started()
        mid, data = list(status.iter_machines())[0]
        bootstrap_host = data['dns-name']

        assess_container_networking(client, args)

    except Exception as e:
        logging.exception(e)
        try:
            if bootstrap_host is None:
                parse_new_state_server_from_error(e)
            else:
                dump_env_logs(client, bootstrap_host, args.logs)
        except Exception as e:
            print_now('exception while dumping logs:\n')
            logging.exception(e)
        exit(1)
    finally:
        if args.clean_environment:
            clean_environment(client, services_only=True)
        else:
            client.destroy_environment()


if __name__ == '__main__':
    main()
