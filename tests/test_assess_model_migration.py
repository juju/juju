"""Tests for assess_model_migration module."""

import argparse
from contextlib import contextmanager
import logging
from mock import call, Mock, patch
import os
import StringIO
from subprocess import CalledProcessError
from textwrap import dedent

import assess_model_migration as amm
from deploy_stack import BootstrapManager
from fakejuju import fake_juju_client
from jujupy import (
    Controller,
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
                use_develop=False,
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


def _get_time_noop_mock_client():
    mock_client = Mock()
    mock_client.check_timeouts.return_value = noop_context()
    mock_client.ignore_soft_deadline.return_value = noop_context()

    return mock_client


class TestWaitForMigration(TestCase):

    no_migration_yaml = dedent("""\
        example-model:
            status:
                current: available
                since: 4 minutes ago
    """)

    migration_status_yaml = dedent("""\
        example-model:
            status:
                current: available
                since: 4 minutes ago
                migration: 'Some message.'
                migration-start: 48 seconds ago
        """)

    def test_gets_show_model_details(self):
        mock_client = _get_time_noop_mock_client()

        mock_client.get_juju_output.return_value = self.migration_status_yaml
        mock_client.env = JujuData(
            'example-model', juju_home='', controller=Controller('foo'))
        with patch.object(amm, 'sleep', autospec=True):
            amm.wait_for_migrating(mock_client)
        mock_client.get_juju_output.assert_called_once_with(
            'show-model', 'foo:example-model',
            '--format', 'yaml', include_e=False)

    def test_returns_when_migration_status_found(self):
        mock_client = _get_time_noop_mock_client()

        mock_client.get_juju_output.return_value = self.migration_status_yaml
        mock_client.env = JujuData(
            'example-model', juju_home='', controller=Controller('foo'))
        with patch.object(amm, 'sleep', autospec=True) as m_sleep:
            amm.wait_for_migrating(mock_client)
        self.assertEqual(m_sleep.call_count, 0)

    def test_checks_multiple_times_for_status(self):
        mock_client = _get_time_noop_mock_client()

        mock_client.get_juju_output.side_effect = [
            self.no_migration_yaml,
            self.no_migration_yaml,
            self.migration_status_yaml
        ]
        mock_client.env = JujuData(
            'example-model', juju_home='', controller=Controller('foo'))
        with patch.object(amm, 'sleep', autospec=True) as m_sleep:
            amm.wait_for_migrating(mock_client)
        self.assertEqual(m_sleep.call_count, 2)

    def test_raises_when_timeout_reached_with_no_migration_status(self):
        mock_client = _get_time_noop_mock_client()

        mock_client.get_juju_output.return_value = self.no_migration_yaml
        mock_client.env = JujuData(
            'example-model', juju_home='', controller=Controller('foo'))
        with patch.object(amm, 'sleep', autospec=True):
            with patch.object(
                    until_timeout, 'next', side_effect=StopIteration()):
                with self.assertRaises(JujuAssertionError):
                    amm.wait_for_migrating(mock_client)


class TestWaitForModel(TestCase):
    # Check that it returns an error if the model never comes up.
    # Pass in a timeout for the model check
    def test_raises_exception_when_timeout_occurs(self):
        mock_client = _get_time_noop_mock_client()
        with patch.object(until_timeout, 'next', side_effect=StopIteration()):
            with self.assertRaises(AssertionError):
                amm.wait_for_model(mock_client, 'TestModelName')

    def test_returns_when_model_found(self):
        controller_client = Mock()
        controller_client.get_models.return_value = dict(
            models=[
                dict(name='TestModelName')])
        mock_client = _get_time_noop_mock_client()
        mock_client.get_controller_client.return_value = controller_client
        amm.wait_for_model(mock_client, 'TestModelName')

    def test_pauses_between_failed_matches(self):
        controller_client = Mock()
        controller_client.get_models.side_effect = [
            dict(models=[]),    # Failed check
            dict(models=[dict(name='TestModelName')]),  # Successful check
            ]
        mock_client = _get_time_noop_mock_client()
        mock_client.get_controller_client.return_value = controller_client

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

            controller_client = Mock()
            controller_client.get_models.side_effect = get_models

            with patch.object(
                    client, 'get_controller_client',
                    autospec=True, return_value=controller_client):
                with patch.object(client, 'check_timeouts', autospec=True):
                    amm.wait_for_model(client, 'TestModelName')

    def test_checks_deadline(self):
        client = EnvJujuClient(JujuData('local', juju_home=''), None, None)
        with client_past_deadline(client):
            def get_models():
                return {'models': [{'name': 'TestModelName'}]}

            controller_client = Mock()
            controller_client.get_models.side_effect = get_models

            with patch.object(
                    client, 'get_controller_client',
                    autospec=True, return_value=controller_client):
                with self.assertRaises(SoftDeadlineExceeded):
                    amm.wait_for_model(client, 'TestModelName')


class TestDeployMongodbToNewModel(TestCase):

    def test_deploys_mongodb_to_new_model(self):
        new_model = Mock()
        source_client = Mock()
        source_client.add_model.return_value = new_model

        with patch.object(
                amm, 'test_deployed_mongo_is_up', autospec=True) as m_tdmiu:
            self.assertEqual(
                new_model,
                amm.deploy_mongodb_to_new_model(source_client, 'test'))

        source_client.add_model.assert_called_once_with(
            source_client.env.clone.return_value)
        new_model.juju.assert_called_once_with('deploy', ('mongodb'))
        new_model.wait_for_started.assert_called_once_with()
        new_model.wait_for_workloads.assert_called_once_with
        m_tdmiu.assert_called_once_with(new_model)


class TestDisableAPIServer(TestCase):

    def test_starts_and_stops_api_server(self):
        remote_client = Mock()
        mock_client = Mock()
        with patch.object(
                amm, 'get_remote_for_controller',
                autospec=True, return_value=remote_client) as m_grfc:
            with amm.disable_apiserver(mock_client):
                pass
        m_grfc.assert_called_once_with(mock_client)
        self.assertItemsEqual(
            [
                call('sudo service jujud-machine-0 stop'),
                call('sudo service jujud-machine-0 start')
            ],
            remote_client.run.mock_calls)

    def test_starts_api_server_if_exception_occurs(self):
        remote_client = Mock()
        mock_client = Mock()
        with patch.object(
                amm, 'get_remote_for_controller',
                autospec=True, return_value=remote_client):
            try:
                with amm.disable_apiserver(mock_client):
                    raise ValueError()
            except ValueError:
                pass            # Expected test exception.
        self.assertItemsEqual(
            [
                call('sudo service jujud-machine-0 stop'),
                call('sudo service jujud-machine-0 start')
            ],
            remote_client.run.mock_calls)


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
        mock_assess.assert_called_once_with(bs_1, bs_2, amm.parse_args(argv))


@contextmanager
def tmp_ctx():
    yield '/tmp/dir'


class TestAssessModelMigration(TestCase):

    def test_runs_develop_tests_when_requested(self):
        argv = [
            'an-env', '/bin/juju', '/tmp/logs', 'an-env-mod', '--use-develop']
        args = amm.parse_args(argv)
        bs1 = Mock()
        bs2 = Mock()
        bs1.booted_context.return_value = noop_context()
        bs2.existing_booted_context.return_value = noop_context()

        with patch.object(
                amm,
                'ensure_migrating_with_insufficient_user_permissions_fails',
                autospec=True) as m_user:
            with patch.object(
                    amm,
                    'ensure_migrating_with_superuser_user_permissions_succeeds',  # NOQA
                    autospec=True) as m_super:
                with patch.object(
                        amm,
                        'ensure_able_to_migrate_model_between_controllers',
                        autospec=True) as m_between:
                    with patch.object(
                            amm,
                            'ensure_migration_rolls_back_on_failure',
                            autospec=True) as m_rollback:
                        with patch.object(
                                amm, 'temp_dir',
                                autospec=True, return_value=tmp_ctx()):
                            amm.assess_model_migration(bs1, bs2, args)
        m_user.assert_called_once_with(bs1, bs2, args.upload_tools, '/tmp/dir')
        m_super.assert_called_once_with(
            bs1, bs2, args.upload_tools, '/tmp/dir')
        m_between.assert_called_once_with(bs1, bs2, args.upload_tools)
        m_rollback.assert_called_once_with(bs1, bs2, args.upload_tools)

    def test_does_not_run_develop_tests_by_default(self):
        argv = [
            'an-env', '/bin/juju', '/tmp/logs', 'an-env-mod']
        args = amm.parse_args(argv)
        bs1 = Mock()
        bs2 = Mock()
        bs1.booted_context.return_value = noop_context()
        bs2.existing_booted_context.return_value = noop_context()

        with patch.object(
                amm,
                'ensure_migrating_with_insufficient_user_permissions_fails',
                autospec=True) as m_user:
            with patch.object(
                    amm,
                    'ensure_migrating_with_superuser_user_permissions_succeeds',  # NOQA
                    autospec=True) as m_super:
                with patch.object(
                        amm,
                        'ensure_able_to_migrate_model_between_controllers',
                        autospec=True) as m_between:
                    with patch.object(
                            amm,
                            'ensure_migration_rolls_back_on_failure',
                            autospec=True) as m_rollback:
                        with patch.object(
                                amm, 'temp_dir',
                                autospec=True, return_value=tmp_ctx()):
                            amm.assess_model_migration(bs1, bs2, args)
        m_user.assert_called_once_with(bs1, bs2, args.upload_tools, '/tmp/dir')
        m_super.assert_called_once_with(
            bs1, bs2, args.upload_tools, '/tmp/dir')
        m_between.assert_called_once_with(bs1, bs2, args.upload_tools)

        self.assertEqual(m_rollback.call_count, 0)
