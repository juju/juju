"""Tests for assess_proxy module."""

import logging
from mock import (
    call,
    Mock,
    patch,
)
import StringIO

import assess_proxy
from tests import (
    parse_error,
    TestCase,
)
from tests.test_jujupy import fake_juju_client
from utility import temp_dir


class TestParseArgs(TestCase):

    def test_common_args(self):
        with temp_dir() as log_dir:
            args = assess_proxy.parse_args(
                ["an-env", "/bin/juju", log_dir, "an-env-mod", 'both-proxied'])
        self.assertEqual("an-env", args.env)
        self.assertEqual("/bin/juju", args.juju_bin)
        self.assertEqual(log_dir, args.logs)
        self.assertEqual("an-env-mod", args.temp_env_name)
        self.assertEqual("both-proxied", args.scenario)
        self.assertEqual("eth0", args.client_interface)
        self.assertEqual("lxdbr0", args.controller_interface)
        self.assertIsFalse(args.debug)

    def test_help(self):
        fake_stdout = StringIO.StringIO()
        with parse_error(self) as fake_stderr:
            with patch("sys.stdout", fake_stdout):
                assess_proxy.parse_args(["--help"])
        self.assertEqual("", fake_stderr.getvalue())
        self.assertNotIn("TODO", fake_stdout.getvalue())


class TestMain(TestCase):

    def test_main(self):
        with temp_dir() as log_dir:
            argv = ["an-env", "/bin/juju", log_dir, "an-env-mod",
                    "both-proxied", "--verbose"]
            client = Mock(spec=["is_jes_enabled"])
            with patch("assess_proxy.configure_logging",
                       autospec=True) as mock_cl:
                with patch("assess_proxy.BootstrapManager.booted_context",
                           autospec=True) as mock_bc:
                    with patch('deploy_stack.client_from_config',
                               return_value=client) as mock_c:
                        with patch("assess_proxy.assess_proxy",
                                   autospec=True) as mock_assess:
                            with patch("assess_proxy.check_network",
                                       autospec=True,
                                       return_value='FORWARD') as mock_check:
                                with patch("assess_proxy.set_firewall",
                                           autospec=True) as mock_set:
                                    with patch("assess_proxy.reset_firewall",
                                               autospec=True) as mock_reset:
                                        assess_proxy.main(argv)
        mock_cl.assert_called_once_with(logging.DEBUG)
        mock_c.assert_called_once_with(
            'an-env', "/bin/juju", debug=False, soft_deadline=None)
        mock_check.assert_called_once_with('eth0', 'lxdbr0')
        mock_set.assert_called_once_with('both-proxied')
        mock_reset.assert_called_once_with()
        self.assertEqual(mock_bc.call_count, 1)
        mock_assess.assert_called_once_with(client, 'both-proxied')


class TestAssess(TestCase):

    def test_proxy(self):
        # Using fake_client means that deploy and get_status have plausible
        # results.  Wrapping it in a Mock causes every call to be recorded, and
        # allows assertions to be made about calls.  Mocks and the fake client
        # can also be used separately.
        fake_client = Mock(wraps=fake_juju_client())
        fake_client.bootstrap()
        assess_proxy.assess_proxy(fake_client, 'both-proxied')
        fake_client.deploy.assert_called_once_with('cs:xenial/ubuntu')
        fake_client.wait_for_started.assert_called_once_with()
        fake_client.wait_for_workloads.assert_called_once_with()
        self.assertEqual(
            1, fake_client.get_status().get_service_unit_count('ubuntu'))
        self.assertNotIn("TODO", self.log_stream.getvalue())

    def test_check_network(self):
        iptables_rule = (
            '-A INPUT -i lxdbr0 -p tcp -m tcp --dport 53 -j ACCEPT\n'
            '-A FORWARD -i lxdbr0 -m comment --comment "by lxd" -j ACCEPT\n'
            '-A FORWARD -0 lxdbr0 -m comment --comment "by lxd" -j ACCEPT')
        with patch('subprocess.check_output', autospec=True,
                   return_value=iptables_rule) as mock_scc:
            with patch('subprocess.call', autospec=True,
                       side_effect=[0, 0]) as mock_sc:
                forward_rule = assess_proxy.check_network('eth0', 'lxdbr0')
        self.assertEqual(
            '-A FORWARD -i lxdbr0 -m comment --comment "by lxd" -j ACCEPT',
            forward_rule)
        mock_scc.assert_called_once_with(
            ['sudo', 'iptables', '-S', 'FORWARD'])
        self.assertEqual(
            [call(['ifconfig', 'eth0']), call(['ifconfig', 'lxdbr0'])],
            mock_sc.mock_calls)

    def test_check_network_forward_rule_no_match_error(self):
        iptables_rule = '-A FORWARD -i lxdbr1 -j ACCEPT'
        with patch('subprocess.check_output', autospec=True,
                   return_value=iptables_rule):
            with patch('subprocess.call', autospec=True,
                       side_effect=[0, 0]):
                with self.assertRaises(ValueError):
                    assess_proxy.check_network('eth0', 'lxdbr0')

    def test_check_network_forward_rule_many_match_error(self):
        iptables_rule = (
            '-A FORWARD -i lxdbr0 -m comment --comment "by lxd" -j ACCEPT\n'
            '-A FORWARD -i lxdbr0 -m comment --comment "by other" -j ACCEPT'
            )
        with patch('subprocess.check_output', autospec=True,
                   return_value=iptables_rule):
            with patch('subprocess.call', autospec=True,
                       side_effect=[0, 0]):
                with self.assertRaises(ValueError):
                    assess_proxy.check_network('eth0', 'lxdbr0')

    def test_check_network_client_interface_error(self):
            with patch('subprocess.call', autospec=True,
                       side_effect=[0, 1]):
                with self.assertRaises(ValueError):
                    assess_proxy.check_network('eth0', 'lxdbr0')

    def test_check_network_controller_interface_error(self):
            with patch('subprocess.call', autospec=True,
                       side_effect=[1, 0]):
                with self.assertRaises(ValueError):
                    assess_proxy.check_network('eth0', 'lxdbr0')

    def test_reset_firewall(self):
        # Verify the ufw wa called to reset and disable even if one of the
        # commands exited with an error.
        with patch('subprocess.call', autospec=True,
                   side_effect=[0, 1, 0]) as mock_sc:
            assess_proxy.reset_firewall()
        self.assertEqual([
            call(('sudo', 'iptables-restore',
                  '/etc/iptables.before-assess_proxy')),
            call(('sudo', 'ufw', '--force', 'reset')),
            call(('sudo', 'ufw', '--force', 'disable'))],
            mock_sc.mock_calls)
        expected_log = (
            "INFO ('sudo', 'iptables-restore',"
            " '/etc/iptables.before-assess_proxy') exited successfully\n"
            "ERROR ('sudo', 'ufw', '--force', 'reset') exited with 1\n"
            "ERROR This host may be in a dirty state.\n"
            "INFO ('sudo', 'ufw', '--force', 'disable') exited successfully\n")
        self.assertEqual(expected_log, self.log_stream.getvalue())
