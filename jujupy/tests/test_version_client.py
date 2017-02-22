from contextlib import contextmanager
import copy
from datetime import (
    datetime,
    timedelta,
    )
import json
import os
import subprocess
import sys
from textwrap import dedent

from mock import (
    call,
    Mock,
    patch,
    )
import yaml

from jujupy import (
    AuthNotAccepted,
    fake_juju_client,
    InvalidEndpoint,
    JujuData,
    JUJU_DEV_FEATURE_FLAGS,
    JESNotSupported,
    ModelClient,
    NameNotAccepted,
    SimpleEnvironment,
    Status,
    _temp_env as temp_env,
    TypeNotAccepted,
    )
from jujupy.client import (
    CannotConnectEnv,
    GroupReporter,
    StatusItem,
    StatusNotMet,
    SYSTEM,
    UpgradeMongoNotSupported,
    WaitMachineNotPresent,
    )
from jujupy.configuration import get_jenv_path
from jujupy.fake import (
    FakeBackend2_1,
    )
from jujupy.version_client import (
    BootstrapMismatch,
    client_from_config,
    EnvJujuClient1X,
    EnvJujuClient22,
    EnvJujuClient24,
    EnvJujuClient25,
    ModelClientRC,
    IncompatibleConfigClass,
    Juju1XBackend,
    ModelClient2_0,
    ModelClient2_1,
    VersionNotTestedError,
    Status1X,
    )
from jujupy.tests.test_client import (
    ClientTest,
    )
from tests import (
    assert_juju_call,
    FakeHomeTestCase,
    FakePopen,
    observable_temp_file,
    TestCase
    )
from jujupy.utility import (
    get_timeout_path,
    )


class TestJuju1XBackend(TestCase):

    def test_full_args_model(self):
        backend = Juju1XBackend('/bin/path/juju', '1.25', set(), False, None)
        full = backend.full_args('help', ('commands',), 'test', None)
        self.assertEqual(('juju', '--show-log', 'help', '-e', 'test',
                          'commands'), full)


class TestClientFromConfig(ClientTest):

    @contextmanager
    def assertRaisesVersionNotTested(self, version):
        with self.assertRaisesRegexp(
                VersionNotTestedError, 'juju ' + version):
            yield

    @patch.object(JujuData, 'from_config', return_value=JujuData('', {}))
    @patch.object(SimpleEnvironment, 'from_config',
                  return_value=SimpleEnvironment('', {}))
    @patch.object(ModelClient, 'get_full_path', return_value='fake-path')
    def test_from_config(self, gfp_mock, se_fc_mock, jd_fc_mock):
        def juju_cmd_iterator():
            yield '1.17'
            yield '1.16'
            yield '1.16.1'
            yield '1.15'
            yield '1.22.1'
            yield '1.24-alpha1'
            yield '1.24.7'
            yield '1.25.1'
            yield '1.26.1'
            yield '1.27.1'
            yield '2.0-alpha1'
            yield '2.0-alpha2'
            yield '2.0-alpha3'
            yield '2.0-beta1'
            yield '2.0-beta2'
            yield '2.0-beta3'
            yield '2.0-beta4'
            yield '2.0-beta5'
            yield '2.0-beta6'
            yield '2.0-beta7'
            yield '2.0-beta8'
            yield '2.0-beta9'
            yield '2.0-beta10'
            yield '2.0-beta11'
            yield '2.0-beta12'
            yield '2.0-beta13'
            yield '2.0-beta14'
            yield '2.0-beta15'
            yield '2.0-rc1'
            yield '2.0-rc2'
            yield '2.0-rc3'
            yield '2.0-delta1'
            yield '2.1.0'
            yield '2.2.0'

        context = patch.object(
            ModelClient, 'get_version',
            side_effect=juju_cmd_iterator().send)
        with context:
            self.assertIs(EnvJujuClient1X,
                          type(client_from_config('foo', None)))

            def test_fc(version, cls):
                if cls is not None:
                    client = client_from_config('foo', None)
                    if isinstance(client, EnvJujuClient1X):
                        self.assertEqual(se_fc_mock.return_value, client.env)
                    else:
                        self.assertEqual(jd_fc_mock.return_value, client.env)
                    self.assertIs(cls, type(client))
                    self.assertEqual(version, client.version)
                else:
                    with self.assertRaisesVersionNotTested(version):
                        client_from_config('foo', None)

            test_fc('1.16', None)
            test_fc('1.16.1', None)
            test_fc('1.15', EnvJujuClient1X)
            test_fc('1.22.1', EnvJujuClient22)
            test_fc('1.24-alpha1', EnvJujuClient24)
            test_fc('1.24.7', EnvJujuClient24)
            test_fc('1.25.1', EnvJujuClient25)
            test_fc('1.26.1', None)
            test_fc('1.27.1', EnvJujuClient1X)
            test_fc('2.0-alpha1', None)
            test_fc('2.0-alpha2', None)
            test_fc('2.0-alpha3', None)
            test_fc('2.0-beta1', None)
            test_fc('2.0-beta2', None)
            test_fc('2.0-beta3', None)
            test_fc('2.0-beta4', None)
            test_fc('2.0-beta5', None)
            test_fc('2.0-beta6', None)
            test_fc('2.0-beta7', None)
            test_fc('2.0-beta8', None)
            test_fc('2.0-beta9', None)
            test_fc('2.0-beta10', None)
            test_fc('2.0-beta11', None)
            test_fc('2.0-beta12', None)
            test_fc('2.0-beta13', None)
            test_fc('2.0-beta14', None)
            test_fc('2.0-beta15', None)
            test_fc('2.0-rc1', ModelClientRC)
            test_fc('2.0-rc2', ModelClientRC)
            test_fc('2.0-rc3', ModelClientRC)
            test_fc('2.0-delta1', ModelClient2_0)
            test_fc('2.1.0', ModelClient2_1)
            test_fc('2.2.0', ModelClient)
            with self.assertRaises(StopIteration):
                client_from_config('foo', None)

    def test_client_from_config_path(self):
        with patch('subprocess.check_output', return_value=' 4.3') as vsn:
            with patch.object(JujuData, 'from_config'):
                client = client_from_config('foo', 'foo/bar/qux')
        vsn.assert_called_once_with(('foo/bar/qux', '--version'))
        self.assertNotEqual(client.full_path, 'foo/bar/qux')
        self.assertEqual(client.full_path, os.path.abspath('foo/bar/qux'))

    def test_client_from_config_keep_home(self):
        env = JujuData({}, juju_home='/foo/bar')
        with patch('subprocess.check_output', return_value='2.0.0'):
            with patch.object(JujuData, 'from_config',
                              side_effect=lambda x: JujuData(x, {})):
                client_from_config('foo', 'foo/bar/qux')
        self.assertEqual('/foo/bar', env.juju_home)

    def test_client_from_config_deadline(self):
        deadline = datetime(2012, 11, 10, 9, 8, 7)
        with patch('subprocess.check_output', return_value='2.0.0'):
            with patch.object(JujuData, 'from_config',
                              side_effect=lambda x: JujuData(x, {})):
                client = client_from_config(
                    'foo', 'foo/bar/qux', soft_deadline=deadline)
        self.assertEqual(client._backend.soft_deadline, deadline)


class TestModelClient2_1(ClientTest):

    client_version = '2.1.0'

    client_class = ModelClient2_1

    fake_backend_class = FakeBackend2_1

    def test_basics(self):
        self.check_basics()

    def test_add_cloud_interactive_maas(self):
        client = self.fake_juju_client()
        clouds = {'foo': {
            'type': 'maas',
            'endpoint': 'http://bar.example.com',
            }}
        client.add_cloud_interactive('foo', clouds['foo'])
        self.assertEqual(client._backend.clouds, clouds)

    def test_add_cloud_interactive_maas_invalid_endpoint(self):
        client = self.fake_juju_client()
        clouds = {'foo': {
            'type': 'maas',
            'endpoint': 'B' * 4000,
            }}
        with self.assertRaises(InvalidEndpoint):
            client.add_cloud_interactive('foo', clouds['foo'])

    def test_add_cloud_interactive_manual(self):
        client = self.fake_juju_client()
        clouds = {'foo': {'type': 'manual', 'endpoint': '127.100.100.1'}}
        client.add_cloud_interactive('foo', clouds['foo'])
        self.assertEqual(client._backend.clouds, clouds)

    def test_add_cloud_interactive_manual_invalid_endpoint(self):
        client = self.fake_juju_client()
        clouds = {'foo': {'type': 'manual', 'endpoint': 'B' * 4000}}
        with self.assertRaises(InvalidEndpoint):
            client.add_cloud_interactive('foo', clouds['foo'])

    def get_openstack_clouds(self):
        return {'foo': {
            'type': 'openstack',
            'endpoint': 'http://bar.example.com',
            'auth-types': ['oauth1', 'oauth12'],
            'regions': {
                'harvey': {'endpoint': 'http://harvey.example.com'},
                'steve': {'endpoint': 'http://steve.example.com'},
                }
            }}

    def test_add_cloud_interactive_openstack(self):
        client = self.fake_juju_client()
        clouds = self.get_openstack_clouds()
        client.add_cloud_interactive('foo', clouds['foo'])
        self.assertEqual(client._backend.clouds, clouds)

    def test_add_cloud_interactive_openstack_invalid_endpoint(self):
        client = self.fake_juju_client()
        clouds = self.get_openstack_clouds()
        clouds['foo']['endpoint'] = 'B' * 4000
        with self.assertRaises(InvalidEndpoint):
            client.add_cloud_interactive('foo', clouds['foo'])

    def test_add_cloud_interactive_openstack_invalid_region_endpoint(self):
        client = self.fake_juju_client()
        clouds = self.get_openstack_clouds()
        clouds['foo']['regions']['harvey']['endpoint'] = 'B' * 4000
        with self.assertRaises(InvalidEndpoint):
            client.add_cloud_interactive('foo', clouds['foo'])

    def test_add_cloud_interactive_openstack_invalid_auth(self):
        client = self.fake_juju_client()
        clouds = self.get_openstack_clouds()
        clouds['foo']['auth-types'] = ['invalid', 'oauth12']
        with self.assertRaises(AuthNotAccepted):
            client.add_cloud_interactive('foo', clouds['foo'])

    def test_add_cloud_interactive_vsphere(self):
        client = self.fake_juju_client()
        clouds = {'foo': {
            'type': 'vsphere',
            'endpoint': 'http://bar.example.com',
            'regions': {
                'harvey': {},
                'steve': {},
                }
            }}
        client.add_cloud_interactive('foo', clouds['foo'])
        self.assertEqual(client._backend.clouds, clouds)

    def test_add_cloud_interactive_bogus(self):
        client = self.fake_juju_client()
        clouds = {'foo': {'type': 'bogus'}}
        with self.assertRaises(TypeNotAccepted):
            client.add_cloud_interactive('foo', clouds['foo'])

    def test_add_cloud_interactive_invalid_name(self):
        client = self.fake_juju_client()
        cloud = {'type': 'manual', 'endpoint': 'example.com'}
        with self.assertRaises(NameNotAccepted):
            client.add_cloud_interactive('invalid/name', cloud)


class TestModelClientRC(ClientTest):

    def test_bootstrap(self):
        env = JujuData('foo', {'type': 'bar', 'region': 'baz'})
        with observable_temp_file() as config_file:
            with patch.object(ModelClientRC, 'juju') as mock:
                client = ModelClientRC(env, '2.0-zeta1', None)
                client.bootstrap()
                mock.assert_called_with(
                    'bootstrap', ('--constraints', 'mem=2G',
                                  'foo', 'bar/baz',
                                  '--config', config_file.name,
                                  '--default-model', 'foo',
                                  '--agent-version', '2.0'), include_e=False)
                config_file.seek(0)
                config = yaml.safe_load(config_file)
        self.assertEqual({'test-mode': True}, config)


