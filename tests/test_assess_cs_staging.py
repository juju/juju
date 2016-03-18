"""Tests for assess_cs_staging module."""

import logging
from mock import Mock, patch
import StringIO

from assess_cs_staging import (
    assess_deploy,
    _get_ssh_script,
    parse_args,
    main,
    _set_charm_store_ip,
)
from tests import (
    parse_error,
    TestCase,
)
from tests.test_jujupy import FakeJujuClient


class TestParseArgs(TestCase):

    def test_common_args(self):
        args = parse_args(["an-ip", "an-env", "/bin/juju", "/tmp/logs",
                           "an-env-mod"])
        self.assertEqual("an-ip", args.charm_store_ip)
        self.assertEqual("an-env", args.env)
        self.assertEqual("/bin/juju", args.juju_bin)
        self.assertEqual("/tmp/logs", args.logs)
        self.assertEqual("an-env-mod", args.temp_env_name)
        self.assertEqual(False, args.debug)
        self.assertEqual("ubuntu", args.charm)

    def test_help(self):
        fake_stdout = StringIO.StringIO()
        with parse_error(self) as fake_stderr:
            with patch("sys.stdout", fake_stdout):
                parse_args(["--help"])
        self.assertEqual("", fake_stderr.getvalue())


class TestSetCharmStoreIP(TestCase):

    def test_default_as_admin(self):
        client = FakeJujuClient()
        client.bootstrap()
        with patch.object(client, 'juju', autospec=True) as juju_mock:
            _set_charm_store_ip(client, '1.2.3.4')
        juju_mock.assert_called_once_with(
            'ssh', ('0', _get_ssh_script('1.2.3.4')))

    def test_separate_admin(self):
        client = FakeJujuClient(jes_enabled=True)
        client.bootstrap()
        admin_client = client.get_admin_client()
        # Force get_admin_client to return the *same* client, instead of an
        # equivalent one.
        with patch.object(client, 'get_admin_client',
                          return_value=admin_client, autospec=True):
            with patch.object(admin_client, 'juju',
                              autospec=True) as juju_mock:
                _set_charm_store_ip(client, '1.2.3.4')
        juju_mock.assert_called_once_with(
            'ssh', ('0', _get_ssh_script('1.2.3.4')))


class TestMain(TestCase):

    def test_main(self):
        argv = ["an-ip", "an-env", "/bin/juju", "/tmp/logs", "an-env-mod",
                "--verbose"]
        env = object()
        client = Mock(spec=["is_jes_enabled", "juju"])
        with patch("assess_cs_staging.configure_logging",
                   autospec=True) as mock_cl:
            with patch("assess_cs_staging.BootstrapManager.booted_context",
                       autospec=True) as mock_bc:
                with patch("jujupy.SimpleEnvironment.from_config",
                           return_value=env) as mock_e:
                    with patch("jujupy.EnvJujuClient.by_version",
                               return_value=client) as mock_c:
                        with patch("assess_cs_staging._set_charm_store_ip",
                                   autospec=True) as mock_set_ip:
                            with patch("assess_cs_staging.assess_deploy",
                                       autospec=True) as mock_assess:
                                main(argv)
        mock_cl.assert_called_once_with(logging.DEBUG)
        mock_e.assert_called_once_with("an-env")
        mock_c.assert_called_once_with(env, "/bin/juju", debug=False)
        self.assertEqual(mock_bc.call_count, 1)
        mock_set_ip.assert_called_once_with(client, 'an-ip')
        mock_assess.assert_called_once_with(client, 'ubuntu')


class TestAssess(TestCase):

    def test_cs_staging_deploy(self):
        mock_client = Mock(spec=["deploy", "juju", "wait_for_started"])
        assess_deploy(mock_client, "charm")
        mock_client.deploy.assert_called_once_with('charm')
        mock_client.wait_for_started.assert_called_once_with()
