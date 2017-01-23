from argparse import Namespace
from contextlib import contextmanager
import logging
from mock import (
    call,
    patch,
)
from unittest import TestCase

from jujupy import (
    ModelClient,
    JujuData,
)
from scale_out import (
    deploy_charms,
    get_service_name,
    parse_args,
    scale_out,
    scaleout_setup,
)


def fake_ModelClient(env, path=None, debug=None):
    return ModelClient(env=env, version='1.2.3.4', full_path=path)


class TestScaleOut(TestCase):

    @contextmanager
    def fake_client_cxt(self):
        env = JujuData('foo', {})
        client = fake_ModelClient(env)
        bv_cxt = patch('scale_out.client_from_config',
                       return_value=client)
        with bv_cxt as bv_mock:
            yield (client, env, bv_mock)

    def test_parse_args(self):
        args = parse_args(
            ['env', '/path/juju', 'logs', 'temp_name', 'foo', 'bar'])
        expected = Namespace(
            agent_stream=None,
            agent_url=None,
            bootstrap_host=None,
            charms=['foo', 'bar'],
            debug=False,
            env='env',
            juju_bin='/path/juju',
            keep_env=False,
            logs='logs',
            machine=[],
            region=None,
            series=None,
            temp_env_name='temp_name',
            upload_tools=False,
            verbose=logging.INFO,
            deadline=None,
        )
        self.assertEqual(args, expected)

    @patch('scale_out.boot_context', autospec=True)
    @patch('jujupy.ModelClient.add_ssh_machines', autospec=True)
    def test_scaleout_setup(
            self,
            add_ssh_machines_func,
            boot_context_func):

        args = Namespace(
            agent_stream=None,
            agent_url=None,
            bootstrap_host=None,
            charms='ubuntu',
            debug=False,
            env='test_env',
            juju_bin='/path/juju',
            keep_env=False,
            logs='/tmp/logs',
            machine=['0'],
            region=None,
            series=None,
            temp_env_name='temp_name',
            upload_tools=False,
            verbose=logging.INFO,
            deadline=None,
        )

        with self.fake_client_cxt() as (fake_client, fake_env, bv_mock):
            with scaleout_setup(args) as client:
                pass
        boot_context_func.assert_called_once_with(
            args.temp_env_name,
            client,
            args.bootstrap_host,
            args.machine,
            'trusty',
            args.agent_url,
            args.agent_stream,
            args.logs,
            args.keep_env,
            args.upload_tools,
            region=args.region)
        bv_mock.assert_called_once_with('test_env', '/path/juju', False,
                                        soft_deadline=None)
        add_ssh_machines_func.assert_called_once_with(client, ['0'])
        self.assertIs(client, fake_client)

    def test_scaleout_setup_sets_series(self):

        args = Namespace(
            agent_stream=None,
            agent_url=None,
            bootstrap_host=None,
            charms='ubuntu some_other_charm',
            debug=False,
            env='test_env',
            juju_bin='/path/juju',
            keep_env=False,
            logs='/tmp/logs',
            machine=['0'],
            region=None,
            series='my_series',
            temp_env_name='temp_name',
            upload_tools=False,
            verbose=logging.INFO,
            deadline=None,
        )

        with self.fake_client_cxt():
            with patch('scale_out.boot_context', autospec=True) as bc_mock:
                with patch('jujupy.ModelClient.add_ssh_machines',
                           autospec=True):
                    with scaleout_setup(args) as client:
                        pass
        bc_mock.assert_called_once_with(
            args.temp_env_name,
            client,
            args.bootstrap_host,
            args.machine,
            'my_series',
            args.agent_url,
            args.agent_stream,
            args.logs,
            args.keep_env,
            args.upload_tools,
            region=args.region)

    def test_deploy_charms(self):
        with self.fake_client_cxt() as (client, env, bv_mock):
            with patch.object(ModelClient, 'deploy') as d_mock:
                with patch.object(ModelClient,
                                  'wait_for_started') as wfs_mock:
                    deploy_charms(client, ['ubuntu', 'mysql'])
        expected = [call('ubuntu', service='ubuntu'),
                    call('mysql', service='mysql')]
        self.assertEqual(d_mock.mock_calls, expected)
        wfs_mock.assert_called_once_with()

    def test_deploy_charms_local(self):
        with self.fake_client_cxt() as (client, env, bv_mock):
            with patch.object(ModelClient, 'deploy') as d_mock:
                with patch.object(ModelClient,
                                  'wait_for_started') as wfs_mock:
                    deploy_charms(client, ['local:foo', 'local:bar'])
        expected = [call('local:foo', service='foo'),
                    call('local:bar', service='bar')]
        self.assertEqual(d_mock.mock_calls, expected)
        wfs_mock.assert_called_once_with()

    def test_scale_out(self):
        with self.fake_client_cxt() as (client, env, bv_mock):
            with patch.object(ModelClient, 'juju') as j_mock:
                with patch.object(ModelClient,
                                  'wait_for_started') as wfs_mock:
                    scale_out(client, 'ubuntu')
        j_mock.assert_called_once_with('add-unit', ('ubuntu', '-n', '5'))
        wfs_mock.assert_called_once_with()

    def test_get_service_name(self):
        charms = [('charm-name', 'charm-name'),
                  ('charm-name-21', 'charm-name-21'),
                  ('series/charm-name-13', 'charm-name-13'),
                  ('local:charm-name', 'charm-name'),
                  ('cs:~user/charm-name', 'charm-name'),
                  ('cs:charm-name-2-1', 'charm-name-2-1'),
                  ('lp:~user/some/path/to/charm-name', 'charm-name')]
        for charm, expected in charms:
            result = get_service_name(charm)
            self.assertEqual(result, expected)
