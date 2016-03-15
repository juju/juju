from argparse import Namespace
from mock import (
    call,
    patch,
)

from assess_recovery import (
    delete_controller_members,
    main,
    make_client_from_args,
    parse_args,
)
from jujupy import (
    EnvJujuClient,
    get_cache_path,
    JujuData,
    Machine,
    _temp_env as temp_env,
)
from tests import (
    FakeHomeTestCase,
    TestCase,
)


class TestParseArgs(TestCase):

    def test_parse_args(self):
        args = parse_args(['foo', 'bar', 'baz'])
        self.assertEqual(args.juju_path, 'foo')
        self.assertEqual(args.env_name, 'bar')
        self.assertEqual(args.logs, 'baz')
        self.assertEqual(args.charm_prefix, '')
        self.assertEqual(args.strategy, 'backup')
        self.assertEqual(args.debug, False)
        self.assertIs(args.agent_stream, None)
        self.assertIs(args.series, None)

    def test_parse_args_ha(self):
        args = parse_args(['foo', 'bar', 'baz', '--ha'])
        self.assertEqual(args.strategy, 'ha')

    def test_parse_args_ha_backup(self):
        args = parse_args(['foo', 'bar', 'baz', '--ha-backup'])
        self.assertEqual(args.strategy, 'ha-backup')

    def test_parse_args_backup(self):
        args = parse_args(['foo', 'bar', 'baz', '--ha', '--backup'])
        self.assertEqual(args.strategy, 'backup')

    def test_parse_args_charm_prefix(self):
        args = parse_args(['foo', 'bar', 'baz', '--charm-prefix', 'qux'])
        self.assertEqual(args.charm_prefix, 'qux')

    def test_parse_args_debug(self):
        args = parse_args(['foo', 'bar', 'baz', '--debug'])
        self.assertEqual(args.debug, True)

    def test_parse_args_temp_env_name(self):
        args = parse_args(['foo', 'bar', 'baz'])
        self.assertIs(args.temp_env_name, None)
        args = parse_args(['foo', 'bar', 'baz', 'qux'])
        self.assertEqual(args.temp_env_name, 'qux')

    def test_parse_args_agent_stream(self):
        args = parse_args(['foo', 'bar', 'baz', '--agent-stream', 'qux'])
        self.assertEqual(args.agent_stream, 'qux')

    def test_parse_args_series(self):
        args = parse_args(['foo', 'bar', 'baz', '--series', 'qux'])
        self.assertEqual(args.series, 'qux')


class TestMakeClientFromArgs(TestCase):

    def test_make_client_from_args(self):
        with temp_env({'environments': {'foo': {}}}):
            with patch.object(EnvJujuClient, 'get_version', return_value=''):
                with patch.object(JujuData, 'load_yaml'):
                    client = make_client_from_args(
                        Namespace(env_name='foo', juju_path='bar',
                                  temp_env_name='temp-foo', debug=False,
                                  agent_stream=None, series=None))
        self.assertEqual(client.env.config, {'name': 'temp-foo'})
        self.assertEqual(client.env.environment, 'temp-foo')


def make_mocked_client(name, status_error=None):
    client = EnvJujuClient(JujuData(
        name, {'type': 'paas', 'region': 'region-foo'}), '1.23', 'path')
    patch.object(client, 'wait_for_ha', autospec=True).start()
    patch.object(
        client, 'get_status', autospec=True, side_effect=status_error).start()
    patch.object(client, 'kill_controller', autospec=True).start()
    patch.object(client, 'is_jes_enabled', autospec=True,
                 return_value=True).start()
    patch.object(client, 'get_admin_client', autospec=True).start()
    return client


@patch('deploy_stack.dump_env_logs_known_hosts', autospec=True)
@patch('assess_recovery.parse_new_state_server_from_error', autospec=True,
       return_value='new_host')
@patch('assess_recovery.delete_controller_members', autospec=True,
       return_value=['0'])
@patch('assess_recovery.deploy_stack', autospec=True)
@patch('deploy_stack.get_machine_dns_name', autospec=True,
       return_value='host')
