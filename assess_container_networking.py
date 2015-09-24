#!/usr/bin/env python
from __future__ import print_function
import subprocess
from copy import (
    copy,
    deepcopy,
)

__metaclass__ = type


KVM_MACHINE = 'kvm'
LXC_MACHINE = 'lxc'


def clean_environment(client, services_only=False):
    """Remove all the services and, optionally, machines from an environment.

    Use as an alternative to destroying an environment and creating a new one
    to save a bit of time.

    :param client: a Juju client
    """
    try:
        client.get_juju_output('status')
    except subprocess.CalledProcessError:
        # If we fail to get status then the environment doesn't exist. Since
        # there is nothing to clean, we return False to say that we failed.
        return False

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
