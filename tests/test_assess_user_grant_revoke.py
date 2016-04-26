"""Tests for assess_user_grant_revoke module."""

import logging
from mock import (
    Mock,
    patch,
)
import StringIO

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

    def test_user_grant_revoke(self):
        mock_client = FakeJujuClient()

        mock_client.bootstrap()
        assess_user_grant_revoke(mock_client)
        # mock_client.juju.assert_called_once_with('deploy',
        # ('local:wordpress',))
        mock_client.wait_for_started.assert_called_once_with()
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

    # This is a fragile pexpect session and returns nothing
    def test_register_user(self):
        username = 'fakeuser'
        mock_client = FakeJujuClient()
        env = mock_client._shell_environ()
        cmd = 'register AaBbCc'

        register_user(username, env, cmd)

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
        output_cmd = 'register AaBbCc'
        register_cmd = _get_register_command(output)
        self.assertEqual(register_cmd, output_cmd)