@patch('subprocess.check_output', autospec=True)
@patch('subprocess.check_call', autospec=True)
@patch('sys.stderr', autospec=True)
class TestMain(FakeHomeTestCase):

    def test_ha(self, so_mock, cc_mock, co_mock,
                dns_mock, ds_mock, dcm_mock, ns_mock, dl_mock):
        client = make_mocked_client('foo')
        with patch('assess_recovery.make_client_from_args', autospec=True,
                   return_value=client) as mc_mock:
            main(['./', 'foo', 'log_dir',
                  '--ha', '--charm-prefix', 'prefix'])
        mc_mock.assert_called_once_with(Namespace(
            agent_stream=None, charm_prefix='prefix', debug=False,
            env_name='foo', juju_path='./', logs='log_dir', strategy='ha',
            temp_env_name=None, series=None))
        client.wait_for_ha.assert_called_once_with()
        client.get_status.assert_called_once_with(600)
        self.assertEqual(2, client.kill_controller.call_count)
        dns_mock.assert_called_once_with(client.get_admin_client.return_value,
                                         '0')
        ds_mock.assert_called_once_with(client, 'prefix')
        dcm_mock.assert_called_once_with(client, leader_only=True)
        cache_path = get_cache_path(client.env.juju_home, models=True)
        dl_mock.assert_called_once_with(client, 'log_dir', cache_path, {})
        self.assertEqual(0, ns_mock.call_count)

    def test_ha_error(self, so_mock, cc_mock, co_mock,
                      dns_mock, ds_mock, dcm_mock, ns_mock, dl_mock):
        error = Exception()
        client = make_mocked_client('foo', status_error=error)
        with patch('assess_recovery.make_client_from_args', autospec=True,
                   return_value=client) as mc_mock:
            with self.assertRaises(SystemExit):
                    main(['./', 'foo', 'log_dir',
                          '--ha', '--charm-prefix', 'prefix'])
        mc_mock.assert_called_once_with(Namespace(
            agent_stream=None, charm_prefix='prefix', debug=False,
            env_name='foo', juju_path='./', logs='log_dir', strategy='ha',
            temp_env_name=None, series=None))
        client.wait_for_ha.assert_called_once_with()
        client.get_status.assert_called_once_with(600)
        self.assertEqual(2, client.kill_controller.call_count)
        dns_mock.assert_called_once_with(
            client.get_admin_client.return_value, '0')
        ds_mock.assert_called_once_with(client, 'prefix')
        dcm_mock.assert_called_once_with(client, leader_only=True)
        ns_mock.assert_called_once_with(error)
        cache_path = get_cache_path(client.env.juju_home, models=True)
        dl_mock.assert_called_once_with(client, 'log_dir', cache_path,
                                        {'0': 'new_host'})

    def test_destroy_on_boot_error(self, so_mock, cc_mock, co_mock,
                                   dns_mock, ds_mock, dcm_mock, ns_mock,
                                   dl_mock):
        client = make_mocked_client('foo')
        with patch('assess_recovery.make_client', autospec=True,
                   return_value=client):
            with patch.object(client, 'bootstrap', side_effect=Exception):
                with self.assertRaises(SystemExit):
                    main(['./', 'foo', 'log_dir',
                          '--ha', '--charm-prefix', 'prefix'])
        self.assertEqual(2, client.kill_controller.call_count)


@patch('assess_recovery.wait_for_state_server_to_shutdown', autospec=True)
@patch('assess_recovery.terminate_instances', autospec=True)
@patch('sys.stderr', autospec=True)
class TestDeleteControllerMembers(FakeHomeTestCase):

    def test_delete_controller_members(self, so_mock, ti_mock, wsss_mock):
        client = make_mocked_client('foo')
        members = [
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
        with patch.object(client, 'get_controller_members',
                          autospec=True, return_value=members) as gcm_mock:
            deleted = delete_controller_members(client)
        self.assertEqual(['2', '0', '3'], deleted)
        gcm_mock.assert_called_once_with()
        # terminate_instance was call in the reverse order of members.
        self.assertEqual(
            [call(client.env, ['juju-cccc-machine-2']),
             call(client.env, ['juju-aaaa-machine-0']),
             call(client.env, ['juju-dddd-machine-3'])],
            ti_mock.mock_calls)
        self.assertEqual(
            [call('10.0.0.2', client, 'juju-cccc-machine-2'),
             call('10.0.0.0', client, 'juju-aaaa-machine-0'),
             call('10.0.0.3', client, 'juju-dddd-machine-3')],
            wsss_mock.mock_calls)

    def test_delete_controller_members_leader_only(
            self, so_mock, ti_mock, wsss_mock):
        client = make_mocked_client('foo')
        leader = Machine('3', {
            'dns-name': '10.0.0.3',
            'instance-id': 'juju-dddd-machine-3',
            'controller-member-status': 'has-vote'})
        with patch.object(client, 'get_controller_leader',
                          autospec=True, return_value=leader) as gcl_mock:
            deleted = delete_controller_members(client, leader_only=True)
        self.assertEqual(['3'], deleted)
        gcl_mock.assert_called_once_with()
        ti_mock.assert_called_once_with(client.env, ['juju-dddd-machine-3'])
        wsss_mock.assert_called_once_with(
            '10.0.0.3', client, 'juju-dddd-machine-3')
