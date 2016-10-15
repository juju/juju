"""Tests for assess_proxy module."""

import logging
from mock import Mock, patch
import StringIO

from assess_proxy import (
    assess_proxy,
    parse_args,
    main,
)
from tests import (
    parse_error,
    TestCase,
)
from tests.test_jujupy import fake_juju_client
from utility import temp_dir


class TestParseArgs(TestCase):

    def test_common_args(self):
        with temp_dir() as log_dir:
            args = parse_args(["an-env", "/bin/juju", log_dir, "an-env-mod",
                               'both-proxied'])
        self.assertEqual("an-env", args.env)
        self.assertEqual("/bin/juju", args.juju_bin)
        self.assertEqual(log_dir, args.logs)
        self.assertEqual("an-env-mod", args.temp_env_name)
        self.assertEqual("both-proxied", args.scenario)
        self.assertIsFalse(args.debug)

    def test_help(self):
        fake_stdout = StringIO.StringIO()
        with parse_error(self) as fake_stderr:
            with patch("sys.stdout", fake_stdout):
                parse_args(["--help"])
        self.assertEqual("", fake_stderr.getvalue())
        self.assertNotIn("TODO", fake_stdout.getvalue())


class TestMain(TestCase):

    def test_main(self):
        with temp_dir() as log_dir:
            argv = ["an-env", "/bin/juju", log_dir, "an-env-mod",
                    "both-proxied", "--verbose"]
            client = Mock(spec=["is_jes_enabled"])
            with patch("assess_proxy.configure_logging",
                       autospec=True) as mock_cl:
                with patch("assess_proxy.BootstrapManager.booted_context",
                           autospec=True) as mock_bc:
                    with patch('deploy_stack.client_from_config',
                               return_value=client) as mock_c:
                        with patch("assess_proxy.assess_proxy",
                                   autospec=True) as mock_assess:
                            with patch("assess_proxy.set_firewall",
                                       autospec=True) as mock_set:
                                with patch("assess_proxy.reset_firewall",
                                           autospec=True) as mock_reset:
                                    main(argv)
        mock_cl.assert_called_once_with(logging.DEBUG)
        mock_c.assert_called_once_with(
            'an-env', "/bin/juju", debug=False, soft_deadline=None)
        mock_set.assert_called_once_with('both-proxied')
        mock_reset.assert_called_once_with()
        self.assertEqual(mock_bc.call_count, 1)
        mock_assess.assert_called_once_with(client, 'both-proxied')


class TestAssess(TestCase):

    def test_proxy(self):
        # Using fake_client means that deploy and get_status have plausible
        # results.  Wrapping it in a Mock causes every call to be recorded, and
        # allows assertions to be made about calls.  Mocks and the fake client
        # can also be used separately.
        fake_client = Mock(wraps=fake_juju_client())
        fake_client.bootstrap()
        assess_proxy(fake_client, 'both-proxied')
        fake_client.deploy.assert_called_once_with('cs:xenial/ubuntu')
        fake_client.wait_for_started.assert_called_once_with()
        fake_client.wait_for_workloads.assert_called_once_with()
        self.assertEqual(
            1, fake_client.get_status().get_service_unit_count('ubuntu'))
        self.assertNotIn("TODO", self.log_stream.getvalue())
