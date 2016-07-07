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

import pexpect

from assess_user_grant_revoke import (
    assert_read,
    assert_user_permissions,
    assert_write,
    assess_user_grant_revoke,
    create_cloned_environment,
    JujuAssertionError,
    main,
    parse_args,
    register_user,
)
from jujupy import JUJU_DEV_FEATURE_FLAGS
from tests import (
    parse_error,
    TestCase,
)
from tests.test_jujupy import (
    fake_juju_client,
    FakeBackend,
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
        env = object()
        client = Mock(spec=["is_jes_enabled"])
        with patch("assess_user_grant_revoke.BootstrapManager.booted_context",
                   autospec=True) as mock_bc:
            with patch("assess_user_grant_revoke.assess_user_grant_revoke",
                       autospec=True) as mock_assess:
                with patch("assess_user_grant_revoke.configure_logging",
                           autospec=True) as mock_cl:
                        with patch("jujupy.SimpleEnvironment.from_config",
                                   return_value=env) as mock_e:
                            with patch("jujupy.EnvJujuClient.by_version",
                                       return_value=client) as mock_c:
                                main(argv)
        mock_cl.assert_called_once_with(logging.DEBUG)
        mock_e.assert_called_once_with("an-env")
        mock_c.assert_called_once_with(env, "/bin/juju", debug=False)
        self.assertEqual(mock_bc.call_count, 1)
        mock_assess.assert_called_once_with(client)


class TestAsserts(TestCase):

    def test_assert_user_permissions(self):
        user = namedtuple('user', ['name', 'permissions', 'expect'])
        read_user = user('readuser', 'read', [True, False, False, False])
        write_user = user('adminuser', 'write', [True, True, True, False])
        users = [read_user, write_user]

        for user in users:
            fake_client = fake_juju_client()
            fake_admin_client = fake_juju_client()
            with patch("jujupy.EnvJujuClient.revoke", return_value=True):
                with patch("assess_user_grant_revoke.assert_read",
                           return_value=True) as read_mock:
                    with patch("assess_user_grant_revoke.assert_write",
                               return_value=True) as write_mock:
                        assert_user_permissions(user, fake_client,
                                                fake_admin_client)
                        self.assertEqual(read_mock.call_count, 2)
                        self.assertEqual(write_mock.call_count, 2)

    def test_assert_read(self):
        fake_client = fake_juju_client()
        with patch.object(fake_client, 'show_status', return_value=True):
            assert_read(fake_client, True)
            with self.assertRaises(JujuAssertionError):
                assert_read(fake_client, False)
        with patch.object(fake_client, 'show_status', return_value=False,
                          side_effect=CalledProcessError(None, None, None)):
            assert_read(fake_client, False)
            with self.assertRaises(JujuAssertionError):
                assert_read(fake_client, True)

    def test_assert_write(self):
        fake_client = fake_juju_client()
        with patch.object(fake_client, 'deploy', return_value=True):
            assert_write(fake_client, True)
            with self.assertRaises(JujuAssertionError):
                assert_write(fake_client, False)
        with patch.object(fake_client, 'deploy', return_value=False,
                          side_effect=CalledProcessError(None, None, None)):
            assert_write(fake_client, False)
            with self.assertRaises(JujuAssertionError):
                assert_write(fake_client, True)


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
        fake_client._backend.controller_state.users['admin'] = {
            'state': '',
            'permission': 'write'
        }
        fake_client._backend.controller_state.name = 'admin'
        fake_client._backend.controller_state.shares = ['admin']
        fake_client.bootstrap()
        assert_read_calls = [True, False, True, True]
        assert_write_calls = [False, False, True, False]
        with patch("assess_user_grant_revoke.assess_change_password",
                   autospec=True) as pass_mock:
            with patch("assess_user_grant_revoke.assess_disable_enable",
                       autospec=True) as able_mock:
                with patch("assess_user_grant_revoke.assess_logout_login",
                           autospec=True) as log_mock:
                    with patch("jujupy.EnvJujuClient.expect", autospec=True):
                        with patch("assess_user_grant_revoke.assert_read",
                                   autospec=True) as read_mock:
                            with patch("assess_user_grant_revoke.assert_write",
                                       autospec=True) as write_mock:
                                assess_user_grant_revoke(fake_client)

                                self.assertEqual(pass_mock.call_count, 2)
                                self.assertEqual(able_mock.call_count, 1)
                                self.assertEqual(log_mock.call_count, 1)

                                self.assertEqual(read_mock.call_count, 4)
                                self.assertEqual(write_mock.call_count, 4)

                                read_calls = [call[0][1] for call in
                                              read_mock.call_args_list]
                                write_calls = [call[0][1] for call in
                                               write_mock.call_args_list]

                                self.assertEqual(read_calls,
                                                 assert_read_calls)
                                self.assertEqual(write_calls,
                                                 assert_write_calls)

    def test_create_cloned_environment(self):
        fake_client = make_fake_client()
        fake_client.bootstrap()
        fake_client_environ = fake_client._shell_environ()
        controller_name = 'user_controller'
        cloned, cloned_environ = create_cloned_environment(
            fake_client,
            'fakehome',
            controller_name
        )
        self.assertIs(fake_client.__class__, type(cloned))
        self.assertEqual(cloned.env.juju_home, 'fakehome')
        self.assertNotEqual(cloned_environ, fake_client_environ)
        self.assertEqual(cloned_environ['JUJU_DATA'], 'fakehome')
        self.assertEqual(cloned.env.controller.name, controller_name)
        self.assertEqual(fake_client.env.controller.name, 'name')

    def test_register_user(self):
        FakeUser = namedtuple('user', ['name', 'permissions'])
        user = FakeUser('fakeuser', 'read')

        class FakeClient:
            """Lightweight fake client for testing."""
            def add_user(self, username, permissions):
                return 'token'

            def expect(self, *args, **kwargs):
                return PexpectInteraction(
                    [
                        user.name + '_controller',
                        user.name + '_password',
                        user.name + '_password',
                        pexpect.EOF],
                    [
                        '(?i)name .*: ',
                        '(?i)password',
                        '(?i)password',
                        pexpect.EOF])

        fake_client = FakeClient()
        with patch(
                'assess_user_grant_revoke.create_cloned_environment',
                return_value=(fake_client, {})):
            register_user(user, fake_client, '/tmp/dir/path')
