from contextlib import contextmanager
import logging
from mock import (
    call,
    patch,
    Mock,
    sentinel,
    )

from assess_recovery import (
    assess_ha_recovery,
    assess_recovery,
    check_token,
    delete_controller_members,
    detect_bootstrap_machine,
    enable_ha,
    HARecoveryError,
    main,
    parse_args,
    restore_missing_state_server,
    )
from deploy_stack import BootstrapManager
from fakejuju import fake_juju_client
from jujupy import (
    Machine,
    )
from subprocess import CalledProcessError
from tests import (
    FakeHomeTestCase,
    TestCase,
    )
from utility import JujuAssertionError


class TestParseArgs(TestCase):

    def test_parse_args(self):
        args = parse_args(['an-env', '/juju', 'log', 'tmp-env'])
        self.assertEqual(args.env, 'an-env')
        self.assertEqual(args.juju_bin, '/juju')
        self.assertEqual(args.logs, 'log')
        self.assertEqual(args.temp_env_name, 'tmp-env')
        self.assertEqual(args.charm_series, '')
        self.assertEqual(args.strategy, 'backup')
        self.assertEqual(args.verbose, logging.INFO)
        self.assertEqual(args.debug, False)
        self.assertIs(args.agent_stream, None)
        self.assertIs(args.series, None)

    def test_parse_args_ha(self):
        args = parse_args(['an-env', '/juju', 'log', 'tmp-env', '--ha'])
        self.assertEqual(args.strategy, 'ha')

    def test_parse_args_ha_backup(self):
        args = parse_args(['an-env', '/juju', 'log', 'tmp-env', '--ha-backup'])
        self.assertEqual(args.strategy, 'ha-backup')

    def test_parse_args_backup(self):
        args = parse_args(['an-env', '/juju', 'log', 'tmp-env', '--ha',
                           '--backup'])
        self.assertEqual(args.strategy, 'backup')

    def test_parse_args_charm_series(self):
        args = parse_args(['an-env', '/juju', 'log', 'tmp-env',
                           '--charm-series', 'qux'])
        self.assertEqual(args.charm_series, 'qux')


class TestAssessRecovery(TestCase):

    @contextmanager
    def assess_recovery_cxt(self, client):
        client.bootstrap()

        def terminate(env, instance_ids):
            model = client._backend.controller_state.controller_model
            for instance_id in instance_ids:
                model.remove_state_server(instance_id)

        with patch('assess_recovery.wait_for_state_server_to_shutdown',
                   autospec=True):
            with patch('assess_recovery.terminate_instances',
                       side_effect=terminate):
                with patch('deploy_stack.wait_for_port', autospec=True):
                    with patch('assess_recovery.restore_present_state_server',
                               autospec=True):
                        with patch('assess_recovery.check_token',
                                   autospec=True,
                                   side_effect=['Token: One', 'Token: Two']):
                            with patch('assess_recovery.show_controller',
                                       autospec=True,
                                       return_value='controller'):
                                yield

    def test_backup(self):
        client = fake_juju_client()
        bs_manager = Mock(client=client, known_hosts={})
        with self.assess_recovery_cxt(client):
            assess_recovery(bs_manager, 'backup', 'trusty')

    def test_ha(self):
        client = fake_juju_client()
        bs_manager = Mock(client=client, known_hosts={})
        with self.assess_recovery_cxt(client):
            assess_recovery(bs_manager, 'ha', 'trusty')

    def test_ha_backup(self):
        client = fake_juju_client()
        bs_manager = Mock(client=client, known_hosts={})
        with self.assess_recovery_cxt(client):
            assess_recovery(bs_manager, 'ha-backup', 'trusty')

    def test_controller_model_backup(self):
        client = fake_juju_client()
        bs_manager = Mock(client=client, known_hosts={})
        with self.assess_recovery_cxt(client):
            assess_recovery(bs_manager, 'backup', 'trusty')

    def test_controller_model_ha(self):
        client = fake_juju_client()
        bs_manager = Mock(client=client, known_hosts={})
        with self.assess_recovery_cxt(client):
            assess_recovery(bs_manager, 'ha', 'trusty')

    def test_controller_model_ha_backup(self):
        client = fake_juju_client()
        bs_manager = Mock(client=client, known_hosts={})
        with self.assess_recovery_cxt(client):
            assess_recovery(bs_manager, 'ha-backup', 'trusty')