class TestEnvJujuClient1X(ClientTest):

    def test_raise_on_juju_data(self):
        env = JujuData('foo', {'type': 'bar'}, 'baz')
        with self.assertRaisesRegexp(
                IncompatibleConfigClass,
                'JujuData cannot be used with EnvJujuClient1X'):
            EnvJujuClient1X(env, '1.25', 'full_path')

    def test_no_duplicate_env(self):
        env = SimpleEnvironment('foo', {})
        client = EnvJujuClient1X(env, '1.25', 'full_path')
        self.assertIs(env, client.env)

    def test_not_supported(self):
        client = EnvJujuClient1X(
            SimpleEnvironment('foo', {}), '1.25', 'full_path')
        with self.assertRaises(JESNotSupported):
            client.add_user_perms('test-user')
        with self.assertRaises(JESNotSupported):
            client.grant('test-user', 'read')
        with self.assertRaises(JESNotSupported):
            client.revoke('test-user', 'read')
        with self.assertRaises(JESNotSupported):
            client.get_model_uuid()

    def test_get_version(self):
        value = ' 5.6 \n'
        with patch('subprocess.check_output', return_value=value) as vsn:
            version = EnvJujuClient1X.get_version()
        self.assertEqual('5.6', version)
        vsn.assert_called_with(('juju', '--version'))

    def test_get_version_path(self):
        with patch('subprocess.check_output', return_value=' 4.3') as vsn:
            EnvJujuClient1X.get_version('foo/bar/baz')
        vsn.assert_called_once_with(('foo/bar/baz', '--version'))

    def test_get_matching_agent_version(self):
        client = EnvJujuClient1X(SimpleEnvironment(None, {'type': 'local'}),
                                 '1.23-series-arch', None)
        self.assertEqual('1.23.1', client.get_matching_agent_version())
        self.assertEqual('1.23', client.get_matching_agent_version(
                         no_build=True))
        client = client.clone(version='1.20-beta1-series-arch')
        self.assertEqual('1.20-beta1.1', client.get_matching_agent_version())

    def test_upgrade_juju_nonlocal(self):
        client = EnvJujuClient1X(
            SimpleEnvironment('foo', {'type': 'nonlocal'}), '1.234-76', None)
        with patch.object(client, 'juju') as juju_mock:
            client.upgrade_juju()
        juju_mock.assert_called_with(
            'upgrade-juju', ('--version', '1.234'))

    def test_upgrade_juju_local(self):
        client = EnvJujuClient1X(
            SimpleEnvironment('foo', {'type': 'local'}), '1.234-76', None)
        with patch.object(client, 'juju') as juju_mock:
            client.upgrade_juju()
        juju_mock.assert_called_with(
            'upgrade-juju', ('--version', '1.234', '--upload-tools',))

    def test_upgrade_juju_no_force_version(self):
        client = EnvJujuClient1X(
            SimpleEnvironment('foo', {'type': 'local'}), '1.234-76', None)
        with patch.object(client, 'juju') as juju_mock:
            client.upgrade_juju(force_version=False)
        juju_mock.assert_called_with(
            'upgrade-juju', ('--upload-tools',))

    def test_upgrade_mongo_exception(self):
        client = EnvJujuClient1X(
            SimpleEnvironment('foo', {'type': 'local'}), '1.234-76', None)
        with self.assertRaises(UpgradeMongoNotSupported):
            client.upgrade_mongo()

    def test_get_cache_path(self):
        client = EnvJujuClient1X(SimpleEnvironment('foo', juju_home='/foo/'),
                                 '1.27', 'full/path', debug=True)
        self.assertEqual('/foo/environments/cache.yaml',
                         client.get_cache_path())

    def test_bootstrap_maas(self):
        env = SimpleEnvironment('maas')
        with patch.object(EnvJujuClient1X, 'juju') as mock:
            client = EnvJujuClient1X(env, None, None)
            with patch.object(client.env, 'maas', lambda: True):
                client.bootstrap()
        mock.assert_called_once_with('bootstrap', ('--constraints', 'mem=2G'))

    def test_bootstrap_joyent(self):
        env = SimpleEnvironment('joyent')
        with patch.object(EnvJujuClient1X, 'juju', autospec=True) as mock:
            client = EnvJujuClient1X(env, None, None)
            with patch.object(client.env, 'joyent', lambda: True):
                client.bootstrap()
        mock.assert_called_once_with(
            client, 'bootstrap', ('--constraints', 'mem=2G cpu-cores=1'))

    def test_bootstrap(self):
        env = SimpleEnvironment('foo')
        client = EnvJujuClient1X(env, None, None)
        with patch.object(client, 'juju') as mock:
            client.bootstrap()
        mock.assert_called_with('bootstrap', ('--constraints', 'mem=2G'))

    def test_bootstrap_upload_tools(self):
        env = SimpleEnvironment('foo')
        client = EnvJujuClient1X(env, None, None)
        with patch.object(client, 'juju') as mock:
            client.bootstrap(upload_tools=True)
        mock.assert_called_with(
            'bootstrap', ('--upload-tools', '--constraints', 'mem=2G'))

    def test_bootstrap_args(self):
        env = SimpleEnvironment('foo', {})
        client = EnvJujuClient1X(env, None, None)
        with self.assertRaisesRegexp(
                BootstrapMismatch,
                '--bootstrap-series angsty does not match default-series:'
                ' None'):
            client.bootstrap(bootstrap_series='angsty')
        env.update_config({
            'default-series': 'angsty',
            })
        with patch.object(client, 'juju') as mock:
            client.bootstrap(bootstrap_series='angsty')
        mock.assert_called_with('bootstrap', ('--constraints', 'mem=2G'))

    def test_bootstrap_async(self):
        env = SimpleEnvironment('foo')
        with patch.object(ModelClient, 'juju_async', autospec=True) as mock:
            client = EnvJujuClient1X(env, None, None)
            client.env.juju_home = 'foo'
            with client.bootstrap_async():
                mock.assert_called_once_with(
                    client, 'bootstrap', ('--constraints', 'mem=2G'))

    def test_bootstrap_async_upload_tools(self):
        env = SimpleEnvironment('foo')
        with patch.object(ModelClient, 'juju_async', autospec=True) as mock:
            client = EnvJujuClient1X(env, None, None)
            with client.bootstrap_async(upload_tools=True):
                mock.assert_called_with(
                    client, 'bootstrap', ('--upload-tools', '--constraints',
                                          'mem=2G'))

    def test_get_bootstrap_args_bootstrap_series(self):
        env = SimpleEnvironment('foo', {})
        client = EnvJujuClient1X(env, None, None)
        with self.assertRaisesRegexp(
                BootstrapMismatch,
                '--bootstrap-series angsty does not match default-series:'
                ' None'):
            client.get_bootstrap_args(upload_tools=True,
                                      bootstrap_series='angsty')
        env.update_config({'default-series': 'angsty'})
        args = client.get_bootstrap_args(upload_tools=True,
                                         bootstrap_series='angsty')
        self.assertEqual(args, ('--upload-tools', '--constraints', 'mem=2G'))

    def test_create_environment_system(self):
        self.do_create_environment(
            'system', 'system create-environment', ('-s', 'foo'))

    def test_create_environment_controller(self):
        self.do_create_environment(
            'controller', 'controller create-environment', ('-c', 'foo'))

    def test_create_environment_hypenated_controller(self):
        self.do_create_environment(
            'kill-controller', 'create-environment', ('-c', 'foo'))

    def do_create_environment(self, jes_command, create_cmd,
                              controller_option):
        controller_client = EnvJujuClient1X(SimpleEnvironment('foo'), '1.26.1',
                                            None)
        model_env = SimpleEnvironment('bar', {'type': 'foo'})
        with patch.object(controller_client, 'get_jes_command',
                          return_value=jes_command):
            with patch.object(controller_client, 'juju') as juju_mock:
                with observable_temp_file() as config_file:
                    controller_client.add_model(model_env)
        juju_mock.assert_called_once_with(
            create_cmd, controller_option + (
                'bar', '--config', config_file.name), include_e=False)

    def test_destroy_environment(self):
        env = SimpleEnvironment('foo', {'type': 'ec2'})
        client = EnvJujuClient1X(env, None, None)
        with patch.object(client, 'juju') as mock:
            client.destroy_environment()
        mock.assert_called_with(
            'destroy-environment', ('foo', '--force', '-y'),
            check=False, include_e=False, timeout=600)

    def test_destroy_environment_no_force(self):
        env = SimpleEnvironment('foo', {'type': 'ec2'})
        client = EnvJujuClient1X(env, None, None)
        with patch.object(client, 'juju') as mock:
            client.destroy_environment(force=False)
            mock.assert_called_with(
                'destroy-environment', ('foo', '-y'),
                check=False, include_e=False, timeout=600)

    def test_destroy_environment_azure(self):
        env = SimpleEnvironment('foo', {'type': 'azure'})
        client = EnvJujuClient1X(env, None, None)
        with patch.object(client, 'juju') as mock:
            client.destroy_environment(force=False)
            mock.assert_called_with(
                'destroy-environment', ('foo', '-y'),
                check=False, include_e=False, timeout=1800)

    def test_destroy_environment_gce(self):
        env = SimpleEnvironment('foo', {'type': 'gce'})
        client = EnvJujuClient1X(env, None, None)
        with patch.object(client, 'juju') as mock:
            client.destroy_environment(force=False)
            mock.assert_called_with(
                'destroy-environment', ('foo', '-y'),
                check=False, include_e=False, timeout=1200)

    def test_destroy_environment_delete_jenv(self):
        env = SimpleEnvironment('foo', {'type': 'ec2'})
        client = EnvJujuClient1X(env, None, None)
        with patch.object(client, 'juju'):
            with temp_env({}) as juju_home:
                client.env.juju_home = juju_home
                jenv_path = get_jenv_path(juju_home, 'foo')
                os.makedirs(os.path.dirname(jenv_path))
                open(jenv_path, 'w')
                self.assertTrue(os.path.exists(jenv_path))
                client.destroy_environment(delete_jenv=True)
                self.assertFalse(os.path.exists(jenv_path))

    def test_destroy_model(self):
        env = SimpleEnvironment('foo', {'type': 'ec2'})
        client = EnvJujuClient1X(env, None, None)
        with patch.object(client, 'juju') as mock:
            client.destroy_model()
        mock.assert_called_with(
            'destroy-environment', ('foo', '-y'),
            check=False, include_e=False, timeout=600)

    def test_kill_controller(self):
        client = EnvJujuClient1X(
            SimpleEnvironment('foo', {'type': 'ec2'}), None, None)
        with patch.object(client, 'juju') as juju_mock:
            client.kill_controller()
        juju_mock.assert_called_once_with(
            'destroy-environment', ('foo', '--force', '-y'), check=False,
            include_e=False, timeout=600)

    def test_kill_controller_check(self):
        client = EnvJujuClient1X(
            SimpleEnvironment('foo', {'type': 'ec2'}), None, None)
        with patch.object(client, 'juju') as juju_mock:
            client.kill_controller(check=True)
        juju_mock.assert_called_once_with(
            'destroy-environment', ('foo', '--force', '-y'), check=True,
            include_e=False, timeout=600)

    def test_destroy_controller(self):
        client = EnvJujuClient1X(
            SimpleEnvironment('foo', {'type': 'ec2'}), None, None)
        with patch.object(client, 'juju') as juju_mock:
            client.destroy_controller()
        juju_mock.assert_called_once_with(
            'destroy-environment', ('foo', '-y'),
            include_e=False, timeout=600)

    def test_get_juju_output(self):
        env = SimpleEnvironment('foo')
        client = EnvJujuClient1X(env, None, 'juju')
        fake_popen = FakePopen('asdf', None, 0)
        with patch('subprocess.Popen', return_value=fake_popen) as mock:
            result = client.get_juju_output('bar')
        self.assertEqual('asdf', result)
        self.assertEqual((('juju', '--show-log', 'bar', '-e', 'foo'),),
                         mock.call_args[0])

    def test_get_juju_output_accepts_varargs(self):
        env = SimpleEnvironment('foo')
        fake_popen = FakePopen('asdf', None, 0)
        client = EnvJujuClient1X(env, None, 'juju')
        with patch('subprocess.Popen', return_value=fake_popen) as mock:
            result = client.get_juju_output('bar', 'baz', '--qux')
        self.assertEqual('asdf', result)
        self.assertEqual((('juju', '--show-log', 'bar', '-e', 'foo', 'baz',
                           '--qux'),), mock.call_args[0])

    def test_get_juju_output_stderr(self):
        env = SimpleEnvironment('foo')
        fake_popen = FakePopen('Hello', 'Error!', 1)
        client = EnvJujuClient1X(env, None, 'juju')
        with self.assertRaises(subprocess.CalledProcessError) as exc:
            with patch('subprocess.Popen', return_value=fake_popen):
                client.get_juju_output('bar')
        self.assertEqual(exc.exception.output, 'Hello')
        self.assertEqual(exc.exception.stderr, 'Error!')

    def test_get_juju_output_full_cmd(self):
        env = SimpleEnvironment('foo')
        fake_popen = FakePopen(None, 'Hello!', 1)
        client = EnvJujuClient1X(env, None, 'juju')
        with self.assertRaises(subprocess.CalledProcessError) as exc:
            with patch('subprocess.Popen', return_value=fake_popen):
                client.get_juju_output('bar', '--baz', 'qux')
        self.assertEqual(
            ('juju', '--show-log', 'bar', '-e', 'foo', '--baz', 'qux'),
            exc.exception.cmd)

    def test_get_juju_output_accepts_timeout(self):
        env = SimpleEnvironment('foo')
        fake_popen = FakePopen('asdf', None, 0)
        client = EnvJujuClient1X(env, None, 'juju')
        with patch('subprocess.Popen', return_value=fake_popen) as po_mock:
            client.get_juju_output('bar', timeout=5)
        self.assertEqual(
            po_mock.call_args[0][0],
            (sys.executable, get_timeout_path(), '5.00', '--', 'juju',
             '--show-log', 'bar', '-e', 'foo'))

    def test__shell_environ_juju_home(self):
        client = EnvJujuClient1X(
            SimpleEnvironment('baz', {'type': 'ec2'}), '1.25-foobar', 'path',
            'asdf')
        env = client._shell_environ()
        self.assertEqual(env['JUJU_HOME'], 'asdf')
        self.assertNotIn('JUJU_DATA', env)

    def test_juju_output_supplies_path(self):
        env = SimpleEnvironment('foo')
        client = EnvJujuClient1X(env, None, '/foobar/bar')

        def check_path(*args, **kwargs):
            self.assertRegexpMatches(os.environ['PATH'], r'/foobar\:')
            return FakePopen(None, None, 0)
        with patch('subprocess.Popen', autospec=True,
                   side_effect=check_path):
            client.get_juju_output('cmd', 'baz')

    def test_get_status(self):
        output_text = dedent("""\
                - a
                - b
                - c
                """)
        env = SimpleEnvironment('foo')
        client = EnvJujuClient1X(env, None, None)
        with patch.object(client, 'get_juju_output',
                          return_value=output_text) as gjo_mock:
            result = client.get_status()
        gjo_mock.assert_called_once_with(
            'status', '--format', 'yaml', controller=False)
        self.assertEqual(Status1X, type(result))
        self.assertEqual(['a', 'b', 'c'], result.status)

    def test_get_status_retries_on_error(self):
        env = SimpleEnvironment('foo')
        client = EnvJujuClient1X(env, None, None)
        client.attempt = 0

        def get_juju_output(command, *args, **kwargs):
            if client.attempt == 1:
                return '"hello"'
            client.attempt += 1
            raise subprocess.CalledProcessError(1, command)

        with patch.object(client, 'get_juju_output', get_juju_output):
            client.get_status()

    def test_get_status_raises_on_timeout_1(self):
        env = SimpleEnvironment('foo')
        client = EnvJujuClient1X(env, None, None)

        def get_juju_output(command, *args, **kwargs):
            raise subprocess.CalledProcessError(1, command)

        with patch.object(client, 'get_juju_output',
                          side_effect=get_juju_output):
            with patch('jujupy.client.until_timeout',
                       lambda x: iter([None, None])):
                with self.assertRaisesRegexp(
                        Exception, 'Timed out waiting for juju status'):
                    client.get_status()

    def test_get_status_raises_on_timeout_2(self):
        env = SimpleEnvironment('foo')
        client = EnvJujuClient1X(env, None, None)
        with patch('jujupy.client.until_timeout',
                   return_value=iter([1])) as mock_ut:
            with patch.object(client, 'get_juju_output',
                              side_effect=StopIteration):
                with self.assertRaises(StopIteration):
                    client.get_status(500)
        mock_ut.assert_called_with(500)

    def test_get_status_controller(self):
        output_text = """\
            - a
            - b
            - c
        """
        env = SimpleEnvironment('foo')
        client = EnvJujuClient1X(env, None, None)
        with patch.object(client, 'get_juju_output',
                          return_value=output_text) as gjo_mock:
            client.get_status(controller=True)
        gjo_mock.assert_called_once_with(
            'status', '--format', 'yaml', controller=True)

    @staticmethod
    def make_status_yaml(key, machine_value, unit_value):
        return dedent("""\
            environment: foo
            machines:
              "0":
                {0}: {1}
            services:
              jenkins:
                units:
                  jenkins/0:
                    {0}: {2}
        """.format(key, machine_value, unit_value))

    def test_deploy_non_joyent(self):
        env = EnvJujuClient1X(
            SimpleEnvironment('foo', {'type': 'local'}), '1.234-76', None)
        with patch.object(env, 'juju') as mock_juju:
            env.deploy('mondogb')
        mock_juju.assert_called_with('deploy', ('mondogb',))

    def test_deploy_joyent(self):
        env = EnvJujuClient1X(
            SimpleEnvironment('foo', {'type': 'local'}), '1.234-76', None)
        with patch.object(env, 'juju') as mock_juju:
            env.deploy('mondogb')
        mock_juju.assert_called_with('deploy', ('mondogb',))

    def test_deploy_repository(self):
        env = EnvJujuClient1X(
            SimpleEnvironment('foo', {'type': 'local'}), '1.234-76', None)
        with patch.object(env, 'juju') as mock_juju:
            env.deploy('mondogb', '/home/jrandom/repo')
        mock_juju.assert_called_with(
            'deploy', ('mondogb', '--repository', '/home/jrandom/repo'))

    def test_deploy_to(self):
        env = EnvJujuClient1X(
            SimpleEnvironment('foo', {'type': 'local'}), '1.234-76', None)
        with patch.object(env, 'juju') as mock_juju:
            env.deploy('mondogb', to='0')
        mock_juju.assert_called_with(
            'deploy', ('mondogb', '--to', '0'))

    def test_deploy_service(self):
        env = EnvJujuClient1X(
            SimpleEnvironment('foo', {'type': 'local'}), '1.234-76', None)
        with patch.object(env, 'juju') as mock_juju:
            env.deploy('local:mondogb', service='my-mondogb')
        mock_juju.assert_called_with(
            'deploy', ('local:mondogb', 'my-mondogb',))

    def test_upgrade_charm(self):
        client = EnvJujuClient1X(
            SimpleEnvironment('foo', {'type': 'local'}), '1.234-76', None)
        with patch.object(client, 'juju') as mock_juju:
            client.upgrade_charm('foo-service',
                                 '/bar/repository/angsty/mongodb')
        mock_juju.assert_called_once_with(
            'upgrade-charm', ('foo-service', '--repository',
                              '/bar/repository',))

    def test_remove_service(self):
        client = EnvJujuClient1X(
            SimpleEnvironment('foo', {'type': 'local'}), '1.234-76', None)
        with patch.object(client, 'juju') as mock_juju:
            client.remove_service('mondogb')
        mock_juju.assert_called_with('destroy-service', ('mondogb',))

    def test_status_until_always_runs_once(self):
        client = EnvJujuClient1X(
            SimpleEnvironment('foo', {'type': 'local'}), '1.234-76', None)
        status_txt = self.make_status_yaml('agent-state', 'started', 'started')
        with patch.object(client, 'get_juju_output', return_value=status_txt):
            result = list(client.status_until(-1))
        self.assertEqual(
            [r.status for r in result], [Status.from_text(status_txt).status])

    def test_status_until_timeout(self):
        client = EnvJujuClient1X(
            SimpleEnvironment('foo', {'type': 'local'}), '1.234-76', None)
        status_txt = self.make_status_yaml('agent-state', 'started', 'started')
        status_yaml = yaml.safe_load(status_txt)

        def until_timeout_stub(timeout, start=None):
            return iter([None, None])

        with patch.object(client, 'get_juju_output', return_value=status_txt):
            with patch('jujupy.client.until_timeout',
                       side_effect=until_timeout_stub) as ut_mock:
                result = list(client.status_until(30, 70))
        self.assertEqual(
            [r.status for r in result], [status_yaml] * 3)
        # until_timeout is called by status as well as status_until.
        self.assertEqual(ut_mock.mock_calls,
                         [call(60), call(30, start=70), call(60), call(60)])

    def test_add_ssh_machines(self):
        client = EnvJujuClient1X(SimpleEnvironment('foo'), None, 'juju')
        with patch('subprocess.check_call', autospec=True) as cc_mock:
            client.add_ssh_machines(['m-foo', 'm-bar', 'm-baz'])
        assert_juju_call(self, cc_mock, client, (
            'juju', '--show-log', 'add-machine', '-e', 'foo', 'ssh:m-foo'), 0)
        assert_juju_call(self, cc_mock, client, (
            'juju', '--show-log', 'add-machine', '-e', 'foo', 'ssh:m-bar'), 1)
        assert_juju_call(self, cc_mock, client, (
            'juju', '--show-log', 'add-machine', '-e', 'foo', 'ssh:m-baz'), 2)
        self.assertEqual(cc_mock.call_count, 3)

    def test_add_ssh_machines_retry(self):
        client = EnvJujuClient1X(SimpleEnvironment('foo'), None, 'juju')
        with patch('subprocess.check_call', autospec=True,
                   side_effect=[subprocess.CalledProcessError(None, None),
                                None, None, None]) as cc_mock:
            client.add_ssh_machines(['m-foo', 'm-bar', 'm-baz'])
        assert_juju_call(self, cc_mock, client, (
            'juju', '--show-log', 'add-machine', '-e', 'foo', 'ssh:m-foo'), 0)
        self.pause_mock.assert_called_once_with(30)
        assert_juju_call(self, cc_mock, client, (
            'juju', '--show-log', 'add-machine', '-e', 'foo', 'ssh:m-foo'), 1)
        assert_juju_call(self, cc_mock, client, (
            'juju', '--show-log', 'add-machine', '-e', 'foo', 'ssh:m-bar'), 2)
        assert_juju_call(self, cc_mock, client, (
            'juju', '--show-log', 'add-machine', '-e', 'foo', 'ssh:m-baz'), 3)
        self.assertEqual(cc_mock.call_count, 4)

    def test_add_ssh_machines_fail_on_second_machine(self):
        client = EnvJujuClient1X(SimpleEnvironment('foo'), None, 'juju')
        with patch('subprocess.check_call', autospec=True, side_effect=[
                None, subprocess.CalledProcessError(None, None), None, None
                ]) as cc_mock:
            with self.assertRaises(subprocess.CalledProcessError):
                client.add_ssh_machines(['m-foo', 'm-bar', 'm-baz'])
        assert_juju_call(self, cc_mock, client, (
            'juju', '--show-log', 'add-machine', '-e', 'foo', 'ssh:m-foo'), 0)
        assert_juju_call(self, cc_mock, client, (
            'juju', '--show-log', 'add-machine', '-e', 'foo', 'ssh:m-bar'), 1)
        self.assertEqual(cc_mock.call_count, 2)

    def test_add_ssh_machines_fail_on_second_attempt(self):
        client = EnvJujuClient1X(SimpleEnvironment('foo'), None, 'juju')
        with patch('subprocess.check_call', autospec=True, side_effect=[
                subprocess.CalledProcessError(None, None),
                subprocess.CalledProcessError(None, None)]) as cc_mock:
            with self.assertRaises(subprocess.CalledProcessError):
                client.add_ssh_machines(['m-foo', 'm-bar', 'm-baz'])
        assert_juju_call(self, cc_mock, client, (
            'juju', '--show-log', 'add-machine', '-e', 'foo', 'ssh:m-foo'), 0)
        assert_juju_call(self, cc_mock, client, (
            'juju', '--show-log', 'add-machine', '-e', 'foo', 'ssh:m-foo'), 1)
        self.assertEqual(cc_mock.call_count, 2)

    def test_wait_for_started(self):
        value = self.make_status_yaml('agent-state', 'started', 'started')
        client = EnvJujuClient1X(SimpleEnvironment('local'), None, None)
        with patch.object(client, 'get_juju_output', return_value=value):
            client.wait_for_started()

    def test_wait_for_started_timeout(self):
        value = self.make_status_yaml('agent-state', 'pending', 'started')
        client = EnvJujuClient1X(SimpleEnvironment('local'), None, None)
        with patch('jujupy.client.until_timeout',
                   lambda x, start=None: range(1)):
            with patch.object(client, 'get_juju_output', return_value=value):
                writes = []
                with patch.object(GroupReporter, '_write', autospec=True,
                                  side_effect=lambda _, s: writes.append(s)):
                    with self.assertRaisesRegexp(
                            StatusNotMet,
                            'Timed out waiting for agents to start in local'):
                        client.wait_for_started()
                self.assertEqual(writes, ['pending: 0', ' .', '\n'])

    def test_wait_for_started_start(self):
        value = self.make_status_yaml('agent-state', 'started', 'pending')
        client = EnvJujuClient1X(SimpleEnvironment('local'), None, None)
        now = datetime.now() + timedelta(days=1)
        with patch('utility.until_timeout.now', return_value=now):
            with patch.object(client, 'get_juju_output', return_value=value):
                writes = []
                with patch.object(GroupReporter, '_write', autospec=True,
                                  side_effect=lambda _, s: writes.append(s)):
                    with self.assertRaisesRegexp(
                            StatusNotMet,
                            'Timed out waiting for agents to start in local'):
                        client.wait_for_started(start=now - timedelta(1200))
                self.assertEqual(writes, ['pending: jenkins/0', '\n'])

    def test_wait_for_started_logs_status(self):
        value = self.make_status_yaml('agent-state', 'pending', 'started')
        client = EnvJujuClient1X(SimpleEnvironment('local'), None, None)
        with patch.object(client, 'get_juju_output', return_value=value):
            writes = []
            with patch.object(GroupReporter, '_write', autospec=True,
                              side_effect=lambda _, s: writes.append(s)):
                with self.assertRaisesRegexp(
                        StatusNotMet,
                        'Timed out waiting for agents to start in local'):
                    client.wait_for_started(0)
            self.assertEqual(writes, ['pending: 0', '\n'])
        self.assertEqual(self.log_stream.getvalue(), 'ERROR %s\n' % value)

    def test_wait_for_subordinate_units(self):
        value = dedent("""\
            machines:
              "0":
                agent-state: started
            services:
              jenkins:
                units:
                  jenkins/0:
                    subordinates:
                      sub1/0:
                        agent-state: started
              ubuntu:
                units:
                  ubuntu/0:
                    subordinates:
                      sub2/0:
                        agent-state: started
                      sub3/0:
                        agent-state: started
        """)
        client = EnvJujuClient1X(SimpleEnvironment('local'), None, None)
        now = datetime.now() + timedelta(days=1)
        with patch('utility.until_timeout.now', return_value=now):
            with patch.object(client, 'get_juju_output', return_value=value):
                with patch(
                        'jujupy.client.GroupReporter.update') as update_mock:
                    with patch(
                            'jujupy.client.GroupReporter.finish'
                            ) as finish_mock:
                        client.wait_for_subordinate_units(
                            'jenkins', 'sub1', start=now - timedelta(1200))
        self.assertEqual([], update_mock.call_args_list)
        finish_mock.assert_called_once_with()

    def test_wait_for_multiple_subordinate_units(self):
        value = dedent("""\
            machines:
              "0":
                agent-state: started
            services:
              ubuntu:
                units:
                  ubuntu/0:
                    subordinates:
                      sub/0:
                        agent-state: started
                  ubuntu/1:
                    subordinates:
                      sub/1:
                        agent-state: started
        """)
        client = EnvJujuClient1X(SimpleEnvironment('local'), None, None)
        now = datetime.now() + timedelta(days=1)
        with patch('utility.until_timeout.now', return_value=now):
            with patch.object(client, 'get_juju_output', return_value=value):
                with patch(
                        'jujupy.client.GroupReporter.update') as update_mock:
                    with patch(
                            'jujupy.client.GroupReporter.finish'
                            ) as finish_mock:
                        client.wait_for_subordinate_units(
                            'ubuntu', 'sub', start=now - timedelta(1200))
        self.assertEqual([], update_mock.call_args_list)
        finish_mock.assert_called_once_with()

    def test_wait_for_subordinate_units_checks_slash_in_unit_name(self):
        value = dedent("""\
            machines:
              "0":
                agent-state: started
            services:
              jenkins:
                units:
                  jenkins/0:
                    subordinates:
                      sub1:
                        agent-state: started
        """)
        client = EnvJujuClient1X(SimpleEnvironment('local'), None, None)
        now = datetime.now() + timedelta(days=1)
        with patch('utility.until_timeout.now', return_value=now):
            with patch.object(client, 'get_juju_output', return_value=value):
                with self.assertRaisesRegexp(
                        StatusNotMet,
                        'Timed out waiting for agents to start in local'):
                    client.wait_for_subordinate_units(
                        'jenkins', 'sub1', start=now - timedelta(1200))

    def test_wait_for_subordinate_units_no_subordinate(self):
        value = dedent("""\
            machines:
              "0":
                agent-state: started
            services:
              jenkins:
                units:
                  jenkins/0:
                    agent-state: started
        """)
        client = EnvJujuClient1X(SimpleEnvironment('local'), None, None)
        now = datetime.now() + timedelta(days=1)
        with patch('utility.until_timeout.now', return_value=now):
            with patch.object(client, 'get_juju_output', return_value=value):
                with self.assertRaisesRegexp(
                        StatusNotMet,
                        'Timed out waiting for agents to start in local'):
                    client.wait_for_subordinate_units(
                        'jenkins', 'sub1', start=now - timedelta(1200))

    def test_wait_for_workload(self):
        initial_status = Status1X.from_text("""\
            machines: {}
            services:
              jenkins:
                units:
                  jenkins/0:
                    workload-status:
                      current: waiting
                  subordinates:
                    ntp/0:
                      workload-status:
                        current: unknown
        """)
        final_status = Status(copy.deepcopy(initial_status.status), None)
        final_status.status['services']['jenkins']['units']['jenkins/0'][
            'workload-status']['current'] = 'active'
        client = EnvJujuClient1X(SimpleEnvironment('local'), None, None)
        writes = []
        with patch('utility.until_timeout', autospec=True, return_value=[1]):
            with patch.object(client, 'get_status', autospec=True,
                              side_effect=[initial_status, final_status]):
                with patch.object(GroupReporter, '_write', autospec=True,
                                  side_effect=lambda _, s: writes.append(s)):
                    client.wait_for_workloads()
        self.assertEqual(writes, ['waiting: jenkins/0', '\n'])

    def test_wait_for_workload_all_unknown(self):
        status = Status.from_text("""\
            services:
              jenkins:
                units:
                  jenkins/0:
                    workload-status:
                      current: unknown
                  subordinates:
                    ntp/0:
                      workload-status:
                        current: unknown
        """)
        client = EnvJujuClient1X(SimpleEnvironment('local'), None, None)
        writes = []
        with patch('utility.until_timeout', autospec=True, return_value=[]):
            with patch.object(client, 'get_status', autospec=True,
                              return_value=status):
                with patch.object(GroupReporter, '_write', autospec=True,
                                  side_effect=lambda _, s: writes.append(s)):
                    client.wait_for_workloads(timeout=1)
        self.assertEqual(writes, [])

    def test_wait_for_workload_no_workload_status(self):
        status = Status.from_text("""\
            services:
              jenkins:
                units:
                  jenkins/0:
                    agent-state: active
        """)
        client = EnvJujuClient1X(SimpleEnvironment('local'), None, None)
        writes = []
        with patch('utility.until_timeout', autospec=True, return_value=[]):
            with patch.object(client, 'get_status', autospec=True,
                              return_value=status):
                with patch.object(GroupReporter, '_write', autospec=True,
                                  side_effect=lambda _, s: writes.append(s)):
                    client.wait_for_workloads(timeout=1)
        self.assertEqual(writes, [])

    def test_wait_for_ha(self):
        value = yaml.safe_dump({
            'machines': {
                '0': {'state-server-member-status': 'has-vote'},
                '1': {'state-server-member-status': 'has-vote'},
                '2': {'state-server-member-status': 'has-vote'},
            },
            'services': {},
        })
        client = EnvJujuClient1X(SimpleEnvironment('local'), None, None)
        with patch.object(client, 'get_juju_output', return_value=value):
            client.wait_for_ha()

    def test_wait_for_ha_no_has_vote(self):
        value = yaml.safe_dump({
            'machines': {
                '0': {'state-server-member-status': 'no-vote'},
                '1': {'state-server-member-status': 'no-vote'},
                '2': {'state-server-member-status': 'no-vote'},
            },
            'services': {},
        })
        client = EnvJujuClient1X(SimpleEnvironment('local'), None, None)
        with patch.object(client, 'get_juju_output', return_value=value):
            writes = []
            with patch('jujupy.client.until_timeout', autospec=True,
                       return_value=[2, 1]):
                with patch.object(GroupReporter, '_write', autospec=True,
                                  side_effect=lambda _, s: writes.append(s)):
                    with self.assertRaisesRegexp(
                            StatusNotMet,
                            'Timed out waiting for voting to be enabled.'):
                        client.wait_for_ha()
            self.assertEqual(writes[:2], ['no-vote: 0, 1, 2', ' .'])
            self.assertEqual(writes[2:-1], ['.'] * (len(writes) - 3))
            self.assertEqual(writes[-1:], ['\n'])

    def test_wait_for_ha_timeout(self):
        value = yaml.safe_dump({
            'machines': {
                '0': {'state-server-member-status': 'has-vote'},
                '1': {'state-server-member-status': 'has-vote'},
            },
            'services': {},
        })
        client = EnvJujuClient1X(SimpleEnvironment('local'), None, None)
        status = client.status_class.from_text(value)
        with patch('jujupy.client.until_timeout',
                   lambda x, start=None: range(0)):
            with patch.object(client, 'get_status', return_value=status):
                with self.assertRaisesRegexp(
                        StatusNotMet,
                        'Timed out waiting for voting to be enabled.'):
                    client.wait_for_ha()

    def test_wait_for_deploy_started(self):
        value = yaml.safe_dump({
            'machines': {
                '0': {'agent-state': 'started'},
            },
            'services': {
                'jenkins': {
                    'units': {
                        'jenkins/1': {'baz': 'qux'}
                    }
                }
            }
        })
        client = EnvJujuClient1X(SimpleEnvironment('local'), None, None)
        with patch.object(client, 'get_juju_output', return_value=value):
            client.wait_for_deploy_started()

    def test_wait_for_deploy_started_timeout(self):
        value = yaml.safe_dump({
            'machines': {
                '0': {'agent-state': 'started'},
            },
            'services': {},
        })
        client = EnvJujuClient1X(SimpleEnvironment('local'), None, None)
        with patch('jujupy.client.until_timeout', lambda x: range(0)):
            with patch.object(client, 'get_juju_output', return_value=value):
                with self.assertRaisesRegexp(
                        StatusNotMet,
                        'Timed out waiting for applications to start.'):
                    client.wait_for_deploy_started()

    def test_wait_for_version(self):
        value = self.make_status_yaml('agent-version', '1.17.2', '1.17.2')
        client = EnvJujuClient1X(SimpleEnvironment('local'), None, None)
        with patch.object(client, 'get_juju_output', return_value=value):
            client.wait_for_version('1.17.2')

    def test_wait_for_version_timeout(self):
        value = self.make_status_yaml('agent-version', '1.17.2', '1.17.1')
        client = EnvJujuClient1X(SimpleEnvironment('local'), None, None)
        writes = []
        with patch('jujupy.client.until_timeout',
                   lambda x, start=None: [x]):
            with patch.object(client, 'get_juju_output', return_value=value):
                with patch.object(GroupReporter, '_write', autospec=True,
                                  side_effect=lambda _, s: writes.append(s)):
                    with self.assertRaisesRegexp(
                            StatusNotMet, 'Some versions did not update'):
                        client.wait_for_version('1.17.2')
        self.assertEqual(writes, ['1.17.1: jenkins/0', ' .', '\n'])

    def test_wait_for_version_handles_connection_error(self):
        err = subprocess.CalledProcessError(2, 'foo')
        err.stderr = 'Unable to connect to environment'
        err = CannotConnectEnv(err)
        status = self.make_status_yaml('agent-version', '1.17.2', '1.17.2')
        actions = [err, status]

        def get_juju_output_fake(*args, **kwargs):
            action = actions.pop(0)
            if isinstance(action, Exception):
                raise action
            else:
                return action

        client = EnvJujuClient1X(SimpleEnvironment('local'), None, None)
        with patch.object(client, 'get_juju_output', get_juju_output_fake):
            client.wait_for_version('1.17.2')

    def test_wait_for_version_raises_non_connection_error(self):
        err = Exception('foo')
        status = self.make_status_yaml('agent-version', '1.17.2', '1.17.2')
        actions = [err, status]

        def get_juju_output_fake(*args, **kwargs):
            action = actions.pop(0)
            if isinstance(action, Exception):
                raise action
            else:
                return action

        client = EnvJujuClient1X(SimpleEnvironment('local'), None, None)
        with patch.object(client, 'get_juju_output', get_juju_output_fake):
            with self.assertRaisesRegexp(Exception, 'foo'):
                client.wait_for_version('1.17.2')

    def test_wait_just_machine_0(self):
        value = yaml.safe_dump({
            'machines': {
                '0': {'agent-state': 'started'},
            },
        })
        client = EnvJujuClient1X(SimpleEnvironment('local'), None, None)
        with patch.object(client, 'get_juju_output', return_value=value):
            client.wait_for(WaitMachineNotPresent('1'), quiet=True)

    def test_wait_just_machine_0_timeout(self):
        value = yaml.safe_dump({
            'machines': {
                '0': {'agent-state': 'started'},
                '1': {'agent-state': 'started'},
            },
        })
        client = EnvJujuClient1X(SimpleEnvironment('local'), None, None)
        with patch.object(client, 'get_juju_output', return_value=value), \
            patch('jujupy.client.until_timeout',
                  lambda x, start=None: range(1)), \
            self.assertRaisesRegexp(
                Exception,
                'Timed out waiting for machine removal 1'):
            client.wait_for(WaitMachineNotPresent('1'), quiet=True)

    def test_set_model_constraints(self):
        client = EnvJujuClient1X(SimpleEnvironment('bar', {}), None, '/foo')
        with patch.object(client, 'juju') as juju_mock:
            client.set_model_constraints({'bar': 'baz'})
        juju_mock.assert_called_once_with('set-constraints', ('bar=baz',))

    def test_get_model_config(self):
        env = SimpleEnvironment('foo', None)
        fake_popen = FakePopen(yaml.safe_dump({'bar': 'baz'}), None, 0)
        client = EnvJujuClient1X(env, None, 'juju')
        with patch('subprocess.Popen', return_value=fake_popen) as po_mock:
            result = client.get_model_config()
        assert_juju_call(
            self, po_mock, client, (
                'juju', '--show-log', 'get-env', '-e', 'foo'))
        self.assertEqual({'bar': 'baz'}, result)

    def test_get_env_option(self):
        env = SimpleEnvironment('foo', None)
        fake_popen = FakePopen('https://example.org/juju/tools', None, 0)
        client = EnvJujuClient1X(env, None, 'juju')
        with patch('subprocess.Popen', return_value=fake_popen) as mock:
            result = client.get_env_option('tools-metadata-url')
        self.assertEqual(
            mock.call_args[0][0],
            ('juju', '--show-log', 'get-env', '-e', 'foo',
             'tools-metadata-url'))
        self.assertEqual('https://example.org/juju/tools', result)

    def test_set_env_option(self):
        env = SimpleEnvironment('foo')
        client = EnvJujuClient1X(env, None, 'juju')
        with patch('subprocess.check_call') as mock:
            client.set_env_option(
                'tools-metadata-url', 'https://example.org/juju/tools')
        environ = dict(os.environ)
        environ['JUJU_HOME'] = client.env.juju_home
        mock.assert_called_with(
            ('juju', '--show-log', 'set-env', '-e', 'foo',
             'tools-metadata-url=https://example.org/juju/tools'),
            stderr=None)

    def test_unset_env_option(self):
        env = SimpleEnvironment('foo')
        client = EnvJujuClient1X(env, None, 'juju')
        with patch('subprocess.check_call') as mock:
            client.unset_env_option('tools-metadata-url')
        environ = dict(os.environ)
        environ['JUJU_HOME'] = client.env.juju_home
        mock.assert_called_with(
            ('juju', '--show-log', 'set-env', '-e', 'foo',
             'tools-metadata-url='), stderr=None)

    @contextmanager
    def run_model_defaults_test(self, operation_name):
        env = SimpleEnvironment('foo', {'type': 'local'})
        client = EnvJujuClient1X(env, '1.23-series-arch', None)
        yield client
        self.assertEqual('INFO No model-defaults stored for client '
                         '(attempted {}).\n'.format(operation_name),
                         self.log_stream.getvalue())

    def test_get_model_defaults(self):
        with self.run_model_defaults_test('get') as client:
            client.get_model_defaults('some-key')

    def test_set_model_defaults(self):
        with self.run_model_defaults_test('set') as client:
            client.set_model_defaults('some-key', 'some-value')

    def test_unset_model_defaults(self):
        with self.run_model_defaults_test('unset') as client:
            client.unset_model_defaults('some-key')

    def test_set_testing_agent_metadata_url(self):
        env = SimpleEnvironment(None, {'type': 'foo'})
        client = EnvJujuClient1X(env, None, None)
        with patch.object(client, 'get_env_option') as mock_get:
            mock_get.return_value = 'https://example.org/juju/tools'
            with patch.object(client, 'set_env_option') as mock_set:
                client.set_testing_agent_metadata_url()
        mock_get.assert_called_with('tools-metadata-url')
        mock_set.assert_called_with(
            'tools-metadata-url',
            'https://example.org/juju/testing/tools')

    def test_set_testing_agent_metadata_url_noop(self):
        env = SimpleEnvironment(None, {'type': 'foo'})
        client = EnvJujuClient1X(env, None, None)
        with patch.object(client, 'get_env_option') as mock_get:
            mock_get.return_value = 'https://example.org/juju/testing/tools'
            with patch.object(client, 'set_env_option') as mock_set:
                client.set_testing_agent_metadata_url()
        mock_get.assert_called_with('tools-metadata-url')
        self.assertEqual(0, mock_set.call_count)

    def test_juju(self):
        env = SimpleEnvironment('qux')
        client = EnvJujuClient1X(env, None, 'juju')
        with patch('subprocess.check_call') as mock:
            client.juju('foo', ('bar', 'baz'))
        environ = dict(os.environ)
        environ['JUJU_HOME'] = client.env.juju_home
        mock.assert_called_with(('juju', '--show-log', 'foo', '-e', 'qux',
                                 'bar', 'baz'), stderr=None)

    def test_juju_env(self):
        env = SimpleEnvironment('qux')
        client = EnvJujuClient1X(env, None, '/foobar/baz')

        def check_path(*args, **kwargs):
            self.assertRegexpMatches(os.environ['PATH'], r'/foobar\:')
        with patch('subprocess.check_call', side_effect=check_path):
            client.juju('foo', ('bar', 'baz'))

    def test_juju_no_check(self):
        env = SimpleEnvironment('qux')
        client = EnvJujuClient1X(env, None, 'juju')
        environ = dict(os.environ)
        environ['JUJU_HOME'] = client.env.juju_home
        with patch('subprocess.call') as mock:
            client.juju('foo', ('bar', 'baz'), check=False)
        mock.assert_called_with(('juju', '--show-log', 'foo', '-e', 'qux',
                                 'bar', 'baz'), stderr=None)

    def test_juju_no_check_env(self):
        env = SimpleEnvironment('qux')
        client = EnvJujuClient1X(env, None, '/foobar/baz')

        def check_path(*args, **kwargs):
            self.assertRegexpMatches(os.environ['PATH'], r'/foobar\:')
        with patch('subprocess.call', side_effect=check_path):
            client.juju('foo', ('bar', 'baz'), check=False)

    def test_juju_timeout(self):
        env = SimpleEnvironment('qux')
        client = EnvJujuClient1X(env, None, '/foobar/baz')
        with patch('subprocess.check_call') as cc_mock:
            client.juju('foo', ('bar', 'baz'), timeout=58)
        self.assertEqual(cc_mock.call_args[0][0], (
            sys.executable, get_timeout_path(), '58.00', '--', 'baz',
            '--show-log', 'foo', '-e', 'qux', 'bar', 'baz'))

    def test_juju_juju_home(self):
        env = SimpleEnvironment('qux')
        os.environ['JUJU_HOME'] = 'foo'
        client = EnvJujuClient1X(env, None, '/foobar/baz')

        def check_home(*args, **kwargs):
            self.assertEqual(os.environ['JUJU_HOME'], 'foo')
            yield
            self.assertEqual(os.environ['JUJU_HOME'], 'asdf')
            yield

        with patch('subprocess.check_call', side_effect=check_home):
            client.juju('foo', ('bar', 'baz'))
            client.env.juju_home = 'asdf'
            client.juju('foo', ('bar', 'baz'))

    def test_juju_extra_env(self):
        env = SimpleEnvironment('qux')
        client = EnvJujuClient1X(env, None, 'juju')
        extra_env = {'JUJU': '/juju', 'JUJU_HOME': client.env.juju_home}

        def check_env(*args, **kwargs):
            self.assertEqual('/juju', os.environ['JUJU'])

        with patch('subprocess.check_call', side_effect=check_env) as mock:
            client.juju('quickstart', ('bar', 'baz'), extra_env=extra_env)
        mock.assert_called_with(
            ('juju', '--show-log', 'quickstart', '-e', 'qux', 'bar', 'baz'),
            stderr=None)

    def test_juju_backup_with_tgz(self):
        env = SimpleEnvironment('qux')
        client = EnvJujuClient1X(env, None, '/foobar/baz')

        def check_env(*args, **kwargs):
            self.assertEqual(os.environ['JUJU_ENV'], 'qux')
            return 'foojuju-backup-24.tgzz'
        with patch('subprocess.check_output',
                   side_effect=check_env) as co_mock:
            backup_file = client.backup()
        self.assertEqual(backup_file, os.path.abspath('juju-backup-24.tgz'))
        assert_juju_call(self, co_mock, client, ['juju', 'backup'])

    def test_juju_backup_with_tar_gz(self):
        env = SimpleEnvironment('qux')
        client = EnvJujuClient1X(env, None, '/foobar/baz')
        with patch('subprocess.check_output',
                   return_value='foojuju-backup-123-456.tar.gzbar'):
            backup_file = client.backup()
        self.assertEqual(
            backup_file, os.path.abspath('juju-backup-123-456.tar.gz'))

    def test_juju_backup_no_file(self):
        env = SimpleEnvironment('qux')
        client = EnvJujuClient1X(env, None, '/foobar/baz')
        with patch('subprocess.check_output', return_value=''):
            with self.assertRaisesRegexp(
                    Exception, 'The backup file was not found in output'):
                client.backup()

    def test_juju_backup_wrong_file(self):
        env = SimpleEnvironment('qux')
        client = EnvJujuClient1X(env, None, '/foobar/baz')
        with patch('subprocess.check_output',
                   return_value='mumu-backup-24.tgz'):
            with self.assertRaisesRegexp(
                    Exception, 'The backup file was not found in output'):
                client.backup()

    def test_juju_backup_environ(self):
        env = SimpleEnvironment('qux')
        client = EnvJujuClient1X(env, None, '/foobar/baz')
        environ = client._shell_environ()
        environ['JUJU_ENV'] = client.env.environment

        def side_effect(*args, **kwargs):
            self.assertEqual(environ, os.environ)
            return 'foojuju-backup-123-456.tar.gzbar'
        with patch('subprocess.check_output', side_effect=side_effect):
            client.backup()
            self.assertNotEqual(environ, os.environ)

    def test_restore_backup(self):
        env = SimpleEnvironment('qux')
        client = EnvJujuClient1X(env, None, '/foobar/baz')
        with patch.object(client, 'get_juju_output') as gjo_mock:
            result = client.restore_backup('quxx')
        gjo_mock.assert_called_once_with('restore', '--constraints',
                                         'mem=2G', 'quxx')
        self.assertIs(gjo_mock.return_value, result)

    def test_restore_backup_async(self):
        env = SimpleEnvironment('qux')
        client = EnvJujuClient1X(env, None, '/foobar/baz')
        with patch.object(client, 'juju_async') as gjo_mock:
            result = client.restore_backup_async('quxx')
        gjo_mock.assert_called_once_with(
            'restore', ('--constraints', 'mem=2G', 'quxx'))
        self.assertIs(gjo_mock.return_value, result)

    def test_enable_ha(self):
        env = SimpleEnvironment('qux')
        client = EnvJujuClient1X(env, None, '/foobar/baz')
        with patch.object(client, 'juju', autospec=True) as eha_mock:
            client.enable_ha()
        eha_mock.assert_called_once_with('ensure-availability', ('-n', '3'))

    def test_juju_async(self):
        env = SimpleEnvironment('qux')
        client = EnvJujuClient1X(env, None, '/foobar/baz')
        with patch('subprocess.Popen') as popen_class_mock:
            with client.juju_async('foo', ('bar', 'baz')) as proc:
                assert_juju_call(self, popen_class_mock, client, (
                    'baz', '--show-log', 'foo', '-e', 'qux', 'bar', 'baz'))
                self.assertIs(proc, popen_class_mock.return_value)
                self.assertEqual(proc.wait.call_count, 0)
                proc.wait.return_value = 0
        proc.wait.assert_called_once_with()

    def test_juju_async_failure(self):
        env = SimpleEnvironment('qux')
        client = EnvJujuClient1X(env, None, '/foobar/baz')
        with patch('subprocess.Popen') as popen_class_mock:
            with self.assertRaises(subprocess.CalledProcessError) as err_cxt:
                with client.juju_async('foo', ('bar', 'baz')):
                    proc_mock = popen_class_mock.return_value
                    proc_mock.wait.return_value = 23
        self.assertEqual(err_cxt.exception.returncode, 23)
        self.assertEqual(err_cxt.exception.cmd, (
            'baz', '--show-log', 'foo', '-e', 'qux', 'bar', 'baz'))

    def test_juju_async_environ(self):
        env = SimpleEnvironment('qux')
        client = EnvJujuClient1X(env, None, '/foobar/baz')
        environ = client._shell_environ()
        proc_mock = Mock()
        with patch('subprocess.Popen') as popen_class_mock:

            def check_environ(*args, **kwargs):
                self.assertEqual(environ, os.environ)
                return proc_mock
            popen_class_mock.side_effect = check_environ
            proc_mock.wait.return_value = 0
            with client.juju_async('foo', ('bar', 'baz')):
                pass
            self.assertNotEqual(environ, os.environ)

    def test_is_jes_enabled(self):
        env = SimpleEnvironment('qux')
        client = EnvJujuClient1X(env, None, '/foobar/baz')
        fake_popen = FakePopen(' %s' % SYSTEM, None, 0)
        with patch('subprocess.Popen',
                   return_value=fake_popen) as po_mock:
            self.assertIsFalse(client.is_jes_enabled())
        self.assertEqual(0, po_mock.call_count)

    def test_get_jes_command(self):
        env = SimpleEnvironment('qux')
        client = EnvJujuClient1X(env, None, '/foobar/baz')
        # Juju 1.24 and older do not have a JES command. It is an error
        # to call get_jes_command when is_jes_enabled is False
        fake_popen = FakePopen(' %s' % SYSTEM, None, 0)
        with patch('subprocess.Popen',
                   return_value=fake_popen) as po_mock:
            with self.assertRaises(JESNotSupported):
                client.get_jes_command()
        self.assertEqual(0, po_mock.call_count)

    def test_get_juju_timings(self):
        env = SimpleEnvironment('foo')
        client = EnvJujuClient1X(env, None, 'my/juju/bin')
        client._backend.juju_timings = {("juju", "op1"): [1],
                                        ("juju", "op2"): [2]}
        flattened_timings = client.get_juju_timings()
        expected = {"juju op1": [1], "juju op2": [2]}
        self.assertEqual(flattened_timings, expected)

    def test_deploy_bundle_1x(self):
        client = EnvJujuClient1X(SimpleEnvironment('an_env', None),
                                 '1.23-series-arch', None)
        with patch.object(client, 'juju') as mock_juju:
            client.deploy_bundle('bundle:~juju-qa/some-bundle')
        mock_juju.assert_called_with(
            'deployer', ('--debug', '--deploy-delay', '10', '--timeout',
                         '3600', '--config', 'bundle:~juju-qa/some-bundle'))

    def test_deploy_bundle_template(self):
        client = EnvJujuClient1X(SimpleEnvironment('an_env', None),
                                 '1.23-series-arch', None)
        with patch.object(client, 'juju') as mock_juju:
            client.deploy_bundle('bundle:~juju-qa/some-{container}-bundle')
        mock_juju.assert_called_with(
            'deployer', (
                '--debug', '--deploy-delay', '10', '--timeout', '3600',
                '--config', 'bundle:~juju-qa/some-lxc-bundle'))

    def test_deployer(self):
        client = EnvJujuClient1X(SimpleEnvironment(None, {'type': 'local'}),
                                 '1.23-series-arch', None)
        with patch.object(EnvJujuClient1X, 'juju') as mock:
            client.deployer('bundle:~juju-qa/some-bundle')
        mock.assert_called_with(
            'deployer', ('--debug', '--deploy-delay', '10', '--timeout',
                         '3600', '--config', 'bundle:~juju-qa/some-bundle'))

    def test_deployer_template(self):
        client = EnvJujuClient1X(SimpleEnvironment(None, {'type': 'local'}),
                                 '1.23-series-arch', None)
        with patch.object(EnvJujuClient1X, 'juju') as mock:
            client.deployer('bundle:~juju-qa/some-{container}-bundle')
        mock.assert_called_with(
            'deployer', (
                '--debug', '--deploy-delay', '10', '--timeout', '3600',
                '--config', 'bundle:~juju-qa/some-lxc-bundle'))

    def test_deployer_with_bundle_name(self):
        client = EnvJujuClient1X(SimpleEnvironment(None, {'type': 'local'}),
                                 '1.23-series-arch', None)
        with patch.object(EnvJujuClient1X, 'juju') as mock:
            client.deployer('bundle:~juju-qa/some-bundle', 'name')
        mock.assert_called_with(
            'deployer', ('--debug', '--deploy-delay', '10', '--timeout',
                         '3600', '--config', 'bundle:~juju-qa/some-bundle',
                         'name'))

    def test_quickstart_maas(self):
        client = EnvJujuClient1X(SimpleEnvironment(None, {'type': 'maas'}),
                                 '1.23-series-arch', '/juju')
        with patch.object(EnvJujuClient1X, 'juju') as mock:
            client.quickstart('bundle:~juju-qa/some-bundle')
        mock.assert_called_with(
            'quickstart', ('--constraints', 'mem=2G', '--no-browser',
                           'bundle:~juju-qa/some-bundle'),
            extra_env={'JUJU': '/juju'})

    def test_quickstart_local(self):
        client = EnvJujuClient1X(SimpleEnvironment(None, {'type': 'local'}),
                                 '1.23-series-arch', '/juju')
        with patch.object(EnvJujuClient1X, 'juju') as mock:
            client.quickstart('bundle:~juju-qa/some-bundle')
        mock.assert_called_with(
            'quickstart', ('--constraints', 'mem=2G', '--no-browser',
                           'bundle:~juju-qa/some-bundle'),
            extra_env={'JUJU': '/juju'})

    def test_quickstart_template(self):
        client = EnvJujuClient1X(SimpleEnvironment(None, {'type': 'local'}),
                                 '1.23-series-arch', '/juju')
        with patch.object(EnvJujuClient1X, 'juju') as mock:
            client.quickstart('bundle:~juju-qa/some-{container}-bundle')
        mock.assert_called_with(
            'quickstart', (
                '--constraints', 'mem=2G', '--no-browser',
                'bundle:~juju-qa/some-lxc-bundle'),
            extra_env={'JUJU': '/juju'})

    def test_list_models(self):
        env = SimpleEnvironment('foo', {'type': 'local'})
        client = EnvJujuClient1X(env, '1.23-series-arch', None)
        client.list_models()
        self.assertEqual(
            'INFO The model is environment foo\n',
            self.log_stream.getvalue())

    def test__get_models(self):
        data = """\
            - name: foo
              model-uuid: aaaa
            - name: bar
              model-uuid: bbbb
        """
        env = SimpleEnvironment('foo', {'type': 'local'})
        client = fake_juju_client(cls=EnvJujuClient1X, env=env)
        with patch.object(client, 'get_juju_output', return_value=data):
            models = client._get_models()
            self.assertEqual(
                [{'name': 'foo', 'model-uuid': 'aaaa'},
                 {'name': 'bar', 'model-uuid': 'bbbb'}],
                models)

    def test__get_models_exception(self):
        env = SimpleEnvironment('foo', {'type': 'local'})
        client = fake_juju_client(cls=EnvJujuClient1X, env=env)
        with patch.object(client, 'get_juju_output',
                          side_effect=subprocess.CalledProcessError('a', 'b')):
            self.assertEqual([], client._get_models())

    def test_get_models(self):
        env = SimpleEnvironment('foo', {'type': 'local'})
        client = EnvJujuClient1X(env, '1.23-series-arch', None)
        self.assertEqual({}, client.get_models())

    def test_iter_model_clients(self):
        data = """\
            - name: foo
              model-uuid: aaaa
              owner: admin@local
            - name: bar
              model-uuid: bbbb
              owner: admin@local
        """
        client = EnvJujuClient1X(SimpleEnvironment('foo', {}), None, None)
        with patch.object(client, 'get_juju_output', return_value=data):
            model_clients = list(client.iter_model_clients())
        self.assertEqual(2, len(model_clients))
        self.assertIs(client, model_clients[0])
        self.assertEqual('bar', model_clients[1].env.environment)

    def test_get_controller_client(self):
        client = EnvJujuClient1X(SimpleEnvironment('foo'), {'bar': 'baz'},
                                 'myhome')
        controller_client = client.get_controller_client()
        self.assertIs(client, controller_client)

    def test_list_controllers(self):
        env = SimpleEnvironment('foo', {'type': 'local'})
        client = EnvJujuClient1X(env, '1.23-series-arch', None)
        client.list_controllers()
        self.assertEqual(
            'INFO The controller is environment foo\n',
            self.log_stream.getvalue())

    def test_get_controller_model_name(self):
        env = SimpleEnvironment('foo', {'type': 'local'})
        client = EnvJujuClient1X(env, '1.23-series-arch', None)
        controller_name = client.get_controller_model_name()
        self.assertEqual('foo', controller_name)

    def test_get_controller_endpoint_ipv4(self):
        env = SimpleEnvironment('foo', {'type': 'local'})
        client = EnvJujuClient1X(env, '1.23-series-arch', None)
        with patch.object(client, 'get_juju_output',
                          return_value='10.0.0.1:17070') as gjo_mock:
            endpoint = client.get_controller_endpoint()
        self.assertEqual(('10.0.0.1', '17070'), endpoint)
        gjo_mock.assert_called_once_with('api-endpoints')

    def test_get_controller_endpoint_ipv6(self):
        env = SimpleEnvironment('foo', {'type': 'local'})
        client = EnvJujuClient1X(env, '1.23-series-arch', None)
        with patch.object(client, 'get_juju_output',
                          return_value='[::1]:17070') as gjo_mock:
            endpoint = client.get_controller_endpoint()
        self.assertEqual(('::1', '17070'), endpoint)
        gjo_mock.assert_called_once_with('api-endpoints')

    def test_action_do(self):
        client = EnvJujuClient1X(SimpleEnvironment(None, {'type': 'local'}),
                                 '1.23-series-arch', None)
        with patch.object(EnvJujuClient1X, 'get_juju_output') as mock:
            mock.return_value = \
                "Action queued with id: 5a92ec93-d4be-4399-82dc-7431dbfd08f9"
            id = client.action_do("foo/0", "myaction", "param=5")
            self.assertEqual(id, "5a92ec93-d4be-4399-82dc-7431dbfd08f9")
        mock.assert_called_once_with(
            'action do', 'foo/0', 'myaction', "param=5"
        )

    def test_action_do_error(self):
        client = EnvJujuClient1X(SimpleEnvironment(None, {'type': 'local'}),
                                 '1.23-series-arch', None)
        with patch.object(EnvJujuClient1X, 'get_juju_output') as mock:
            mock.return_value = "some bad text"
            with self.assertRaisesRegexp(Exception,
                                         "Action id not found in output"):
                client.action_do("foo/0", "myaction", "param=5")

    def test_action_fetch(self):
        client = EnvJujuClient1X(SimpleEnvironment(None, {'type': 'local'}),
                                 '1.23-series-arch', None)
        with patch.object(EnvJujuClient1X, 'get_juju_output') as mock:
            ret = "status: completed\nfoo: bar"
            mock.return_value = ret
            out = client.action_fetch("123")
            self.assertEqual(out, ret)
        mock.assert_called_once_with(
            'action fetch', '123', "--wait", "1m"
        )

    def test_action_fetch_timeout(self):
        client = EnvJujuClient1X(SimpleEnvironment(None, {'type': 'local'}),
                                 '1.23-series-arch', None)
        ret = "status: pending\nfoo: bar"
        with patch.object(EnvJujuClient1X,
                          'get_juju_output', return_value=ret):
            with self.assertRaisesRegexp(Exception,
                                         "timed out waiting for action"):
                client.action_fetch("123")

    def test_action_do_fetch(self):
        client = EnvJujuClient1X(SimpleEnvironment(None, {'type': 'local'}),
                                 '1.23-series-arch', None)
        with patch.object(EnvJujuClient1X, 'get_juju_output') as mock:
            ret = "status: completed\nfoo: bar"
            # setting side_effect to an iterable will return the next value
            # from the list each time the function is called.
            mock.side_effect = [
                "Action queued with id: 5a92ec93-d4be-4399-82dc-7431dbfd08f9",
                ret]
            out = client.action_do_fetch("foo/0", "myaction", "param=5")
            self.assertEqual(out, ret)

    def test_run(self):
        env = SimpleEnvironment('name', {}, 'foo')
        client = fake_juju_client(cls=EnvJujuClient1X, env=env)
        run_list = [
            {"MachineId": "1",
             "Stdout": "Linux\n",
             "ReturnCode": 255,
             "Stderr": "Permission denied (publickey,password)"}]
        run_output = json.dumps(run_list)
        with patch.object(client._backend, 'get_juju_output',
                          return_value=run_output) as gjo_mock:
            result = client.run(('wname',), applications=['foo', 'bar'])
        self.assertEqual(run_list, result)
        gjo_mock.assert_called_once_with(
            'run', ('--format', 'json', '--service', 'foo,bar', 'wname'),
            frozenset(['migration']),
            'foo', 'name', user_name=None)

    def test_list_space(self):
        client = EnvJujuClient1X(SimpleEnvironment(None, {'type': 'local'}),
                                 '1.23-series-arch', None)
        yaml_dict = {'foo': 'bar'}
        output = yaml.safe_dump(yaml_dict)
        with patch.object(client, 'get_juju_output', return_value=output,
                          autospec=True) as gjo_mock:
            result = client.list_space()
        self.assertEqual(result, yaml_dict)
        gjo_mock.assert_called_once_with('space list')

    def test_add_space(self):
        client = EnvJujuClient1X(SimpleEnvironment(None, {'type': 'local'}),
                                 '1.23-series-arch', None)
        with patch.object(client, 'juju', autospec=True) as juju_mock:
            client.add_space('foo-space')
        juju_mock.assert_called_once_with('space create', ('foo-space'))

    def test_add_subnet(self):
        client = EnvJujuClient1X(SimpleEnvironment(None, {'type': 'local'}),
                                 '1.23-series-arch', None)
        with patch.object(client, 'juju', autospec=True) as juju_mock:
            client.add_subnet('bar-subnet', 'foo-space')
        juju_mock.assert_called_once_with('subnet add',
                                          ('bar-subnet', 'foo-space'))

    def test__shell_environ_uses_pathsep(self):
        client = EnvJujuClient1X(SimpleEnvironment('foo'), None,
                                 'foo/bar/juju')
        with patch('os.pathsep', '!'):
            environ = client._shell_environ()
        self.assertRegexpMatches(environ['PATH'], r'foo/bar\!')

    def test_set_config(self):
        client = EnvJujuClient1X(SimpleEnvironment('bar', {}), None, '/foo')
        with patch.object(client, 'juju') as juju_mock:
            client.set_config('foo', {'bar': 'baz'})
        juju_mock.assert_called_once_with('set', ('foo', 'bar=baz'))

    def test_get_config(self):
        def output(*args, **kwargs):
            return yaml.safe_dump({
                'charm': 'foo',
                'service': 'foo',
                'settings': {
                    'dir': {
                        'default': 'true',
                        'description': 'bla bla',
                        'type': 'string',
                        'value': '/tmp/charm-dir',
                    }
                }
            })
        expected = yaml.safe_load(output())
        client = EnvJujuClient1X(SimpleEnvironment('bar', {}), None, '/foo')
        with patch.object(client, 'get_juju_output',
                          side_effect=output) as gjo_mock:
            results = client.get_config('foo')
        self.assertEqual(expected, results)
        gjo_mock.assert_called_once_with('get', 'foo')

    def test_get_service_config(self):
        def output(*args, **kwargs):
            return yaml.safe_dump({
                'charm': 'foo',
                'service': 'foo',
                'settings': {
                    'dir': {
                        'default': 'true',
                        'description': 'bla bla',
                        'type': 'string',
                        'value': '/tmp/charm-dir',
                    }
                }
            })
        expected = yaml.safe_load(output())
        client = EnvJujuClient1X(SimpleEnvironment('bar', {}), None, '/foo')
        with patch.object(client, 'get_juju_output', side_effect=output):
            results = client.get_service_config('foo')
        self.assertEqual(expected, results)

    def test_get_service_config_timesout(self):
        client = EnvJujuClient1X(SimpleEnvironment('foo', {}), None, '/foo')
        with patch('jujupy.client.until_timeout', return_value=range(0)):
            with self.assertRaisesRegexp(
                    Exception, 'Timed out waiting for juju get'):
                client.get_service_config('foo')

    def test_ssh_keys(self):
        client = EnvJujuClient1X(SimpleEnvironment('foo', {}), None, None)
        given_output = 'ssh keys output'
        with patch.object(client, 'get_juju_output', autospec=True,
                          return_value=given_output) as mock:
            output = client.ssh_keys()
        self.assertEqual(output, given_output)
        mock.assert_called_once_with('authorized-keys list')

    def test_ssh_keys_full(self):
        client = EnvJujuClient1X(SimpleEnvironment('foo', {}), None, None)
        given_output = 'ssh keys full output'
        with patch.object(client, 'get_juju_output', autospec=True,
                          return_value=given_output) as mock:
            output = client.ssh_keys(full=True)
        self.assertEqual(output, given_output)
        mock.assert_called_once_with('authorized-keys list', '--full')

    def test_add_ssh_key(self):
        client = EnvJujuClient1X(SimpleEnvironment('foo', {}), None, None)
        with patch.object(client, 'get_juju_output', autospec=True,
                          return_value='') as mock:
            output = client.add_ssh_key('ak', 'bk')
        self.assertEqual(output, '')
        mock.assert_called_once_with(
            'authorized-keys add', 'ak', 'bk', merge_stderr=True)

    def test_remove_ssh_key(self):
        client = EnvJujuClient1X(SimpleEnvironment('foo', {}), None, None)
        with patch.object(client, 'get_juju_output', autospec=True,
                          return_value='') as mock:
            output = client.remove_ssh_key('ak', 'bk')
        self.assertEqual(output, '')
        mock.assert_called_once_with(
            'authorized-keys delete', 'ak', 'bk', merge_stderr=True)

    def test_import_ssh_key(self):
        client = EnvJujuClient1X(SimpleEnvironment('foo', {}), None, None)
        with patch.object(client, 'get_juju_output', autospec=True,
                          return_value='') as mock:
            output = client.import_ssh_key('gh:au', 'lp:bu')
        self.assertEqual(output, '')
        mock.assert_called_once_with(
            'authorized-keys import', 'gh:au', 'lp:bu', merge_stderr=True)

    def test_disable_commands_properties(self):
        client = EnvJujuClient1X(SimpleEnvironment('foo'), None, None)
        self.assertEqual(
            'destroy-environment', client.command_set_destroy_model)
        self.assertEqual('remove-object', client.command_set_remove_object)
        self.assertEqual('all-changes', client.command_set_all)

    def test_list_disabled_commands(self):
        client = EnvJujuClient1X(SimpleEnvironment('foo'), None, None)
        with patch.object(client, 'get_juju_output', autospec=True,
                          return_value=dedent("""\
             - command-set: destroy-model
               message: Lock Models
             - command-set: remove-object""")) as mock:
            output = client.list_disabled_commands()
        self.assertEqual([{'command-set': 'destroy-model',
                           'message': 'Lock Models'},
                          {'command-set': 'remove-object'}], output)
        mock.assert_called_once_with('block list',
                                     '--format', 'yaml')

    def test_disable_command(self):
        client = EnvJujuClient1X(SimpleEnvironment('foo'), None, None)
        with patch.object(client, 'juju', autospec=True) as mock:
            client.disable_command('all', 'message')
        mock.assert_called_once_with('block all', ('message', ))

    def test_enable_command(self):
        client = EnvJujuClient1X(SimpleEnvironment('foo'), None, None)
        with patch.object(client, 'juju', autospec=True) as mock:
            client.enable_command('all')
        mock.assert_called_once_with('unblock', 'all')


