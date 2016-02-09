"""Tests for assess_update_mongo module."""

import logging
from mock import Mock, patch
import StringIO

from assess_update_mongo import (
    assess_update_mongo,
    DEP_SCRIPT,
    parse_args,
    main,
)
from test_jujupy import FakePopen
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
        argv = ["an-env", "/bin/juju", "/tmp/logs", "an-env-mod", "--verbose",
                "--series", "trusty"]
        env = object()
        client = Mock(spec=["is_jes_enabled"])
        with patch("assess_update_mongo.configure_logging",
                   autospec=True) as mock_cl:
            with patch("assess_update_mongo.BootstrapManager.booted_context",
                       autospec=True) as mock_bc:
                with patch("jujupy.SimpleEnvironment.from_config",
                           return_value=env) as mock_e:
                    with patch("jujupy.EnvJujuClient.by_version",
                               return_value=client) as mock_c:
                        with patch("assess_update_mongo.assess_update_mongo",
                                   autospec=True) as mock_assess:
                            main(argv)
        mock_cl.assert_called_once_with(logging.DEBUG)
        mock_e.assert_called_once_with("an-env")
        mock_c.assert_called_once_with(env, "/bin/juju", debug=False)
        self.assertEqual(mock_bc.call_count, 1)
        # This should verify bs_manager.bootstrap_host was passed, not None.
        mock_assess.assert_called_once_with(client, 'trusty', None)


class TestAssess(TestCase):

    def test_update_mongo(self):
        mock_client = Mock(
            spec=["juju", "wait_for_started", "deploy", "upgrade_mongo"])
        mock_remote = Mock(spec=['run'])
        with patch('assess_update_mongo.remote_from_address',
                   autospec=True, return_value=mock_remote) as r_mock:
            assess_update_mongo(mock_client, 'trusty', '10.0.0.2')
        mock_client.deploy.assert_called_once_with('local:trusty/ubuntu')
        mock_client.wait_for_started.assert_called_once_with()
        mock_client.upgrade_mongo.assert_called_once_with()
        r_mock.assert_called_once_with('10.0.0.2', series='trusty')
        mock_remote.run.assert_called_once_with(DEP_SCRIPT)
        self.assertNotIn("TODO", self.log_stream.getvalue())
