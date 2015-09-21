#!/usr/bin/env python
from __future__ import print_function

__metaclass__ = type


KVM_MACHINE = 'kvm'
LXC_MACHINE = 'lxc'


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
