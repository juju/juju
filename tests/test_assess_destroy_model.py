"""Tests for assess_destroy_model module."""

import logging
import StringIO
import yaml
from textwrap import dedent

from mock import (
    Mock,
    call,
    patch,
    )

from assess_destroy_model import (
    assess_destroy_model,
    parse_args,
    main,
    )
from jujupy import (
    fake_juju_client,
    Status
    )
from tests import (
    parse_error,
    TestCase,
    )

status_value = dedent("""\
    model:
        controller: foo
""")
status = Status(yaml.safe_load(status_value), status_value)


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
        with patch("assess_destroy_model.configure_logging",
                   autospec=True) as mock_cl:
            with patch("assess_destroy_model.BootstrapManager.booted_context",
                       autospec=True) as mock_bc:
                with patch('deploy_stack.client_from_config',
                           return_value=client) as mock_cfc:
                    with patch("assess_destroy_model.assess_destroy_model",
                               autospec=True) as mock_assess:
                        main(argv)
        mock_cl.assert_called_once_with(logging.DEBUG)
        mock_cfc.assert_called_once_with('an-env', "/bin/juju", debug=False,
                                         soft_deadline=None)
        self.assertEqual(mock_bc.call_count, 1)
        mock_assess.assert_called_once_with(client)


class TestAssess(TestCase):

    def test_assess_destroy_model(self):
        fake_client = Mock(wraps=fake_juju_client())
        fake_client.bootstrap()
        fake_client.get_status.return_value = status
        assess_destroy_model(fake_client)
        self.assertEqual(
            [call.bootstrap(),
             call.get_status(),
             call.add_model('test'),
             call.destroy_model(model='test'),
             call.get_status()],
            fake_client.mock_calls)
