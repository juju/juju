"""Tests for assess_user_grant_revoke module."""

from collections import namedtuple
from functools import partial
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
    FakeJujuClient,
    FakeBackend,
)


class FakeBackendShellEnv(FakeBackend):

    def shell_environ(self, used_feature_flags, juju_home):
        env = dict(os.environ)
        if self.full_path is not None:
            env['PATH'] = '{}{}{}'.format(os.path.dirname(self.full_path),
                                          os.pathsep, env['PATH'])
        flags = self._feature_flags.intersection(used_feature_flags)
        if flags:
            env[JUJU_DEV_FEATURE_FLAGS] = ','.join(sorted(flags))
        env['JUJU_DATA'] = juju_home
        return env


class RegisterUserProcess:

    @classmethod
    def get_process(cls, username, expected_strings):
        return partial(cls, username, expected_strings)

    def __init__(self, sendline_str, expected_str, register_cmd):
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
            fake_client = FakeJujuClient()
            fake_admin_client = FakeJujuClient()
            with patch("test_jujupy.FakeJujuClient.revoke", return_value=True):
                with patch("assess_user_grant_revoke.assert_read",
                           return_value=True) as read_mock:
                    with patch("assess_user_grant_revoke.assert_write",
                               return_value=True) as write_mock:
                        assert_user_permissions(user, fake_client,
                                                fake_admin_client)
                        self.assertEqual(read_mock.call_count, 2)
                        self.assertEqual(write_mock.call_count, 2)

    def test_assert_read(self):
        fake_client = FakeJujuClient()
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
        fake_client = FakeJujuClient()
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
    fake_client = FakeJujuClient()
    old_backend = fake_client._backend
    fake_client._backend = FakeBackendShellEnv(
        old_backend.controller_state, old_backend._feature_flags,
        old_backend.version, old_backend.full_path, old_backend.debug)
    return fake_client


class TestAssess(TestCase):

    def test_user_grant_revoke(self):
        fake_client = make_fake_client()
        fake_client.bootstrap()

        user = namedtuple('user', ['name', 'permissions', 'expect'])
        read_user = user('readuser', 'read', [True, False, False, False])
        write_user = user('adminuser', 'write', [True, True, True, False])

        with patch("assess_user_grant_revoke.register_user",
                   return_value=True) as reg_mock:
            with patch("assess_user_grant_revoke.assert_user_permissions",
                       autospec=True) as perm_mock:
                assess_user_grant_revoke(fake_client)

                self.assertEqual(reg_mock.call_count, 2)
                self.assertEqual(perm_mock.call_count, 2)

                read_user_call, write_user_call = perm_mock.call_args_list
                read_user_args, read_user_kwargs = read_user_call
                write_user_args, write_user_kwargs = write_user_call

                self.assertEqual(read_user_args[0], read_user)
                self.assertEqual(read_user_args[2], fake_client)
                self.assertEqual(write_user_args[0], write_user)
                self.assertEqual(write_user_args[2], fake_client)

    def test_create_cloned_environment(self):
        fake_client = make_fake_client()
        fake_client.bootstrap()
        fake_client_environ = fake_client._shell_environ()
        cloned, cloned_environ = create_cloned_environment(fake_client,
                                                           'fakehome')
        self.assertIs(FakeJujuClient, type(cloned))
        self.assertEqual(cloned.env.juju_home, 'fakehome')
        self.assertNotEqual(cloned_environ, fake_client_environ)
        self.assertEqual(cloned_environ['JUJU_DATA'], 'fakehome')

    def test_register_user(self):
        username = 'fakeuser'
        fake_client = make_fake_client()
        environ = fake_client._shell_environ()
        cmd = 'juju register AaBbCc'

        register_process = RegisterUserProcess.get_process(
            [
                username + '_controller',
                username + '_password',
                username + '_password',
                pexpect.exceptions.EOF
            ],
            [
                '(?i)name .*: ',
                '(?i)password',
                '(?i)password',
                pexpect.exceptions.EOF,
            ]
        )

        register_user(username, environ, cmd,
                      register_process=register_process)
