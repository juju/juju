from copy import deepcopy
from contextlib import contextmanager
import logging

from mock import (
    patch,
    Mock,
)

from jujupy import (
    EnvJujuClient,
    JujuData,
    KVM_MACHINE,
    LXC_MACHINE,
    LXD_MACHINE,
    SimpleEnvironment,
    Status,
)

import assess_container_networking as jcnet
from tests import (
    FakeHomeTestCase,
    parse_error,
    TestCase,
)
from tests.test_jujupy import fake_juju_client


__metaclass__ = type


class JujuMock(EnvJujuClient):
    """A mock of the parts of the Juju command that the tests hit."""

    def __init__(self, *args, **kwargs):
        super(JujuMock, self).__init__(*args, **kwargs)
        self._call_n = 0
        self.commands = []
        self.next_machine = 1
        self._ssh_output = []

    def add_machine(self, args):
        if isinstance(args, tuple) and args[0] == '-n':
            for n in range(int(args[1])):
                self._add_machine('')
        else:
            self._add_machine(args)

    @property
    def _model_state(self):
        return self._backend.controller_state.models[self.model_name]

    def _add_machine(self, name):
        if name == '':
            name = str(self.next_machine)
            self.next_machine += 1

        bits = name.split(':')
        if len(bits) > 1:
            # is a container
            machine = bits[1]
            container_type = bits[0]

            n = 0
            self._model_state.add_container(container_type, machine, n)
        else:
            # normal machine
            self._model_state.add_machine(machine_id=name)

    def add_service(self, name, machine=0, instance_number=1):
        self._model_state.add_unit(name)

    def juju(self, cmd, *rargs, **kwargs):
        if len(rargs) == 1:
            args = rargs[0]
        else:
            args = rargs
        if cmd != 'bootstrap':
            self.commands.append((cmd, args))
        if cmd == 'ssh':
            if len(self._ssh_output) == 0:
                return ""

            try:
                return self._ssh_output[self._call_number()]
            except IndexError:
                # If we ran out of values, just return the last one
                return self._ssh_output[-1]
        else:
            return super(JujuMock, self).juju(cmd, *rargs, **kwargs)

    def _call_number(self):
        call_number = self._call_n
        self._call_n += 1
        return call_number

    def set_ssh_output(self, ssh_output):
        self._ssh_output = deepcopy(ssh_output)

    def reset_calls(self):
        self._call_n = 0


class TestContainerNetworking(TestCase):
    def setUp(self):
        self.client = EnvJujuClient(
            JujuData('foo', {'type': 'local'}), '1.234-76', None)

        self.juju_mock = fake_juju_client(cls=JujuMock)
        self.juju_mock.bootstrap()
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
        self.assertEqual(args, [('ssh', '--proxy', machine, cmd), ])

    def test_parse_args(self):
        # Test a simple command line that should work
        cmdline = ['env', '/juju', 'logs', 'ten']
        args = jcnet.parse_args(cmdline)
        self.assertEqual(args.machine_type, None)
        self.assertEqual(args.juju_bin, '/juju')
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

        # Machine type can also be lxd
        opts = ['--machine-type', jcnet.LXD_MACHINE]
        args = jcnet.parse_args(cmdline + opts)
        self.assertEqual(args.machine_type, jcnet.LXD_MACHINE)

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
                               '--proxy', machine,
                               'ip route show to match ' + addr))])

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
        self.juju_mock.add_machine('0')
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
        self.juju_mock._model_state.add_machine()
        self.juju_mock._model_state.add_container('lxc', '0')

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
        self.juju_mock._model_state.add_machine()
        self.juju_mock._model_state.add_container('lxc', '0')
        self.juju_mock._model_state.add_container('lxc', '0')
        self.juju_mock.set_ssh_output([
            'default via 192.168.0.1 dev eth3',
            '2: eth3    inet 192.168.0.22/24 brd 192.168.0.255 scope '
            'global eth3\       valid_lft forever preferred_lft forever',
            '192.168.0.0/24',
        ])

        jcnet.assess_address_range(self.client, targets)

    def test_test_address_range_fail(self):
        targets = ['0/lxc/0', '0/lxc/1']
        self.juju_mock._model_state.add_machine()
        self.juju_mock._model_state.add_container('lxc', '0')
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
        model_state = self.juju_mock._model_state
        model_state.add_machine(host_name='0-dns-name')
        model_state.add_container('lxc', '0')
        model_state.add_container('lxc', '0')

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


