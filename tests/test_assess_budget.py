"""Tests for assess_budget module."""

import logging
import StringIO

from mock import (
    Mock,
    patch,
    )

from assess_budget import (
    assess_budget,
    parse_args,
    main,
    )
from fakejuju import fake_juju_client
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
        # env = object()
        client = Mock(spec=["is_jes_enabled"])
        with patch("assess_budget.configure_logging",
                   autospec=True) as mock_cl:
            with patch("assess_budget.BootstrapManager.booted_context",
                       autospec=True) as mock_bc:
                with patch('deploy_stack.client_from_config',
                           return_value=client) as mock_cfc:
                    with patch("assess_budget.assess_budget",
                               autospec=True) as mock_assess:
                        main(argv)
        mock_cl.assert_called_once_with(logging.DEBUG)
        mock_cfc.assert_called_once_with('an-env', "/bin/juju", debug=False,
                                         soft_deadline=None)
        self.assertEqual(mock_bc.call_count, 1)
        mock_assess.assert_called_once_with(client)


class TestAssess(TestCase):

    def test_budget(self):
        # Using fake_client means that deploy and get_status have plausible
        # results.  Wrapping it in a Mock causes every call to be recorded, and
        # allows assertions to be made about calls.  Mocks and the fake client
        # can also be used separately.
        fake_client = Mock(wraps=fake_juju_client())
        fake_client.bootstrap()
        assess_budget(fake_client)
        fake_client.deploy.assert_called_once_with(
            'local:trusty/my-charm')
        fake_client.wait_for_started.assert_called_once_with()
        self.assertEqual(
            1, fake_client.get_status().get_service_unit_count('my-charm'))
        self.assertNotIn("TODO", self.log_stream.getvalue())