class TestEnvJujuClient25(ClientTest):

    client_class = EnvJujuClient25

    def test_enable_jes(self):
        client = self.client_class(
            SimpleEnvironment('baz', {}),
            '1.25-foobar', 'path')
        with self.assertRaises(JESNotSupported):
            client.enable_jes()

    def test_disable_jes(self):
        client = self.client_class(
            SimpleEnvironment('baz', {}),
            '1.25-foobar', 'path')
        client.feature_flags.add('jes')
        client.disable_jes()
        self.assertNotIn('jes', client.feature_flags)

    def test_clone_unchanged(self):
        client1 = self.client_class(
            SimpleEnvironment('foo'), '1.27', 'full/path', debug=True)
        client2 = client1.clone()
        self.assertIsNot(client1, client2)
        self.assertIs(type(client1), type(client2))
        self.assertIs(client1.env, client2.env)
        self.assertEqual(client1.version, client2.version)
        self.assertEqual(client1.full_path, client2.full_path)
        self.assertIs(client1.debug, client2.debug)
        self.assertEqual(client1._backend, client2._backend)

    def test_clone_changed(self):
        client1 = self.client_class(
            SimpleEnvironment('foo'), '1.27', 'full/path', debug=True)
        env2 = SimpleEnvironment('bar')
        client2 = client1.clone(env2, '1.28', 'other/path', debug=False,
                                cls=EnvJujuClient1X)
        self.assertIs(EnvJujuClient1X, type(client2))
        self.assertIs(env2, client2.env)
        self.assertEqual('1.28', client2.version)
        self.assertEqual('other/path', client2.full_path)
        self.assertIs(False, client2.debug)

    def test_clone_defaults(self):
        client1 = self.client_class(
            SimpleEnvironment('foo'), '1.27', 'full/path', debug=True)
        client2 = client1.clone()
        self.assertIsNot(client1, client2)
        self.assertIs(self.client_class, type(client2))
        self.assertEqual(set(), client2.feature_flags)


