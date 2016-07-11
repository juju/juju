"""Tests for assess_model_migration module."""

import argparse
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
from utility import (
    temp_dir,
    until_timeout,
)


class TestParseArgs(TestCase):

    def test_default_args(self):
        args = amm.parse_args(
            ["an-env", "/bin/juju", "/tmp/logs", "an-env-mod"])
        self.assertEqual(
            args,
            argparse.Namespace(
                env="an-env",
                juju_bin='/bin/juju',
                logs='/tmp/logs',
                temp_env_name='an-env-mod',
                debug=False,
                agent_stream=None,
                agent_url=None,
                bootstrap_host=None,
                keep_env=False,
                machine=[],
                region=None,
                series=None,
                upload_tools=False,
                verbose=20))

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
        with temp_dir() as log_dir:
            args = argparse.Namespace(logs=log_dir)
            with patch.object(
                    BootstrapManager, 'from_args', side_effect=ret_bs):
                bs1, bs2 = amm.get_bootstrap_managers(args)
                self.assertEqual(bs1, ret_bs[0])
                self.assertEqual(bs2, ret_bs[1])

    def test_gives_second_manager_unique_env(self):
        mock_bs1 = Mock()
        mock_bs1.temp_env_name = 'testing-env-name'
        ret_bs = [mock_bs1, Mock()]
        with temp_dir() as log_dir:
            args = argparse.Namespace(logs=log_dir)
            with patch.object(BootstrapManager, 'from_args',
                              side_effect=ret_bs):
                bs1, bs2 = amm.get_bootstrap_managers(args)
                self.assertEqual(bs2.temp_env_name, 'testing-env-name-b')

    def test_creates_unique_log_dirs(self):
        ret_bs = [Mock(), Mock()]
        args = argparse.Namespace(logs='/some/path')
        with patch.object(BootstrapManager, 'from_args', side_effect=ret_bs):
            with patch.object(amm, '_new_log_dir') as log_dir:
                bs1, bs2 = amm.get_bootstrap_managers(args)
                self.assertEqual(2, log_dir.call_count)
                self.assertEqual(
                    log_dir.mock_calls,
                    [call(args.logs, 'a'), call(args.logs, 'b')])


class TestNewLogDir(TestCase):

    def test_returns_created_log_path(self):
        with temp_dir() as log_dir_path:
            post_fix = 'testing'
            expected_path = '{}/env-testing'.format(log_dir_path)
            log_dir = amm._new_log_dir(log_dir_path, post_fix)
            self.assertEqual(log_dir, expected_path)

    def test_creates_new_log_dir(self):
        with temp_dir() as log_dir_path:
            post_fix = 'testing'
            expected_path = '{}/env-testing'.format(log_dir_path)
            amm._new_log_dir(log_dir_path, post_fix)
            self.assertTrue(os.path.exists(expected_path))


class TestWaitForModel(TestCase):
    # Check that it returns an error if the model never comes up.
    # Pass in a timeout for the model check
    def test_raises_exception_when_timeout_occurs(self):
        with patch.object(until_timeout, 'next', side_effect=StopIteration()):
            with self.assertRaises(AssertionError):
                amm.wait_for_model(Mock(), 'TestModelName')

    def test_returns_when_model_found(self):
        mock_client = Mock()
        mock_client.get_models.return_value = dict(
            models=[
                dict(name='TestModelName')])
        amm.wait_for_model(mock_client, 'TestModelName')

    def test_pauses_between_failed_matches(self):
        mock_client = Mock()
        mock_client.get_models.side_effect = [
            dict(models=[]),    # Failed check
            dict(models=[dict(name='TestModelName')]),  # Successful check
            ]
        with patch.object(amm, 'sleep') as mock_sleep:
            amm.wait_for_model(mock_client, 'TestModelName')
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
