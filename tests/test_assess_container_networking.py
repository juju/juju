from argparse import Namespace
from copy import deepcopy
from contextlib import contextmanager

from mock import (
    patch,
    Mock,
    MagicMock,
)

from jujupy import (
    EnvJujuClient,
    SimpleEnvironment,
    Status,
)

import assess_container_networking as jcnet
from tests import (
    parse_error,
    TestCase,
)


__metaclass__ = type


class JujuMock:
    """A mock of the parts of the Juju command that the tests hit."""

    def __init__(self):
        self._call_n = 0
        self._status = {'services': {},
                        'machines': {'0': {}}}
        self.commands = []
        self.next_machine = 1
        self._ssh_output = []

    def add_machine(self, args):
        if isinstance(args, tuple) and args[0] == '-n':
            for n in range(int(args[1])):
                self._add_machine('')
        else:
            self._add_machine(args)

    def _add_machine(self, name):
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

        self.juju_mock = JujuMock()
        self.ssh_mock = Mock()

        patches = [
            patch.object(self.client, 'juju', self.juju_mock.juju),
            patch.object(self.client, 'get_status', self.juju_mock.get_status),
            patch.object(self.client, 'juju_async', self.juju_mock.juju_async),
            patch.object(self.client, 'wait_for', lambda *args, **kw: None),
            patch.object(self.client, 'wait_for_started',
                         self.juju_mock.get_status),
            patch.object(self.client, 'get_juju_output', self.juju_mock.juju),
        ]

        for patcher in patches:
            patcher.start()
            self.addCleanup(patcher.stop)

    def assert_ssh(self, args, machine, cmd):
        self.assertEqual(args, [('ssh', machine, cmd), ])

    def test_parse_args(self):
        # Test a simple command line that should work
        cmdline = ['env', 'juju_bin', 'logs', 'ten']
        args = jcnet.parse_args(cmdline)
        self.assertEqual(args.machine_type, None)
        self.assertEqual(args.juju_bin, 'juju_bin')
        self.assertEqual(args.env, 'env')
        self.assertEqual(args.logs, 'logs')
        self.assertEqual(args.temp_env_name, 'ten')
        self.assertEqual(args.debug, False)
        self.assertEqual(args.upload_tools, False)
        self.assertEqual(args.clean_environment, False)

        # check the optional arguments
        opts = ['--machine-type', jcnet.KVM_MACHINE, '--debug',
                '--upload-tools', '--clean-environment']
        args = jcnet.parse_args(cmdline + opts)
        self.assertEqual(args.machine_type, jcnet.KVM_MACHINE)
        self.assertEqual(args.debug, True)
        self.assertEqual(args.upload_tools, True)
        self.assertEqual(args.clean_environment, True)

        # Now check that we can only set machine_type to kvm or lxc
        opts = ['--machine-type', jcnet.LXC_MACHINE]
        args = jcnet.parse_args(cmdline + opts)
        self.assertEqual(args.machine_type, jcnet.LXC_MACHINE)

        # Set up an error (bob is invalid)
        opts = ['--machine-type', 'bob']
        with parse_error(self) as stderr:
            jcnet.parse_args(cmdline + opts)
        self.assertRegexpMatches(
            stderr.getvalue(),
            ".*error: argument --machine-type: invalid choice: 'bob'.*")

    def test_ssh(self):
        machine, addr = '0', 'foobar'
        with patch.object(self.client, 'get_juju_output',
                          autospec=True) as ssh_mock:
            jcnet.ssh(self.client, machine, addr)
            self.assertEqual(1, ssh_mock.call_count)
            self.assert_ssh(ssh_mock.call_args, machine, addr)

    def test_find_network(self):
        machine, addr = '0', '1.1.1.1'
        self.assertRaisesRegexp(
            ValueError, "Unable to find route to '1.1.1.1'",
            jcnet.find_network, self.client, machine, addr)

        self.juju_mock.set_ssh_output([
            'default via 192.168.0.1 dev eth3\n'
            '1.1.1.0/24 dev eth3  proto kernel  scope link  src 1.1.1.22',
        ])
        self.juju_mock.commands = []
        jcnet.find_network(self.client, machine, addr)
        self.assertItemsEqual(self.juju_mock.commands,
                              [('ssh', (
                               machine, 'ip route show to match ' + addr))])

    def test_clean_environment(self):
        self.juju_mock.add_machine('1')
        self.juju_mock.add_machine('2')
        self.juju_mock.add_service('name')

        jcnet.clean_environment(self.client)
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
        self.assertEqual(self.juju_mock.commands, [
            ('remove-service', 'name'),
        ])

    def test_make_machines(self):
        hosts, containers = jcnet.make_machines(
            self.client, [jcnet.LXC_MACHINE, jcnet.KVM_MACHINE])
        self.assertEqual(hosts, ['0', '1'])
        expected = {
            'kvm': {'0': ['0/kvm/1', '0/kvm/0'],
                    '1': ['1/kvm/0']},
            'lxc': {'0': ['0/lxc/0', '0/lxc/1'],
                    '1': ['1/lxc/0']}
        }
        self.assertDictEqual(containers, expected)

        hosts, containers = jcnet.make_machines(
            self.client, [jcnet.LXC_MACHINE])
        self.assertEqual(hosts, ['0', '1'])
        expected = {
            'lxc': {'0': ['0/lxc/0', '0/lxc/1'],
                    '1': ['1/lxc/0']}
        }
        self.assertDictEqual(containers, expected)

        hosts, containers = jcnet.make_machines(
            self.client, [jcnet.KVM_MACHINE])
        self.assertEqual(hosts, ['0', '1'])
        expected = {
            'kvm': {'0': ['0/kvm/1', '0/kvm/0'],
                    '1': ['1/kvm/0']},
        }
        self.assertDictEqual(containers, expected)

    def test_test_network_traffic(self):
        targets = ['0/lxc/0', '0/lxc/1']
        self.juju_mock.set_status({'machines': {'0': {
            'containers': {targets[0]: {'dns-name': '0-dns-name'}}}}})

        with patch('assess_container_networking.get_random_string',
                   lambda *args, **kw: 'hello'):

            self.juju_mock.set_ssh_output(['', '', 'hello'])
            jcnet.assess_network_traffic(self.client, targets)

            self.juju_mock.reset_calls()
            self.juju_mock.set_ssh_output(['', '', 'fail'])
            self.assertRaisesRegexp(
                ValueError, "Wrong or missing message: 'fail'",
                jcnet.assess_network_traffic, self.client, targets)

    def test_test_address_range(self):
        targets = ['0/lxc/0', '0/lxc/1']
        self.juju_mock.set_status({'machines': {'0': {
            'containers': {
                targets[0]: {'dns-name': 'lxc0-dns-name'},
                targets[1]: {'dns-name': 'lxc1-dns-name'},
            },
            'dns-name': '0-dns-name',
        }}})
        self.juju_mock.set_ssh_output([
            'default via 192.168.0.1 dev eth3',
            '2: eth3    inet 192.168.0.22/24 brd 192.168.0.255 scope '
            'global eth3\       valid_lft forever preferred_lft forever',
            '192.168.0.0/24',
        ])

        jcnet.assess_address_range(self.client, targets)

    def test_test_address_range_fail(self):
        targets = ['0/lxc/0', '0/lxc/1']
        self.juju_mock.set_status({'machines': {'0': {
            'containers': {
                targets[0]: {'dns-name': 'lxc0-dns-name'},
                targets[1]: {'dns-name': 'lxc1-dns-name'},
            },
            'dns-name': '0-dns-name',
        }}})
        self.juju_mock.set_ssh_output([
            'default via 192.168.0.1 dev eth3',
            '2: eth3    inet 192.168.0.22/24 brd 192.168.0.255 scope '
            'global eth3\       valid_lft forever preferred_lft forever',
            '192.168.0.0/24',
            '192.168.1.0/24',
            '192.168.2.0/24',
            '192.168.3.0/24',
        ])

        self.assertRaisesRegexp(
            ValueError, '0/lxc/0 \S+ not on the same subnet as 0 \S+',
            jcnet.assess_address_range, self.client, targets)

    def test_test_internet_connection(self):
        targets = ['0/lxc/0', '0/lxc/1']
        self.juju_mock.set_status({'machines': {'0': {
            'containers': {
                targets[0]: {'dns-name': 'lxc0-dns-name'},
                targets[1]: {'dns-name': 'lxc1-dns-name'},
            },
            'dns-name': '0-dns-name',
        }}})

        # Can ping default route
        self.juju_mock.set_ssh_output([
            'default via 192.168.0.1 dev eth3', 0,
            'default via 192.168.0.1 dev eth3', 0])
        jcnet.assess_internet_connection(self.client, targets)

        # Can't ping default route
        self.juju_mock.set_ssh_output([
            'default via 192.168.0.1 dev eth3', 1])
        self.juju_mock.reset_calls()
        self.assertRaisesRegexp(
            ValueError, "0/lxc/0 unable to ping default route",
            jcnet.assess_internet_connection, self.client, targets)

        # Can't find default route
        self.juju_mock.set_ssh_output(['', 1])
        self.juju_mock.reset_calls()
        self.assertRaisesRegexp(
            ValueError, "Default route not found",
            jcnet.assess_internet_connection, self.client, targets)

    def test_get_client(self):
        args = Namespace(env="e", juju_bin="jb", debug=False,
            agent_stream="http://tools.testing/agents", temp_env_name="te",
            series="s", bootstrap_host="bh", agent_url="au", region="r")
        
        upenv = MagicMock()
        with patch.object(EnvJujuClient, "by_version") as ejc, \
             patch.object(SimpleEnvironment, "from_config") as se, \
             patch("assess_container_networking.update_env", upenv):

                jcnet.get_client(args)
                self.assertEqual(upenv.call_args[0][1], args.temp_env_name)
                for key, value in upenv.call_args[1].iteritems():
                    self.assertEqual(vars(args)[key], value)
