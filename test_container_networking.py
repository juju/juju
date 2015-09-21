from mock import (
    patch,
    Mock,
)
from unittest import TestCase
from jujupy import (
    EnvJujuClient,
    SimpleEnvironment,
    Status,
)

import assess_container_networking as jcnet
from copy import deepcopy
from contextlib import contextmanager


class JujuMock(object):
    """A mock of the parts of the Juju command that the tests hit."""

    def __init__(self):
        self._call_n = 0
        self._status = {'services': {},
                        'machines': {'0': {}}}
        self.commands = []
        self.next_machine = 1
        self._ssh_output = []

    def add_machine(self, name):
        if name == '':
            name = str(self.next_machine)
            self.next_machine += 1

        bits = name.split(':')
        if len(bits) > 1:
            # is a container
            machine = bits[1]
            container_type = bits[0]
            if machine not in self._status['machines']:
                self._status['machines'][machine] = {}
            if 'containers' not in self._status['machines'][machine]:
                self._status['machines'][machine]['containers'] = {}

            n = 0
            c_name = machine + '/' + container_type + '/' + str(n)
            while c_name in self._status['machines'][machine]['containers']:
                n += 1
                c_name = machine + '/' + container_type + '/' + str(n)
            self._status['machines'][machine]['containers'][c_name] = {}
        else:
            # normal machine
            self._status['machines'][name] = {}

    def add_service(self, name, machine=0, instance_number=1):
        # We just add a hunk of data captured from a real Juju run and don't
        # worry about how it doesn't match reality. It is enough to exercise
        # the code under test.
        new_service = {
            'units': {
                name + '/' + str(instance_number): {
                    'machine': str(machine),
                    'public-address': 'noxious-disgust.maas',
                    'workload-status': {
                        'current': 'unknown',
                        'since': '06 Aug 2015 11:39:29+01:00'},
                    'agent-status': {
                        'current': 'idle',
                        'since': '06 Aug 2015 11:39:33+01:00',
                        'version': '1.25-alpha1.1'},
                    'agent-state': 'started',
                    'agent-version': '1.25-alpha1.1'}
            },
            'service-status': {
                'current': 'unknown',
                'since': '06 Aug 2015 11:39:29+01:00'
            },
            'charm': 'cs:trusty/{}-26'.format(name),
            'relations': {'cluster': [name]},
            'exposed': False}
        self._status['services'][name] = deepcopy(new_service)

    def juju(self, cmd, *args, **kwargs):
        if len(args) == 1:
            args = args[0]
        self.commands.append((cmd, args))
        if cmd == 'remove-service' and args in self._status['services']:
            del self._status['services'][args]

        elif cmd == 'remove-machine':
            if args in self._status['machines']:
                del self._status['machines'][args]
            else:
                machine = args.split('/')[0]
                del self._status['machines'][machine]['containers'][args]

                if len(self._status['machines'][machine]['containers']) == 0:
                    del self._status['machines'][machine]['containers']

        elif cmd == 'add-machine':
            self.add_machine(args)

        elif cmd == 'ssh':
            if len(self._ssh_output) == 0:
                return ""

            try:
                return self._ssh_output[self._call_number()]
            except IndexError:
                # If we ran out of values, just return the last one
                return self._ssh_output[-1]

    @contextmanager
    def juju_async(self, cmd, args):
        self.juju(cmd, args)
        yield
        pass

    def _call_number(self):
        call_number = self._call_n
        self._call_n += 1
        return call_number

    def get_status(self, machine_id=None):
        return Status(deepcopy(self._status), "")

    def set_status(self, status):
        self._status = deepcopy(status)

    def set_ssh_output(self, ssh_output):
        self._ssh_output = deepcopy(ssh_output)

    def reset_calls(self):
        self._call_n = 0


