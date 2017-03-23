"""Tests for assess_sla module."""

import logging
import StringIO

from mock import (
    Mock,
    patch,
)

from assess_sla import (
    assert_sla_state,
    assess_sla,
    parse_args,
    main,
)
from jujupy import (
    fake_juju_client,
)
from tests import (
    parse_error,
    TestCase,
)
from utility import (
    JujuAssertionError,
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


class TestMain(TestCase):

    def test_main(self):
        argv = ["an-env", "/bin/juju", "/tmp/logs", "an-env-mod", "--verbose"]
        client = Mock(spec=["is_jes_enabled"])
        with patch("assess_sla.configure_logging",
                   autospec=True) as mock_cl:
            with patch("assess_sla.BootstrapManager.booted_context",
                       autospec=True) as mock_bc:
                with patch('deploy_stack.client_from_config',
                           return_value=client) as mock_cfc:
                    with patch("assess_sla.assess_sla",
                               autospec=True) as mock_assess:
                        main(argv)
        mock_cl.assert_called_once_with(logging.DEBUG)
        mock_cfc.assert_called_once_with('an-env', "/bin/juju", debug=False,
                                         soft_deadline=None)
        self.assertEqual(mock_bc.call_count, 1)
        mock_assess.assert_called_once_with(client)


class TestAssess(TestCase):

    def test_assess_sla(self):
        fake_client = Mock(wraps=fake_juju_client())
        fake_client.bootstrap()
        deploy_charm = 'dummy-source'
        with patch("assess_sla.assert_sla_state",
                   autospec=True) as sla_mock:
            assess_sla(fake_client)
            fake_client.deploy.assert_called_once_with(
                charm=deploy_charm)
            fake_client.wait_for_started.assert_called_once_with()
            self.assertEqual(
                1,
                fake_client.get_status().get_service_unit_count(deploy_charm))
            self.assertEqual(sla_mock.call_count, 1)

    def test_assert_matching_sla_state(self):
        fake_sla_state = 'unsupported'
        with patch("assess_sla.list_sla", return_value=fake_sla_state):
            assert_sla_state(fake_sla_state, fake_sla_state)

    def test_assert_sla_state_raises_mismatched_state(self):
        fake_sla_state = 'unsupported'
        with patch("assess_sla.list_sla", return_value=fake_sla_state):
            with self.assertRaises(JujuAssertionError) as ex:
                assert_sla_state(fake_sla_state, 'foo')
                self.assertEqual((ex.exception.message,
                                 "Unexpected State found foo, "
                                  "Expected {}".format(fake_sla_state)))
