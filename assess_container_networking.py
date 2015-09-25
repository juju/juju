#!/usr/bin/env python
from __future__ import print_function
import subprocess
from copy import (
    copy,
    deepcopy,
)
import time

__metaclass__ = type


KVM_MACHINE = 'kvm'
LXC_MACHINE = 'lxc'


def clean_environment(client, services_only=False):
    """Remove all the services and, optionally, machines from an environment.

    Use as an alternative to destroying an environment and creating a new one
    to save a bit of time.

    :param client: a Juju client
    """
    # A short timeout is used for get_status here because if we don't get a
    # response from  get_status quickly then the environment almost
    # certainly doesn't exist or needs recreating.
    status = client.get_status(5)

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
                try:
                    client.juju('remove-machine', m)
                except subprocess.CalledProcessError:
                    # Sometimes this fails because while we have asked Juju
                    # to remove a container and it says that it has, when we
                    # ask it to remove the host Juju thinks it still has
                    # containers on it. Normally a small pause and trying
                    # again is all that is needed to resolve this issue.
                    time.sleep(2)
                    s = client.wait_for_started()
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
