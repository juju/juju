"""Tests for assess_unregister module."""

import logging
from mock import (
    Mock,
    call,
    patch,
    )
import StringIO
import subprocess
from textwrap import dedent

import assess_unregister as a_unreg
from jujupy import fake_juju_client
from tests import (
    parse_error,
    TestCase,
    )
from utility import JujuAssertionError


class TestParseArgs(TestCase):

    def test_common_args(self):
        args = a_unreg.parse_args(
            ["an-env", "/bin/juju", "/tmp/logs", "an-env-mod"])
        self.assertEqual("an-env", args.env)
        self.assertEqual("/bin/juju", args.juju_bin)
        self.assertEqual("/tmp/logs", args.logs)
        self.assertEqual("an-env-mod", args.temp_env_name)
        self.assertEqual(False, args.debug)

    def test_help(self):
        fake_stdout = StringIO.StringIO()
        with parse_error(self) as fake_stderr:
            with patch("sys.stdout", fake_stdout):
                a_unreg.parse_args(["--help"])
        self.assertEqual("", fake_stderr.getvalue())
        self.assertIn("Test unregister feature", fake_stdout.getvalue())


class TestMain(TestCase):

    def test_main(self):
        argv = ["an-env", "/bin/juju", "/tmp/logs", "an-env-mod", "--verbose"]
        client = Mock(spec=["is_jes_enabled"])
        with patch("assess_unregister.configure_logging",
                   autospec=True) as mock_cl:
            with patch("assess_unregister.BootstrapManager.booted_context",
                       autospec=True) as mock_bc:
                with patch("deploy_stack.client_from_config",
                           return_value=client) as mock_c:
                    with patch("assess_unregister.assess_unregister",
                               autospec=True) as mock_assess:
                        a_unreg.main(argv)
        mock_cl.assert_called_once_with(logging.DEBUG)
        mock_c.assert_called_once_with('an-env', "/bin/juju", debug=False,
                                       soft_deadline=None)
        self.assertEqual(mock_bc.call_count, 1)
        mock_assess.assert_called_once_with(client)


class TestAssess(TestCase):

    def test_unregister(self):
        fake_user = Mock()
        with patch('jujupy.EnvJujuClient.register_user',
                   return_value=fake_user):
            with patch.object(
                    a_unreg, 'assert_controller_list',
                    autospec=True) as mock_assert_list:
                with patch.object(
                        a_unreg, 'assert_switch_raises_error', autospec=True):
                    fake_client = Mock(wraps=fake_juju_client())
                    fake_client.env.controller.name = 'testing-controller'
                    fake_client.bootstrap()
                    a_unreg.assess_unregister(fake_client)
        self.assertEqual(
            mock_assert_list.mock_calls,
            [
                call(fake_user, ['testuser_controller']),
                call(fake_user, []),
                call(fake_client, ['testing-controller'])
            ])


class TestAssertControllerList(TestCase):

    def test_passes_on_empty_controller_list_expecting_empty(self):
        json_output = '{"controllers":null,"current-controller":""}\n'
        fake_client = Mock(spec=['get_juju_output'])
        fake_client.get_juju_output.return_value = json_output
        a_unreg.assert_controller_list(fake_client, [])

    def test_raises_when_no_controllers_with_expected_list(self):
        json_output = '{"controllers":null,"current-controller":""}\n'
        fake_client = Mock(spec=['get_juju_output'])
        fake_client.get_juju_output.return_value = json_output
        self.assertRaises(
            JujuAssertionError,
            a_unreg.assert_controller_list,
            fake_client,
            ['some_controller'])

    def test_raises_when_list_controllers_expecting_none(self):
        json_output = """\
        {"controllers":
        {"local-temp": {"current-model":"testing-model","user":"admin"}},
        "current-controller":"local-temp"}\n"""

        fake_client = Mock(spec=['get_juju_output'])
        fake_client.get_juju_output.return_value = json_output
        self.assertRaises(
            JujuAssertionError,
            a_unreg.assert_controller_list,
            fake_client,
            [])

    def test_passes_when_expected_and_listed_match(self):
        json_output = """\
        {"controllers":
        {"local-temp": {"current-model":"testing-model","user":"admin"}},
        "current-controller":"local-temp"}\n"""

        fake_client = Mock(spec=['get_juju_output'])
        fake_client.get_juju_output.return_value = json_output
        self.assertRaises(
            JujuAssertionError,
            a_unreg.assert_controller_list,
            fake_client,
            ['testing-model'])

    def test_exception_message_is_correct(self):
        json_output = '{"controllers":null,"current-controller":""}\n'
        fake_client = Mock(spec=['get_juju_output'])
        fake_client.get_juju_output.return_value = json_output
        expected_message = dedent("""\
        Unexpected controller names.
        Expected: ['testing-model']
        Got: []""")

        try:
            a_unreg.assert_controller_list(fake_client, ['testing-model'])
        except JujuAssertionError as e:
            if str(e) != expected_message:
                raise


class TestAssessSwitchRaisesError(TestCase):

    def test_raises_exception_if_switch_doesnt_fail_at_all(self):
        fake_client = Mock(spec=['get_juju_output'])
        self.assertRaises(
            JujuAssertionError,
            a_unreg.assert_switch_raises_error,
            fake_client)

    def test_raises_exception_when_switch_doesnt_fail_as_expected(self):
        fake_client = Mock(spec=['get_juju_output'])
        exception = subprocess.CalledProcessError(-1, '')
        exception.stderr = ''
        fake_client.get_juju_output.side_effect = exception
        self.assertRaises(
            JujuAssertionError,
            a_unreg.assert_switch_raises_error,
            fake_client)

    def test_passes_when_switch_errors_as_expected(self):
        fake_client = Mock(spec=['get_juju_output'])
        exception = subprocess.CalledProcessError(-1, '')
        exception.stderr = 'no currently specified model'
        fake_client.get_juju_output.side_effect = exception

        a_unreg.assert_switch_raises_error(fake_client)
