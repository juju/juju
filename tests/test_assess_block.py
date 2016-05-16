"""Tests for assess_block module."""

import logging
import StringIO

from mock import Mock, patch, call
import yaml

from assess_block import (
    assess_block,
    make_block_list,
    main,
    parse_args,
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
        env = object()
        client = Mock(spec=["is_jes_enabled"])
        with patch("assess_block.configure_logging",
                   autospec=True) as mock_cl:
            with patch("assess_block.BootstrapManager.booted_context",
                       autospec=True) as mock_bc:
                with patch("jujupy.SimpleEnvironment.from_config",
                           return_value=env) as mock_e:
                    with patch("jujupy.EnvJujuClient.by_version",
                               return_value=client) as mock_c:
                        with patch("assess_block.assess_block",
                                   autospec=True) as mock_assess:
                            main(argv)
        mock_cl.assert_called_once_with(logging.DEBUG)
        mock_e.assert_called_once_with("an-env")
        mock_c.assert_called_once_with(env, "/bin/juju", debug=False)
        self.assertEqual(mock_bc.call_count, 1)
        mock_assess.assert_called_once_with(client, 'trusty')


class TestAssess(TestCase):

    def test_block(self):
        mock_client = Mock(spec=[
            "juju", "wait_for_started", "get_juju_output",
            "remove_service", "env", "deploy", "expose",
            "destroy-model", "remove-machine", "get_status"])
        mock_client.destroy_model_command = 'destroy-model'
        mock_client.get_juju_output.side_effect = [
            yaml.dump(make_block_list(mock_client, False, False, False)),
            yaml.dump(make_block_list(mock_client, True, False, False)),
            yaml.dump(make_block_list(mock_client, False, False, False)),
            yaml.dump(make_block_list(mock_client, False, True, False)),
            yaml.dump(make_block_list(mock_client, False, False, False)),
            yaml.dump(make_block_list(mock_client, False, False, True)),
            yaml.dump(make_block_list(mock_client, False, False, False))
            ]
        mock_client.env.environment = 'foo'
        mock_client.version = '1.25'
        with patch('assess_block.deploy_dummy_stack', autospec=True):
            with patch('assess_block.wait_for_removed_services',
                       autospec=True):
                assess_block(mock_client, 'trusty')
        mock_client.wait_for_started.assert_called_with()
        self.assertEqual([call('block destroy-model', ()),
                          call('destroy-model',
                               ('-y', 'foo'), include_e=False),
                          call('unblock destroy-model', ()),
                          call('block remove-object', ()),
                          call('destroy-model',
                               ('-y', 'foo'), include_e=False),
                          call('remove-service',
                               ('dummy-source',), include_e=True),
                          call('remove-unit',
                               ('dummy-source/1',), include_e=True),
                          call('remove-relation',
                               ('dummy-source', 'dummy-sink'), include_e=True),
                          call('unblock remove-object', ()),
                          call('block all-changes', ()),
                          call('destroy-model',
                               ('-y', 'foo'), include_e=False),
                          call('unblock all-changes', ())],
                         mock_client.juju.mock_calls)