@patch('assess_recovery.configure_logging', autospec=True)
@patch('assess_recovery.BootstrapManager.booted_context', autospec=True)
class TestMain(FakeHomeTestCase):

    def test_main(self, mock_bc, mock_cl):
        client = Mock(spec=['is_jes_enabled', 'version'])
        client.version = '1.25.5'
        with patch('deploy_stack.client_from_config',
                   return_value=client) as mock_c:
            with patch('assess_recovery.assess_recovery',
                       autospec=True) as mock_assess:
                main(['an-env', '/juju', 'log_dir', 'tmp-env', '--backup',
                      '--charm-series', 'a-series'])
        mock_cl.assert_called_once_with(logging.INFO)
        mock_c.assert_called_once_with('an-env', '/juju', debug=False,
                                       soft_deadline=None)
        self.assertEqual(mock_bc.call_count, 1)
        self.assertEqual(mock_assess.call_count, 1)
        bs_manager, strategy, series = mock_assess.call_args[0]
        self.assertEqual((bs_manager.client, strategy, series),
                         (client, 'backup', 'a-series'))

    def test_error(self, mock_bc, mock_cl):
        class FakeError(Exception):
            """Custom exception to validate error handling."""
        error = FakeError('An error during test')
        client = Mock(spec=['is_jes_enabled', 'version'])
        client.version = '2.0.0'
        with patch('deploy_stack.client_from_config',
                   return_value=client) as mock_c:
            with patch('assess_recovery.parse_new_state_server_from_error',
                       autospec=True, return_value='a-host') as mock_pe:
                with patch('assess_recovery.assess_recovery', autospec=True,
                           side_effect=error) as mock_assess:
                    with self.assertRaises(FakeError) as ctx:
                        main(['an-env', '/juju', 'log_dir', 'tmp-env', '--ha',
                              '--verbose', '--charm-series', 'a-series'])
                    self.assertIs(ctx.exception, error)
        mock_cl.assert_called_once_with(logging.DEBUG)
        mock_c.assert_called_once_with('an-env', '/juju', debug=False,
                                       soft_deadline=None)
        mock_pe.assert_called_once_with(error)
        self.assertEqual(mock_bc.call_count, 1)
        self.assertEqual(mock_assess.call_count, 1)
        bs_manager, strategy, series = mock_assess.call_args[0]
        self.assertEqual((bs_manager.client, strategy, series),
                         (client, 'ha', 'a-series'))
        self.assertEqual(bs_manager.known_hosts['0'], 'a-host')


class TestHA(FakeHomeTestCase):

    def test_enable_ha(self):
        controller_client = fake_juju_client()
        bs_manager = Mock(
            client=controller_client, known_hosts={}, has_controller=False)
        machines = {'0': 'a.local', '1': 'b.local', '2': 'c.local'}
        with patch.object(controller_client, 'enable_ha',
                          autospec=True) as eh_mock:
            with patch.object(controller_client, 'show_controller',
                              autospec=True) as sc_mock:
                with patch.object(controller_client, 'wait_for_ha',
                                  autospec=True) as wh_mock:
                    with patch('deploy_stack.iter_remote_machines',
                               autospec=True, return_value=machines.items()):
                        enable_ha(bs_manager, controller_client)
        eh_mock.assert_called_once_with()
        sc_mock.assert_called_once_with(format='yaml')
        wh_mock.assert_called_once_with()
        self.assertEqual(machines, bs_manager.known_hosts)

    def test_assess_ha_recovery(self):
        client = fake_juju_client()
        bs_manager = Mock(client=client, known_hosts={}, has_controller=False)
        with patch.object(client, 'juju', autospec=True) as j_mock:
            with patch.object(client, 'get_status', autospec=True) as gs_mock:
                assess_ha_recovery(bs_manager, client)
        j_mock.assert_called_once_with('status', (), check=True, timeout=300)
        gs_mock.assert_called_once_with(timeout=300)
        self.assertIsTrue(bs_manager.has_controller)

    def test_assess_ha_recovery_status_error(self):
        client = fake_juju_client()
        bs_manager = Mock(client=client, known_hosts={}, has_controller=False)
        with patch.object(client, 'juju', autospec=True,
                          side_effect=CalledProcessError('foo', 'bar')):
            with self.assertRaises(HARecoveryError):
                assess_ha_recovery(bs_manager, client)
        self.assertIsFalse(bs_manager.has_controller)

    def test_assess_ha_recovery_get_status_error(self):
        client = fake_juju_client()
        bs_manager = Mock(client=client, known_hosts={}, has_controller=False)
        with patch.object(client, 'juju', autospec=True):
            with patch.object(client, 'get_status', autospec=True,
                              side_effect=CalledProcessError('foo', 'bar')):
                with self.assertRaises(HARecoveryError):
                    assess_ha_recovery(bs_manager, client)
        self.assertIsFalse(bs_manager.has_controller)


