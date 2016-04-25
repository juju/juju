"""Tests for assess_user_grant_revoke module."""

import logging
from mock import (
    Mock,
    patch,
    call,
)
import StringIO
import subprocess

from assess_user_grant_revoke import (
    assess_user_grant_revoke,
    parse_args,
    main,
)
from tests import (
    parse_error,
    TestCase,
)


class TestParseArgs(TestCase):

    def test_parse_args(self):
        args = parse_args(["an-env", "/bin/juju", "/tmp/logs", "an-env-mod"])
        self.assertEqual("an-env", args.env)
        self.assertEqual("/bin/juju", args.juju_bin)
        self.assertEqual("/tmp/logs", args.logs)
        self.assertEqual("an-env-mod", args.temp_env_name)
        self.assertEqual(False, args.debug)

    def test_help(self):
        fake_stdout = StringIO.StringIO()
        with parse_error(self) as fake_stderr:
            with patch("sys.stdout", fake_stdout):
                parse_args(["--help"])
        self.assertEqual("", fake_stderr.getvalue())


class TestMain(TestCase):

    def test_main(self):
        argv = ["an-env", "/bin/juju", "/tmp/logs", "an-env-mod", "--verbose"]
        env = object()
        client = Mock(spec=["is_jes_enabled"])
        with patch("assess_user_grant_revoke.configure_logging",
                   autospec=True) as mock_cl:
            with patch("assess_user_grant_revoke.BootstrapManager.booted_context",
                       autospec=True) as mock_bc:
                with patch("jujupy.SimpleEnvironment.from_config",
                           return_value=env) as mock_e:
                    with patch("jujupy.EnvJujuClient.by_version",
                               return_value=client) as mock_c:
                        with patch("assess_user_grant_revoke.assess_user_grant_revoke",
                                   autospec=True) as mock_assess:
                            main(argv)
        mock_cl.assert_called_once_with(logging.DEBUG)
        mock_e.assert_called_once_with("an-env")
        mock_c.assert_called_once_with(env, "/bin/juju", debug=False)
        self.assertEqual(mock_bc.call_count, 1)
        mock_assess.assert_called_once_with(client)


class TestAssess(TestCase):

    def test_user_grant_revoke(self):
        mock_client = Mock(spec=["juju", "wait_for_started"])
        assess_user_grant_revoke(mock_client)
        #mock_client.juju.assert_called_once_with(
        #    'deploy', ('local:trusty/my-charm',))
        mock_client.wait_for_started.assert_called_once_with()
        self.assertNotIn("TODO", self.log_stream.getvalue())

    def test_create_cloned_environment(self):

    def test_remove_user_permissions(self):

    def test_create_user_permissions(self):

    def test__get_register_command(self):
        mock_call = Mock(spec=["get_version"])
        mock_call._get_register_command.return_value = 'juju register AaBbCc'
        ver = get_current_version(mock_call, '/tmp/bin')
        self.assertEqual(ver, '2.0-beta4')

        mock_client.get_version.return_value = '1.25.4-trusty-amd64'
        ver = get_current_version(mock_client, '/tmp/bin')
        self.assertEqual(ver, '1.25.4')