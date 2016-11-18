"""Tests for assess_block module."""

import logging
import StringIO

from mock import Mock, patch, call

from assess_block import (
    assess_block,
    make_block_list,
    main,
    parse_args,
    )
from jujupy import (
    EnvJujuClient,
    EnvJujuClient1X,
    SimpleEnvironment
    )
from tests import (
    parse_error,
    FakeHomeTestCase,
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
        with patch("assess_block.configure_logging",
                   autospec=True) as mock_cl:
            with patch("assess_block.BootstrapManager.booted_context",
                       autospec=True) as mock_bc:
                with patch("deploy_stack.client_from_config",
                           return_value=client) as mock_c:
                    with patch("assess_block.assess_block",
                               autospec=True) as mock_assess:
                        main(argv)
        mock_cl.assert_called_once_with(logging.DEBUG)
        mock_c.assert_called_once_with('an-env', "/bin/juju", debug=False,
                                       soft_deadline=None)
        self.assertEqual(mock_bc.call_count, 1)
        mock_assess.assert_called_once_with(client, 'trusty')


class TestAssess(FakeHomeTestCase):

    def test_block_2x(self):
        mock_client = Mock(spec=[
            "juju", "wait_for_started", "list_disabled_commands",
            "disable_command", "enable_command", "is_juju1x",
            "remove_service", "env", "deploy", "expose",
            "destroy-model", "remove-machine", "get_status"],
            command_set_destroy_model='destroy-model',
            command_set_remove_object='remove-object',
            command_set_all='all',
            destroy_model_command='destroy-model')
        mock_client.is_juju1x.return_value = False
        mock_client.list_disabled_commands.side_effect = [
            make_block_list(mock_client, []),
            make_block_list(
                mock_client, [EnvJujuClient.command_set_destroy_model]),
            make_block_list(mock_client, []),
            make_block_list(
                mock_client, [EnvJujuClient.command_set_remove_object]),
            make_block_list(mock_client, []),
            make_block_list(mock_client, [EnvJujuClient.command_set_all]),
            make_block_list(mock_client, []),
            ]
        mock_client.env.environment = 'foo'
        mock_client.version = '2.0.0'
        with patch('assess_block.deploy_dummy_stack', autospec=True):
            assess_block(mock_client, 'trusty')
        mock_client.wait_for_started.assert_called_with()
        self.assertEqual([call('destroy-model',
                               ('-y', 'foo'), include_e=False),
                          call('destroy-model',
                               ('-y', 'foo'), include_e=False),
                          call('remove-unit',
                               ('dummy-source/1',), include_e=True),
                          call('remove-relation',
                               ('dummy-source', 'dummy-sink'), include_e=True),
                          call('remove-relation',
                               ('dummy-source', 'dummy-sink')),
                          call('add-relation',
                               ('dummy-source', 'dummy-sink'), include_e=True),
                          call('unexpose',
                               ('dummy-sink',), include_e=True),
                          call('unexpose', ('dummy-sink',)),
                          call('expose', ('dummy-sink',), include_e=True),
                          call('destroy-model',
                               ('-y', 'foo'), include_e=False)],
                         mock_client.juju.mock_calls)
        self.assertEqual([call('destroy-model'),
                          call('remove-object'),
                          call('all'),
                          call('all'),
                          call('all')],
                         mock_client.disable_command.mock_calls)
        self.assertEqual([call('destroy-model'),
                          call('remove-object'),
                          call('all'),
                          call('all'),
                          call('all')],
                         mock_client.enable_command.mock_calls)
        self.assertEqual([call('dummy-source'),
                          call('dummy-sink'),
                          call('dummy-source'),
                          call('dummy-sink'),
                          call('dummy-sink')],
                         mock_client.remove_service.mock_calls)

    def test_block_1x(self):
        client = EnvJujuClient1X(
            SimpleEnvironment('foo'), '1.25.0-series-arch', None)
        side_effects = [
            make_block_list(client, []),
            make_block_list(
                client, [EnvJujuClient1X.command_set_destroy_model]),
            make_block_list(client, []),
            make_block_list(
                client, [EnvJujuClient1X.command_set_remove_object]),
            make_block_list(client, []),
            make_block_list(
                client, [EnvJujuClient1X.command_set_all]),
            make_block_list(client, []),
            ]
        with patch.object(client, 'list_disabled_commands',
                          autospec=True, side_effect=side_effects):
            with patch('assess_block.deploy_dummy_stack', autospec=True):
                with patch.object(client, 'remove_service') as mock_rs:
                    with patch.object(client, 'juju') as mock_juju:
                        with patch.object(
                                client, 'wait_for_started') as mock_ws:
                            assess_block(client, 'trusty')
        mock_ws.assert_called_with()
        self.assertEqual(
            [call('block destroy-environment', ('',)),
             call('destroy-environment', ('-y', 'foo'), include_e=False),
             call('unblock', 'destroy-environment'),
             call('block remove-object', ('',)),
             call('destroy-environment', ('-y', 'foo'), include_e=False),
             call('remove-unit', ('dummy-source/1',), include_e=True),
             call('remove-relation', ('dummy-source', 'dummy-sink'),
                  include_e=True),
             call('unblock', 'remove-object'),
             call('remove-relation', ('dummy-source', 'dummy-sink')),
             call('block all-changes', ('',)),
             call('add-relation', ('dummy-source', 'dummy-sink'),
                  include_e=True),
             call('unexpose', ('dummy-sink',), include_e=True),
             call('unblock', 'all-changes'),
             call('unexpose', ('dummy-sink',)),
             call('block all-changes', ('',)),
             call('expose', ('dummy-sink',), include_e=True),
             call('unblock', 'all-changes'),
             call('block all-changes', ('',)),
             call('deploy', (('dummy-sink',),)),
             call('destroy-environment', ('-y', 'foo'), include_e=False),
             call('unblock', 'all-changes')],
            mock_juju.mock_calls)
        self.assertEqual([call('dummy-source'),
                          call('dummy-sink'),
                          call('dummy-source'),
                          call('dummy-sink'),
                          call('dummy-sink')],
                         mock_rs.mock_calls)