@patch('assess_recovery.wait_for_state_server_to_shutdown', autospec=True)
@patch('assess_recovery.terminate_instances', autospec=True)
class TestDeleteControllerMembers(FakeHomeTestCase):

    def test_delete_controller_members(self, ti_mock, wsss_mock):
        client = Mock(spec=['env', 'get_controller_members'])
        client.env = sentinel.env
        client.env.provider = 'lxd'
        client.get_controller_members.return_value = [
            Machine('3', {
                'dns-name': '10.0.0.3',
                'instance-id': 'juju-dddd-machine-3',
                'controller-member-status': 'has-vote'}),
            Machine('0', {
                'dns-name': '10.0.0.0',
                'instance-id': 'juju-aaaa-machine-0',
                'controller-member-status': 'has-vote'}),
            Machine('2', {
                'dns-name': '10.0.0.2',
                'instance-id': 'juju-cccc-machine-2',
                'controller-member-status': 'has-vote'}),
        ]
        known_hosts = {'0': '10.0.0.0', '2': '10.0.0.2', '3': '10.0.0.3'}
        bs_manager = Mock(
            client=client, known_hosts=known_hosts, has_controller=False)
        deleted = delete_controller_members(bs_manager, client)
        self.assertEqual(['2', '0', '3'], deleted)
        client.get_controller_members.assert_called_once_with()
        # terminate_instance was call in the reverse order of members.
        self.assertEqual(
            [call(client.env, ['juju-cccc-machine-2']),
             call(client.env, ['juju-aaaa-machine-0']),
             call(client.env, ['juju-dddd-machine-3'])],
            ti_mock.mock_calls)
        self.assertEqual(
            [call('10.0.0.2', client, 'juju-cccc-machine-2', timeout=120),
             call('10.0.0.0', client, 'juju-aaaa-machine-0', timeout=120),
             call('10.0.0.3', client, 'juju-dddd-machine-3', timeout=120)],
            wsss_mock.mock_calls)
        self.assertEqual(
            self.log_stream.getvalue(),
            'INFO Instrumenting node failure for member 2:'
            ' juju-cccc-machine-2 at 10.0.0.2\n'
            'INFO Instrumenting node failure for member 0:'
            ' juju-aaaa-machine-0 at 10.0.0.0\n'
            'INFO Instrumenting node failure for member 3:'
            ' juju-dddd-machine-3 at 10.0.0.3\n'
            "INFO Deleted ['2', '0', '3']\n")
        self.assertEqual({}, bs_manager.known_hosts)

    def test_delete_controller_members_leader_only(self, ti_mock, wsss_mock):
        client = Mock(spec=['env', 'get_controller_leader'])
        client.env = sentinel.env
        client.env.provider = 'lxd'
        client.get_controller_leader.return_value = Machine('3', {
            'dns-name': '10.0.0.3',
            'instance-id': 'juju-dddd-machine-3',
            'controller-member-status': 'has-vote'})
        known_hosts = {'1': 'a', '2': 'b', '3': '10.0.0.3'}
        bs_manager = Mock(
            client=client, known_hosts=known_hosts, has_controller=False)
        deleted = delete_controller_members(
            bs_manager, client, leader_only=True)
        self.assertEqual(['3'], deleted)
        client.get_controller_leader.assert_called_once_with()
        ti_mock.assert_called_once_with(client.env, ['juju-dddd-machine-3'])
        wsss_mock.assert_called_once_with(
            '10.0.0.3', client, 'juju-dddd-machine-3', timeout=120)
        self.assertEqual(
            self.log_stream.getvalue(),
            'INFO Instrumenting node failure for member 3:'
            ' juju-dddd-machine-3 at 10.0.0.3\n'
            "INFO Deleted ['3']\n")
        self.assertEqual({'1': 'a', '2': 'b'}, bs_manager.known_hosts)

    def test_delete_controller_members_azure(self, ti_mock, wsss_mock):
        client = Mock(spec=['env', 'get_controller_leader'])
        client.env = sentinel.env
        client.env.provider = 'azure'
        client.get_controller_leader.return_value = Machine('3', {
            'dns-name': '10.0.0.3',
            'instance-id': 'juju-dddd-machine-3',
            'controller-member-status': 'has-vote'})
        known_hosts = {'1': 'a', '2': 'b', '3': '10.0.0.3'}
        bs_manager = Mock(
            client=client, known_hosts=known_hosts, has_controller=False)
        with patch('assess_recovery.convert_to_azure_ids', autospec=True,
                   return_value=['juju-azure-id']):
            deleted = delete_controller_members(
                bs_manager, client, leader_only=True)
        self.assertEqual(['3'], deleted)
        client.get_controller_leader.assert_called_once_with()
        ti_mock.assert_called_once_with(client.env, ['juju-azure-id'])
        wsss_mock.assert_called_once_with(
            '10.0.0.3', client, 'juju-azure-id', timeout=120)
        self.assertEqual(
            self.log_stream.getvalue(),
            'INFO Instrumenting node failure for member 3:'
            ' juju-azure-id at 10.0.0.3\n'
            "INFO Deleted ['3']\n")
        self.assertEqual({'1': 'a', '2': 'b'}, bs_manager.known_hosts)

    def test_leader_only_has_controller(self, ti_mock, wsss_mock):
        client = fake_juju_client()
        bs_manager = BootstrapManager('foo', client, client, None, [], None,
                                      None, None, None, None, None, None,
                                      None)
        client.bootstrap()
        bs_manager.has_controller = True
        delete_controller_members(
            bs_manager, client.get_controller_client(), leader_only=True)
        self.assertIs(True, bs_manager.has_controller)

    def test_no_leader_only_has_controller(self, ti_mock, wsss_mock):
        client = fake_juju_client()
        bs_manager = BootstrapManager('foo', client, client, None, [], None,
                                      None, None, None, None, None, None,
                                      None)
        client.bootstrap()
        bs_manager.has_controller = True
        delete_controller_members(
            bs_manager, client.get_controller_client(), leader_only=False)
        self.assertIs(False, bs_manager.has_controller)