class TestEnvJujuClient22(ClientTest):

    client_class = EnvJujuClient22

    def test__shell_environ(self):
        client = self.client_class(
            SimpleEnvironment('baz', {'type': 'ec2'}), '1.22-foobar', 'path')
        env = client._shell_environ()
        self.assertEqual(env.get(JUJU_DEV_FEATURE_FLAGS), 'actions')

    def test__shell_environ_juju_home(self):
        client = self.client_class(
            SimpleEnvironment('baz', {'type': 'ec2'}), '1.22-foobar', 'path',
            'asdf')
        env = client._shell_environ()
        self.assertEqual(env['JUJU_HOME'], 'asdf')


class TestEnvJujuClient24(ClientTest):

    client_class = EnvJujuClient24

    def test_no_jes(self):
        client = self.client_class(
            SimpleEnvironment('baz', {}),
            '1.25-foobar', 'path')
        with self.assertRaises(JESNotSupported):
            client.enable_jes()
        client._use_jes = True
        env = client._shell_environ()
        self.assertNotIn('jes', env.get(JUJU_DEV_FEATURE_FLAGS, '').split(","))

    def test_add_ssh_machines(self):
        client = self.client_class(SimpleEnvironment('foo', {}), None, 'juju')
        with patch('subprocess.check_call', autospec=True) as cc_mock:
            client.add_ssh_machines(['m-foo', 'm-bar', 'm-baz'])
        assert_juju_call(self, cc_mock, client, (
            'juju', '--show-log', 'add-machine', '-e', 'foo', 'ssh:m-foo'), 0)
        assert_juju_call(self, cc_mock, client, (
            'juju', '--show-log', 'add-machine', '-e', 'foo', 'ssh:m-bar'), 1)
        assert_juju_call(self, cc_mock, client, (
            'juju', '--show-log', 'add-machine', '-e', 'foo', 'ssh:m-baz'), 2)
        self.assertEqual(cc_mock.call_count, 3)

    def test_add_ssh_machines_no_retry(self):
        client = self.client_class(SimpleEnvironment('foo', {}), None, 'juju')
        with patch('subprocess.check_call', autospec=True,
                   side_effect=[subprocess.CalledProcessError(None, None),
                                None, None, None]) as cc_mock:
            with self.assertRaises(subprocess.CalledProcessError):
                client.add_ssh_machines(['m-foo', 'm-bar', 'm-baz'])
        assert_juju_call(self, cc_mock, client, (
            'juju', '--show-log', 'add-machine', '-e', 'foo', 'ssh:m-foo'))


