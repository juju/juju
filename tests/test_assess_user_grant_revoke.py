"""Tests for assess_user_grant_revoke module."""

import logging
from mock import (
    Mock,
    patch,
)
from functools import partial
import StringIO
import pexpect

from assess_user_grant_revoke import (
    assess_user_grant_revoke,
    parse_args,
    create_cloned_environment,
    register_user,
    create_user_permissions,
    remove_user_permissions,
    _get_register_command,
    main,
)
from tests import (
    parse_error,
    TestCase,
)
from tests.test_jujupy import (
    FakeJujuClient,
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
            self._expect_strings = expected_str
            self._sendline_strings = sendline_str

        def _check_string(self, string, string_list):
            expected_string = string_list.pop(0)
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

    def test_user_grant_revoke(self):
        mock_client = FakeJujuClient()
        mock_client.bootstrap()
        assess_user_grant_revoke(mock_client)

        # mock_client.juju.assert_called_once_with('deploy',
        # ('local:wordpress',))
        mock_client.wait_for_started.assert_called_once_with()
        mock_client.juju.assert_called_once_with('deploy', ('wordpress',))
        mock_client.juju.assert_called_once_with('deploy', ('wordpress',))

        #read_user_client.list_controllers()
        #read_user_client.show_user()
        #read_user_client.show_status()
        mock_client.get_juju_output.assert_called_once_with('add-user', ('bob'))
        mock_client.get_juju_output.assert_called_once_with('add-user', ('carol'))
        mock_client.juju.assert_called_once_with('revoke', ('bob',))
        mock_client.juju.assert_called_once_with('revoke', ('carol',))

        # self.assertNotIn("TODO", self.log_stream.getvalue())

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

    def test_remove_user_permissions(self):
        mock_client = FakeJujuClient()
        username = 'fakeuser'
        model = 'foo'

        remove_user_permissions(mock_client, username)
        self.assertIn(
            "'juju', '--show-log', 'revoke', 'fakeuser'," +
            " 'name', '--acl', 'read'",
            self.log_stream.getvalue())

        remove_user_permissions(mock_client, username, model)
        self.assertIn(
            "'juju', '--show-log', 'revoke', 'fakeuser'," +
            " 'foo', '--acl', 'read'",
            self.log_stream.getvalue())

        remove_user_permissions(
            mock_client, username, model, permissions='write')
        self.assertIn(
            "'juju', '--show-log', 'revoke', 'fakeuser'," +
            " 'foo', '--acl', 'write'",
            self.log_stream.getvalue())

        remove_user_permissions(
            mock_client, username, model, permissions='read')
        self.assertIn(
            "'juju', '--show-log', 'revoke', 'fakeuser'," +
            " 'foo', '--acl', 'read'",
            self.log_stream.getvalue())

    def test_create_user_permissions(self):
        mock_client = FakeJujuClient()
        username = 'fakeuser'
        model = 'foo'

        create_user_permissions(mock_client, username)
        self.assertIn(
            "'juju', '--show-log', 'add-user', 'fakeuser'," +
            " '--models', 'name', '--acl', 'read'",
            self.log_stream.getvalue())

        create_user_permissions(mock_client, username, model)
        self.assertIn(
            "'juju', '--show-log', 'add-user', 'fakeuser'," +
            " '--models', 'foo', '--acl', 'read'",
            self.log_stream.getvalue())

        create_user_permissions(
            mock_client, username, model, permissions='write')
        self.assertIn(
            "'juju', '--show-log', 'add-user', 'fakeuser'," +
            " '--models', 'foo', '--acl', 'write'",
            self.log_stream.getvalue())

        create_user_permissions(
            mock_client, username, model, permissions='read')
        self.assertIn(
            "'juju', '--show-log', 'add-user', 'fakeuser'," +
            " '--models', 'foo', '--acl', 'read'",
            self.log_stream.getvalue())

    def test__get_register_command(self):
        output = ''.join(['User "x" added\nUser "x" granted read access ',
                          'to model "y"\nPlease send this command to x:\n',
                          '    juju register AaBbCc'])
        output_cmd = 'juju register AaBbCc'
        register_cmd = _get_register_command(output)
        self.assertEqual(register_cmd, output_cmd)
