"""Tests for assess_user_grant_revoke module."""

from collections import namedtuple
import logging
from mock import (
    Mock,
    patch,
)
import os
import StringIO
from subprocess import CalledProcessError

from assess_user_grant_revoke import (
    assert_read_model,
    assert_user_permissions,
    assert_write_model,
    assess_user_grant_revoke,
    JujuAssertionError,
    main,
    parse_args,
)
from jujupy import (
    fake_juju_client,
    FakeBackend,
    JUJU_DEV_FEATURE_FLAGS,
)
from tests import (
    parse_error,
    TestCase,
    patch_juju_call,
)


class FakeBackendShellEnv(FakeBackend):

    def shell_environ(self, used_feature_flags, juju_home):
        env = dict(os.environ)
        if self.full_path is not None:
            env['PATH'] = '{}{}{}'.format(os.path.dirname(self.full_path),
                                          os.pathsep, env['PATH'])
        flags = self.feature_flags.intersection(used_feature_flags)
        if flags:
            env[JUJU_DEV_FEATURE_FLAGS] = ','.join(sorted(flags))
        env['JUJU_DATA'] = juju_home
        return env


class PexpectInteraction:

    def __init__(self, sendline_str, expected_str):
        self._expect_strings = iter(expected_str)
        self._sendline_strings = iter(sendline_str)

    def _check_string(self, string, string_list):
        expected_string = next(string_list)
        if string != expected_string:
            raise ValueError(
                'Expected {} got {}'.format(expected_string, string)
            )

    def expect(self, string):
        self._check_string(string, self._expect_strings)

    def sendline(self, string):
        self._check_string(string, self._sendline_strings)

    def isalive(self):
        return False


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
        client = Mock(spec=["is_jes_enabled"])
        with patch("assess_user_grant_revoke.BootstrapManager.booted_context",
                   autospec=True) as mock_bc:
            with patch("assess_user_grant_revoke.assess_user_grant_revoke",
                       autospec=True) as mock_assess:
                with patch("assess_user_grant_revoke.configure_logging",
                           autospec=True) as mock_cl:
                            with patch("deploy_stack.client_from_config",
                                       return_value=client) as mock_c:
                                main(argv)
        mock_cl.assert_called_once_with(logging.DEBUG)
        mock_c.assert_called_once_with("an-env", "/bin/juju", debug=False,
                                       soft_deadline=None)
        self.assertEqual(mock_bc.call_count, 1)
        mock_assess.assert_called_once_with(client)


class TestAsserts(TestCase):

    def test_assert_user_permissions(self):
        User = namedtuple('user', ['name', 'permissions', 'expect'])
        read_user = User('readuser', 'read',
                         [True, False, False, False, False, False])
        write_user = User('writeuser', 'write',
                          [True, True, False, True, False, False])
        admin_user = User('adminuser', 'admin',
                          [True, True, True, True, False, False])
        users = [read_user, write_user, admin_user]

        for user in users:
            fake_client = fake_juju_client()
            fake_admin_client = fake_juju_client()
            ac = patch("assess_user_grant_revoke.assert_admin_model",
                       return_value=True)
            with patch("jujupy.ModelClient.revoke", return_value=True):
                with patch("assess_user_grant_revoke.assert_read_model",
                           return_value=True) as read_mock:
                    with patch("assess_user_grant_revoke.assert_write_model",
                               return_value=True) as write_mock:
                        with ac as admin_mock:
                            assert_user_permissions(user, fake_client,
                                                    fake_admin_client)
                            self.assertEqual(read_mock.call_count, 2)
                            self.assertEqual(write_mock.call_count, 2)
                            self.assertEqual(admin_mock.call_count, 2)

    def test_assert_read_model(self):
        fake_client = fake_juju_client()
        with patch.object(fake_client, 'show_status', return_value=True):
            with patch.object(fake_client, 'juju',
                              return_value=True):
                assert_read_model(fake_client, 'read', True)
                with self.assertRaises(JujuAssertionError):
                    assert_read_model(fake_client, 'read', False)
        with patch.object(fake_client, 'show_status', return_value=False,
                          side_effect=CalledProcessError(None, None, None)):
            with patch.object(fake_client, 'juju',
                              return_value=False,
                              side_effect=CalledProcessError(
                                None, None, None)):
                assert_read_model(fake_client, 'read', False)
                with self.assertRaises(JujuAssertionError):
                    assert_read_model(fake_client, 'read', True)

    def test_assert_write_model(self):
        fake_client = fake_juju_client()
        with patch.object(fake_client, 'wait_for_started'):
            with patch_juju_call(fake_client):
                assert_write_model(fake_client, 'write', True)
                with self.assertRaises(JujuAssertionError):
                    assert_write_model(fake_client, 'write', False)
            deploy_side_effect = CalledProcessError(None, None, None)
            with patch.object(fake_client, 'juju', return_value=False,
                              side_effect=deploy_side_effect):
                assert_write_model(fake_client, 'write', False)
                with self.assertRaises(JujuAssertionError):
                    assert_write_model(fake_client, 'write', True)


def make_fake_client():
    fake_client = fake_juju_client()
    old_backend = fake_client._backend
    fake_client._backend = FakeBackendShellEnv(
        old_backend.controller_state, old_backend.feature_flags,
        old_backend.version, old_backend.full_path, old_backend.debug)
    return fake_client


class TestAssess(TestCase):

    def test_user_grant_revoke(self):
        fake_client = make_fake_client()
        fake_client.bootstrap()
        assert_read_calls = [True, False, True, True, True, True]
        assert_write_calls = [False, False, True, False, True, True]
        assert_admin_calls = [False, False, False, False, True, True]
        cpass = patch("assess_user_grant_revoke.assert_change_password",
                      autospec=True)
        able = patch("assess_user_grant_revoke.assert_disable_enable",
                     autospec=True)
        log = patch("assess_user_grant_revoke.assert_logout_login",
                    autospec=True)
        expect = patch("jujupy.ModelClient.expect",
                       autospec=True)
        read = patch("assess_user_grant_revoke.assert_read_model",
                     autospec=True)
        write = patch("assess_user_grant_revoke.assert_write_model",
                      autospec=True)
        admin = patch("assess_user_grant_revoke.assert_admin_model",
                      autospec=True)
        with cpass as pass_mock, able as able_mock, log as log_mock:
            with read as read_mock, write as write_mock, admin as admin_mock:
                with expect as expect_mock:
                    with patch("jujupy.ModelClient._end_pexpect_session",
                               autospec=True):
                        expect_mock.return_value.isalive.return_value = False
                        assess_user_grant_revoke(fake_client)

                        self.assertEqual(pass_mock.call_count, 3)
                        self.assertEqual(able_mock.call_count, 3)
                        self.assertEqual(log_mock.call_count, 3)

                        self.assertEqual(read_mock.call_count, 6)
                        self.assertEqual(write_mock.call_count, 6)
                        self.assertEqual(admin_mock.call_count, 6)

                        read_calls = [
                            call[0][2] for call in
                            read_mock.call_args_list]
                        write_calls = [
                            call[0][2] for call in
                            write_mock.call_args_list]
                        admin_calls = [
                            call[0][3] for call in
                            admin_mock.call_args_list]

                        self.assertEqual(read_calls,
                                         assert_read_calls)
                        self.assertEqual(write_calls,
                                         assert_write_calls)
                        self.assertEqual(admin_calls,
                                         assert_admin_calls)