class TestStatus1X(FakeHomeTestCase):

    def test_model_name(self):
        status = Status1X({'environment': 'bar'}, '')
        self.assertEqual('bar', status.model_name)

    def test_get_applications_gets_services(self):
        status = Status1X({
            'services': {'service': {}},
            'applications': {'application': {}},
            }, '')
        self.assertEqual({'service': {}}, status.get_applications())

    def test_condense_status(self):
        status = Status1X({}, '')
        self.assertEqual(status.condense_status(
                             {'agent-state': 'started',
                              'agent-state-info': 'all good',
                              'agent-version': '1.25.1'}),
                         {'current': 'started', 'message': 'all good',
                          'version': '1.25.1'})

    def test_condense_status_no_info(self):
        status = Status1X({}, '')
        self.assertEqual(status.condense_status(
                             {'agent-state': 'started',
                              'agent-version': '1.25.1'}),
                         {'current': 'started', 'version': '1.25.1'})

    @staticmethod
    def run_iter_status():
        status = Status1X({
            'environment': 'fake-unit-test',
            'machines': {
                '0': {
                    'agent-state': 'started',
                    'agent-state-info': 'all good',
                    'agent-version': '1.25.1',
                    },
                },
            'services': {
                'dummy-sink': {
                    'units': {
                        'dummy-sink/0': {
                            'agent-state': 'started',
                            'agent-version': '1.25.1',
                            },
                        'dummy-sink/1': {
                            'workload-status': {
                                'current': 'active',
                                },
                            'agent-status': {
                                'current': 'executing',
                                },
                            'agent-state': 'started',
                            'agent-version': '1.25.1',
                            },
                        }
                    },
                'dummy-source': {
                    'service-status': {
                        'current': 'active',
                        },
                    'units': {
                        'dummy-source/0': {
                            'agent-state': 'started',
                            'agent-version': '1.25.1',
                            }
                        }
                    },
                },
            }, '')
        for sub_status in status.iter_status():
            yield sub_status

    def test_iter_status_range(self):
        status_set = set([(status_item.item_name, status_item.status_name,
                           status_item.current)
                          for status_item in self.run_iter_status()])
        APP = StatusItem.APPLICATION
        WORK = StatusItem.WORKLOAD
        JUJU = StatusItem.JUJU
        self.assertEqual({
            ('0', JUJU, 'started'), ('dummy-sink/0', JUJU, 'started'),
            ('dummy-sink/1', JUJU, 'executing'),
            ('dummy-sink/1', WORK, 'active'), ('dummy-source', APP, 'active'),
            ('dummy-source/0', JUJU, 'started'),
            }, status_set)

    def test_iter_status_data(self):
        iterator = self.run_iter_status()
        self.assertEqual(iterator.next().status,
                         dict(current='started', message='all good',
                              version='1.25.1'))

    def test_iter_status_container(self):
        status_dict = {'machines': {'0': {
            'containers': {'0/lxd/0': {
                'juju-status': 'bar',
                }}
            }}}
        status = Status1X(status_dict, '')
        self.assertEqual([
            StatusItem(StatusItem.JUJU, '0', {}),
            StatusItem(StatusItem.JUJU, '0/lxd/0', {}),
            ], list(status.iter_status()))

    def test_iter_status_subordinate(self):
        status_dict = {
            'machines': {},
            'services': {
                'dummy': {
                    'service-status': {},
                    'units': {'dummy/0': {
                        'workload-status': {},
                        'agent-status': {},
                        'subordinates': {
                            'dummy-sub/0': {
                                'workload-status': {},
                                'agent-status': {},
                                }
                            }
                        }},
                    }
                },
            }
        status = Status1X(status_dict, '')
        dummy_data = status.status['services']['dummy']
        dummy_0_data = dummy_data['units']['dummy/0']
        dummy_sub_0_data = dummy_0_data['subordinates']['dummy-sub/0']
        self.assertEqual([
            StatusItem(StatusItem.APPLICATION, 'dummy', {}),
            StatusItem(StatusItem.WORKLOAD, 'dummy/0', dummy_0_data),
            StatusItem(StatusItem.JUJU, 'dummy/0', {}),
            StatusItem(StatusItem.WORKLOAD, 'dummy-sub/0', dummy_sub_0_data),
            StatusItem(StatusItem.JUJU, 'dummy-sub/0', {}),
            ], list(status.iter_status()))


def fast_timeout(count):
    if False:
        yield