class TestRestoreMissingStateServer(FakeHomeTestCase):

    def test_restore_missing_state_server_with_check_controller(self):
        client = Mock(spec=['env', 'set_config', 'wait_for_started',
                            'wait_for_workloads'])
        controller_client = Mock(spec=['restore_backup', 'wait_for_started'])
        with patch('assess_recovery.check_token',
                   autospec=True, return_value='Token: Two'):
            with patch('assess_recovery.show_controller', autospec=True):
                restore_missing_state_server(
                    client, controller_client, 'backup_file',
                    check_controller=True)
        controller_client.restore_backup.assert_called_once_with('backup_file')
        controller_client.wait_for_started.assert_called_once_with(600)
        client.set_config.assert_called_once_with(
            'dummy-source', {'token': 'Two'})
        client.wait_for_started.assert_called_once_with()
        client.wait_for_workloads.assert_called_once_with()

    def test_restore_missing_state_server_without_check_controller(self):
        client = Mock(spec=['env', 'set_config', 'wait_for_started',
                            'wait_for_workloads'])
        controller_client = Mock(spec=['restore_backup', 'wait_for_started'])
        with patch('assess_recovery.check_token',
                   autospec=True, return_value='Token: Two'):
            with patch('assess_recovery.show_controller', autospec=True):
                restore_missing_state_server(
                    client, controller_client, 'backup_file',
                    check_controller=False)
        self.assertEqual(0, controller_client.wait_for_started.call_count)


class TestCheckToken(TestCase):

    def test_check_token_found(self):
        client = Mock()
        with patch('assess_recovery.get_token_from_status', autospec=True,
                   side_effect=['Token: foo']):
            found = check_token(client, 'foo')
        self.assertEqual('Token: foo', found)

    def test_check_token_none_before_found(self):
        client = Mock()
        with patch('assess_recovery.get_token_from_status', autospec=True,
                   side_effect=[None, 'foo']):
            found = check_token(client, 'foo')
        self.assertEqual('foo', found)

    def test_check_token_other_before_found(self):
        client = Mock()
        with patch('assess_recovery.get_token_from_status', autospec=True,
                   side_effect=['Starting', 'foo']):
            found = check_token(client, 'foo')
        self.assertEqual('foo', found)

    def test_check_token_not_found(self):
        client = Mock()
        with patch('assess_recovery.get_token_from_status', autospec=True,
                   return_value='other'):
            with patch('assess_recovery.until_timeout', autospec=True,
                       side_effect=['1', '0']):
                with self.assertRaises(JujuAssertionError):
                    check_token(client, 'foo')


class TestDetectBootstrapMachine(TestCase):

    def test_no_error(self):
        fake_manager = object()
        with detect_bootstrap_machine(fake_manager):
            pass

    def test_error_with_address(self):
        fake_manager = Mock(spec_set=['known_hosts'])
        fake_manager.known_hosts = {}
        error = Exception('Attempting to connect to 127.0.0.1:22')
        with self.assertRaises(Exception) as ctx:
            with detect_bootstrap_machine(fake_manager):
                raise error
        self.assertIs(ctx.exception, error)
        self.assertEqual(fake_manager.known_hosts, {'0': '127.0.0.1'})

    def test_error_without_address(self):
        fake_manager = Mock(spec_set=['known_hosts'])
        fake_manager.known_hosts = {}
        error = Exception('Some other error')
        with self.assertRaises(Exception) as ctx:
            with detect_bootstrap_machine(fake_manager):
                raise error
        self.assertIs(ctx.exception, error)
        self.assertEqual(fake_manager.known_hosts, {})
