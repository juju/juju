"""Tests for assess_model_migration module."""

import argparse
from contextlib import contextmanager
import logging
import os
import StringIO
from subprocess import CalledProcessError
from textwrap import dedent

from mock import (
    call,
    Mock,
    patch,
    )
import yaml

import assess_model_migration as amm
from assess_model_migration import (
    assert_data_file_lists_correct_controller_for_model,
    assert_model_has_correct_controller_uuid,
    ensure_api_login_redirects,
    )
from assess_user_grant_revoke import User
from deploy_stack import (
    BootstrapManager,
    get_random_string,
    )
from jujupy import (
    ModelClient,
    fake_juju_client,
    JujuData,
    SoftDeadlineExceeded,
    )
from jujupy.version_client import (
    ModelClient2_0,
    ModelClient2_1,
    ModelClientRC,
)
from tests import (
    client_past_deadline,
    FakeHomeTestCase,
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
                to=None,
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


class TestAfter22Beta4(TestCase):

    def test_returns_False_for_prev_final_releases(self):
        self.assertFalse(amm.after_22beta4('2.1'))

    def test_returns_True_for_final_release(self):
        self.assertTrue(amm.after_22beta4('2.2.0'))
        self.assertTrue(amm.after_22beta4('2.2'))
        self.assertTrue(amm.after_22beta4('2.2.1'))

    def test_returns_True_when_newer(self):
        self.assertTrue(amm.after_22beta4('2.2-beta8-xenial-amd64'))
        self.assertTrue(amm.after_22beta4('2.3-beta4-xenial-amd64'))

    def test_returns_True_when_equal(self):
        self.assertTrue(amm.after_22beta4('2.2-beta4-xenial-amd64'))

    def test_returns_False_when_older(self):
        self.assertFalse(amm.after_22beta4('2.1-beta8-xenial-amd64'))
        self.assertFalse(amm.after_22beta4('2.2-beta2-xenial-amd64'))


class TestDeploySimpleResourceServer(TestCase):

    def test_deploys_with_resource(self):
        contents = 'test123'
        client = Mock()
        with temp_dir() as tmp_dir:
            resource_file_path = os.path.join(tmp_dir, 'index.html')
            with patch.object(
                    amm, 'temp_dir', autospec=True) as m_td:
                m_td.return_value.__enter__.return_value = tmp_dir
                amm.deploy_simple_resource_server(client, contents)
        client.deploy.assert_called_once_with(
            'local:simple-resource-http',
            resource='index={}'.format(resource_file_path))

    def test_deploys_file_with_requested_contents(self):
        contents = 'test123'
        client = Mock()
        with temp_dir() as tmp_dir:
            resource_file_path = os.path.join(tmp_dir, 'index.html')
            with patch.object(
                    amm, 'temp_dir', autospec=True) as m_td:
                m_td.return_value.__enter__.return_value = tmp_dir
                amm.deploy_simple_resource_server(client, contents)

            with open(resource_file_path, 'rt') as f:
                self.assertEqual(f.read(), contents)

    def test_returns_application_name(self):
        client = Mock()
        with temp_dir() as tmp_dir:
            with patch.object(
                    amm, 'temp_dir', autospec=True) as m_td:
                m_td.return_value.__enter__.return_value = tmp_dir

                self.assertEqual(
                    amm.deploy_simple_resource_server(client, ''),
                    'simple-resource-http')

    def test_waits_and_exposes(self):
        client = Mock()
        with temp_dir() as tmp_dir:
            with patch.object(
                    amm, 'temp_dir', autospec=True) as m_td:
                m_td.return_value.__enter__.return_value = tmp_dir
                amm.deploy_simple_resource_server(client, '')

        client.wait_for_started.assert_called_once_with()
        client.wait_for_workloads.assert_called_once_with()
        client.juju.assert_called_once_with(
            'expose', ('simple-resource-http'))


class TestGetServerResponse(TestCase):

    def test_uses_protocol_and_ipaddress(self):
        with patch.object(
                amm, 'urlopen', autospec=True) as m_uopen:
            amm.get_server_response('192.168.1.2')
            m_uopen.assert_called_once_with('http://192.168.1.2')

    def test_returns_stripped_value(self):
        response = StringIO.StringIO('simple server.\n')
        with patch.object(
                amm, 'urlopen', autospec=True, return_value=response):
            self.assertEqual(
                amm.get_server_response('192.168.1.2'),
                'simple server.'
            )


class TestAssertDeployedCharmIsResponding(TestCase):

    def test_passes_when_charm_is_responding_correctly(self):
        expected_output = get_random_string()
        client = Mock()

        with patch.object(
                amm, 'get_unit_ipaddress',
                autospec=True, return_value='192.168.1.2') as m_gui:
            with patch.object(
                    amm, 'get_server_response',
                    autospec=True, return_value=expected_output) as m_gsr:
                amm.assert_deployed_charm_is_responding(
                    client, expected_output)
        m_gui.assert_called_once_with(client, 'simple-resource-http/0')
        m_gsr.assert_called_once_with('192.168.1.2')

    def test_raises_when_charm_does_not_respond_correctly(self):
        expected_output = get_random_string()
        client = Mock()

        with patch.object(
                amm, 'get_unit_ipaddress',
                autospec=True, return_value='192.168.1.2'):
            with patch.object(
                    amm, 'get_server_response',
                    autospec=True, return_value='abc'):

                with self.assertRaises(AssertionError) as ex:
                    amm.assert_deployed_charm_is_responding(
                        client, expected_output)
                self.assertEqual(
                    ex.exception.message,
                    'Server charm is not responding as expected.')


class TestCreateUserOnControllers(TestCase):

    def test_creates_user_home_dir(self):
        source_client = Mock()
        dest_client = Mock()
        with patch.object(amm.os, 'makedirs', autospec=True) as m_md:
            amm.create_user_on_controllers(
                source_client,
                dest_client,
                '/tmp/path',
                'foo-username',
                'permission')
        m_md.assert_called_once_with('/tmp/path/foo-username')

    def test_registers_and_grants_user_permissions_on_both_controllers(self):
        source_client = Mock()
        dest_client = Mock()
        username = 'foo-username'
        user = User(username, 'write', [])
        with temp_dir() as tmp_dir:
            with patch.object(
                    amm, 'User', autospec=True, return_value=user) as m_user:
                amm.create_user_on_controllers(
                    source_client,
                    dest_client,
                    tmp_dir,
                    username,
                    'permission')
        m_user.assert_called_once_with(username, 'write', [])
        source_client.register_user.assert_called_once_with(
            user, os.path.join(tmp_dir, username))
        source_client.grant.assert_called_once_with(username, 'permission')
        dest_client.register_user.assert_called_once_with(
            user,
            os.path.join(tmp_dir, username),
            controller_name='foo-username_controllerb')
        dest_client.grant.assert_called_once_with(username, 'permission')

    def test_returns_created_clients(self):
        source_client = Mock()
        dest_client = Mock()
        with temp_dir() as tmp:
            new_source, new_dest = amm.create_user_on_controllers(
                source_client, dest_client, tmp, 'foo-username', 'write')
        self.assertEqual(new_source, source_client.register_user.return_value)
        self.assertEqual(new_dest, dest_client.register_user.return_value)


class TestAssertModelMigratedSuccessfully(TestCase):

    def test_assert_model_migrated_successfully_default_values(self):
        client = Mock()
        with patch.object(
                amm,
                'assert_deployed_charm_is_responding',
                autospec=True) as m_tdmiu:
            with patch.object(
                    amm, 'ensure_model_is_functional',
                    autospec=True) as m_emif:
                amm.assert_model_migrated_successfully(client, 'test')
        client.wait_for_workloads.assert_called_once_with()
        m_tdmiu.assert_called_once_with(client, None)
        m_emif.assert_called_once_with(client, 'test')

    def test_assert_model_migrated_successfully_explicit_values(self):
        client = Mock()
        with patch.object(
                amm,
                'assert_deployed_charm_is_responding',
                autospec=True) as m_tdmiu:
            with patch.object(
                    amm, 'ensure_model_is_functional',
                    autospec=True) as m_emif:
                amm.assert_model_migrated_successfully(
                    client, 'test', 'abc123')
        client.wait_for_workloads.assert_called_once_with()
        m_tdmiu.assert_called_once_with(client, 'abc123')
        m_emif.assert_called_once_with(client, 'test')


class TestExpectMigrationAttemptToFail(TestCase):
    source_client = fake_juju_client()
    dest_client = fake_juju_client()

    def test_raises_when_no_failure_detected(self):
        with patch.object(
                self.source_client, 'get_juju_output', autospec=True):
            with self.assertRaises(JujuAssertionError) as ex:
                with patch('sys.stderr', StringIO.StringIO()):
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
        name:
            status:
                current: available
                since: 4 minutes ago
        """)

    migration_status_yaml = dedent("""\
        name:
            status:
                current: available
                since: 4 minutes ago
                migration: 'Some message.'
                migration-start: 48 seconds ago
        """)

    def test_returns_when_migration_status_found(self):
        client = fake_juju_client()
        with patch.object(
                client, 'get_juju_output',
                autospec=True, return_value=self.migration_status_yaml):
            with patch.object(amm, 'sleep', autospec=True) as m_sleep:
                amm.wait_for_migrating(client)
        self.assertEqual(m_sleep.call_count, 0)

    def test_checks_multiple_times_for_status(self):
        client = fake_juju_client()
        client.bootstrap()

        show_model_calls = [
            self.no_migration_yaml,
            self.no_migration_yaml,
            self.migration_status_yaml
        ]
        with patch.object(
                client, 'get_juju_output',
                autospec=True, side_effect=show_model_calls):
            with patch.object(amm, 'sleep', autospec=True) as m_sleep:
                amm.wait_for_migrating(client)
        self.assertEqual(m_sleep.call_count, 2)

    def test_raises_when_timeout_reached_with_no_migration_status(self):
        client = fake_juju_client()
        client.bootstrap()
        with patch.object(
                client, 'get_juju_output',
                autospec=True, return_value=self.no_migration_yaml):
            with patch.object(amm, 'sleep', autospec=True):
                with patch.object(
                        until_timeout, 'next', side_effect=StopIteration()):
                    with self.assertRaises(JujuAssertionError):
                        amm.wait_for_migrating(client)


class TestWaitForModelCheck(TestCase):
    def test_raises_exception_when_timeout_occurs(self):
        mock_client = _get_time_noop_mock_client()
        with patch.object(until_timeout, 'next', side_effect=StopIteration()):
            with self.assertRaises(amm.ModelCheckFailed):
                amm._wait_for_model_check(
                    mock_client, lambda x: False, timeout=60)

    def test_returns_when_model_found(self):
        mock_client = _get_time_noop_mock_client()
        amm._wait_for_model_check(mock_client, lambda x: True, timeout=60)

    def test_pauses_between_failed_matches(self):
        mock_client = _get_time_noop_mock_client()

        test_callable = Mock()
        test_callable.side_effect = [False, True]

        with patch.object(amm, 'sleep') as mock_sleep:
            amm._wait_for_model_check(mock_client, test_callable, timeout=60)
            mock_sleep.assert_called_once_with(1)

    def test_suppresses_deadline(self):
        client = ModelClient(JujuData('local', juju_home=''), None, None)
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
                    amm._wait_for_model_check(
                        client, lambda x: 'test', timeout=60)

    def test_checks_deadline(self):
        client = ModelClient(JujuData('local', juju_home=''), None, None)
        with client_past_deadline(client):
            with patch.object(client, 'get_controller_client', autospec=True):
                with self.assertRaises(SoftDeadlineExceeded):
                    amm._wait_for_model_check(
                        client, lambda x: 'test', timeout=60)


class TestWaitUntilModelDisappears(TestCase):

    def test_returns_when_model_has_disappeared(self):
        mock_client = _get_time_noop_mock_client()
        model_data = {'models': [{'name': ''}]}

        controller_client = Mock()
        controller_client.get_models.return_value = model_data
        mock_client.get_controller_client.return_value = controller_client

        amm.wait_until_model_disappears(mock_client, 'test_model', timeout=60)

    def test_raises_when_model_doesnt_disappear(self):
        mock_client = _get_time_noop_mock_client()
        model_data = {'models': [{'name': 'test_model'}]}

        controller_client = Mock()
        controller_client.get_models.return_value = model_data
        mock_client.get_controller_client.return_value = controller_client

        with patch.object(until_timeout, 'next', side_effect=StopIteration()):
            with self.assertRaises(amm.JujuAssertionError):
                amm.wait_until_model_disappears(
                    mock_client, 'test_model', timeout=60)

    def test_ignores_model_detail_exceptions(self):
        """ignore errors for model details as this might happen many times."""
        mock_client = _get_time_noop_mock_client()
        model_data = {'models': [{'name': ''}]}

        exp = CalledProcessError(-1, 'blah')
        exp.stderr = 'cannot get model details'
        controller_client = Mock()
        controller_client.get_models.side_effect = [
            exp,
            model_data]
        mock_client.get_controller_client.return_value = controller_client
        with patch.object(amm, 'sleep') as mock_sleep:
            amm.wait_until_model_disappears(
                mock_client, 'test_model', timeout=60)
            mock_sleep.assert_called_once_with(1)

    def test_fails_on_non_expected_exception(self):
        mock_client = _get_time_noop_mock_client()

        exp = CalledProcessError(-1, 'blah')
        exp.stderr = '"" is not a valid tag'
        controller_client = Mock()
        controller_client.get_models.side_effect = [exp]
        mock_client.get_controller_client.return_value = controller_client
        with self.assertRaises(CalledProcessError):
            amm.wait_until_model_disappears(
                mock_client, 'test_model', timeout=60)


class TestWaitForModel(TestCase):

    def test_returns_when_model_appears(self):
        mock_client = _get_time_noop_mock_client()
        model_data = {'models': [{'name': 'test_model'}]}

        controller_client = Mock()
        controller_client.get_models.return_value = model_data
        mock_client.get_controller_client.return_value = controller_client

        amm.wait_for_model(mock_client, 'test_model', timeout=60)

    def test_raises_when_model_doesnt_appear(self):
        mock_client = _get_time_noop_mock_client()
        model_data = {'models': [{'name': ''}]}

        controller_client = Mock()
        controller_client.get_models.return_value = model_data
        mock_client.get_controller_client.return_value = controller_client

        with patch.object(until_timeout, 'next', side_effect=StopIteration()):
            with self.assertRaises(amm.JujuAssertionError):
                amm.wait_for_model(
                    mock_client, 'test_model', timeout=1)


class TestEnsureApiLoginRedirects(FakeHomeTestCase):

    def test_ensure_api_login_redirects(self):
        def named_fake_juju_client(name):
            env = JujuData(name, {'name': name, 'type': 'foo'})
            return fake_juju_client(env=env)

        patch_assert_data = patch.object(
            amm, 'assert_data_file_lists_correct_controller_for_model')
        patch_assert_uuid = patch.object(
            amm, 'assert_model_has_correct_controller_uuid')
        client1 = named_fake_juju_client('source')
        client2 = named_fake_juju_client('target')
        client3 = named_fake_juju_client('dest')
        model_details_list = [
            {client.env.environment: {'controller-uuid': uuid}}
            for (client, uuid)
            in [(client1, '12345'), (client3, 'ABCDE')]]
        with patch('logging.Logger.info', autospec=True):
            with patch('jujupy.ModelClient.show_model', autospec=True,
                       side_effect=model_details_list) as show_model_mock:
                with patch.object(amm, 'migrate_model_to_controller',
                                  return_value=client3):
                    with patch_assert_data, patch_assert_uuid:
                        ensure_api_login_redirects(client1, client2)
        show_model_mock.assert_has_calls([call(client1), call(client3)])

    def create_models_yaml(self, client, controllers_to_models):
        data = dict([(controller, {'models': models})
                     for (controller, models)
                     in controllers_to_models.items()])
        with open(os.path.join(client.env.juju_home, 'models.yaml'), 'w',
                  ) as models_yaml:
            yaml.safe_dump({'controllers': data}, models_yaml)

    def test_assert_data_file_lists_correct_controller_for_model(self):
        client = fake_juju_client(juju_home=self.juju_home)
        self.create_models_yaml(client, {'testing': [client.env.environment]})
        assert_data_file_lists_correct_controller_for_model(client, 'testing')

    def test_assert_data_file_lists_correct_controller_for_model_fails(self):
        client = fake_juju_client(juju_home=self.juju_home)
        self.create_models_yaml(client, {'testing': ['not-the-client-name']})
        with self.assertRaises(JujuAssertionError):
            assert_data_file_lists_correct_controller_for_model(
                client, 'testing')

    def patch_uuid_client(self, client, uuid_model, uuid_controller):
        def get_juju_output(command, *args, **kwargs):
            m_name = client.env.environment
            c_name = client.env.controller.name
            if 'show-model' == command:
                return yaml.safe_dump({
                    m_name: {'controller-uuid': uuid_model}})
            elif 'show-controller' == command:
                return yaml.safe_dump({
                    c_name: {'details': {'uuid': uuid_controller}}})
            else:
                raise Exception(
                    'get_juju_output mock does not handle ' + command)

        return patch.object(client, 'get_juju_output', get_juju_output)

    def test_assert_model_has_correct_controller_uuid(self):
        client = fake_juju_client()
        with self.patch_uuid_client(client, '12345', '12345'):
            assert_model_has_correct_controller_uuid(client)

    def test_assert_model_has_correct_controller_uuid_fails(self):
        client = fake_juju_client()
        with self.patch_uuid_client(client, '12345', 'ABCDE'):
            with self.assertRaises(JujuAssertionError):
                assert_model_has_correct_controller_uuid(client)


class TestMigrateModelToController(FakeHomeTestCase):

    def make_clients(self):
        config = {'type': 'aws', 'region': 'east', 'name': 'model-a'}
        env = JujuData('model-a', config, juju_home='foo')
        env.user_name = 'me'
        source_client = fake_juju_client(env)
        dest_client = fake_juju_client(env)
        return source_client, dest_client

    def test_migrate_model_to_controller_owner(self):
        source_client, dest_client = self.make_clients()
        mt_client = fake_juju_client(source_client.env)
        mt_client._backend.controller_state.add_model('model-a')
        with patch.object(dest_client, 'clone', autospec=True,
                          return_value=mt_client) as dc_mock:
            with patch.object(source_client, 'controller_juju',
                              autospec=True) as cj_mock:
                with patch('assess_model_migration.wait_for_model',
                           autospec=True) as wm_mock:
                    found_client = amm.migrate_model_to_controller(
                        source_client, dest_client, include_user_name=False)
        self.assertIs(mt_client, found_client)
        args, kwargs = dc_mock.call_args
        self.assertEqual(source_client.env, args[0])
        wm_mock.assert_called_once_with(mt_client, 'model-a')
        cj_mock.assert_called_once_with('migrate', ('model-a', 'model-a'))

    def test_migrate_model_to_controller_admin(self):
        source_client, dest_client = self.make_clients()
        mt_client = fake_juju_client(source_client.env)
        mt_client._backend.controller_state.add_model('model-a')
        with patch.object(dest_client, 'clone', return_value=mt_client):
            with patch.object(source_client, 'controller_juju') as cj_mock:
                with patch('assess_model_migration.wait_for_model'):
                    amm.migrate_model_to_controller(
                        source_client, dest_client, include_user_name=True)
        cj_mock.assert_called_once_with('migrate', ('me/model-a', 'model-a'))

    def test_ensure_migrating_with_superuser_user_permissions_succeeds(self):
        func = amm.ensure_migrating_with_superuser_user_permissions_succeeds
        source_client, dest_client = self.make_clients()
        clients = self.make_clients()
        user_source_client, user_dest_client = clients
        user_new_model = user_source_client.add_model(
            user_source_client.env.clone('model-a'))
        with patch.object(amm, 'create_user_on_controllers', autospec=True,
                          return_value=clients) as cu_mock:
            with patch.object(amm, 'migrate_model_to_controller',
                              autospec=True) as mc_mock:
                with patch.object(user_source_client, 'add_model',
                                  autospec=True, return_value=user_new_model):
                    with patch('assess_model_migration.wait_for_model'):
                        func(source_client, dest_client, 'home_dir')
        cu_mock.assert_called_once_with(
            source_client, dest_client, 'home_dir', 'passuser', 'superuser')
        mc_mock.assert_called_once_with(
            user_new_model, user_dest_client, include_user_name=True)


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
                pass  # Expected test exception.
        self.assertItemsEqual(
            [
                call('sudo service jujud-machine-0 stop'),
                call('sudo service jujud-machine-0 start')
            ],
            remote_client.run.mock_calls)

    def test_starts_and_stops_request_machine_number(self):
        remote_client = Mock()
        mock_client = Mock()
        with patch.object(
                amm, 'get_remote_for_controller',
                autospec=True, return_value=remote_client) as m_grfc:
            with amm.disable_apiserver(mock_client, '123'):
                pass
        m_grfc.assert_called_once_with(mock_client)
        self.assertItemsEqual(
            [
                call('sudo service jujud-machine-123 stop'),
                call('sudo service jujud-machine-123 start')
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


class TestClientIsAtLeast21(TestCase):

    def test_returns_true_when_21(self):
        client = ModelClient2_1(JujuData('local', juju_home=''), None, None)
        self.assertTrue(amm.client_is_at_least_2_1(client))

    def test_returns_true_when_greater_than_21(self):
        client = ModelClient(JujuData('local', juju_home=''), None, None)
        self.assertTrue(amm.client_is_at_least_2_1(client))

    def test_returns_false_when_not_at_least_21(self):
        client = ModelClient2_0(JujuData('local', juju_home=''), None, None)
        self.assertFalse(amm.client_is_at_least_2_1(client))

        client = ModelClientRC(JujuData('local', juju_home=''), None, None)
        self.assertFalse(amm.client_is_at_least_2_1(client))


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


def patch_amm(target):
    return patch.object(amm, target, autospec=True)


class TestAssessUserPermissionModelMigrations(TestCase):

    def test_all_test_called(self):
        source_client = Mock()
        dest_client = Mock()
        patch_insuff = patch_amm(
            'ensure_migrating_with_insufficient_user_permissions_fails')
        patch_super = patch_amm(
            'ensure_migrating_with_superuser_user_permissions_succeeds')
        with patch.object(
                amm, 'temp_dir',
                autospec=True,
                return_value=tmp_ctx()):
            with patch_insuff as m_insuff:
                with patch_super as m_super:
                    amm.assess_user_permission_model_migrations(
                        source_client, dest_client)
        m_insuff.assert_called_once_with(
            source_client, dest_client, '/tmp/dir')
        m_super.assert_called_once_with(source_client, dest_client, '/tmp/dir')


class TestAssessDevelopmentBranchMigrations(TestCase):
    def test_all_test_called(self):
        source_client = Mock()
        dest_client = Mock()
        patch_super = patch_amm(
            'ensure_superuser_can_migrate_other_user_models')
        patch_rollback = patch_amm('ensure_migration_rolls_back_on_failure')
        patch_api = patch_amm('ensure_api_login_redirects')
        with patch.object(
                amm, 'temp_dir',
                autospec=True,
                return_value=tmp_ctx()):
            with patch_super as m_super:
                with patch_rollback as m_rollback:
                    with patch_api as m_api:
                        amm.assess_development_branch_migrations(
                            source_client, dest_client)
        m_super.assert_called_once_with(source_client, dest_client, '/tmp/dir')
        m_rollback.assert_called_once_with(source_client, dest_client)
        m_api.assert_called_once_with(source_client, dest_client)


class TestAssessModelMigration(TestCase):

    def test_runs_develop_tests_when_requested(self):
        argv = [
            'an-env', '/bin/juju', '/tmp/logs', 'an-env-mod', '--use-develop']
        args = amm.parse_args(argv)
        bs1 = Mock()
        bs2 = Mock()
        bs1.booted_context.return_value = noop_context()
        bs2.existing_booted_context.return_value = noop_context()

        patch_user_tests = patch_amm('assess_user_permission_model_migrations')
        patch_dev_tests = patch_amm('assess_development_branch_migrations')
        patch_between = patch_amm(
            'ensure_migration_with_resources_succeeds')
        patch_rollback = patch_amm('ensure_migration_rolls_back_on_failure')
        patch_logs = patch_amm('ensure_model_logs_are_migrated')
        patch_assert_migrated = patch_amm('assert_model_migrated_successfully')
        patch_21_client = patch_amm('client_is_at_least_2_1')

        mig_client = Mock()
        app = 'application'
        resource_string = 'resource'

        with patch_user_tests as m_user:
            with patch_between as m_between:
                m_between.return_value = mig_client, app, resource_string
                with patch_rollback as m_rollback:
                    with patch_logs as m_logs:
                        with patch_assert_migrated as m_am:
                            with patch_dev_tests as m_dev_tests:
                                with patch_21_client as m_21:
                                    m_21.return_value = True
                                    amm.assess_model_migration(bs1, bs2, args)
        source_client = bs2.client
        dest_client = bs1.client
        m_user.assert_called_once_with(source_client, dest_client)
        m_between.assert_called_once_with(source_client, dest_client)
        m_logs.assert_called_once_with(source_client, dest_client)
        m_am.assert_called_once_with(mig_client, app, resource_string)
        m_dev_tests.assert_called_once_with(source_client, dest_client)

        self.assertEqual(m_rollback.call_count, 0)

    def test_does_not_run_develop_tests_by_default(self):
        argv = [
            'an-env', '/bin/juju', '/tmp/logs', 'an-env-mod']
        args = amm.parse_args(argv)
        bs1 = Mock()
        bs2 = Mock()
        bs1.booted_context.return_value = noop_context()
        bs2.existing_booted_context.return_value = noop_context()

        patch_user_tests = patch_amm('assess_user_permission_model_migrations')
        patch_between = patch_amm(
            'ensure_migration_with_resources_succeeds')
        patch_rollback = patch_amm('ensure_migration_rolls_back_on_failure')
        patch_logs = patch_amm('ensure_model_logs_are_migrated')
        patch_assert_migrated = patch_amm('assert_model_migrated_successfully')
        patch_21_client = patch_amm('client_is_at_least_2_1')

        mig_client = Mock()
        app = 'application'
        resource_string = 'resource'

        with patch_user_tests as m_user:
            with patch_between as m_between:
                m_between.return_value = mig_client, app, resource_string
                with patch_rollback as m_rollback:
                    with patch_logs as m_logs:
                        with patch_assert_migrated as m_am:
                            with patch_21_client as m_21:
                                m_21.return_value = True
                                amm.assess_model_migration(bs1, bs2, args)
        source_client = bs2.client
        dest_client = bs1.client
        m_user.assert_called_once_with(source_client, dest_client)
        m_between.assert_called_once_with(source_client, dest_client)
        m_logs.assert_called_once_with(source_client, dest_client)
        m_am.assert_called_once_with(mig_client, app, resource_string)

        self.assertEqual(m_rollback.call_count, 0)


class TestAssertLogsAppearInClientModel(TestCase):

    def test_raises_exception_when_timeout_occurs(self):
        mock_client = Mock()
        with patch.object(until_timeout, 'next', side_effect=StopIteration()):
            with self.assertRaises(JujuAssertionError):
                amm.assert_logs_appear_in_client_model(
                    mock_client, 'test logs', timeout=60)

    def test_returns_early_when_logs_found(self):
        mock_client = Mock()
        mock_client.get_juju_output.return_value = 'test logs'
        with patch.object(amm, 'sleep', autospec=True) as m_sleep:
            amm.assert_logs_appear_in_client_model(
                mock_client, 'test logs', timeout=60)
        self.assertEqual(m_sleep.call_count, 0)

    def test_tries_multiple_times_to_get_log_output(self):
        mock_client = Mock()
        mock_client.get_juju_output.side_effect = ['', 'test logs']
        with patch.object(
                until_timeout, 'next',
                side_effect=[None, None, StopIteration()]):
            with patch.object(amm, 'sleep', autospec=True) as m_sleep:
                amm.assert_logs_appear_in_client_model(
                    mock_client, 'test logs', timeout=60)
                self.assertEqual(m_sleep.call_count, 1)
