"""Tests for assess_user_grant_revoke module."""

import logging
from functools import partial
import StringIO


from mock import (
    Mock,
    patch,
)
import pexpect

from assess_user_grant_revoke import (
    assert_read,
    assert_write,
    assess_user_grant_revoke,
    create_cloned_environment,
    main,
    parse_args,
    register_user,
)
from tests import (
    parse_error,
    TestCase,
)
from tests.test_jujupy import (
    FakeJujuClient,
    FakeBackend,
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
            with patch(
                "assess_user_grant_revoke.BootstrapManager.booted_context",
                       autospec=True) as mock_bc:
                with patch("jujupy.SimpleEnvironment.from_config",
                           return_value=env) as mock_e:
                    with patch("jujupy.EnvJujuClient.by_version",
                               return_value=client) as mock_c:
                        with patch(
                            "assess_user_grant_revoke.assess_user_grant_revoke",
                             autospec=True) as mock_assess:
                            main(argv)
        mock_cl.assert_called_once_with(logging.DEBUG)
        mock_e.assert_called_once_with("an-env")
        mock_c.assert_called_once_with(env, "/bin/juju", debug=False)
        self.assertEqual(mock_bc.call_count, 1)
        mock_assess.assert_called_once_with(client)

class TestAssess(TestCase):

    class RegisterUserProcess:

        @classmethod
        def get_process(cls, username, expected_strings):
            return partial(cls, username, expected_strings)

        def __init__(self, sendline_str, expected_str, register_cmd, env):
            self._expect_strings = iter(expected_str)
            self._sendline_strings = iter(sendline_str)

        def _check_string(self, string, string_list):
            if string != next(string_list):
                raise ValueError(
                    'Expected {} got {}'.format(expected_string, string)
                )

        def expect(self, string):
            self._check_string(string, self._expect_strings)

        def sendline(self, string):
            self._check_string(string, self._sendline_strings)

        def isalive(self):
            return False


    def test_user_grant_revoke(self):
        mock_client = FakeJujuClient()
        mock_client.bootstrap()

        with patch("assess_user_grant_revoke.register_user", return_value=True):
            assess_user_grant_revoke(mock_client)
            mock_client.wait_for_started.assert_called_once_with()
            #mock_client.create_user_permissions.assert_called_with()
            #mock_client.create_user_permissions.assert_called_with()
            #mock_client.create_cloned_environment.assert_called_with()
            #mock_client.create_cloned_environment.assert_called_with()
            #mock_client.register_user.assert_called_with()
            #mock_client.register_user.assert_called_with()

    def test_create_cloned_environment(self):
        mock_client = FakeJujuClient()
        mock_client.bootstrap()
        mock_client_env = mock_client._shell_environ()
        cloned, cloned_env = create_cloned_environment(mock_client, 'fakehome')
        self.assertIs(FakeJujuClient, type(cloned))
        self.assertEqual(cloned.env.juju_home, 'fakehome')
        self.assertNotEqual(cloned_env, mock_client_env)
        self.assertEqual(cloned_env['JUJU_DATA'], 'fakehome')

    def test_register_user(self):
        username = 'fakeuser'
        mock_client = FakeJujuClient()
        env = mock_client._shell_environ()
        cmd = 'juju register AaBbCc'

        register_process = TestAssess.RegisterUserProcess.get_process(
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

        register_user(username, env, cmd, register_process=register_process)