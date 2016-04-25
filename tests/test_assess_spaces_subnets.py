from argparse import ArgumentParser
from contextlib import contextmanager
from copy import deepcopy
from unittest import TestCase

from mock import (
    Mock,
    patch,
    )
import yaml

import assess_spaces_subnets as jss
from jujupy import (
    EnvJujuClient,
    JujuData,
    Status,
    )

__metaclass__ = type


class JujuMock:
    """A mock of the parts of the Juju command that the tests hit."""
    # XXX This is the version from assess-spaces-subnets r 1103, which has
    # enough differences from the assess_container_networking version that the
    # tests break.
    # These differences should be reconciled in a future branch.

    def __init__(self):
        self._call_n = 0
        self._status = {'services': {},
                        'machines': {'0': {}}}
        self.commands = []
        self.next_machine = 1
        self._ssh_output = []
        self._spaces = {}
        self._subnets = {}
        self._subnet_count = 0
        self._ssh_machine_output = {}
        self._next_service_machine = 1
        self._services = {}

    def add_machine(self, args):
        if isinstance(args, tuple) and args[0] == '-n':
            for n in range(int(args[1])):
                self._add_machine()
        else:
            self._add_machine(args)

    def _add_machine(self, name=None):
        if name is None or name == '':
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

        return name

    def add_service(self, name):
        # We just add a hunk of data captured from a real Juju run and don't
        # worry about how it doesn't match reality. It is enough to exercise
        # the code under test.
        new_service = {
            'units': {},
            'service-status': {
                'current': 'unknown',
                'since': '06 Aug 2015 11:39:29+01:00'
            },
            'charm': 'cs:trusty/{}-26'.format(name),
            'relations': {'cluster': [name]},
            'exposed': False}
        self._status['services'][name] = deepcopy(new_service)
        self._services[name] = 0
        self.add_unit(name)

    def add_unit(self, name):
        machine = self._add_machine()
        unit_name = name + '/' + str(self._services[name])
        self._services[name] += 1
        self._status['services'][name]['units'][unit_name] = {
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
            'agent-version': '1.25-alpha1.1',
        }

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
            if args[1] in self._ssh_machine_output:
                ssh_output = self._ssh_machine_output[args[1]]
            else:
                ssh_output = self._ssh_output

            if len(ssh_output) == 0:
                return ""

            try:
                return ssh_output[self._call_number()]
            except IndexError:
                # If we ran out of values, just return the last one
                return ssh_output[-1]

        elif cmd == 'add-space':
            self._spaces[args] = []
        elif cmd == 'list-space':
            return yaml.dump({'spaces': self._spaces})
        elif cmd == 'add-subnet':
            subnet = '10.{}.0.0/16'.format(self._subnet_count)
            self._subnet_count += 1

            self._spaces[args[1]].append(subnet)
            self._subnets[subnet] = args[0]
        elif cmd == 'deploy':
            parser = ArgumentParser()
            # Due to a long standing bug in argparse, we can't use positional
            # arguments with a '-' in their value. If you got here wondering
            # if you could deploy a charm with a name foo-bar, yes, you can
            # with Juju, but you can't with this test framework. Sorry.
            parser.add_argument('charm')
            parser.add_argument('name', nargs='?', default=None)
            parser.add_argument('--constraints', nargs=1, default="")
            args = parser.parse_args(args)

            if args.name:
                self.add_service(args.name)
            else:
                self.add_service(args.charm)

            self._next_service_machine += 1
        elif cmd == 'add-unit':
            self.add_unit(args)

        elif cmd == 'scp':
            pass

        else:
            raise ValueError("Unpatched command: {} {}".format(cmd, args))

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

    def set_ssh_output(self, ssh_output, machine_id=None):
        if machine_id is not None:
            self._ssh_machine_output[machine_id] = deepcopy(ssh_output)
        else:
            self._ssh_output = deepcopy(ssh_output)

    def reset_calls(self):
        self._call_n = 0


class JujuMockTestCase(TestCase):
    def setUp(self):
        self.client = EnvJujuClient(
            JujuData('foo', {'type': 'local'}), '1.234-76', None)

        def nil_func():
            return None

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


