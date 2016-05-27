"""Tests for assess_storage module."""

import logging
import StringIO

from mock import Mock, patch, call
import yaml

from assess_storage import (
    assess_storage,
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
        with patch("assess_storage.configure_logging",
                   autospec=True) as mock_cl:
            with patch("assess_storage.BootstrapManager.booted_context",
                       autospec=True) as mock_bc:
                with patch("jujupy.SimpleEnvironment.from_config",
                           return_value=env) as mock_e:
                    with patch("jujupy.EnvJujuClient.by_version",
                               return_value=client) as mock_c:
                        with patch("assess_storage.assess_storage",
                                   autospec=True) as mock_assess:
                            main(argv)
        mock_cl.assert_called_once_with(logging.DEBUG)
        mock_e.assert_called_once_with("an-env")
        mock_c.assert_called_once_with(env, "/bin/juju", debug=False)
        self.assertEqual(mock_bc.call_count, 1)
        mock_assess.assert_called_once_with(client)


class TestAssess(TestCase):

    def test_storage(self):
        mock_client = Mock(spec=["juju", "wait_for_started", "deploy", "get_juju_output"])
        assess_storage(mock_client)
        self.assertEqual([call('create-storage-pool', ('loopy', 'loop', 'size=1G')),
                          call('create-storage-pool', ('rooty', 'rootfs', 'size=1G')),
                          call('create-storage-pool', ('tempy', 'tmpfs', 'size=1G')),
                          call('create-storage-pool', ('ebsy', 'ebs', 'size=1G'))],
                         mock_client.juju.mock_calls)
