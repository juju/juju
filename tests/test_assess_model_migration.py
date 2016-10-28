"""Tests for assess_model_migration module."""

import argparse
import logging
from mock import call, Mock, patch
import os
import StringIO
from subprocess import CalledProcessError

import assess_model_migration as amm
from deploy_stack import BootstrapManager
from fakejuju import fake_juju_client
from jujupy import (
    EnvJujuClient,
    JujuData,
    SoftDeadlineExceeded,
    )
from tests import (
    client_past_deadline,
    parse_error,
    TestCase,
)
from utility import (
    JujuAssertionError,
    noop_context,
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
                verbose=20,
                deadline=None,
                ))

    def test_help(self):
        fake_stdout = StringIO.StringIO()
        with parse_error(self) as fake_stderr:
            with patch("sys.stdout", fake_stdout):
                amm.parse_args(["--help"])
        self.assertEqual("", fake_stderr.getvalue())
        self.assertIn(
            "Test model migration feature", fake_stdout.getvalue())


class TestExpectMigrationAttemptToFail(TestCase):
    source_client = fake_juju_client()
    dest_client = fake_juju_client()

    def test_raises_when_no_failure_detected(self):
        with patch.object(self.source_client, 'get_juju_output'):
            with self.assertRaises(JujuAssertionError) as ex:
                amm.expect_migration_attempt_to_fail(
                    self.source_client, self.dest_client)
            self.assertEqual(
                ex.exception.message, 'Migration did not fail as expected.')

    def test_raises_when_incorrect_failure_detected(self):
        with patch.object(self.source_client, 'get_juju_output',
                          side_effect=CalledProcessError(-1, None, '')):
            with self.assertRaises(CalledProcessError):
                amm.expect_migration_attempt_to_fail(
                    self.source_client, self.dest_client)

    def test_outputs_log_on_non_failure(self):
        with patch.object(self.source_client, 'get_juju_output',
                          return_value='foo'):
            with self.assertRaises(JujuAssertionError):
                with parse_error(self) as fake_stderr:
                    amm.expect_migration_attempt_to_fail(
                        self.source_client, self.dest_client)
                    self.assertEqual(fake_stderr.getvalue(), 'foo\n')

    def test_outputs_log_on_expected_failure(self):
        side_effect = CalledProcessError(
            '-1', None, 'ERROR: permission denied')
        with patch.object(self.source_client, 'get_juju_output',
                          side_effect=side_effect):
            stderr = StringIO.StringIO()
            with patch('sys.stderr', stderr) as fake_stderr:
                amm.expect_migration_attempt_to_fail(
                    self.source_client, self.dest_client)
        error = fake_stderr.getvalue()
        self.assertIn('permission denied', error)


class TestGetBootstrapManagers(TestCase):
    def test_returns_two_bs_managers(self):
        ret_bs = [
            Mock(client=fake_juju_client()),
            Mock(client=fake_juju_client())]
        with temp_dir() as log_dir:
            args = argparse.Namespace(logs=log_dir)
            with patch.object(
                    BootstrapManager, 'from_args', side_effect=ret_bs):
                bs1, bs2 = amm.get_bootstrap_managers(args)
                self.assertEqual(bs1, ret_bs[0])
                self.assertEqual(bs2, ret_bs[1])

    def test_gives_second_manager_unique_env(self):
        ret_bs = [
            Mock(client=fake_juju_client(), temp_env_name='testing-env-name'),
            Mock(client=fake_juju_client())]
        with temp_dir() as log_dir:
            args = argparse.Namespace(logs=log_dir)
            with patch.object(BootstrapManager, 'from_args',
                              side_effect=ret_bs):
                bs1, bs2 = amm.get_bootstrap_managers(args)
                self.assertEqual(bs2.temp_env_name, 'testing-env-name-b')

    def test_creates_unique_log_dirs(self):
        ret_bs = [
            Mock(client=fake_juju_client()),
            Mock(client=fake_juju_client())]
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
        mock_client = Mock()
        mock_client.check_timeouts.return_value = noop_context()
        mock_client.ignore_soft_deadline.return_value = noop_context()
        with patch.object(until_timeout, 'next', side_effect=StopIteration()):
            with self.assertRaises(AssertionError):
                amm.wait_for_model(mock_client, 'TestModelName')

    def test_returns_when_model_found(self):
        mock_client = Mock()
        mock_client.check_timeouts.return_value = noop_context()
        mock_client.ignore_soft_deadline.return_value = noop_context()
        mock_client.get_models.return_value = dict(
            models=[
                dict(name='TestModelName')])
        amm.wait_for_model(mock_client, 'TestModelName')

    def test_pauses_between_failed_matches(self):
        mock_client = Mock()
        mock_client.check_timeouts.return_value = noop_context()
        mock_client.ignore_soft_deadline.return_value = noop_context()
        mock_client.get_models.side_effect = [
            dict(models=[]),    # Failed check
            dict(models=[dict(name='TestModelName')]),  # Successful check
            ]
        with patch.object(amm, 'sleep') as mock_sleep:
            amm.wait_for_model(mock_client, 'TestModelName')
            mock_sleep.assert_called_once_with(1)

    def test_suppresses_deadline(self):
        client = EnvJujuClient(JujuData('local', juju_home=''), None, None)
        with client_past_deadline(client):

            real_check_timeouts = client.check_timeouts

            def get_models():
                with real_check_timeouts():
                    return {'models': [{'name': 'TestModelName'}]}

            with patch.object(client, 'get_models', side_effect=get_models,
                              autospec=True):
                with patch.object(client, 'check_timeouts', autospec=True):
                    amm.wait_for_model(client, 'TestModelName')

    def test_checks_deadline(self):
        client = EnvJujuClient(JujuData('local', juju_home=''), None, None)
        with client_past_deadline(client):

            def get_models():
                return {'models': [{'name': 'TestModelName'}]}

            with patch.object(client, 'get_models', side_effect=get_models,
                              autospec=True):
                with self.assertRaises(SoftDeadlineExceeded):
                    amm.wait_for_model(client, 'TestModelName')


class TestRaiseIfSharedMachines(TestCase):

    def test_empty_list_raises(self):
        with self.assertRaises(ValueError):
            amm.raise_if_shared_machines([])

    def test_unique_list_does_not_raise_single_item(self):
        amm.raise_if_shared_machines([1])

    def test_unique_list_does_not_raise_many_item(self):
        amm.raise_if_shared_machines([1, 2, 3])

    def test_double_up_raises(self):
        with self.assertRaises(JujuAssertionError):
            amm.raise_if_shared_machines([1, 1])


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