class TestSubnetsSpaces(JujuMockTestCase):
    def test_ipv4_to_int(self):
        self.assertEqual(
            jss.ipv4_to_int('1.2.3.4'),
            0x01020304)

        self.assertEqual(
            jss.ipv4_to_int('255.255.255.255'),
            0xFFFFFFFF)

    def test_ipv4_in_cidr(self):
        self.assertTrue(jss.ipv4_in_cidr('1.1.1.1', '1.1.1.1/32'))
        self.assertTrue(jss.ipv4_in_cidr('1.1.1.1', '1.1.1.0/24'))
        self.assertTrue(jss.ipv4_in_cidr('1.1.1.1', '1.1.0.0/16'))
        self.assertTrue(jss.ipv4_in_cidr('1.1.1.1', '1.0.0.0/8'))
        self.assertTrue(jss.ipv4_in_cidr('1.1.1.1', '0.0.0.0/0'))

        self.assertFalse(jss.ipv4_in_cidr('2.1.1.1', '1.1.1.1/32'))
        self.assertFalse(jss.ipv4_in_cidr('2.1.1.1', '1.1.1.0/24'))
        self.assertFalse(jss.ipv4_in_cidr('2.1.1.1', '1.1.0.0/16'))
        self.assertFalse(jss.ipv4_in_cidr('2.1.1.1', '1.0.0.0/8'))

    network_config = {
        'apps': ['10.0.0.0/16', '10.1.0.0/16'],
        'backend': ['10.2.0.0/16', '10.3.0.0/16'],
        'default': ['10.4.0.0/16', '10.5.0.0/16'],
        'dmz': ['10.6.0.0/16', '10.7.0.0/16'],
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

    def test_assess_spaces_subnets(self):
        # The following table is derived from the above settings

        # Charm ---------------- space --- address in subnet
        # haproxy              - dmz     - 10.6.0.2
        # mediawiki, memcached - apps    - 10.0.0.2
        # mysql, mysql-slace   - backend - 10.2.0.2

        # We translate the above table into these responses to "ip -o addr",
        # which are assigned to machines that we have found by running this
        # test. The order is fixed because we iterate over keys in dictionaries
        # in a sorted order.
        self.juju_mock.set_ssh_output(['2: eth0 inet 10.6.0.2'], '1')
        self.juju_mock.set_ssh_output(['2: eth0 inet 10.0.0.2'], '2')
        self.juju_mock.set_ssh_output(['2: eth0 inet 10.0.0.2'], '3')
        self.juju_mock.set_ssh_output(['2: eth0 inet 10.2.0.2'], '4')
        self.juju_mock.set_ssh_output(['2: eth0 inet 10.2.0.2'], '5')
        self.juju_mock.set_ssh_output(['2: eth0 inet 10.2.0.2'], '6')
        self.juju_mock.set_ssh_output(['2: eth0 inet 10.0.0.2'], '7')

        jss._assess_spaces_subnets(
            self.client, self.network_config, self.charms_to_space)

    def test_assess_spaces_subnets_fail(self):
        # The output in this test is set to be the same as in
        # test_assess_spaces_subnets with machines 1 and 2 swapped.
        # This results in mediawiki/0 appearing in the dmz instead of apps
        self.juju_mock.set_ssh_output(['2: eth0 inet 10.0.0.2'], '1')
        self.juju_mock.set_ssh_output(['2: eth0 inet 10.6.0.2'], '2')
        self.juju_mock.set_ssh_output(['2: eth0 inet 10.0.0.2'], '3')
        self.juju_mock.set_ssh_output(['2: eth0 inet 10.2.0.2'], '4')
        self.juju_mock.set_ssh_output(['2: eth0 inet 10.2.0.2'], '5')
        self.juju_mock.set_ssh_output(['2: eth0 inet 10.2.0.2'], '6')
        self.juju_mock.set_ssh_output(['2: eth0 inet 10.0.0.2'], '7')

        self.assertRaisesRegexp(
            ValueError, 'Found mediawiki/0 in dmz, expected apps',
            jss._assess_spaces_subnets,
            self.client, self.network_config, self.charms_to_space)

    def test_assess_spaces_subnets_fail_to_find_all_spaces(self):
        # Should get an error if we can't find the space for each unit
        self.juju_mock.set_ssh_output(['2: eth0 inet 10.6.0.2'], '1')
        self.juju_mock.set_ssh_output(['2: eth0 inet 10.0.0.2'], '2')
        self.juju_mock.set_ssh_output(['2: eth0 inet 10.0.0.2'], '3')
        self.assertRaisesRegexp(
            ValueError, 'Could not find spaces for all units',
            jss._assess_spaces_subnets,
            self.client, self.network_config, self.charms_to_space)
