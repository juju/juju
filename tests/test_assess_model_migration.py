"""Tests for assess_model_migration module."""

import logging
from mock import call, Mock, patch
import os
import StringIO

import assess_model_migration as amm
from deploy_stack import BootstrapManager
from tests import (
    parse_error,
    TestCase,
)
from utility import until_timeout


class TestParseArgs(TestCase):

    def test_common_args(self):
        args = amm.parse_args(
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
                amm.parse_args(["--help"])
        self.assertEqual("", fake_stderr.getvalue())
        self.assertIn(
            "Test model migration feature", fake_stdout.getvalue())


class TestGetBootstrapManagers(TestCase):
    def test_returns_two_bs_managers(self):
        ret_bs = [Mock(), Mock()]
        args = Mock()
        with patch.object(BootstrapManager, 'from_args', side_effect=ret_bs):
            with patch.object(amm, '_new_log_dir'):
                bs1, bs2 = amm.get_bootstrap_managers(args)
                self.assertEqual(bs1, ret_bs[0])
                self.assertEqual(bs2, ret_bs[1])

    def test_gives_second_manager_unique_env(self):
        args = Mock()
        fake_bs1 = Mock()
        fake_bs1.temp_env_name = 'testing-env-name'
        ret_bs = [fake_bs1, Mock()]
        with patch.object(BootstrapManager, 'from_args', side_effect=ret_bs):
            with patch.object(amm, '_new_log_dir'):
                bs1, bs2 = amm.get_bootstrap_managers(args)
                self.assertEqual(bs2.temp_env_name, 'testing-env-name-b')

    def test_creates_unique_log_dirs(self):
        ret_bs = [Mock(), Mock()]
        args = Mock()
        with patch.object(BootstrapManager, 'from_args', side_effect=ret_bs):
            with patch.object(amm, '_new_log_dir') as log_dir:
                bs1, bs2 = amm.get_bootstrap_managers(args)
                self.assertEqual(2, log_dir.call_count)
                log_dir.assert_has_calls(
                    [call(args.logs, 'a'), call(args.logs, 'b')])


class TestNewLogDir(TestCase):

    def test_returns_created_log_path(self):
        log_dir_path = '/some/path'
        post_fix = 'testing'
        expected_path = '/some/path/env-testing'
        with patch.object(os, 'mkdir'):
            log_dir = amm._new_log_dir(log_dir_path, post_fix)
            self.assertEqual(log_dir, expected_path)

    def test_creates_new_log_dir(self):
        log_dir_path = '/some/path'
        post_fix = 'testing'
        expected_path = '/some/path/env-testing'

        with patch.object(os, 'mkdir') as fake_mkdir:
            amm._new_log_dir(log_dir_path, post_fix)
            fake_mkdir.assert_called_once_with(expected_path)


class TestWaitForModel(TestCase):
    # Check that it returns an error if the model never comes up.
    # Pass in a timeout for the model check
    def test_raises_exception_when_timeout_occurs(self):
        with patch.object(until_timeout, 'next', side_effect=StopIteration()):
            with self.assertRaises(AssertionError):
                amm.wait_for_model(Mock(), 'TestModelName')

    def test_returns_when_model_found(self):
        fake_client = Mock()
        fake_client.get_models.return_value = dict(
            models=[
                dict(name='TestModelName')])
        amm.wait_for_model(fake_client, 'TestModelName')

    def test_pauses_between_failed_matches(self):
        fake_client = Mock()
        fake_client.get_models.side_effect = [
            dict(models=[]),    # Failed check
            dict(models=[dict(name='TestModelName')]),  # Successful check
            ]
        with patch.object(amm, 'sleep') as mock_sleep:
            amm.wait_for_model(fake_client, 'TestModelName')
            mock_sleep.assert_called_once_with(1)


class TestMain(TestCase):

    def test_main(self):
        argv = ["an-env", "/bin/juju", "/tmp/logs", "an-env-mod", "--verbose"]
        bs_1 = Mock()
        bs_2 = Mock()
        with patch.object(amm, "configure_logging", autospec=True) as mock_cl:
            with patch.object(amm, "assess_model_migration",
                              autospec=True) as mock_assess:
                with patch.object(amm, "get_bootstrap_managers",
                                  return_value=[bs_1, bs_2]):
                    amm.main(argv)
        mock_cl.assert_called_once_with(logging.DEBUG)
        mock_assess.assert_called_once_with(bs_1, bs_2, False)
