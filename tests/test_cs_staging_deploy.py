from contextlib import contextmanager
from mock import (
    patch
)
import subprocess

from jujupy import (
    EnvJujuClient,
    SimpleEnvironment,
    )
from cs_staging_deploy import CSStagingTest
from tests import (
    FakeHomeTestCase,
)


def fake_EnvJujuClient_by_version(env, path=None, debug=None):
    return env, path


def fake_SimpleEnvironment_from_config(name):
    return SimpleEnvironment(name, {})


class TestCSStagingDeploy(FakeHomeTestCase):

    @contextmanager
    def from_args_cxt(self):
        with patch('jujupy.EnvJujuClient.by_version',
                   side_effect=fake_EnvJujuClient_by_version):
            with patch('jujupy.SimpleEnvironment.from_config',
                       side_effect=fake_SimpleEnvironment_from_config):
                yield

    @contextmanager
    def from_args_cxt2(self):
        with patch('jujupy.EnvJujuClient.get_version',
                   side_effect=lambda x, juju_path=None: ''):
            with patch('jujupy.SimpleEnvironment.from_config',
                       side_effect=fake_SimpleEnvironment_from_config):
                yield

    def test_from_args(self):
        with self.from_args_cxt():
            csstaging = CSStagingTest.from_args(
                'base_env', 'temp_env_name', '/foo/bin/juju', '/tmp/tmp',
                '0.0.0.0'
            )
        self.assertIs(type(csstaging), CSStagingTest)
        self.assertEqual(csstaging.client[0].environment, 'temp_env_name')
        self.assertIs(csstaging.client[1], '/foo/bin/juju')
        self.assertEqual(csstaging.log_dir, '/tmp/tmp')
        self.assertEqual(csstaging.charm_store_ip, '0.0.0.0')
        self.assertEqual(csstaging.client[0].config, {
            'name': 'temp_env_name',
            })

    def test_from_args_agent_url(self):
        with self.from_args_cxt():
            csstaging = CSStagingTest.from_args(
                'base_env', 'temp_env_name', '/foo/bin/juju', '/tmp/tmp',
                '0.0.0.0', agent_url='http://agent_url.com'
            )
        self.assertEqual(csstaging.client[0].config['tools-metadata-url'],
                         'http://agent_url.com')

    def test_from_args_agent_stream(self):
        with self.from_args_cxt2():
            csstaging = CSStagingTest.from_args(
                'base_env', 'temp_env_name', '/foo/bin/juju', '/tmp/tmp',
                '0.0.0.0', agent_stream='stream-foo',
            )
        self.assertEqual(csstaging.client.env.config['agent-stream'],
                         'stream-foo')

    def test_from_args_series(self):
        with self.from_args_cxt():
            csstaging = CSStagingTest.from_args(
                'base_env', 'temp_env_name', '/foo/bin/juju', '/tmp/tmp',
                '0.0.0.0', series='precise'
            )
        self.assertEqual(csstaging.client[0].config['default-series'],
                         'precise')

    def test_from_args_charm(self):
        with self.from_args_cxt():
            csstaging = CSStagingTest.from_args(
                'base_env', 'temp_env_name', '/foo/bin/juju', '/tmp/tmp',
                '0.0.0.0', charm='some_charm'
            )
        self.assertEqual(csstaging.charm, 'some_charm')

    def test_from_args_debug(self):
        with self.from_args_cxt2():
            csstaging = CSStagingTest.from_args(
                'base_env', 'temp_env_name', '/foo/bin/juju', '/tmp/tmp',
                '0.0.0.0', debug_flag=True
            )
        self.assertEqual(csstaging.client.debug, True)

    def test_from_args_region(self):
        with self.from_args_cxt2():
            csstaging = CSStagingTest.from_args(
                'base_env', 'temp_env_name', '/foo/bin/juju', '/tmp/tmp',
                '0.0.0.0', region='region-foo',
            )
        self.assertEqual(csstaging.client.env.config['region'], 'region-foo')

    def test_from_args_bootstrap_host(self):
        with self.from_args_cxt2():
            csstaging = CSStagingTest.from_args(
                'base_env', 'temp_env_name', '/foo/bin/juju', '/tmp/tmp',
                '0.0.0.0', bootstrap_host='host-foo',
            )
        self.assertEqual(csstaging.client.env.config['bootstrap-host'],
                         'host-foo')

    def test_run(self):
        client = EnvJujuClient(
            SimpleEnvironment('foo', {'type': 'local'}), '1.234-76', None)
        csstaging = CSStagingTest(client, '0.0.0.0', 'charm', '/tmp/logs')
        with patch.object(client, 'destroy_environment') as qs_mock:
            with patch('cs_staging_deploy.safe_print_status') as ps_mock:
                with patch.object(csstaging, 'bootstrap') as bs_mock:
                    with patch.object(csstaging, 'remote_run') as rr_mock:
                        with patch('jujupy.EnvJujuClient.deploy'):
                            with patch(
                                    'jujupy.EnvJujuClient.wait_for_started'):
                                csstaging.run()
        bs_mock.assert_called_once_with()
        rr_call = (
            '''sudo bash -c "echo '%s store.juju.ubuntu.com' >> /etc/hosts"'''
            % '0.0.0.0')
        rr_mock.assert_called_once_with('0', rr_call)
        qs_mock.assert_called_once_with(delete_jenv=True)
        ps_mock.assert_called_once_with(client)

    def test_run_exception(self):
        client = EnvJujuClient(
            SimpleEnvironment('foo', {'type': 'local'}), '1.234-76', None)
        csstaging = CSStagingTest(client, '0.0.0.0', 'charm', '/tmp/logs')
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
        csstaging = CSStagingTest(client, '0.0.0.0', 'charm', '/tmp/logs')
        with patch('jujupy.EnvJujuClient.get_juju_output') as out_mock:
            csstaging.remote_run('machine', 'some_cmd')
        out_mock.assert_called_once_with('ssh', 'machine', 'some_cmd')

    def test_remote_run_exception(self):
        client = EnvJujuClient(
            SimpleEnvironment('foo', {'type': 'local'}), '1.234-76', None)
        csstaging = CSStagingTest(client, '0.0.0.0', 'charm', '/tmp/logs')
        with self.assertRaises(subprocess.CalledProcessError):
            with patch('jujupy.EnvJujuClient.get_juju_output',
                       side_effect=subprocess.CalledProcessError(1, 'cmd')):
                csstaging.remote_run('machine', 'some_cmd')

    def test_bootstrap(self):
        client = EnvJujuClient(
            SimpleEnvironment('foo', {'type': 'local'}), '1.234-76', None)
        csstaging = CSStagingTest(client, '0.0.0.0', 'charm', '/tmp/logs')
        with patch('cs_staging_deploy.get_juju_home',
                   return_value='foo') as gjh_mock:
            with patch('cs_staging_deploy.bootstrap_from_env') as bfe_mock:
                with patch(
                        'cs_staging_deploy.get_machine_dns_name') as dns_mock:
                    csstaging.bootstrap()
        gjh_mock.assert_called_once_with()
        bfe_mock.assert_called_once_with('foo', client)
        dns_mock.assert_called_once_with(client, '0')
