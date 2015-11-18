from argparse import Namespace
from contextlib import contextmanager
import logging
from mock import (
    call,
    patch,
)
from unittest import TestCase

from jujupy import (
    EnvJujuClient,
    SimpleEnvironment,
)
from scale_out import (
    deploy_charms,
    get_service_name,
    parse_args,
    scale_out,
    scaleout_setup,
)


def fake_SimpleEnvironment(name):
    return SimpleEnvironment(name, {})


def fake_EnvJujuClient(env, path=None, debug=None):
    return EnvJujuClient(env=env, version='1.2.3.4', full_path=path)


class TestScaleOut(TestCase):

    @contextmanager
    def fake_client_cxt(self):
        env = fake_SimpleEnvironment('foo')
        client = fake_EnvJujuClient(env)
        bv_cxt = patch('jujupy.EnvJujuClient.by_version',
                       return_value=client)
        fc_cxt = patch('jujupy.SimpleEnvironment.from_config',
                       return_value=env)
        with bv_cxt, fc_cxt:
            yield (client, env)

    def test_parse_args(self):
        args = parse_args(
            ['env', '/path/juju', 'logs', 'temp_name', 'charms'])
        expected = Namespace(
            agent_stream=None,
            agent_url=None,
            bootstrap_host=None,
            charms='charms',
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
            verbose=logging.INFO
        )
        self.assertEqual(args, expected)

    @patch('scale_out.boot_context', autospec=True)
    @patch('scale_out.EnvJujuClient.add_ssh_machines', autospec=True)
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
            verbose=logging.INFO
        )

        with self.fake_client_cxt() as (fake_client, fake_env):
            with scaleout_setup(args) as client:
                # Get a reference to the by_version function that was patched
                # in fake_client_cxt()
                bv_mock = EnvJujuClient.by_version
                pass

        # Test that boot_context was called with expected args
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
            permanent=False,
            region=args.region)

        # Test that the expected args were passed when creating the env.
        bv_mock.assert_called_once_with(fake_env, '/path/juju', False)

        # Test that client.add_ssh_machines is called with expected args
        add_ssh_machines_func.assert_called_once_with(client, ['0'])

        # Test that the expected client type is yielded
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
            verbose=logging.INFO
        )

        with self.fake_client_cxt():
            with patch('scale_out.boot_context', autospec=True) as bc_mock:
                with patch('scale_out.EnvJujuClient.add_ssh_machines',
                           autospec=True):
                    with scaleout_setup(args) as client:
                        pass

        # Test that boot_context was called with series given in args.
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
            permanent=False,
            region=args.region)

    def test_deploy_charms(self):
        with self.fake_client_cxt() as (client, env):
            with patch.object(EnvJujuClient, 'deploy') as d_mock:
                with patch.object(EnvJujuClient,
                                  'wait_for_started') as wfs_mock:
                    deploy_charms(client, ['ubuntu', 'mysql'])
        # Test client.deploy was called for each charm.
        expected = [call('ubuntu', service='ubuntu'),
                    call('mysql', service='mysql')]
        self.assertEqual(d_mock.mock_calls, expected)
        # Test client.wait_for_started was called.
        wfs_mock.assert_called_once_with()

    def test_scale_out(self):
        with self.fake_client_cxt() as (client, env):
            with patch.object(EnvJujuClient, 'juju') as j_mock:
                with patch.object(EnvJujuClient,
                                  'wait_for_started') as wfs_mock:
                    scale_out(client, 'ubuntu')
        # Test client.juju was called with expected args.
        j_mock.assert_called_once_with('add-unit', ('ubuntu', '-n', '5'))
        # Test client.wait_for_started was called.
        wfs_mock.assert_called_once_with()

    def test_get_service_name(self):
        charms = [('charm-name', 'charm-name'),
                  ('charm-name-21', 'charm-name-21'),
                  ('series/charm-name-13', 'charm-name-13'),
                  ('local:charm-name', 'charm-name'),
                  ('cs:~user/charm-name', 'charm-name'),
                  ('lp:~user/some/path/to/charm-name', 'charm-name')]
        for charm, expected in charms:
            result = get_service_name(charm)
            self.assertEqual(result, expected)
