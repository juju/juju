from contextlib import contextmanager
from mock import patch
from unittest import TestCase

from assess_recovery import (
    main,
    parse_args,
)
from jujupy import (
    EnvJujuClient,
    SimpleEnvironment,
)
from utility import (
    setup_test_logging,
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


@contextmanager
def make_mocked_client(name, status_error=None):
    client = EnvJujuClient(SimpleEnvironment(
        name, {'type': 'paas'}), '1.23', 'path')
    with patch.object(client, 'wait_for_ha', autospec=True):
        with patch.object(client, 'get_status', autospec=True,
                          side_effect=status_error):
            with patch.object(client,
                              'destroy_environment', autospec=True):
                yield client


@patch('assess_recovery.dump_env_logs', autospec=True)
@patch('assess_recovery.parse_new_state_server_from_error', autospec=True,
       return_value='new_host')
@patch('assess_recovery.wait_for_state_server_to_shutdown', autospec=True)
@patch('assess_recovery.delete_instance', autospec=True)
@patch('assess_recovery.deploy_stack', autospec=True, return_value='i_id')
@patch('assess_recovery.get_machine_dns_name', autospec=True,
       return_value='host')
@patch('subprocess.check_output', autospec=True)
@patch('subprocess.check_call', autospec=True)
@patch('sys.stdout', autospec=True)
class TestMain(TestCase):

    def setUp(self):
        setup_test_logging(self)

    def test_ha(self, so_mock, cc_mock, co_mock,
                dns_mock, ds_mock, di_mock, ws_mock, ns_mock, dl_mock):
        with make_mocked_client('foo') as client:
            with patch('assess_recovery.make_client', autospec=True,
                       return_value=client) as mc_mock:
                main(['./', 'foo', 'log_dir',
                      '--ha', '--charm-prefix', 'prefix'])
                mc_mock.assert_called_once_with('./', False, 'foo', None)
                client.wait_for_ha.assert_called_once_with()
                client.get_status.assert_called_once_with(600)
                client.destroy_environment.assert_called_once_with()
        dns_mock.assert_called_once_with(client, 0)
        ds_mock.assert_called_once_with(client, 'prefix')
        di_mock.assert_called_once_with(client, 'i_id')
        ws_mock.assert_called_once_with('host', client, 'i_id')
        dl_mock.assert_called_once_with(client, None, 'log_dir')
        self.assertEqual(0, ns_mock.call_count)

    def test_ha_error(self, so_mock, cc_mock, co_mock,
                      dns_mock, ds_mock, di_mock, ws_mock, ns_mock, dl_mock):
        error = Exception()
        with make_mocked_client('foo', status_error=error) as client:
            with patch('assess_recovery.make_client', autospec=True,
                       return_value=client) as mc_mock:
                with self.assertRaises(SystemExit):
                    main(['./', 'foo', 'log_dir',
                          '--ha', '--charm-prefix', 'prefix'])
                    mc_mock.assert_called_once_with('./', False, 'foo', None)
                    client.wait_for_ha.assert_called_once_with()
                    client.get_status.assert_called_once_with(600)
                    client.destroy_environment.assert_called_once_with()
        dns_mock.assert_called_once_with(client, 0)
        ds_mock.assert_called_once_with(client, 'prefix')
        di_mock.assert_called_once_with(client, 'i_id')
        ws_mock.assert_called_once_with('host', client, 'i_id')
        ns_mock.assert_called_once_with(error)
        dl_mock.assert_called_once_with(client, 'new_host', 'log_dir')
