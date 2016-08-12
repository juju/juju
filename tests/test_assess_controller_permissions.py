"""Tests for assess_controller_permissions module."""

import logging
from mock import Mock, patch
import StringIO

from assess_controller_permissions import (
    assess_controller_permissions,
    parse_args,
    main,
)
from assess_user_grant_revoke import User
from tests import (
    parse_error,
    TestCase,
)
from tests.test_jujupy import fake_juju_client


class TestParseArgs(TestCase):

    def test_common_args(self):
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
        self.assertNotIn("TODO", fake_stdout.getvalue())


class TestMain(TestCase):

    def test_main(self):
        argv = ["an-env", "/bin/juju", "/tmp/logs", "an-env-mod", "--verbose"]
        client = Mock(spec=["is_jes_enabled"])
        with patch("assess_controller_permissions.configure_logging",
                   autospec=True) as mock_cl:
            with patch("assess_controller_permissions."
                       "BootstrapManager.booted_context",
                       autospec=True) as mock_bc:
                with patch('deploy_stack.client_from_config',
                           return_value=client) as mock_c:
                    with patch("assess_controller_permissions."
                               "assess_controller_permissions",
                               autospec=True) as mock_assess:
                        main(argv)
        mock_cl.assert_called_once_with(logging.DEBUG)
        mock_c.assert_called_once_with('an-env', "/bin/juju", debug=False)
        self.assertEqual(mock_bc.call_count, 1)
        mock_assess.assert_called_once_with(client)


class TestAssess(TestCase):

    @patch('assess_controller_permissions.assert_superuser_controller',
           autospec=True)
    @patch('assess_controller_permissions.assert_addmodel_controller',
           autospec=True)
    @patch('assess_controller_permissions.assert_login_controller',
           autospec=True)
    def test_controller_permissions(self, lc_mock, ac_mock, sc_mock):
        fake_client = Mock(wraps=fake_juju_client())
        fake_client.bootstrap()
        patch('assess_controller_permissions.assert_login_controller.')
        assess_controller_permissions(fake_client)
        lc_mock.assert_called_once_with(
            fake_client, User('login_controller', 'login', []))
        ac_mock.assert_called_once_with(
            fake_client, User('addmodel_controller', 'addmodel', []))
        sc_mock.assert_called_once_with(
            fake_client, User('superuser_controller', 'superuser', []))
        self.assertNotIn("TODO", self.log_stream.getvalue())
