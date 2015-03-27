from mock import (
    patch
)
import subprocess
from unittest import TestCase

from jujupy import (
    EnvJujuClient,
    SimpleEnvironment,
    )
from cs_staging_deploy import CSStagingTest


class TestCSStagingDeploy(TestCase):

    def test_from_args(self):
        side_effect = lambda x, y=None, debug=False: (x, y)
        with patch('jujupy.EnvJujuClient.by_version', side_effect=side_effect):
            with patch('jujupy.SimpleEnvironment.from_config',
                       side_effect=lambda x: SimpleEnvironment(x, {})):
                csstaging = CSStagingTest.from_args(
                    'base_env', 'temp_env_name', '/foo/bin/juju', '/tmp/tmp',
                    '0.0.0.0'
                )
        self.assertIs(type(csstaging), CSStagingTest)
        self.assertEqual(csstaging.client[0].environment, 'temp_env_name')
        self.assertIs(csstaging.client[1], '/foo/bin/juju')
        self.assertEqual(csstaging.log_dir, '/tmp/tmp')
        self.assertEqual(csstaging.charm_store_ip, '0.0.0.0')

    def test_from_args_agent_url(self):
        side_effect = lambda x, y=None, debug=False: (x, y)
        with patch('jujupy.EnvJujuClient.by_version', side_effect=side_effect):
            with patch('jujupy.SimpleEnvironment.from_config',
                       side_effect=lambda x: SimpleEnvironment(x, {})):
                csstaging = CSStagingTest.from_args(
                    'base_env', 'temp_env_name', '/foo/bin/juju', '/tmp/tmp',
                    '0.0.0.0', agent_url='http://agent_url.com'
                )
        self.assertEqual(csstaging.client[0].config['agent_url'],
                         'http://agent_url.com')

    def test_from_args_series(self):
        side_effect = lambda x, y=None, debug=False: (x, y)
        with patch('jujupy.EnvJujuClient.by_version', side_effect=side_effect):
            with patch('jujupy.SimpleEnvironment.from_config',
                       side_effect=lambda x: SimpleEnvironment(x, {})):
                csstaging = CSStagingTest.from_args(
                    'base_env', 'temp_env_name', '/foo/bin/juju', '/tmp/tmp',
                    '0.0.0.0', series='precise'
                )
        self.assertEqual(csstaging.client[0].config['series'],
                         'precise')

    def test_from_args_debug(self):
        with patch('jujupy.EnvJujuClient.get_version',
                   side_effect=lambda x, juju_path=None: ''):
            with patch('jujupy.SimpleEnvironment.from_config',
                       side_effect=lambda x: SimpleEnvironment(x, {})):
                csstaging = CSStagingTest.from_args(
                    'base_env', 'temp_env_name', '/foo/bin/juju', '/tmp/tmp',
                    '0.0.0.0', debug_flag=True
                )
        self.assertEqual(csstaging.client.debug, True)

    def test_run_finally(self):
        client = EnvJujuClient(
            SimpleEnvironment('foo', {'type': 'local'}), '1.234-76', None)
        csstaging = CSStagingTest(client, '0.0.0.0', '/tmp/logs')
        with patch.object(client, 'destroy_environment') as qs_mock:
            with patch('cs_staging_deploy.safe_print_status') as ps_mock:
                with patch.object(csstaging, 'bootstrap'):
                    with patch.object(csstaging, 'remote_run'):
                        with patch('jujupy.EnvJujuClient.deploy'):
                            with patch(
                                    'jujupy.EnvJujuClient.wait_for_started'):
                                csstaging.run()
        qs_mock.assert_called_once_with(delete_jenv=True)
        ps_mock.assert_called_once_with(client)

    def test_run_exception(self):
        client = EnvJujuClient(
            SimpleEnvironment('foo', {'type': 'local'}), '1.234-76', None)
        csstaging = CSStagingTest(client, '0.0.0.0', '/tmp/logs')
        with patch.object(client, 'destroy_environment') as qs_mock:
            with patch('cs_staging_deploy.safe_print_status') as ps_mock:
                with patch('cs_staging_deploy.dump_env_logs') as dl_mock:
                    with patch('cs_staging_deploy.bootstrap_from_env'):
                        with patch('cs_staging_deploy.get_machine_dns_name',
                                   return_value='foo'):
                            with patch.object(csstaging, 'remote_run',
                                              side_effect=Exception()):
                                csstaging.run()
        dl_mock.assert_called_once_with(client, 'foo', '/tmp/logs')
        qs_mock.assert_called_once_with(delete_jenv=True)
        ps_mock.assert_called_once_with(client)

    def test_remote_run(self):
        client = EnvJujuClient(
            SimpleEnvironment('foo', {'type': 'local'}), '1.234-76', None)
        csstaging = CSStagingTest(client, '0.0.0.0', '/tmp/logs')
        with patch('jujupy.EnvJujuClient.get_juju_output') as out_mock:
            csstaging.remote_run('machine', 'some_cmd')
        out_mock.assert_called_once_with('ssh', 'machine', 'some_cmd')

    def test_remote_run_exception(self):
        client = EnvJujuClient(
            SimpleEnvironment('foo', {'type': 'local'}), '1.234-76', None)
        csstaging = CSStagingTest(client, '0.0.0.0', '/tmp/logs')
        with self.assertRaises(subprocess.CalledProcessError):
            with patch('jujupy.EnvJujuClient.get_juju_output',
                       side_effect=subprocess.CalledProcessError(1, 'cmd')):
                csstaging.remote_run('machine', 'some_cmd')