class TestMain(FakeHomeTestCase):

    @contextmanager
    def patch_main(self, argv, client, log_level, debug=False):
        env = SimpleEnvironment(argv[0], {"type": "ec2"})
        client.env = env
        with patch("assess_container_networking.configure_logging",
                   autospec=True) as mock_cl:
            with patch("jujupy.SimpleEnvironment.from_config",
                       return_value=env) as mock_e:
                with patch("jujupy.EnvJujuClient.by_version",
                           return_value=client) as mock_c:
                    yield
        mock_cl.assert_called_once_with(log_level)
        mock_e.assert_called_once_with(argv[0])
        mock_c.assert_called_once_with(env, argv[1], debug=debug)

    @contextmanager
    def patch_bootstrap_manager(self, runs=True):
        with patch("deploy_stack.BootstrapManager.top_context",
                   autospec=True) as mock_tc:
            with patch("deploy_stack.BootstrapManager.bootstrap_context",
                       autospec=True) as mock_bc:
                with patch("deploy_stack.BootstrapManager.runtime_context",
                           autospec=True) as mock_rc:
                    yield mock_bc
        self.assertEqual(mock_tc.call_count, 1)
        if runs:
            self.assertEqual(mock_rc.call_count, 1)

    def test_bootstrap_required(self):
        argv = ["an-env", "/bin/juju", "/tmp/logs", "an-env-mod", "--verbose"]
        client = Mock(spec=["bootstrap", "enable_feature", "is_jes_enabled"])
        client.supported_container_types = frozenset([KVM_MACHINE,
                                                      LXC_MACHINE])
        with patch("assess_container_networking.assess_container_networking",
                   autospec=True) as mock_assess:
            with self.patch_bootstrap_manager() as mock_bc:
                with self.patch_main(argv, client, logging.DEBUG):
                    ret = jcnet.main(argv)
        client.enable_feature.assert_called_once_with('address-allocation')
        client.bootstrap.assert_called_once_with(False)
        self.assertEqual("", self.log_stream.getvalue())
        self.assertEqual(mock_bc.call_count, 1)
        mock_assess.assert_called_once_with(client, [KVM_MACHINE, "lxc"])
        self.assertEqual(ret, 0)

    def test_clean_existing_env(self):
        argv = ["an-env", "/bin/juju", "/tmp/logs", "an-env-mod",
                "--clean-environment"]
        client = Mock(spec=["enable_feature", "env", "get_status",
                            "is_jes_enabled", "wait_for", "wait_for_started"])
        client.supported_container_types = frozenset([KVM_MACHINE,
                                                      LXC_MACHINE])
        client.get_status.return_value = Status.from_text("""
            machines:
                "0":
                    agent-state: started
        """)
        with patch("assess_container_networking.assess_container_networking",
                   autospec=True) as mock_assess:
            with self.patch_bootstrap_manager() as mock_bc:
                with self.patch_main(argv, client, logging.INFO):
                    ret = jcnet.main(argv)
        client.enable_feature.assert_called_once_with('address-allocation')
        self.assertEqual(client.env.environment, "an-env-mod")
        self.assertEqual("", self.log_stream.getvalue())
        self.assertEqual(mock_bc.call_count, 0)
        mock_assess.assert_called_once_with(client, ["kvm", "lxc"])
        self.assertEqual(ret, 0)

    def test_clean_missing_env(self):
        argv = ["an-env", "/bin/juju", "/tmp/logs", "an-env-mod",
                "--clean-environment"]
        client = Mock(spec=["bootstrap", "enable_feature", "env", "get_status",
                            "is_jes_enabled", "wait_for", "wait_for_started"])
        client.supported_container_types = frozenset([KVM_MACHINE,
                                                      LXC_MACHINE])
        client.get_status.side_effect = [
            Exception("Timeout"),
            Status.from_text("""
                machines:
                    "0":
                        agent-state: started
            """),
        ]
        with patch("assess_container_networking.assess_container_networking",
                   autospec=True) as mock_assess:
            with self.patch_bootstrap_manager() as mock_bc:
                with self.patch_main(argv, client, logging.INFO):
                    ret = jcnet.main(argv)
        client.enable_feature.assert_called_once_with('address-allocation')
        self.assertEqual(client.env.environment, "an-env-mod")
        client.bootstrap.assert_called_once_with(False)
        self.assertEqual(
            "INFO Could not clean existing env: Timeout\n",
            self.log_stream.getvalue())
        self.assertEqual(mock_bc.call_count, 1)
        mock_assess.assert_called_once_with(client, ["kvm", "lxc"])
        self.assertEqual(ret, 0)

    def test_final_clean_fails(self):
        argv = ["an-env", "/bin/juju", "/tmp/logs", "an-env-mod",
                "--clean-environment"]
        client = Mock(spec=["enable_feature", "env", "get_status",
                            "is_jes_enabled", "wait_for", "wait_for_started"])
        client.supported_container_types = frozenset([KVM_MACHINE,
                                                      LXC_MACHINE])
        client.get_status.side_effect = [
            Status.from_text("""
                machines:
                    "0":
                        agent-state: started
            """),
            Exception("Timeout"),
        ]
        with patch("assess_container_networking.assess_container_networking",
                   autospec=True) as mock_assess:
            with self.patch_bootstrap_manager() as mock_bc:
                with self.patch_main(argv, client, logging.INFO):
                    ret = jcnet.main(argv)
        client.enable_feature.assert_called_once_with('address-allocation')
        self.assertEqual(client.env.environment, "an-env-mod")
        self.assertEqual(
            "INFO Could not clean existing env: Timeout\n",
            self.log_stream.getvalue())
        self.assertEqual(mock_bc.call_count, 0)
        mock_assess.assert_called_once_with(client, ["kvm", "lxc"])
        self.assertEqual(ret, 1)

    def test_lxd_unsupported_on_juju_1(self):
        argv = ["an-env", "/bin/juju", "/tmp/logs", "an-env-mod", "--verbose",
                "--machine-type=lxd"]
        client = Mock(spec=["bootstrap", "enable_feature", "is_jes_enabled"])
        client.version = "1.25.5"
        client.supported_container_types = frozenset([LXC_MACHINE,
                                                      KVM_MACHINE])
        with self.patch_main(argv, client, logging.DEBUG):
            with self.assertRaises(Exception) as exc_ctx:
                jcnet.main(argv)
            self.assertEqual(
                str(exc_ctx.exception),
                "no lxd support on juju 1.25.5")
        client.enable_feature.assert_called_once_with('address-allocation')
        self.assertEqual(client.bootstrap.call_count, 0)
        self.assertEqual("", self.log_stream.getvalue())

    def test_lxd_tested_on_juju_2(self):
        argv = ["an-env", "/bin/juju", "/tmp/logs", "an-env-mod", "--verbose"]
        client = Mock(spec=["bootstrap", "enable_feature", "is_jes_enabled"])
        client.supported_container_types = frozenset([
            LXD_MACHINE, KVM_MACHINE, LXC_MACHINE])
        with patch("assess_container_networking.assess_container_networking",
                   autospec=True) as mock_assess:
            with self.patch_bootstrap_manager() as mock_bc:
                with self.patch_main(argv, client, logging.DEBUG):
                    ret = jcnet.main(argv)
        client.enable_feature.assert_called_once_with('address-allocation')
        client.bootstrap.assert_called_once_with(False)
        self.assertEqual("", self.log_stream.getvalue())
        self.assertEqual(mock_bc.call_count, 1)
        mock_assess.assert_called_once_with(client, [
            KVM_MACHINE, LXC_MACHINE, LXD_MACHINE])
        self.assertEqual(ret, 0)
