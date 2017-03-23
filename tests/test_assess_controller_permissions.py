"""Tests for assess_controller_permissions module."""

import logging
from mock import Mock, patch
import StringIO

from assess_controller_permissions import (
    assess_controller_permissions,
    parse_args,
    main,
)
from test_assess_user_grant_revoke import (
    make_fake_client,
)
from tests import (
    parse_error,
    TestCase,
)


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
        mock_c.assert_called_once_with('an-env', "/bin/juju", debug=False,
                                       soft_deadline=None)
        self.assertEqual(mock_bc.call_count, 1)
        mock_assess.assert_called_once_with(client)


class TestAssess(TestCase):

    def test_assess_controller_permissions(self):
        fake_client = make_fake_client()
        fake_client.bootstrap()
        assert_lc_calls = [True, True, True]
        assert_ac_calls = [False, True, True]
        assert_sc_calls = [False, False, True]
        lc = patch('assess_controller_permissions.assert_login_permission',
                   autospec=True)
        ac = patch('assess_controller_permissions.assert_addmodel_permission',
                   autospec=True)
        sc = patch('assess_controller_permissions.assert_superuser_permission',
                   autospec=True)
        with lc as lc_mock, ac as ac_mock, sc as sc_mock:
            with patch("jujupy.ModelClient.expect",
                       autospec=True) as expect_mock:
                with patch("jujupy.ModelClient._end_pexpect_session",
                           autospec=True):
                    expect_mock.return_value.isalive.return_value = False
                    assess_controller_permissions(fake_client)
                    lc_calls = [
                        call[0][4] for call in
                        lc_mock.call_args_list]
                    ac_calls = [
                        call[0][2] for call in
                        ac_mock.call_args_list]
                    sc_calls = [
                        call[0][2] for call in
                        sc_mock.call_args_list]
                    self.assertEqual(lc_calls,
                                     assert_lc_calls)
                    self.assertEqual(ac_calls,
                                     assert_ac_calls)
                    self.assertEqual(sc_calls,
                                     assert_sc_calls)
