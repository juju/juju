"""Tests for assess_block module."""

import logging
import StringIO

from mock import Mock, patch, call
import yaml

from assess_block import (
    assess_block,
    parse_args,
    main,
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
        mock_assess.assert_called_once_with(client)


class TestAssess(TestCase):

    def test_block(self):
        mock_client = Mock([
            "juju", "wait_for_started", "get_juju_output", "deploy"])
        mock_client.get_juju_output.side_effect = [
            yaml.dump([
                {'block': 'destroy-model', 'enabled': False},
                {'block': 'remove-object', 'enabled': False},
                {'block': 'all-changes', 'enabled': False}]),
            yaml.dump([
                {'block': 'destroy-model', 'enabled': False},
                {'block': 'remove-object', 'enabled': False},
                {'block': 'all-changes', 'enabled': True, 'message': ''}]),
            yaml.dump([
                {'block': 'destroy-model', 'enabled': False},
                {'block': 'remove-object', 'enabled': False},
                {'block': 'all-changes', 'enabled': False}]),
            ]
        assess_block(mock_client)
        mock_client.deploy.assert_called_once_with('mediawiki-single')
        mock_client.wait_for_started.assert_called_once_with()
        self.assertEqual([
            call('expose', ('mediawiki',)),
            call('block all-changes', ()),
            call('unblock all-changes', ())], mock_client.juju.mock_calls)
