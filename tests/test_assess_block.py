"""Tests for assess_block module."""

import logging
import StringIO

from mock import Mock, patch, call
import yaml

from assess_block import (
    make_block_list,
    assess_block,
    parse_args,
    main,
)
from tests import (
    parse_error,
    TestCase,
)
from tests.test_jujupy import FakeJujuClient


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
        mock_assess.assert_called_once_with(client)


class TestAssess(TestCase):

    def test_block(self):
        mock_client = Mock(spec=[
            "juju", "wait_for_started", "get_juju_output", "remove_service",
            "env", "deploy", "expose", "destroy-model", "remove-machine", "get_status"])
        mock_client.get_juju_output.side_effect = [
            yaml.dump(make_block_list(False, False, False)),
            yaml.dump(make_block_list(True, False, False)),
            yaml.dump(make_block_list(False, False, False)),
            yaml.dump(make_block_list(False, True, False)),
            yaml.dump(make_block_list(False, False, False)),
            yaml.dump(make_block_list(False, False, True)),
            yaml.dump(make_block_list(False, False, False))
            ]
        mock_client.env.environment = 'foo'
        mock_client.version = '1.25'
        assess_block(mock_client, 'xenial')
        mock_client.wait_for_started.assert_called_with()
        self.assertEqual([
            call('block destroy-model', ()),
            call('destroy-model', ('-y', 'foo'), include_e=False),
            call('expose', ('mediawiki',)),
            call('remove-service', ('mediawiki',)),
            call('unblock destroy-model', ()),
            call('block remove-object', ()),
            call('destroy-model', ('-y', 'foo'), include_e=False),
            call('expose', ('mediawiki',)),
            call('remove-service', ('mediawiki',)),
            call('remove-unit', ('mediawiki/1',)),
            call('expose', ('mysql',)),
            call('add-relation', ('mediawiki:db', 'mysql:db')),
            call('remove-relation', ('mediawiki:db', 'mysql:db')),
            call('unblock remove-object', ()),
            call('remove-service', ('mediawiki',)),
            call('remove-service', ('mysql',)),
            call('block all-changes', ()),
            call('destroy-model', ('-y', 'foo'), include_e=False),
            call('expose', ('mediawiki',)),
            call('unblock all-changes', ())], mock_client.juju.mock_calls)
        self.assertEqual([call('dummy-source'),
                          call('dummy-sink'),
                          call('dummy-source'),
                          call('dummy-sink'),
                          call('dummy-source'),
                          call('dummy-sink')], mock_client.deploy.mock_calls)