class TestContainerNetworking(TestCase):
    def setUp(self):
        self.client = EnvJujuClient(
            SimpleEnvironment('foo', {'type': 'local'}), '1.234-76', None)

        nil_func = lambda *args, **kw: None
        self.juju_mock = JujuMock()
        self.ssh_mock = Mock()

        patches = [
            patch.object(self.client, 'juju', self.juju_mock.juju),
            patch.object(self.client, 'get_status', self.juju_mock.get_status),
            patch.object(self.client, 'juju_async', self.juju_mock.juju_async),
            patch.object(self.client, 'wait_for', nil_func),
            patch.object(self.client, 'wait_for_started',
                         self.juju_mock.get_status),
            patch.object(self.client, 'get_juju_output', self.juju_mock.juju),
        ]

        for patcher in patches:
            patcher.start()
            self.addCleanup(patcher.stop)

    def test_clean_environment(self):
        self.juju_mock.add_machine('1')
        self.juju_mock.add_machine('2')
        self.juju_mock.add_service('name')

        jcnet.clean_environment(self.client)
        self.assertEqual(3, len(self.juju_mock.commands))
        self.assertItemsEqual(self.juju_mock.commands, [
            ('remove-service', 'name'),
            ('remove-machine', '1'),
            ('remove-machine', '2'),
        ])

    def test_clean_environment_with_containers(self):
        self.juju_mock.add_machine('1')
        self.juju_mock.add_machine('2')
        self.juju_mock.add_machine('lxc:0')
        self.juju_mock.add_machine('kvm:0')

        jcnet.clean_environment(self.client)
        self.assertEqual(4, len(self.juju_mock.commands))
        self.assertItemsEqual(self.juju_mock.commands, [
            ('remove-machine', '0/lxc/0'),
            ('remove-machine', '0/kvm/0'),
            ('remove-machine', '1'),
            ('remove-machine', '2')
        ])

    def test_clean_environment_just_services(self):
        self.juju_mock.add_machine('1')
        self.juju_mock.add_machine('2')
        self.juju_mock.add_machine('lxc:0')
        self.juju_mock.add_machine('kvm:0')
        self.juju_mock.add_service('name')

        jcnet.clean_environment(self.client, True)
        self.assertEqual(1, len(self.juju_mock.commands))
        self.assertEqual(self.juju_mock.commands, [
            ('remove-service', 'name'),
        ])

    def test_request_machines(self):
        mg = jcnet.MachineGetter(self.client)
        # Requesting a machine with none have been allocated will always
        # return machine 0, which always exists if we have an environment.
        self.assertEqual(mg.get(), ['0'])

        # Since we haven't done anything with machine 0, we will still get
        # it back
        self.assertEqual(mg.get(), ['0'])

        # Adding a service will remove the machine it is on from the
        # available pool. Requesting a machine will allocate a new one.
        self.juju_mock.add_service('name', 0)
        self.assertEqual(mg.get(), ['1'])

        # Request a specific machine without services allocated to it.
        self.assertEqual(mg.get(machine=1), ['1'])

        # Request a specific machine with services allocated to it
        self.assertEqual(mg.get(machine=0), ['0'])

        # Requesting a specific machine that doesn't exist returns nothing
        self.assertEqual(mg.get(machine=2), [])

        # Request more machines than are allocated - a new one appears
        self.assertEqual(mg.get(count=2), ['1', '2'])

        # Still there!
        self.assertEqual(mg.get(count=2), ['1', '2'])

        # Now containers (kvm). Ask for a kvm
        self.assertEqual(mg.get(jcnet.KVM_MACHINE), ['0/kvm/0'])

        # Ask for a kvm on machine 1
        self.assertEqual(mg.get(jcnet.KVM_MACHINE, '1'), ['1/kvm/0'])

        # Ask for two kvms on machine 1
        machines = mg.get(jcnet.KVM_MACHINE, '1', count=2)
        self.assertEqual(machines, ['1/kvm/0', '1/kvm/1'])

        # Ask for a specific kvm
        self.assertEqual(mg.get(jcnet.KVM_MACHINE, '1', '1'), ['1/kvm/1'])

        # Ask for a specific kvm that doesn't exist
        self.assertEqual(mg.get(jcnet.KVM_MACHINE, '1', '2'), [])

        # Now containers (lxc). Ask for an lxc
        self.assertEqual(mg.get(jcnet.LXC_MACHINE), ['0/lxc/0'])

        # Ask for a lxc on machine 1
        self.assertEqual(mg.get(jcnet.LXC_MACHINE, '1'), ['1/lxc/0'])

        # Ask for two lxcs on machine 1
        machines = mg.get(jcnet.LXC_MACHINE, '1', count=2)
        self.assertEqual(machines, ['1/lxc/0', '1/lxc/1'])

        # Ask for a specific lxc
        self.assertEqual(mg.get(jcnet.LXC_MACHINE, '1', '1'), ['1/lxc/1'])

        # Ask for a specific lxc that doesn't exist
        self.assertEqual(mg.get(jcnet.LXC_MACHINE, '1', '2'), [])
