from contextlib import contextmanager
import copy
from datetime import (
    datetime,
    timedelta,
    )
import json
import logging
import os
import socket
try:
    from StringIO import StringIO
except ImportError:
    from io import StringIO
import subprocess
import sys
from textwrap import dedent

try:
    from mock import (
        call,
        Mock,
        patch,
    )
except ImportError:
    from unittest.mock import (
        call,
        Mock,
        patch,
    )
import yaml

from jujupy.configuration import (
    get_bootstrap_config_path,
    )
from jujupy import (
    fake_juju_client,
    )
from jujupy.exceptions import (
    AuthNotAccepted,
    AppError,
    CannotConnectEnv,
    ErroredUnit,
    InvalidEndpoint,
    MachineError,
    NameNotAccepted,
    SoftDeadlineExceeded,
    StatusError,
    StatusNotMet,
    StatusTimeout,
    TypeNotAccepted,
    UnitError,
)
from jujupy.client import (
    Controller,
    describe_substrate,
    GroupReporter,
    get_cache_path,
    get_machine_dns_name,
    get_stripped_version_number,
    get_version_string_parts,
    juju_home_path,
    JujuData,
    Machine,
    make_safe_config,
    ModelClient,
    parse_new_state_server_from_error,
    temp_bootstrap_env,
    temp_yaml_file,
    )
from jujupy.fake import (
    get_user_register_command_info,
    get_user_register_token,
)
from jujupy.status import (
    Status,
    StatusItem,
    )
from jujupy.wait_condition import (
    CommandTime,
    ConditionList,
    WaitMachineNotPresent,
    )
from tests import (
    assert_juju_call,
    client_past_deadline,
    make_fake_juju_return,
    FakeHomeTestCase,
    FakePopen,
    observable_temp_file,
    patch_juju_call,
    TestCase,
    )
from jujupy.utility import (
    get_timeout_path,
    JujuResourceTimeout,
    scoped_environ,
    temp_dir,
    )


__metaclass__ = type


class TestVersionStringHelpers(TestCase):

    def test_parts_handles_non_tagged_releases(self):
        self.assertEqual(
            ('2.0.5', 'alpha', 'arch'),
            get_version_string_parts('2.0.5-alpha-arch')
        )

    def test_parts_handles_tagged_releases(self):
        self.assertEqual(
            ('2.0-beta1', 'alpha', 'arch'),
            get_version_string_parts('2.0-beta1-alpha-arch')
        )

    def test_stripped_version_handles_only_version(self):
        self.assertEqual(
            'a',
            get_stripped_version_number('a')
        )

    def test_stripped_version_returns_only_version(self):
        self.assertEqual(
            'a',
            get_stripped_version_number('a-b-c')
        )


class TestErroredUnit(TestCase):

    def test_output(self):
        e = ErroredUnit('bar', 'baz')
        self.assertEqual('bar is in state baz', str(e))


class ClientTest(FakeHomeTestCase):

    def setUp(self):
        super(ClientTest, self).setUp()
        patcher = patch('jujupy.client.pause')
        backend_patcher = patch('jujupy.backend.pause')
        self.addCleanup(patcher.stop)
        self.addCleanup(backend_patcher.stop)
        self.pause_mock = patcher.start()
        backend_patcher.start()


class TestTempYamlFile(TestCase):

    def test_temp_yaml_file(self):
        with temp_yaml_file({'foo': 'bar'}) as yaml_file:
            with open(yaml_file) as f:
                self.assertEqual({'foo': 'bar'}, yaml.safe_load(f))


def backend_call(client, cmd, args, model=None, check=True, timeout=None,
                 extra_env=None):
    """Return the mock.call for this command."""
    return call(cmd, args, client.used_feature_flags,
                client.env.juju_home, model, check, timeout, extra_env,
                suppress_err=False)


def make_resource_list(service_app_id='applicationId'):
    return {'resources': [{
        'expected': {
            'origin': 'upload', 'used': True, 'description': 'foo resource.',
            'username': 'admin', 'resourceid': 'dummy-resource/foo',
            'name': 'foo', service_app_id: 'dummy-resource', 'size': 27,
            'fingerprint': '1234', 'type': 'file', 'path': 'foo.txt'},
        'unit': {
            'origin': 'upload', 'username': 'admin', 'used': True,
            'name': 'foo', 'resourceid': 'dummy-resource/foo',
            service_app_id: 'dummy-resource', 'fingerprint': '1234',
            'path': 'foo.txt', 'size': 27, 'type': 'file',
            'description': 'foo resource.'}}]}


class TestModelClient(ClientTest):

    def test_get_full_path(self):
        with patch('subprocess.check_output',
                   return_value=b'asdf\n') as co_mock:
            with patch('sys.platform', 'linux2'):
                path = ModelClient.get_full_path()
        co_mock.assert_called_once_with(('which', 'juju'))
        expected = u'asdf'
        self.assertEqual(expected, path)
        self.assertIs(type(expected), type(path))

    def test_get_full_path_encoding(self):
        # Test with non-ascii-compatible encoding
        with patch('subprocess.check_output',
                   return_value='asdf\n'.encode('EBCDIC-CP-BE')) as co_mock:
            with patch('sys.platform', 'linux2'):
                with patch('jujupy.client.getpreferredencoding',
                           return_value='EBCDIC-CP-BE'):
                    path = ModelClient.get_full_path()
        co_mock.assert_called_once_with(('which', 'juju'))
        expected = u'asdf'
        self.assertEqual(expected, path)
        self.assertIs(type(expected), type(path))

    def test_no_duplicate_env(self):
        env = JujuData('foo', {})
        client = ModelClient(env, '1.25', 'full_path')
        self.assertIs(env, client.env)

    def test_get_version(self):
        value = ' 5.6 \n'.encode('ascii')
        with patch('subprocess.check_output', return_value=value) as vsn:
            version = ModelClient.get_version()
        self.assertEqual('5.6', version)
        vsn.assert_called_with(('juju', '--version'))

    def test_get_version_path(self):
        with patch('subprocess.check_output',
                   return_value=' 4.3'.encode('ascii')) as vsn:
            ModelClient.get_version('foo/bar/baz')
        vsn.assert_called_once_with(('foo/bar/baz', '--version'))

    def test_get_matching_agent_version(self):
        client = ModelClient(
            JujuData(None, {'type': 'lxd'}, juju_home='foo'),
            '1.23-series-arch', None)
        self.assertEqual('1.23', client.get_matching_agent_version())

    def test_upgrade_juju_nonlocal(self):
        client = ModelClient(
            JujuData('foo', {'type': 'nonlocal'}), '2.0-betaX', None)
        with patch.object(client, '_upgrade_juju') as juju_mock:
            client.upgrade_juju()
        juju_mock.assert_called_with(('--agent-version', '2.0'))

    def test_upgrade_juju_no_force_version(self):
        client = ModelClient(
            JujuData('foo', {'type': 'lxd'}), '2.0-betaX', None)
        with patch.object(client, '_upgrade_juju') as juju_mock:
            client.upgrade_juju(force_version=False)
        juju_mock.assert_called_with(())

    def test_clone_unchanged(self):
        client1 = ModelClient(JujuData('foo'), '1.27', 'full/path',
                              debug=True)
        client2 = client1.clone()
        self.assertIsNot(client1, client2)
        self.assertIs(type(client1), type(client2))
        self.assertIs(client1.env, client2.env)
        self.assertEqual(client1.version, client2.version)
        self.assertEqual(client1.full_path, client2.full_path)
        self.assertIs(client1.debug, client2.debug)
        self.assertEqual(client1.feature_flags, client2.feature_flags)
        self.assertEqual(client1._backend, client2._backend)

    def test_get_cache_path(self):
        client = ModelClient(JujuData('foo', juju_home='/foo/'),
                             '1.27', 'full/path', debug=True)
        self.assertEqual('/foo/models/cache.yaml',
                         client.get_cache_path())

    def test_make_model_config_prefers_agent_metadata_url(self):
        env = JujuData('qux', {
            'agent-metadata-url': 'foo',
            'tools-metadata-url': 'bar',
            'type': 'baz',
            })
        client = ModelClient(env, None, 'my/juju/bin')
        self.assertEqual({
            'agent-metadata-url': 'foo',
            'test-mode': True,
            }, client.make_model_config())

    def test__bootstrap_config(self):
        env = JujuData('foo', {
            'access-key': 'foo',
            'admin-secret': 'foo',
            'agent-stream': 'foo',
            'application-id': 'foo',
            'application-password': 'foo',
            'auth-url': 'foo',
            'authorized-keys': 'foo',
            'availability-sets-enabled': 'foo',
            'bootstrap-host': 'foo',
            'bootstrap-timeout': 'foo',
            'bootstrap-user': 'foo',
            'client-email': 'foo',
            'client-id': 'foo',
            'container': 'foo',
            'control-bucket': 'foo',
            'default-series': 'foo',
            'development': False,
            'enable-os-upgrade': 'foo',
            'host': 'foo',
            'image-metadata-url': 'foo',
            'location': 'foo',
            'maas-oauth': 'foo',
            'maas-server': 'foo',
            'manta-key-id': 'foo',
            'manta-user': 'foo',
            'management-subscription-id': 'foo',
            'management-certificate': 'foo',
            'name': 'foo',
            'password': 'foo',
            'prefer-ipv6': 'foo',
            'private-key': 'foo',
            'region': 'foo',
            'sdc-key-id': 'foo',
            'sdc-url': 'foo',
            'sdc-user': 'foo',
            'secret-key': 'foo',
            'storage-account-name': 'foo',
            'subscription-id': 'foo',
            'tenant-id': 'foo',
            'tenant-name': 'foo',
            'test-mode': False,
            'tools-metadata-url': 'steve',
            'type': 'foo',
            'username': 'foo',
            }, 'home')
        client = ModelClient(env, None, 'my/juju/bin')
        with client._bootstrap_config() as config_filename:
            with open(config_filename) as f:
                self.assertEqual({
                    'agent-metadata-url': 'steve',
                    'agent-stream': 'foo',
                    'authorized-keys': 'foo',
                    'availability-sets-enabled': 'foo',
                    'bootstrap-timeout': 'foo',
                    'bootstrap-user': 'foo',
                    'container': 'foo',
                    'default-series': 'foo',
                    'development': False,
                    'enable-os-upgrade': 'foo',
                    'image-metadata-url': 'foo',
                    'prefer-ipv6': 'foo',
                    'test-mode': True,
                    }, yaml.safe_load(f))

    def test_get_cloud_region(self):
        self.assertEqual(
            'foo/bar', ModelClient.get_cloud_region('foo', 'bar'))
        self.assertEqual(
            'foo', ModelClient.get_cloud_region('foo', None))

    def test_bootstrap_maas(self):
        env = JujuData('maas', {'type': 'foo', 'region': 'asdf'})
        with patch_juju_call(ModelClient) as mock:
            client = ModelClient(env, '2.0-zeta1', None)
            with patch.object(client.env, 'maas', lambda: True):
                with observable_temp_file() as config_file:
                    client.bootstrap()
            mock.assert_called_with(
                'bootstrap', (
                    '--constraints', 'mem=2G spaces=^endpoint-bindings-data,'
                    '^endpoint-bindings-public',
                    'foo/asdf', 'maas',
                    '--config', config_file.name, '--default-model', 'maas',
                    '--agent-version', '2.0'),
                include_e=False)

    def test_bootstrap_maas_spaceless(self):
        # Disable space constraint with environment variable
        os.environ['JUJU_CI_SPACELESSNESS'] = "1"
        env = JujuData('maas', {'type': 'foo', 'region': 'asdf'})
        with patch_juju_call(ModelClient) as mock:
            client = ModelClient(env, '2.0-zeta1', None)
            with patch.object(client.env, 'maas', lambda: True):
                with observable_temp_file() as config_file:
                    client.bootstrap()
            mock.assert_called_with(
                'bootstrap', (
                    '--constraints', 'mem=2G',
                    'foo/asdf', 'maas',
                    '--config', config_file.name, '--default-model', 'maas',
                    '--agent-version', '2.0'),
                include_e=False)

    def test_bootstrap(self):
        env = JujuData('foo', {'type': 'bar', 'region': 'baz'})
        with observable_temp_file() as config_file:
            with patch_juju_call(ModelClient) as mock:
                client = ModelClient(env, '2.0-zeta1', None)
                client.bootstrap()
                mock.assert_called_with(
                    'bootstrap', ('--constraints', 'mem=2G',
                                  'bar/baz', 'foo',
                                  '--config', config_file.name,
                                  '--default-model', 'foo',
                                  '--agent-version', '2.0'), include_e=False)
                config_file.seek(0)
                config = yaml.safe_load(config_file)
        self.assertEqual({'test-mode': True}, config)

    def test_bootstrap_upload_tools(self):
        env = JujuData('foo', {'type': 'foo', 'region': 'baz'})
        client = ModelClient(env, '2.0-zeta1', None)
        with observable_temp_file() as config_file:
            with patch_juju_call(client) as mock:
                client.bootstrap(upload_tools=True)
        mock.assert_called_with(
            'bootstrap', (
                '--upload-tools', '--constraints', 'mem=2G',
                'foo/baz', 'foo',
                '--config', config_file.name,
                '--default-model', 'foo'), include_e=False)

    def test_bootstrap_credential(self):
        env = JujuData('foo', {'type': 'foo', 'region': 'baz'})
        client = ModelClient(env, '2.0-zeta1', None)
        with observable_temp_file() as config_file:
            with patch_juju_call(client) as mock:
                client.bootstrap(credential='credential_name')
        mock.assert_called_with(
            'bootstrap', (
                '--constraints', 'mem=2G',
                'foo/baz', 'foo',
                '--config', config_file.name,
                '--default-model', 'foo', '--agent-version', '2.0',
                '--credential', 'credential_name'), include_e=False)

    def test_bootstrap_bootstrap_series(self):
        env = JujuData('foo', {'type': 'bar', 'region': 'baz'})
        client = ModelClient(env, '2.0-zeta1', None)
        with patch_juju_call(client) as mock:
            with observable_temp_file() as config_file:
                client.bootstrap(bootstrap_series='angsty')
        mock.assert_called_with(
            'bootstrap', (
                '--constraints', 'mem=2G',
                'bar/baz', 'foo',
                '--config', config_file.name, '--default-model', 'foo',
                '--agent-version', '2.0',
                '--bootstrap-series', 'angsty'), include_e=False)

    def test_bootstrap_auto_upgrade(self):
        env = JujuData('foo', {'type': 'bar', 'region': 'baz'})
        client = ModelClient(env, '2.0-zeta1', None)
        with patch_juju_call(client) as mock:
            with observable_temp_file() as config_file:
                client.bootstrap(auto_upgrade=True)
        mock.assert_called_with(
            'bootstrap', (
                '--constraints', 'mem=2G',
                'bar/baz', 'foo',
                '--config', config_file.name, '--default-model', 'foo',
                '--agent-version', '2.0', '--auto-upgrade'), include_e=False)

    def test_bootstrap_no_gui(self):
        env = JujuData('foo', {'type': 'bar', 'region': 'baz'})
        client = ModelClient(env, '2.0-zeta1', None)
        with patch_juju_call(client) as mock:
            with observable_temp_file() as config_file:
                client.bootstrap(no_gui=True)
        mock.assert_called_with(
            'bootstrap', (
                '--constraints', 'mem=2G',
                'bar/baz', 'foo',
                '--config', config_file.name, '--default-model', 'foo',
                '--agent-version', '2.0', '--no-dashboard'), include_e=False)

    def test_bootstrap_metadata(self):
        env = JujuData('foo', {'type': 'bar', 'region': 'baz'})
        client = ModelClient(env, '2.0-zeta1', None)
        with patch_juju_call(client) as mock:
            with observable_temp_file() as config_file:
                client.bootstrap(metadata_source='/var/test-source')
        mock.assert_called_with(
            'bootstrap', (
                '--constraints', 'mem=2G',
                'bar/baz', 'foo',
                '--config', config_file.name, '--default-model', 'foo',
                '--agent-version', '2.0',
                '--metadata-source', '/var/test-source'), include_e=False)

    def test_get_bootstrap_args_bootstrap_to(self):
        env = JujuData(
            'foo', {'type': 'bar', 'region': 'baz'}, bootstrap_to='zone=fnord')
        client = ModelClient(env, '2.0-zeta1', None)
        args = client.get_bootstrap_args(
            upload_tools=False, config_filename='config')
        self.assertEqual(
            ('--constraints', 'mem=2G', 'bar/baz', 'foo',
             '--config', 'config', '--default-model', 'foo',
             '--agent-version', '2.0', '--to', 'zone=fnord'),
            args)

    def test_bootstrap_async(self):
        env = JujuData('foo', {'type': 'bar', 'region': 'baz'})
        with patch.object(ModelClient, 'juju_async', autospec=True) as mock:
            client = ModelClient(env, '2.0-zeta1', None)
            client.env.juju_home = 'foo'
            with observable_temp_file() as config_file:
                with client.bootstrap_async():
                    mock.assert_called_once_with(
                        client, 'bootstrap', (
                            '--constraints', 'mem=2G',
                            'bar/baz', 'foo',
                            '--config', config_file.name,
                            '--default-model', 'foo',
                            '--agent-version', '2.0'), include_e=False)

    def test_bootstrap_async_upload_tools(self):
        env = JujuData('foo', {'type': 'bar', 'region': 'baz'})
        with patch.object(ModelClient, 'juju_async', autospec=True) as mock:
            client = ModelClient(env, '2.0-zeta1', None)
            with observable_temp_file() as config_file:
                with client.bootstrap_async(upload_tools=True):
                    mock.assert_called_with(
                        client, 'bootstrap', (
                            '--upload-tools', '--constraints', 'mem=2G',
                            'bar/baz', 'foo',
                            '--config', config_file.name,
                            '--default-model', 'foo',
                            ),
                        include_e=False)

    def test_get_bootstrap_args_bootstrap_series(self):
        env = JujuData('foo', {'type': 'bar', 'region': 'baz'})
        client = ModelClient(env, '2.0-zeta1', None)
        args = client.get_bootstrap_args(upload_tools=True,
                                         config_filename='config',
                                         bootstrap_series='angsty')
        self.assertEqual(args, (
            '--upload-tools', '--constraints', 'mem=2G',
            'bar/baz', 'foo',
            '--config', 'config', '--default-model', 'foo',
            '--bootstrap-series', 'angsty'))

    def test_get_bootstrap_args_agent_version(self):
        env = JujuData('foo', {'type': 'bar', 'region': 'baz'})
        client = ModelClient(env, '2.0-zeta1', None)
        args = client.get_bootstrap_args(upload_tools=False,
                                         config_filename='config',
                                         agent_version='2.0-lambda1')
        self.assertEqual(('--constraints', 'mem=2G',
                          'bar/baz', 'foo',
                          '--config', 'config', '--default-model', 'foo',
                          '--agent-version', '2.0-lambda1'), args)

    def test_get_bootstrap_args_upload_tools_and_agent_version(self):
        env = JujuData('foo', {'type': 'bar', 'region': 'baz'})
        client = ModelClient(env, '2.0-zeta1', None)
        with self.assertRaises(ValueError):
            client.get_bootstrap_args(upload_tools=True,
                                      config_filename='config',
                                      agent_version='2.0-lambda1')

    def test_add_model_hypenated_controller(self):
        self.do_add_model('add-model', ('-c', 'foo'))

    def do_add_model(self, create_cmd, controller_option):
        controller_client = ModelClient(JujuData('foo'), None, None)
        model_data = JujuData('bar', {'type': 'foo'})
        with patch_juju_call(controller_client) as ccj_mock:
            with observable_temp_file() as config_file:
                controller_client.add_model(model_data)
        ccj_mock.assert_called_once_with(
            create_cmd, controller_option + (
                'bar', '--config', config_file.name), include_e=False)

    def test_add_model_explicit_region(self):
        client = fake_juju_client()
        client.bootstrap()
        client.env.controller.explicit_region = True
        model = client.env.clone('new-model')
        with patch_juju_call(client._backend) as juju_mock:
            with observable_temp_file() as config_file:
                client.add_model(model)
        juju_mock.assert_called_once_with('add-model', (
            '-c', 'name', 'new-model', 'foo/bar', '--credential', 'creds',
            '--config', config_file.name),
            frozenset({'migration'}), 'foo', None, True, None, None,
            suppress_err=False)

    def test_add_model_by_name(self):
        client = fake_juju_client()
        client.bootstrap()
        with patch_juju_call(client._backend) as juju_mock:
            with observable_temp_file() as config_file:
                client.add_model('new-model')
        juju_mock.assert_called_once_with('add-model', (
            '-c', 'name', 'new-model', '--config', config_file.name),
            frozenset({'migration'}), 'foo', None, True, None, None,
            suppress_err=False)

    def test_destroy_model(self):
        env = JujuData('foo', {'type': 'ec2'})
        client = ModelClient(env, None, None)
        with patch_juju_call(client) as mock:
            client.destroy_model()
        mock.assert_called_with(
            'destroy-model', ('foo:foo', '-y', '--destroy-storage'),
            include_e=False, timeout=600)

    def test_destroy_model_azure(self):
        env = JujuData('foo', {'type': 'azure'})
        client = ModelClient(env, None, None)
        with patch_juju_call(client) as mock:
            client.destroy_model()
        mock.assert_called_with(
            'destroy-model', ('foo:foo', '-y', '--destroy-storage'),
            include_e=False, timeout=2700)

    def test_destroy_model_gce(self):
        env = JujuData('foo', {'type': 'gce'})
        client = ModelClient(env, None, None)
        with patch_juju_call(client) as mock:
            client.destroy_model()
        mock.assert_called_with(
            'destroy-model', ('foo:foo', '-y', '--destroy-storage'),
            include_e=False, timeout=1200)

    def test_kill_controller(self):
        client = ModelClient(JujuData('foo', {'type': 'ec2'}), None, None)
        with patch_juju_call(client) as juju_mock:
            client.kill_controller()
        juju_mock.assert_called_once_with(
            'kill-controller', ('foo', '-y'), check=False, include_e=False,
            timeout=600)

    def test_kill_controller_check(self):
        client = ModelClient(JujuData('foo', {'type': 'ec2'}), None, None)
        with patch_juju_call(client) as juju_mock:
            client.kill_controller(check=True)
        juju_mock.assert_called_once_with(
            'kill-controller', ('foo', '-y'), check=True, include_e=False,
            timeout=600)

    def test_kill_controller_gce(self):
        client = ModelClient(JujuData('foo', {'type': 'gce'}), None, None)
        with patch_juju_call(client) as juju_mock:
            client.kill_controller()
        juju_mock.assert_called_once_with(
            'kill-controller', ('foo', '-y'), check=False, include_e=False,
            timeout=1200)

    def test_destroy_controller(self):
        client = ModelClient(JujuData('foo', {'type': 'ec2'}), None, None)
        with patch_juju_call(client) as juju_mock:
            client.destroy_controller()
        juju_mock.assert_called_once_with(
            'destroy-controller', ('foo', '-y'), include_e=False,
            timeout=600)

    def test_destroy_controller_all_models(self):
        client = ModelClient(JujuData('foo', {'type': 'ec2'}), None, None)
        with patch_juju_call(client) as juju_mock:
            client.destroy_controller(all_models=True)
        juju_mock.assert_called_once_with(
            'destroy-controller', ('foo', '-y', '--destroy-all-models'),
            include_e=False, timeout=600)

    @contextmanager
    def mock_tear_down(self, client, destroy_raises=False, kill_raises=False):
        @contextmanager
        def patch_raise(target, attribute, raises):
            def raise_error(*args, **kwargs):
                raise subprocess.CalledProcessError(
                    1, ('juju', attribute.replace('_', '-'), '-y'))
            if raises:
                with patch.object(target, attribute, autospec=True,
                                  side_effect=raise_error) as mock:
                    yield mock
            else:
                with patch.object(
                        target, attribute, autospec=True,
                        return_value=make_fake_juju_return()) as mock:
                    yield mock

        with patch_raise(client, 'destroy_controller', destroy_raises
                         ) as mock_destroy:
            with patch_raise(client, 'kill_controller', kill_raises
                             ) as mock_kill:
                yield (mock_destroy, mock_kill)

    def test_tear_down(self):
        """Check that a successful tear_down calls destroy."""
        client = ModelClient(JujuData('foo', {'type': 'gce'}), None, None)
        with self.mock_tear_down(client) as (mock_destroy, mock_kill):
            client.tear_down()
        mock_destroy.assert_called_once_with(all_models=True)
        self.assertIsFalse(mock_kill.called)

    def test_tear_down_fall_back(self):
        """Check that tear_down uses kill_controller if destroy fails."""
        client = ModelClient(JujuData('foo', {'type': 'gce'}), None, None)
        with self.mock_tear_down(client, True) as (mock_destroy, mock_kill):
            with self.assertRaises(subprocess.CalledProcessError) as err:
                client.tear_down()
        self.assertEqual('destroy-controller', err.exception.cmd[1])
        mock_destroy.assert_called_once_with(all_models=True)
        mock_kill.assert_called_once_with()

    def test_tear_down_double_fail(self):
        """Check tear_down when both destroy and kill fail."""
        client = ModelClient(JujuData('foo', {'type': 'gce'}), None, None)
        with self.mock_tear_down(client, True, True) as (
                mock_destroy, mock_kill):
            with self.assertRaises(subprocess.CalledProcessError) as err:
                client.tear_down()
        self.assertEqual('kill-controller', err.exception.cmd[1])
        mock_destroy.assert_called_once_with(all_models=True)
        mock_kill.assert_called_once_with()

    def test_get_juju_output(self):
        env = JujuData('foo')
        client = ModelClient(env, None, 'juju')
        fake_popen = FakePopen('asdf', None, 0)
        with patch('subprocess.Popen', return_value=fake_popen) as mock:
            result = client.get_juju_output('bar')
        self.assertEqual('asdf'.encode('ascii'), result)
        self.assertEqual((('juju', '--show-log', 'bar', '-m', 'foo:foo'),),
                         mock.call_args[0])

    def test_get_juju_output_accepts_varargs(self):
        env = JujuData('foo')
        fake_popen = FakePopen('asdf', None, 0)
        client = ModelClient(env, None, 'juju')
        with patch('subprocess.Popen', return_value=fake_popen) as mock:
            result = client.get_juju_output('bar', 'baz', '--qux')
        self.assertEqual('asdf'.encode('ascii'), result)
        self.assertEqual((('juju', '--show-log', 'bar', '-m', 'foo:foo', 'baz',
                           '--qux'),), mock.call_args[0])

    def test_get_juju_output_stderr(self):
        env = JujuData('foo')
        fake_popen = FakePopen(None, 'Hello!', 1)
        client = ModelClient(env, None, 'juju')
        with self.assertRaises(subprocess.CalledProcessError) as exc:
            with patch('subprocess.Popen', return_value=fake_popen):
                client.get_juju_output('bar')
        self.assertEqual(exc.exception.stderr, 'Hello!'.encode('ascii'))

    def test_get_juju_output_merge_stderr(self):
        env = JujuData('foo')
        fake_popen = FakePopen('Err on out', None, 0)
        client = ModelClient(env, None, 'juju')
        with patch('subprocess.Popen', return_value=fake_popen) as mock_popen:
            result = client.get_juju_output('bar', merge_stderr=True)
        self.assertEqual(result, 'Err on out'.encode('ascii'))
        mock_popen.assert_called_once_with(
            ('juju', '--show-log', 'bar', '-m', 'foo:foo'),
            stdin=subprocess.PIPE, stderr=subprocess.STDOUT,
            stdout=subprocess.PIPE)

    def test_get_juju_output_full_cmd(self):
        env = JujuData('foo')
        fake_popen = FakePopen(None, 'Hello!', 1)
        client = ModelClient(env, None, 'juju')
        with self.assertRaises(subprocess.CalledProcessError) as exc:
            with patch('subprocess.Popen', return_value=fake_popen):
                client.get_juju_output('bar', '--baz', 'qux')
        self.assertEqual(
            ('juju', '--show-log', 'bar', '-m', 'foo:foo', '--baz', 'qux'),
            exc.exception.cmd)

    def test_get_juju_output_accepts_timeout(self):
        env = JujuData('foo')
        fake_popen = FakePopen('asdf', None, 0)
        client = ModelClient(env, None, 'juju')
        with patch('subprocess.Popen', return_value=fake_popen) as po_mock:
            client.get_juju_output('bar', timeout=5)
        self.assertEqual(
            po_mock.call_args[0][0],
            (sys.executable, get_timeout_path(), '5.00', '--', 'juju',
             '--show-log', 'bar', '-m', 'foo:foo'))

    def test__shell_environ_juju_data(self):
        client = ModelClient(
            JujuData('baz', {'type': 'ec2'}), '1.25-foobar', 'path', 'asdf')
        env = client._shell_environ()
        self.assertEqual(env['JUJU_DATA'], 'asdf')
        self.assertNotIn('JUJU_HOME', env)

    def test_juju_output_supplies_path(self):
        env = JujuData('foo')
        client = ModelClient(env, None, '/foobar/bar')

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
                """).encode('ascii')
        env = JujuData('foo')
        client = ModelClient(env, None, None)
        with patch.object(client, 'get_juju_output',
                          return_value=output_text) as gjo_mock:
            result = client.get_status()
        gjo_mock.assert_called_once_with(
            'show-status', '--format', 'yaml', controller=False)
        self.assertEqual(Status, type(result))
        self.assertEqual(['a', 'b', 'c'], result.status)

    def test_get_status_retries_on_error(self):
        env = JujuData('foo')
        client = ModelClient(env, None, None)
        client.attempt = 0

        def get_juju_output(command, *args, **kwargs):
            if client.attempt == 1:
                return '"hello"'.encode('ascii')
            client.attempt += 1
            raise subprocess.CalledProcessError(1, command)

        with patch.object(client, 'get_juju_output', get_juju_output):
            client.get_status()

    def test_get_status_raises_on_timeout_1(self):
        env = JujuData('foo')
        client = ModelClient(env, None, None)

        def get_juju_output(command, *args, **kwargs):
            raise subprocess.CalledProcessError(1, command)

        with patch.object(client, 'get_juju_output',
                          side_effect=get_juju_output):
            with patch('jujupy.client.until_timeout',
                       lambda x: iter([None, None])):
                with self.assertRaisesRegexp(
                        Exception, 'Timed out waiting for juju status'):
                        with patch('jujupy.client.time.sleep'):
                            client.get_status()

    def test_get_status_raises_on_timeout_2(self):
        env = JujuData('foo')
        client = ModelClient(env, None, None)
        with patch('jujupy.client.until_timeout',
                   return_value=iter([1])) as mock_ut:
            with patch.object(client, 'get_juju_output',
                              side_effect=StopIteration):
                with self.assertRaises(StopIteration):
                    with patch('jujupy.client.time.sleep'):
                        client.get_status(500)
        mock_ut.assert_called_with(500)

    def test_show_model_uses_provided_model_name(self):
        env = JujuData('foo')
        client = ModelClient(env, None, None)
        show_model_output = dedent("""\
            bar:
                status:
                    current: available
                    since: 4 minutes ago
                    migration: 'Some message.'
                    migration-start: 48 seconds ago
        """)
        with patch.object(
                client, 'get_juju_output',
                autospect=True, return_value=show_model_output) as m_gjo:
            output = client.show_model('bar')
        self.assertItemsEqual(['bar'], output.keys())
        m_gjo.assert_called_once_with(
            'show-model', 'foo:bar', '--format', 'yaml', include_e=False)

    def test_show_model_defaults_to_own_model_name(self):
        env = JujuData('foo')
        client = ModelClient(env, None, None)
        show_model_output = dedent("""\
            foo:
                status:
                    current: available
                    since: 4 minutes ago
                    migration: 'Some message.'
                    migration-start: 48 seconds ago
        """)
        with patch.object(
                client, 'get_juju_output',
                autospect=True, return_value=show_model_output) as m_gjo:
            output = client.show_model()
        self.assertItemsEqual(['foo'], output.keys())
        m_gjo.assert_called_once_with(
            'show-model', 'foo:foo', '--format', 'yaml', include_e=False)

    @staticmethod
    def make_status_yaml(key, machine_value, unit_value):
        return dedent("""\
            model:
              name: foo
            machines:
              "0":
                {0}: {1}
            applications:
              jenkins:
                units:
                  jenkins/0:
                    {0}: {2}
        """.format(key, machine_value, unit_value)).encode('ascii')

    def test_deploy_repository(self):
        env = ModelClient(
            JujuData('foo', {'type': 'lxd'}), '1.234-76', None)
        with patch_juju_call(env) as mock_juju:
            env.deploy('/home/jrandom/repo/mongodb')
        mock_juju.assert_called_with(
            'deploy', ('/home/jrandom/repo/mongodb',))

    def test_deploy_to(self):
        env = ModelClient(
            JujuData('foo', {'type': 'lxd'}), '1.234-76', None)
        with patch_juju_call(env) as mock_juju:
            env.deploy('mondogb', to='0')
        mock_juju.assert_called_with(
            'deploy', ('mondogb', '--to', '0'))

    def test_deploy_service(self):
        env = ModelClient(
            JujuData('foo', {'type': 'lxd'}), '1.234-76', None)
        with patch_juju_call(env) as mock_juju:
            env.deploy('local:mondogb', service='my-mondogb')
        mock_juju.assert_called_with(
            'deploy', ('local:mondogb', 'my-mondogb',))

    def test_deploy_force(self):
        env = ModelClient(
            JujuData('foo', {'type': 'lxd'}), '1.234-76', None)
        with patch_juju_call(env) as mock_juju:
            env.deploy('local:mondogb', force=True)
        mock_juju.assert_called_with('deploy', ('local:mondogb', '--force',))

    def test_deploy_xenial_series(self):
        env = ModelClient(
            JujuData('foo', {'type': 'lxd'}), '1.234-76', None)
        with patch_juju_call(env) as mock_juju:
            env.deploy('local:blah', series='xenial')
        mock_juju.assert_called_with(
            'deploy', ('local:blah', '--series', 'xenial'))

    def test_deploy_bionic_series(self):
        env = ModelClient(
            JujuData('foo', {'type': 'lxd'}), '1.234-76', None)
        with patch_juju_call(env) as mock_juju:
            env.deploy('local:blah', series='bionic')
        mock_juju.assert_called_with(
            'deploy', ('local:blah', '--series', 'bionic'))

    def test_deploy_multiple(self):
        env = ModelClient(
            JujuData('foo', {'type': 'lxd'}), '1.234-76', None)
        with patch_juju_call(env) as mock_juju:
            env.deploy('local:blah', num=2)
        mock_juju.assert_called_with(
            'deploy', ('local:blah', '-n', '2'))

    def test_deploy_resource(self):
        env = ModelClient(JujuData('foo', {'type': 'lxd'}), None, None)
        with patch_juju_call(env) as mock_juju:
            env.deploy('local:blah', resource='foo=/path/dir')
        mock_juju.assert_called_with(
            'deploy', ('local:blah', '--resource', 'foo=/path/dir'))

    def test_deploy_storage(self):
        env = ModelClient(
            JujuData('foo', {'type': 'lxd'}), '1.234-76', None)
        with patch_juju_call(env) as mock_juju:
            env.deploy('mondogb', storage='rootfs,1G')
        mock_juju.assert_called_with(
            'deploy', ('mondogb', '--storage', 'rootfs,1G'))

    def test_deploy_constraints(self):
        env = ModelClient(
            JujuData('foo', {'type': 'lxd'}), '1.234-76', None)
        with patch_juju_call(env) as mock_juju:
            env.deploy('mondogb', constraints='virt-type=kvm')
        mock_juju.assert_called_with(
            'deploy', ('mondogb', '--constraints', 'virt-type=kvm'))

    def test_deploy_bind(self):
        env = ModelClient(JujuData('foo', {'type': 'lxd'}), None, None)
        with patch_juju_call(env) as mock_juju:
            env.deploy('mydb', bind='backspace')
        mock_juju.assert_called_with('deploy', ('mydb', '--bind', 'backspace'))

    def test_deploy_aliased(self):
        env = ModelClient(JujuData('foo', {'type': 'lxd'}), None, None)
        with patch_juju_call(env) as mock_juju:
            env.deploy('local:blah', alias='blah-blah')
        mock_juju.assert_called_with(
            'deploy', ('local:blah', 'blah-blah'))

    def test_attach(self):
        env = ModelClient(JujuData('foo', {'type': 'lxd'}), None, None)
        with patch_juju_call(env) as mock_juju:
            env.attach('foo', resource='foo=/path/dir')
        mock_juju.assert_called_with('attach', ('foo', 'foo=/path/dir'))

    def test_list_resources(self):
        data = 'resourceid: resource/foo'
        client = ModelClient(JujuData('lxd'), None, None)
        with patch.object(
                client, 'get_juju_output', return_value=data) as mock_gjo:
            status = client.list_resources('foo')
        self.assertEqual(status, yaml.safe_load(data))
        mock_gjo.assert_called_with(
            'list-resources', '--format', 'yaml', 'foo', '--details')

    def test_wait_for_resource(self):
        client = ModelClient(JujuData('lxd'), None, None)
        with patch.object(
                client, 'list_resources',
                return_value=make_resource_list()) as mock_lr:
            client.wait_for_resource('dummy-resource/foo', 'foo')
        mock_lr.assert_called_once_with('foo')

    def test_wait_for_resource_timeout(self):
        client = ModelClient(JujuData('lxd'), None, None)
        resource_list = make_resource_list()
        resource_list['resources'][0]['expected']['resourceid'] = 'bad_id'
        with patch.object(
                client, 'list_resources',
                return_value=resource_list) as mock_lr:
            with patch('jujupy.client.until_timeout', autospec=True,
                       return_value=[0, 1]) as mock_ju:
                with patch('time.sleep', autospec=True) as mock_ts:
                    with self.assertRaisesRegexp(
                            JujuResourceTimeout,
                            'Timeout waiting for a resource to be downloaded'):
                        client.wait_for_resource('dummy-resource/foo', 'foo')
        calls = [call('foo'), call('foo')]
        self.assertEqual(mock_lr.mock_calls, calls)
        self.assertEqual(mock_ts.mock_calls, [call(.1), call(.1)])
        self.assertEqual(mock_ju.mock_calls, [call(60)])

    def test_wait_for_resource_suppresses_deadline(self):
        client = ModelClient(JujuData('lxd', juju_home=''), None, None)
        with client_past_deadline(client):
            real_check_timeouts = client.check_timeouts

            def list_resources(service_or_unit):
                with real_check_timeouts():
                    return make_resource_list()

            with patch.object(client, 'check_timeouts', autospec=True):
                with patch.object(client, 'list_resources', autospec=True,
                                  side_effect=list_resources):
                        client.wait_for_resource('dummy-resource/foo',
                                                 'app_unit')

    def test_wait_for_resource_checks_deadline(self):
        resource_list = make_resource_list()
        client = ModelClient(JujuData('lxd', juju_home=''), None, None)
        with client_past_deadline(client):
            with patch.object(client, 'list_resources', autospec=True,
                              return_value=resource_list):
                with self.assertRaises(SoftDeadlineExceeded):
                    client.wait_for_resource('dummy-resource/foo', 'app_unit')

    def test_deploy_bundle_2x(self):
        client = ModelClient(JujuData('an_env', None),
                             '1.23-series-arch', None)
        with patch_juju_call(client) as mock_juju:
            client.deploy_bundle('bundle:~juju-qa/some-bundle')
        mock_juju.assert_called_with(
            'deploy', ('bundle:~juju-qa/some-bundle'), timeout=3600)

    def test_deploy_bundle_template(self):
        client = ModelClient(JujuData('an_env', None),
                             '1.23-series-arch', None)
        with patch_juju_call(client) as mock_juju:
            client.deploy_bundle('bundle:~juju-qa/some-{container}-bundle')
        mock_juju.assert_called_with(
            'deploy', ('bundle:~juju-qa/some-lxd-bundle'), timeout=3600)

    def test_upgrade_charm(self):
        env = ModelClient(
            JujuData('foo', {'type': 'lxd'}), '2.34-74', None)
        with patch_juju_call(env) as mock_juju:
            env.upgrade_charm('foo-service',
                              '/bar/repository/angsty/mongodb')
        mock_juju.assert_called_once_with(
            'upgrade-charm', ('foo-service', '--path',
                              '/bar/repository/angsty/mongodb',))

    def test_remove_service(self):
        env = ModelClient(
            JujuData('foo', {'type': 'lxd'}), '1.234-76', None)
        with patch_juju_call(env) as mock_juju:
            env.remove_application('mondogb')
        mock_juju.assert_called_with('remove-application', ('mondogb',))

    def test_status_until_always_runs_once(self):
        client = ModelClient(
            JujuData('foo', {'type': 'lxd'}), '1.234-76', None)
        status_txt = self.make_status_yaml('agent-state', 'started', 'started')
        with patch.object(client, 'get_juju_output', return_value=status_txt):
            result = list(client.status_until(-1))
        self.assertEqual(
            [r.status for r in result],
            [Status.from_text(status_txt.decode('ascii')).status])

    def test_status_until_timeout(self):
        client = ModelClient(
            JujuData('foo', {'type': 'lxd'}), '1.234-76', None)
        status_txt = self.make_status_yaml('agent-state', 'started', 'started')
        status_yaml = yaml.safe_load(status_txt)

        def until_timeout_stub(timeout, start=None):
            return iter([None, None])

        with patch.object(client, 'get_juju_output', return_value=status_txt):
            with patch('jujupy.client.until_timeout',
                       side_effect=until_timeout_stub) as ut_mock:
                    with patch('jujupy.client.time.sleep'):
                        result = list(client.status_until(30, 70))
        self.assertEqual(
            [r.status for r in result], [status_yaml] * 3)
        # until_timeout is called by status as well as status_until.
        self.assertEqual(ut_mock.mock_calls,
                         [call(60), call(30, start=70), call(60), call(60)])

    def test_status_until_suppresses_deadline(self):
        with self.only_status_checks() as client:
            list(client.status_until(0))

    def test_status_until_checks_deadline(self):
        with self.status_does_not_check() as client:
            with self.assertRaises(SoftDeadlineExceeded):
                list(client.status_until(0))

    def test_add_ssh_machines(self):
        client = ModelClient(JujuData('foo'), None, 'juju')
        with patch('subprocess.check_call', autospec=True) as cc_mock:
            client.add_ssh_machines(['m-foo', 'm-bar', 'm-baz'])
        assert_juju_call(
            self,
            cc_mock,
            client,
            ('juju', '--show-log', 'add-machine',
             '-m', 'foo:foo', 'ssh:m-foo'),
            0)
        assert_juju_call(
            self,
            cc_mock,
            client,
            ('juju', '--show-log', 'add-machine',
             '-m', 'foo:foo', 'ssh:m-bar'),
            1)
        assert_juju_call(
            self,
            cc_mock,
            client,
            ('juju', '--show-log', 'add-machine',
             '-m', 'foo:foo', 'ssh:m-baz'),
            2)
        self.assertEqual(cc_mock.call_count, 3)

    def test_make_remove_machine_condition(self):
        client = fake_juju_client()
        condition = client.make_remove_machine_condition('0')
        self.assertIs(WaitMachineNotPresent, type(condition))
        self.assertEqual('0', condition.machine)
        self.assertEqual(600, condition.timeout)

    def test_make_remove_machine_condition_azure(self):
        client = fake_juju_client()
        client.env._config['type'] = 'azure'
        condition = client.make_remove_machine_condition('0')
        self.assertIs(WaitMachineNotPresent, type(condition))
        self.assertEqual('0', condition.machine)
        self.assertEqual(1200, condition.timeout)

    def test_add_ssh_machines_retry(self):
        client = ModelClient(JujuData('foo'), None, 'juju')
        with patch('subprocess.check_call', autospec=True,
                   side_effect=[subprocess.CalledProcessError(None, None),
                                None, None, None]) as cc_mock:
            client.add_ssh_machines(['m-foo', 'm-bar', 'm-baz'])
        assert_juju_call(
            self,
            cc_mock,
            client,
            ('juju', '--show-log', 'add-machine',
             '-m', 'foo:foo', 'ssh:m-foo'),
            0)
        self.pause_mock.assert_called_once_with(30)
        assert_juju_call(
            self,
            cc_mock,
            client,
            ('juju', '--show-log', 'add-machine',
             '-m', 'foo:foo', 'ssh:m-foo'),
            1)
        assert_juju_call(
            self,
            cc_mock,
            client,
            ('juju', '--show-log', 'add-machine',
             '-m', 'foo:foo', 'ssh:m-bar'),
            2)
        assert_juju_call(
            self,
            cc_mock,
            client,
            ('juju', '--show-log', 'add-machine',
             '-m', 'foo:foo', 'ssh:m-baz'),
            3)
        self.assertEqual(cc_mock.call_count, 4)

    def test_add_ssh_machines_fail_on_second_machine(self):
        client = ModelClient(JujuData('foo'), None, 'juju')
        with patch('subprocess.check_call', autospec=True, side_effect=[
                None, subprocess.CalledProcessError(None, None), None, None
                ]) as cc_mock:
            with self.assertRaises(subprocess.CalledProcessError):
                client.add_ssh_machines(['m-foo', 'm-bar', 'm-baz'])
        assert_juju_call(
            self,
            cc_mock,
            client,
            ('juju', '--show-log', 'add-machine',
             '-m', 'foo:foo', 'ssh:m-foo'),
            0)
        assert_juju_call(
            self,
            cc_mock,
            client,
            ('juju', '--show-log', 'add-machine',
             '-m', 'foo:foo', 'ssh:m-bar'),
            1)
        self.assertEqual(cc_mock.call_count, 2)

    def test_add_ssh_machines_fail_on_second_attempt(self):
        client = ModelClient(JujuData('foo'), None, 'juju')
        with patch('subprocess.check_call', autospec=True, side_effect=[
                subprocess.CalledProcessError(None, None),
                subprocess.CalledProcessError(None, None)]) as cc_mock:
            with self.assertRaises(subprocess.CalledProcessError):
                client.add_ssh_machines(['m-foo', 'm-bar', 'm-baz'])
        assert_juju_call(
            self,
            cc_mock,
            client,
            ('juju', '--show-log', 'add-machine',
             '-m', 'foo:foo', 'ssh:m-foo'),
            0)
        assert_juju_call(
            self,
            cc_mock,
            client,
            ('juju', '--show-log', 'add-machine',
             '-m', 'foo:foo', 'ssh:m-foo'),
            1)
        self.assertEqual(cc_mock.call_count, 2)

    def test_remove_machine(self):
        client = fake_juju_client()
        with patch_juju_call(client._backend) as juju_mock:
            condition = client.remove_machine('0')
        call = backend_call(
            client, 'remove-machine', ('0',), 'name:name')
        juju_mock.assert_called_once_with(*call[1], **call[2])
        self.assertEqual(condition, WaitMachineNotPresent('0', 600))

    def test_remove_machine_force(self):
        client = fake_juju_client()
        with patch_juju_call(client._backend) as juju_mock:
            client.remove_machine('0', force=True)
        call = backend_call(
            client, 'remove-machine', ('--force', '0'), 'name:name')
        juju_mock.assert_called_once_with(*call[1], **call[2])

    def test_remove_machine_azure(self):
        client = fake_juju_client(JujuData('name', {
            'type': 'azure',
            'location': 'usnorth',
            }))
        client.bootstrap()
        client.juju('add-machine', ())
        condition = client.remove_machine('0')
        self.assertEqual(condition, WaitMachineNotPresent('0', 1200))

    def test_wait_for_started(self):
        value = self.make_status_yaml('agent-state', 'started', 'started')
        client = ModelClient(JujuData('lxd'), None, None)
        with patch.object(client, 'get_juju_output', return_value=value):
            client.wait_for_started()

    def test_wait_for_started_timeout(self):
        value = self.make_status_yaml('agent-state', 'pending', 'started')
        client = ModelClient(JujuData('lxd'), None, None)
        with patch('jujupy.client.until_timeout',
                   lambda x, start=None: range(1)):
            with patch.object(client, 'get_juju_output', return_value=value):
                writes = []
                with patch.object(GroupReporter, '_write', autospec=True,
                                  side_effect=lambda _, s: writes.append(s)):
                    with self.assertRaisesRegexp(
                            StatusNotMet,
                            'Timed out waiting for agents to start in lxd'):
                        with patch('jujupy.client.time.sleep'):
                            client.wait_for_started()
                self.assertEqual(writes, ['pending: 0', ' .', '\n'])

    def test_wait_for_started_start(self):
        value = self.make_status_yaml('agent-state', 'started', 'pending')
        client = ModelClient(JujuData('lxd'), None, None)
        now = datetime.now() + timedelta(days=1)
        with patch('utility.until_timeout.now', return_value=now):
            with patch.object(client, 'get_juju_output', return_value=value):
                writes = []
                with patch.object(GroupReporter, '_write', autospec=True,
                                  side_effect=lambda _, s: writes.append(s)):
                    with self.assertRaisesRegexp(
                            StatusNotMet,
                            'Timed out waiting for agents to start in lxd'):
                        with patch('jujupy.client.time.sleep'):
                            client.wait_for_started(
                                start=now - timedelta(1200))
                self.assertEqual(writes, ['pending: jenkins/0', '\n'])

    def make_ha_status(self, voting='has-vote'):
        return {'machines': {
            '0': {'controller-member-status': voting},
            '1': {'controller-member-status': voting},
            '2': {'controller-member-status': voting},
            }}

    @contextmanager
    def only_status_checks(self, client=None, status=None):
        """This context manager ensure only get_status calls check_timeouts.

        Everything else will get a mock object.

        Also, the client is patched so that the soft_deadline has been hit.
        """
        if client is None:
            client = ModelClient(JujuData('lxd', juju_home=''), None, None)
        with client_past_deadline(client):
            # This will work even after we patch check_timeouts below.
            real_check_timeouts = client.check_timeouts

            def check(timeout=60, controller=False):
                with real_check_timeouts():
                    return client.status_class(status, '')

            with patch.object(client, 'get_status', autospec=True,
                              side_effect=check):
                with patch.object(client, 'check_timeouts', autospec=True):
                    yield client

    def test__wait_for_status_suppresses_deadline(self):

        def translate(x):
            return None

        with self.only_status_checks() as client:
            client._wait_for_status(Mock(), translate)

    @contextmanager
    def status_does_not_check(self, client=None, status=None):
        """This context manager ensure get_status never calls check_timeouts.

        Also, the client is patched so that the soft_deadline has been hit.
        """
        if client is None:
            client = ModelClient(JujuData('lxd', juju_home=''), None, None)
        with client_past_deadline(client):
            status_obj = client.status_class(status, '')
            with patch.object(client, 'get_status', autospec=True,
                              return_value=status_obj):
                yield client

    def test__wait_for_status_checks_deadline(self):

        def translate(x):
            return None

        with self.status_does_not_check() as client:
            with self.assertRaises(SoftDeadlineExceeded):
                client._wait_for_status(Mock(), translate)

    @contextmanager
    def client_status_errors(self, client, errors):
        """Patch get_status().iter_errors keeping ignore_recoverable."""
        def fake_iter_errors(ignore_recoverable):
            for error in errors.pop(0):
                if not (ignore_recoverable and error.recoverable):
                    yield error

        with patch.object(client.get_status(), 'iter_errors', autospec=True,
                          side_effect=fake_iter_errors) as errors_mock:
            yield errors_mock

    def test__wait_for_status_no_error(self):
        def translate(x):
            return {'waiting': '0'}

        errors = [[], []]
        with self.status_does_not_check() as client:
            with self.client_status_errors(client, errors) as errors_mock:
                with self.assertRaises(StatusNotMet):
                    with patch('jujupy.client.time.sleep'):
                        client._wait_for_status(Mock(), translate, timeout=0)
        errors_mock.assert_has_calls(
            [call(ignore_recoverable=True), call(ignore_recoverable=False)])

    def test__wait_for_status_raises_error(self):
        def translate(x):
            return {'waiting': '0'}

        errors = [[MachineError('0', 'error not recoverable')]]
        with self.status_does_not_check() as client:
            with self.client_status_errors(client, errors) as errors_mock:
                with self.assertRaises(MachineError):
                    with patch('jujupy.client.time.sleep'):
                        client._wait_for_status(Mock(), translate, timeout=0)
        errors_mock.assert_called_once_with(ignore_recoverable=True)

    def test__wait_for_status_delays_recoverable(self):
        def translate(x):
            return {'waiting': '0'}

        errors = [[StatusError('fake', 'error is recoverable')],
                  [UnitError('fake/0', 'error is recoverable')]]
        with self.status_does_not_check() as client:
            with self.client_status_errors(client, errors) as errors_mock:
                with self.assertRaises(UnitError):
                    with patch('jujupy.client.time.sleep'):
                        client._wait_for_status(Mock(), translate, timeout=0)
        self.assertEqual(2, errors_mock.call_count)
        errors_mock.assert_has_calls(
            [call(ignore_recoverable=True), call(ignore_recoverable=False)])

    def test_wait_for_started_logs_status(self):
        value = self.make_status_yaml('agent-state', 'pending', 'started')
        client = ModelClient(JujuData('lxd'), None, None)
        with patch.object(client, 'get_juju_output', return_value=value):
            writes = []
            with patch.object(GroupReporter, '_write', autospec=True,
                              side_effect=lambda _, s: writes.append(s)):
                with self.assertRaisesRegexp(
                        StatusNotMet,
                        'Timed out waiting for agents to start in lxd'):
                    with patch('jujupy.client.time.sleep'):
                        client.wait_for_started(0)
            self.assertEqual(writes, ['pending: 0', '\n'])
        self.assertEqual(
            self.log_stream.getvalue(), 'ERROR %s\n' % value.decode('ascii'))

    def test_wait_for_subordinate_units(self):
        value = dedent("""\
            machines:
              "0":
                agent-state: started
            applications:
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
        """).encode('ascii')
        client = ModelClient(JujuData('lxd'), None, None)
        now = datetime.now() + timedelta(days=1)
        with patch('utility.until_timeout.now', return_value=now):
            with patch.object(client, 'get_juju_output', return_value=value):
                with patch(
                        'jujupy.client.GroupReporter.update') as update_mock:
                    with patch(
                            'jujupy.client.GroupReporter.finish'
                            ) as finish_mock:
                        with patch('jujupy.client.time.sleep'):
                            client.wait_for_subordinate_units(
                                'jenkins', 'sub1', start=now - timedelta(1200))
        self.assertEqual([], update_mock.call_args_list)
        finish_mock.assert_called_once_with()

    def test_wait_for_subordinate_units_with_agent_status(self):
        value = dedent("""\
            machines:
              "0":
                agent-state: started
            applications:
              jenkins:
                units:
                  jenkins/0:
                    subordinates:
                      sub1/0:
                        agent-status:
                          current: idle
              ubuntu:
                units:
                  ubuntu/0:
                    subordinates:
                      sub2/0:
                        agent-status:
                          current: idle
                      sub3/0:
                        agent-status:
                          current: idle
        """).encode('ascii')
        client = ModelClient(JujuData('lxd'), None, None)
        now = datetime.now() + timedelta(days=1)
        with patch('utility.until_timeout.now', return_value=now):
            with patch.object(client, 'get_juju_output', return_value=value):
                with patch(
                        'jujupy.client.GroupReporter.update') as update_mock:
                    with patch(
                            'jujupy.client.GroupReporter.finish'
                            ) as finish_mock:
                        with patch('jujupy.client.time.sleep'):
                            client.wait_for_subordinate_units(
                                'jenkins', 'sub1', start=now - timedelta(1200))
        self.assertEqual([], update_mock.call_args_list)
        finish_mock.assert_called_once_with()

    def test_wait_for_multiple_subordinate_units(self):
        value = dedent("""\
            machines:
              "0":
                agent-state: started
            applications:
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
        """).encode('ascii')
        client = ModelClient(JujuData('lxd'), None, None)
        now = datetime.now() + timedelta(days=1)
        with patch('utility.until_timeout.now', return_value=now):
            with patch.object(client, 'get_juju_output', return_value=value):
                with patch(
                        'jujupy.client.GroupReporter.update') as update_mock:
                    with patch(
                            'jujupy.client.GroupReporter.finish'
                            ) as finish_mock:
                        with patch('jujupy.client.time.sleep'):
                            client.wait_for_subordinate_units(
                                'ubuntu', 'sub', start=now - timedelta(1200))
        self.assertEqual([], update_mock.call_args_list)
        finish_mock.assert_called_once_with()

    def test_wait_for_subordinate_units_checks_slash_in_unit_name(self):
        value = dedent("""\
            machines:
              "0":
                agent-state: started
            applications:
              jenkins:
                units:
                  jenkins/0:
                    subordinates:
                      sub1:
                        agent-state: started
        """).encode('ascii')
        client = ModelClient(JujuData('lxd'), None, None)
        now = datetime.now() + timedelta(days=1)
        with patch('utility.until_timeout.now', return_value=now):
            with patch.object(client, 'get_juju_output', return_value=value):
                with self.assertRaisesRegexp(
                        StatusNotMet,
                        'Timed out waiting for agents to start in lxd'):
                    with patch('jujupy.client.time.sleep'):
                        client.wait_for_subordinate_units(
                            'jenkins', 'sub1', start=now - timedelta(1200))

    def test_wait_for_subordinate_units_no_subordinate(self):
        value = dedent("""\
            machines:
              "0":
                agent-state: started
            applications:
              jenkins:
                units:
                  jenkins/0:
                    agent-state: started
        """).encode('ascii')
        client = ModelClient(JujuData('lxd'), None, None)
        now = datetime.now() + timedelta(days=1)
        with patch('utility.until_timeout.now', return_value=now):
            with patch.object(client, 'get_juju_output', return_value=value):
                with self.assertRaisesRegexp(
                        StatusNotMet,
                        'Timed out waiting for agents to start in lxd'):
                    with patch('jujupy.client.time.sleep'):
                        client.wait_for_subordinate_units(
                            'jenkins', 'sub1', start=now - timedelta(1200))

    def test_wait_for_workload(self):
        initial_status = Status.from_text("""\
            machines: {}
            applications:
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
        final_status.status['applications']['jenkins']['units']['jenkins/0'][
            'workload-status']['current'] = 'active'
        client = ModelClient(JujuData('lxd'), None, None)
        writes = []
        with patch('utility.until_timeout', autospec=True, return_value=[1]):
            with patch.object(client, 'get_status', autospec=True,
                              side_effect=[initial_status, final_status]):
                with patch.object(GroupReporter, '_write', autospec=True,
                                  side_effect=lambda _, s: writes.append(s)):
                    with patch('jujupy.client.time.sleep'):
                        client.wait_for_workloads()
        self.assertEqual(writes, ['waiting: jenkins/0', '\n'])

    def test_wait_for_workload_all_unknown(self):
        status = Status.from_text("""\
            applications:
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
        client = ModelClient(JujuData('lxd'), None, None)
        writes = []
        with patch('utility.until_timeout', autospec=True, return_value=[]):
            with patch.object(client, 'get_status', autospec=True,
                              return_value=status):
                with patch.object(GroupReporter, '_write', autospec=True,
                                  side_effect=lambda _, s: writes.append(s)):
                    with patch('jujupy.client.time.sleep'):
                        client.wait_for_workloads(timeout=1)
        self.assertEqual(writes, [])

    def test_wait_for_workload_no_workload_status(self):
        status = Status.from_text("""\
            applications:
              jenkins:
                units:
                  jenkins/0:
                    agent-state: active
        """)
        client = ModelClient(JujuData('lxd'), None, None)
        writes = []
        with patch('utility.until_timeout', autospec=True, return_value=[]):
            with patch.object(client, 'get_status', autospec=True,
                              return_value=status):
                with patch.object(GroupReporter, '_write', autospec=True,
                                  side_effect=lambda _, s: writes.append(s)):
                    with patch('jujupy.client.time.sleep'):
                        client.wait_for_workloads(timeout=1)
        self.assertEqual(writes, [])

    def test_list_models(self):
        client = ModelClient(JujuData('foo'), None, None)
        with patch_juju_call(client) as j_mock:
            client.list_models()
        j_mock.assert_called_once_with(
            'list-models', ('-c', 'foo'), include_e=False)

    def test_get_models(self):
        data = """\
            models:
            - name: foo
              model-uuid: aaaa
              owner: admin
            - name: bar
              model-uuid: bbbb
              owner: admin
            current-model: foo
        """
        client = ModelClient(JujuData('baz'), None, None)
        with patch.object(client, 'get_juju_output',
                          return_value=data) as gjo_mock:
            models = client.get_models()
        gjo_mock.assert_called_once_with(
            'list-models', '-c', 'baz', '--format', 'yaml',
            include_e=False, timeout=120)
        expected_models = {
            'models': [
                {'name': 'foo', 'model-uuid': 'aaaa', 'owner': 'admin'},
                {'name': 'bar', 'model-uuid': 'bbbb', 'owner': 'admin'}],
            'current-model': 'foo'
        }
        self.assertEqual(expected_models, models)

    def test_iter_model_clients(self):
        data = """\
            models:
            - name: foo
              model-uuid: aaaa
              owner: admin
            - name: bar
              model-uuid: bbbb
              owner: admin
            - name: baz
              model-uuid: bbbb
              owner: user1
            current-model: foo
        """
        client = ModelClient(JujuData('foo', {}), None, None)
        with patch.object(client, 'get_juju_output', return_value=data):
            model_clients = list(client.iter_model_clients())
        self.assertEqual(3, len(model_clients))
        self.assertIs(client, model_clients[0])
        self.assertEqual('admin/bar', model_clients[1].env.environment)
        self.assertEqual('user1/baz', model_clients[2].env.environment)

    def test__acquire_model_client_returns_self_when_match(self):
        client = ModelClient(JujuData('foo', {}), None, None)

        self.assertEqual(client._acquire_model_client('foo'), client)
        self.assertEqual(client._acquire_model_client('foo', None), client)

    def test__acquire_model_client_adds_username_component(self):
        client = ModelClient(JujuData('foo', {}), None, None)

        new_client = client._acquire_model_client('bar', None)
        self.assertEqual(new_client.model_name, 'bar')

        new_client = client._acquire_model_client('bar', 'user1')
        self.assertEqual(new_client.model_name, 'user1/bar')

        client.env.user_name = 'admin'
        new_client = client._acquire_model_client('baz', 'admin')
        self.assertEqual(new_client.model_name, 'baz')

    def test_get_controller_model_name(self):
        models = {
            'models': [
                {'name': 'controller', 'model-uuid': 'aaaa'},
                {'name': 'bar', 'model-uuid': 'bbbb'}],
            'current-model': 'bar'
        }
        client = ModelClient(JujuData('foo'), None, None)
        with patch.object(client, 'get_models',
                          return_value=models) as gm_mock:
            controller_name = client.get_controller_model_name()
        self.assertEqual(0, gm_mock.call_count)
        self.assertEqual('controller', controller_name)

    def test_get_controller_model_name_without_controller(self):
        models = {
            'models': [
                {'name': 'bar', 'model-uuid': 'aaaa'},
                {'name': 'baz', 'model-uuid': 'bbbb'}],
            'current-model': 'bar'
        }
        client = ModelClient(JujuData('foo'), None, None)
        with patch.object(client, 'get_models', return_value=models):
            controller_name = client.get_controller_model_name()
        self.assertEqual('controller', controller_name)

    def test_get_controller_model_name_no_models(self):
        client = ModelClient(JujuData('foo'), None, None)
        with patch.object(client, 'get_models', return_value={}):
            controller_name = client.get_controller_model_name()
        self.assertEqual('controller', controller_name)

    def test_get_model_uuid_returns_uuid(self):
        model_uuid = '9ed1bde9-45c6-4d41-851d-33fdba7fa194'
        yaml_string = dedent("""\
        foo:
          name: foo
          model-uuid: {uuid}
          controller-uuid: eb67e1eb-6c54-45f5-8b6a-b6243be97202
          owner: admin
          cloud: lxd
          region: localhost
          type: lxd
          life: alive
          status:
            current: available
            since: 1 minute ago
          users:
            admin:
              display-name: admin
              access: admin
              last-connection: just now
            """.format(uuid=model_uuid))
        client = ModelClient(JujuData('foo'), None, None)
        with patch.object(client, 'get_juju_output') as m_get_juju_output:
            m_get_juju_output.return_value = yaml_string
            self.assertEqual(
                client.get_model_uuid(),
                model_uuid
            )
            m_get_juju_output.assert_called_once_with(
                'show-model', '--format', 'yaml', 'foo:foo', include_e=False)

    def test_get_controller_model_uuid_returns_uuid(self):
        controller_uuid = 'eb67e1eb-6c54-45f5-8b6a-b6243be97202'
        controller_model_uuid = '1c908e10-4f07-459a-8419-bb61553a4660'
        yaml_string = dedent("""\
        controller:
          name: controller
          model-uuid: {model}
          controller-uuid: {controller}
          controller-name: localtempveebers
          owner: admin
          cloud: lxd
          region: localhost
          type: lxd
          life: alive
          status:
            current: available
            since: 59 seconds ago
          users:
            admin:
              display-name: admin
              access: admin
              last-connection: just now""".format(model=controller_model_uuid,
                                                  controller=controller_uuid))
        client = ModelClient(JujuData('foo'), None, None)
        with patch.object(client, 'get_juju_output') as m_get_juju_output:
            m_get_juju_output.return_value = yaml_string
            self.assertEqual(
                client.get_controller_model_uuid(),
                controller_model_uuid
            )
            m_get_juju_output.assert_called_once_with(
                'show-model', 'controller',
                '--format', 'yaml', include_e=False)

    def test_get_controller_uuid_returns_uuid(self):
        controller_uuid = 'eb67e1eb-6c54-45f5-8b6a-b6243be97202'
        yaml_string = dedent("""\
        foo:
          details:
            uuid: {uuid}
            api-endpoints: ['10.194.140.213:17070']
            cloud: lxd
            region: localhost
          models:
            controller:
              uuid: {uuid}
            default:
              uuid: 772cdd39-b454-4bd5-8704-dc9aa9ff1750
          current-model: default
          account:
            user: admin
          bootstrap-config:
            config:
            cloud: lxd
            cloud-type: lxd
            region: localhost""".format(uuid=controller_uuid))
        client = ModelClient(JujuData('foo'), None, None)
        with patch.object(client, 'get_juju_output') as m_get_juju_output:
            m_get_juju_output.return_value = yaml_string
            self.assertEqual(
                client.get_controller_uuid(),
                controller_uuid
            )
            m_get_juju_output.assert_called_once_with(
                'show-controller', 'foo', '--format', 'yaml', include_e=False)

    def test_get_controller_client(self):
        client = ModelClient(
            JujuData('foo', {'bar': 'baz'}, 'myhome'), None, None)
        controller_client = client.get_controller_client()
        controller_env = controller_client.env
        self.assertEqual('controller', controller_env.environment)
        self.assertEqual(
            {'bar': 'baz', 'name': 'controller'}, controller_env._config)

    def test_list_controllers(self):
        client = ModelClient(JujuData('foo'), None, None)
        with patch_juju_call(client) as j_mock:
            client.list_controllers()
        j_mock.assert_called_once_with('list-controllers', (), include_e=False)

    def test_get_controller_endpoint_ipv4(self):
        data = """\
          foo:
            details:
              api-endpoints: ['10.0.0.1:17070', '10.0.0.2:17070']
        """
        client = ModelClient(JujuData('foo'), None, None)
        with patch.object(client, 'get_juju_output',
                          return_value=data) as gjo_mock:
            endpoint = client.get_controller_endpoint()
        self.assertEqual(('10.0.0.1', '17070'), endpoint)
        gjo_mock.assert_called_once_with(
            'show-controller', 'foo', include_e=False)

    def test_get_controller_endpoint_ipv6(self):
        data = """\
          foo:
            details:
              api-endpoints: ['[::1]:17070', '[fe80::216:3eff:0:9dc7]:17070']
        """
        client = ModelClient(JujuData('foo'), None, None)
        with patch.object(client, 'get_juju_output',
                          return_value=data) as gjo_mock:
            endpoint = client.get_controller_endpoint()
        self.assertEqual(('::1', '17070'), endpoint)
        gjo_mock.assert_called_once_with(
            'show-controller', 'foo', include_e=False)

    def test_get_controller_controller_name(self):
        data = """\
          bar:
            details:
              api-endpoints: ['[::1]:17070', '[fe80::216:3eff:0:9dc7]:17070']
        """
        client = ModelClient(JujuData('foo', {}), None, None)
        controller_client = client.get_controller_client()
        client.env.controller.name = 'bar'
        with patch.object(controller_client, 'get_juju_output',
                          return_value=data) as gjo:
            endpoint = controller_client.get_controller_endpoint()
        gjo.assert_called_once_with('show-controller', 'bar',
                                    include_e=False)
        self.assertEqual(('::1', '17070'), endpoint)

    def test_get_controller_members(self):
        status = Status.from_text("""\
            model: controller
            machines:
              "0":
                dns-name: 10.0.0.0
                instance-id: juju-aaaa-machine-0
                controller-member-status: has-vote
              "1":
                dns-name: 10.0.0.1
                instance-id: juju-bbbb-machine-1
              "2":
                dns-name: 10.0.0.2
                instance-id: juju-cccc-machine-2
                controller-member-status: has-vote
              "3":
                dns-name: 10.0.0.3
                instance-id: juju-dddd-machine-3
                controller-member-status: has-vote
        """)
        client = ModelClient(JujuData('foo'), None, None)
        with patch.object(client, 'get_status', autospec=True,
                          return_value=status):
            with patch.object(client, 'get_controller_endpoint', autospec=True,
                              return_value=('10.0.0.3', '17070')) as gce_mock:
                with patch.object(client, 'get_controller_member_status',
                                  wraps=client.get_controller_member_status,
                                  ) as gcms_mock:
                    members = client.get_controller_members()
        # Machine 1 was ignored. Machine 3 is the leader, thus first.
        expected = [
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
        self.assertEqual(expected, members)
        gce_mock.assert_called_once_with()
        # get_controller_member_status must be called to ensure compatibility
        # with all version of Juju.
        self.assertEqual(4, gcms_mock.call_count)

    def test_get_controller_members_one(self):
        status = Status.from_text("""\
            model: controller
            machines:
              "0":
                dns-name: 10.0.0.0
                instance-id: juju-aaaa-machine-0
                controller-member-status: has-vote
        """)
        client = ModelClient(JujuData('foo'), None, None)
        with patch.object(client, 'get_status', autospec=True,
                          return_value=status):
            with patch.object(client, 'get_controller_endpoint') as gce_mock:
                members = client.get_controller_members()
        # Machine 0 was the only choice, no need to find the leader.
        expected = [
            Machine('0', {
                'dns-name': '10.0.0.0',
                'instance-id': 'juju-aaaa-machine-0',
                'controller-member-status': 'has-vote'}),
        ]
        self.assertEqual(expected, members)
        self.assertEqual(0, gce_mock.call_count)

    def test_get_controller_leader(self):
        members = [
            Machine('3', {}),
            Machine('0', {}),
            Machine('2', {}),
        ]
        client = ModelClient(JujuData('foo'), None, None)
        with patch.object(client, 'get_controller_members', autospec=True,
                          return_value=members):
            leader = client.get_controller_leader()
        self.assertEqual(Machine('3', {}), leader)

    def make_controller_client(self):
        client = ModelClient(JujuData('lxd', {'name': 'test'}), None, None)
        return client.get_controller_client()

    def test_wait_for_ha(self):
        value = yaml.safe_dump(self.make_ha_status()).encode('ascii')
        client = self.make_controller_client()
        with patch.object(client, 'get_juju_output',
                          return_value=value) as gjo_mock:
            client.wait_for_ha()
        gjo_mock.assert_called_once_with(
            'show-status', '--format', 'yaml', controller=False)

    def test_wait_for_ha_requires_controller_client(self):
        client = fake_juju_client()
        with self.assertRaisesRegexp(ValueError, 'wait_for_ha'):
            client.wait_for_ha()

    def test_wait_for_ha_no_has_vote(self):
        value = yaml.safe_dump(
            self.make_ha_status(voting='no-vote')).encode('ascii')
        client = self.make_controller_client()
        with patch.object(client, 'get_juju_output', return_value=value):
            writes = []
            with patch('jujupy.client.until_timeout', autospec=True,
                       return_value=[2, 1]):
                with patch.object(GroupReporter, '_write', autospec=True,
                                  side_effect=lambda _, s: writes.append(s)):
                    with self.assertRaisesRegexp(
                            Exception,
                            'Timed out waiting for voting to be enabled.'):
                        with patch('jujupy.client.time.sleep'):
                            client.wait_for_ha()
        dots = len(writes) - 3
        expected = ['no-vote: 0, 1, 2', ' .'] + (['.'] * dots) + ['\n']
        self.assertEqual(writes, expected)

    def test_wait_for_ha_timeout(self):
        value = yaml.safe_dump({
            'machines': {
                '0': {'controller-member-status': 'has-vote'},
                '1': {'controller-member-status': 'has-vote'},
            },
            'services': {},
        })
        client = self.make_controller_client()
        status = client.status_class.from_text(value)
        with patch('jujupy.client.until_timeout',
                   lambda x, start=None: range(0)):
            with patch.object(client, 'get_status', return_value=status
                              ) as get_status_mock:
                with patch('jujupy.client.time.sleep'):
                    with self.assertRaisesRegexp(
                            StatusNotMet,
                            'Timed out waiting for voting to be enabled.'):
                        client.wait_for_ha()
        get_status_mock.assert_called_once_with()

    def test_wait_for_ha_timeout_with_status_error(self):
        value = yaml.safe_dump({
            'machines': {
                '0': {'agent-state-info': 'running'},
                '1': {'agent-state-info': 'error: foo'},
            },
            'services': {},
        }).encode('ascii')
        client = self.make_controller_client()
        with patch('jujupy.client.until_timeout', autospec=True,
                   return_value=[2, 1]):
            with patch.object(client, 'get_juju_output', return_value=value):
                with self.assertRaisesRegexp(
                        ErroredUnit, '1 is in state error: foo'):
                    with patch('jujupy.client.time.sleep'):
                        client.wait_for_ha()

    def test_wait_for_ha_suppresses_deadline(self):
        with self.only_status_checks(self.make_controller_client(),
                                     self.make_ha_status()) as client:
            client.wait_for_ha()

    def test_wait_for_ha_checks_deadline(self):
        with self.status_does_not_check(self.make_controller_client(),
                                        self.make_ha_status()) as client:
            with self.assertRaises(SoftDeadlineExceeded):
                client.wait_for_ha()

    def test_wait_for_deploy_started(self):
        value = yaml.safe_dump({
            'machines': {
                '0': {'agent-state': 'started'},
            },
            'applications': {
                'jenkins': {
                    'units': {
                        'jenkins/1': {'baz': 'qux'}
                    }
                }
            }
        }).encode('ascii')
        client = ModelClient(JujuData('lxd'), None, None)
        with patch.object(client, 'get_juju_output', return_value=value):
            client.wait_for_deploy_started()

    def test_wait_for_deploy_started_timeout(self):
        value = yaml.safe_dump({
            'machines': {
                '0': {'agent-state': 'started'},
            },
            'applications': {},
        })
        client = ModelClient(JujuData('lxd'), None, None)
        with patch('jujupy.client.until_timeout', lambda x: range(0)):
            with patch.object(client, 'get_juju_output', return_value=value):
                with self.assertRaisesRegexp(
                        StatusNotMet,
                        'Timed out waiting for applications to start.'):
                    with patch('jujupy.client.time.sleep'):
                        client.wait_for_deploy_started()

    def make_deployed_status(self):
        return {
            'machines': {
                '0': {'agent-state': 'started'},
            },
            'applications': {
                'jenkins': {
                    'units': {
                        'jenkins/1': {'baz': 'qux'}
                    }
                }
            }
        }

    def test_wait_for_deploy_started_suppresses_deadline(self):
        with self.only_status_checks(
                status=self.make_deployed_status()) as client:
            client.wait_for_deploy_started()

    def test_wait_for_deploy_started_checks_deadline(self):
        with self.status_does_not_check(
                status=self.make_deployed_status()) as client:
            with self.assertRaises(SoftDeadlineExceeded):
                client.wait_for_deploy_started()

    def test_wait_for_version(self):
        value = self.make_status_yaml('agent-version', '1.17.2', '1.17.2')
        client = ModelClient(JujuData('lxd'), None, None)
        with patch.object(client, 'get_juju_output', return_value=value):
            client.wait_for_version('1.17.2')

    def test_wait_for_version_timeout(self):
        value = self.make_status_yaml('agent-version', '1.17.2', '1.17.1')
        client = ModelClient(JujuData('lxd'), None, None)
        writes = []
        with patch('jujupy.client.until_timeout',
                   lambda x, start=None: [x]):
            with patch.object(client, 'get_juju_output', return_value=value):
                with patch.object(GroupReporter, '_write', autospec=True,
                                  side_effect=lambda _, s: writes.append(s)):
                    with self.assertRaisesRegexp(
                            StatusNotMet, 'Some versions did not update'):
                            with patch('jujupy.client.time.sleep'):
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

        client = ModelClient(JujuData('lxd'), None, None)
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

        client = ModelClient(JujuData('lxd'), None, None)
        with patch.object(client, 'get_juju_output', get_juju_output_fake):
            with self.assertRaisesRegexp(Exception, 'foo'):
                client.wait_for_version('1.17.2')

    def test_wait_just_machine_0(self):
        value = yaml.safe_dump({
            'machines': {
                '0': {'agent-state': 'started'},
            },
        }).encode('ascii')
        client = ModelClient(JujuData('lxd'), None, None)
        with patch.object(client, 'get_juju_output', return_value=value):
            with patch('jujupy.client.time.sleep'):
                client.wait_for(WaitMachineNotPresent('1'), quiet=True)

    def test_wait_just_machine_0_timeout(self):
        value = yaml.safe_dump({
            'machines': {
                '0': {'agent-state': 'started'},
                '1': {'agent-state': 'started'},
            },
        }).encode('ascii')
        client = ModelClient(JujuData('lxd'), None, None)
        with patch.object(client, 'get_juju_output', return_value=value), \
            patch('jujupy.client.until_timeout',
                  lambda x, start=None: range(1)), \
            self.assertRaisesRegexp(
                Exception,
                'Timed out waiting for machine removal 1'):
            with patch('jujupy.client.time.sleep'):
                client.wait_for(WaitMachineNotPresent('1'), quiet=True)

    class NeverSatisfied:

        timeout = 1234

        already_satisfied = False

        class NeverSatisfiedException(Exception):
            pass

        def iter_blocking_state(self, ignored):
            yield ('global state', 'unsatisfied')

        def do_raise(self, model_name, status):
            raise self.NeverSatisfiedException()

    def test_wait_timeout(self):
        client = fake_juju_client()
        client.bootstrap()
        never_satisfied = self.NeverSatisfied()
        with self.assertRaises(never_satisfied.NeverSatisfiedException):
            with patch.object(client, 'status_until', return_value=iter(
                    [Status({'machines': {}}, '')])) as mock_su:
                with patch('jujupy.client.time.sleep'):
                    client.wait_for(never_satisfied, quiet=True)
        mock_su.assert_called_once_with(1234)

    def test_wait_for_emits_output(self):
        client = fake_juju_client()
        client.bootstrap()
        mock_wait = Mock(timeout=300, already_satisfied=False)
        mock_wait.iter_blocking_state.side_effect = [
            [('0', 'still-present')],
            [('0', 'still-present')],
            [('0', 'still-present')],
            [],
            ]
        writes = []
        with patch.object(GroupReporter, '_write', autospec=True,
                          side_effect=lambda _, s: writes.append(s)):
            with patch('jujupy.client.time.sleep'):
                client.wait_for(mock_wait)
        self.assertEqual('still-present: 0 ..\n', ''.join(writes))

    def test_wait_for_quiet(self):
        client = fake_juju_client()
        client.bootstrap()
        mock_wait = Mock(timeout=300)
        mock_wait.iter_blocking_state.side_effect = [
            [('0', 'still-present')],
            [('0', 'still-present')],
            [('0', 'still-present')],
            [],
            ]
        writes = []
        with patch.object(GroupReporter, '_write', autospec=True,
                          side_effect=lambda _, s: writes.append(s)):
            with patch('jujupy.client.time.sleep'):
                client.wait_for(mock_wait, quiet=True)
        self.assertEqual('', ''.join(writes))

    def test_wait_bad_status(self):
        client = fake_juju_client()
        client.bootstrap()

        never_satisfied = self.NeverSatisfied()
        bad_status = Status({'machines': {'0': {StatusItem.MACHINE: {
            'current': 'error'
            }}}}, '')
        with self.assertRaises(MachineError):
            with patch.object(client, 'status_until', lambda timeout: iter(
                    [bad_status])):
                with patch('jujupy.client.time.sleep'):
                    client.wait_for(never_satisfied, quiet=True)

    def test_wait_bad_status_recoverable_recovered(self):
        client = fake_juju_client()
        client.bootstrap()

        never_satisfied = self.NeverSatisfied()
        bad_status = Status({
            'machines': {},
            'applications': {'0': {StatusItem.APPLICATION: {
                'current': 'error'
                }}}
            }, '')
        good_status = Status({'machines': {}}, '')
        with self.assertRaises(never_satisfied.NeverSatisfiedException):
            with patch.object(client, 'status_until', lambda timeout: iter(
                    [bad_status, good_status])):
                with patch('jujupy.client.time.sleep'):
                    client.wait_for(never_satisfied, quiet=True)

    def test_wait_bad_status_recoverable_timed_out(self):
        client = fake_juju_client()
        client.bootstrap()

        never_satisfied = self.NeverSatisfied()
        bad_status = Status({
            'machines': {},
            'applications': {'0': {StatusItem.APPLICATION: {
                'current': 'error'
                }}}
            }, '')
        with self.assertRaises(AppError):
            with patch.object(client, 'status_until', lambda timeout: iter(
                    [bad_status])):
                with patch('jujupy.client.time.sleep'):
                    client.wait_for(never_satisfied, quiet=True)

    def test_wait_empty_list(self):
        client = fake_juju_client()
        client.bootstrap()
        with patch.object(client, 'status_until', side_effect=StatusTimeout):
            with patch('jujupy.client.time.sleep'):
                self.assertEqual(client.wait_for(
                    ConditionList([]), quiet=True).status,
                    client.get_status().status)

    def test_set_model_constraints(self):
        client = ModelClient(JujuData('bar', {}), None, '/foo')
        with patch_juju_call(client) as juju_mock:
            client.set_model_constraints({'bar': 'baz'})
        juju_mock.assert_called_once_with('set-model-constraints',
                                          ('bar=baz',))

    def test_get_model_config(self):
        env = JujuData('foo', None)
        fake_popen = FakePopen(yaml.safe_dump({'bar': 'baz'}), None, 0)
        client = ModelClient(env, None, 'juju')
        with patch('subprocess.Popen', return_value=fake_popen) as po_mock:
            result = client.get_model_config()
        assert_juju_call(
            self, po_mock, client, (
                'juju', '--show-log',
                'model-config', '-m', 'foo:foo', '--format', 'yaml'))
        self.assertEqual({'bar': 'baz'}, result)

    def test_get_env_option(self):
        env = JujuData('foo', None)
        fake_popen = FakePopen('https://example.org/juju/tools', None, 0)
        client = ModelClient(env, None, 'juju')
        with patch('subprocess.Popen', return_value=fake_popen) as mock:
            result = client.get_env_option('tools-metadata-url')
        self.assertEqual(
            mock.call_args[0][0],
            ('juju', '--show-log', 'model-config', '-m', 'foo:foo',
             'tools-metadata-url'))
        self.assertEqual('https://example.org/juju/tools', result)

    def test_set_env_option(self):
        env = JujuData('foo')
        client = ModelClient(env, None, 'juju')
        with patch('subprocess.check_call') as mock:
            client.set_env_option(
                'tools-metadata-url', 'https://example.org/juju/tools')
        environ = dict(os.environ)
        environ['JUJU_HOME'] = client.env.juju_home
        mock.assert_called_with(
            ('juju', '--show-log', 'model-config', '-m', 'foo:foo',
             'tools-metadata-url=https://example.org/juju/tools'), stderr=None)

    def test_unset_env_option(self):
        env = JujuData('foo')
        client = ModelClient(env, None, 'juju')
        with patch('subprocess.check_call') as mock:
            client.unset_env_option('tools-metadata-url')
        environ = dict(os.environ)
        environ['JUJU_HOME'] = client.env.juju_home
        mock.assert_called_with(
            ('juju', '--show-log', 'model-config', '-m', 'foo:foo',
             '--reset', 'tools-metadata-url'), stderr=None)

    def test__format_cloud_region(self):
        fcr = ModelClient._format_cloud_region
        self.assertEqual(('aws/us-east-1',), fcr('aws', 'us-east-1'))
        self.assertEqual(('us-east-1',), fcr(region='us-east-1'))
        self.assertRaises(ValueError, fcr, cloud='aws')
        self.assertEqual((), fcr())

    def test_get_model_defaults(self):
        data = {'some-key': {'default': 'black'}}
        raw_yaml = yaml.safe_dump(data)
        client = fake_juju_client()
        with patch.object(client, 'get_juju_output', autospec=True,
                          return_value=raw_yaml) as output_mock:
            retval = client.get_model_defaults('some-key')
        self.assertEqual(data, retval)
        output_mock.assert_called_once_with(
            'model-defaults', '--format', 'yaml', 'some-key', include_e=False)

    def test_get_model_defaults_cloud_region(self):
        raw_yaml = yaml.safe_dump({'some-key': {'default': 'red'}})
        client = fake_juju_client()
        with patch.object(client, 'get_juju_output', autospec=True,
                          return_value=raw_yaml) as output_mock:
            client.get_model_defaults('some-key', region='us-east-1')
        output_mock.assert_called_once_with(
            'model-defaults', '--format', 'yaml', 'us-east-1', 'some-key',
            include_e=False)

    def test_set_model_defaults(self):
        client = fake_juju_client()
        with patch.object(client, 'juju', autospec=True) as juju_mock:
            client.set_model_defaults('some-key', 'white')
        juju_mock.assert_called_once_with(
            'model-defaults', ('some-key=white',), include_e=False)

    def test_set_model_defaults_cloud_region(self):
        client = fake_juju_client()
        with patch.object(client, 'juju', autospec=True) as juju_mock:
            client.set_model_defaults('some-key', 'white', region='us-east-1')
        juju_mock.assert_called_once_with(
            'model-defaults', ('us-east-1', 'some-key=white',),
            include_e=False)

    def test_unset_model_defaults(self):
        client = fake_juju_client()
        with patch.object(client, 'juju', autospec=True) as juju_mock:
            client.unset_model_defaults('some-key')
        juju_mock.assert_called_once_with(
            'model-defaults', ('--reset', 'some-key'), include_e=False)

    def test_unset_model_defaults_cloud_region(self):
        client = fake_juju_client()
        with patch.object(client, 'juju', autospec=True) as juju_mock:
            client.unset_model_defaults('some-key', region='us-east-1')
        juju_mock.assert_called_once_with(
            'model-defaults', ('us-east-1', '--reset', 'some-key'),
            include_e=False)

    def test_set_testing_agent_metadata_url(self):
        env = JujuData(None, {'type': 'foo'})
        client = ModelClient(env, None, None)
        with patch.object(client, 'get_env_option') as mock_get:
            mock_get.return_value = 'https://example.org/juju/tools'
            with patch.object(client, 'set_env_option') as mock_set:
                client.set_testing_agent_metadata_url()
        mock_get.assert_called_with('agent-metadata-url')
        mock_set.assert_called_with(
            'agent-metadata-url',
            'https://example.org/juju/testing/tools')

    def test_set_testing_agent_metadata_url_noop(self):
        env = JujuData(None, {'type': 'foo'})
        client = ModelClient(env, None, None)
        with patch.object(client, 'get_env_option') as mock_get:
            mock_get.return_value = 'https://example.org/juju/testing/tools'
            with patch.object(client, 'set_env_option') as mock_set:
                client.set_testing_agent_metadata_url()
        mock_get.assert_called_with('agent-metadata-url',)
        self.assertEqual(0, mock_set.call_count)

    def test_juju(self):
        env = JujuData('qux')
        client = ModelClient(env, None, 'juju')
        with patch('subprocess.check_call') as mock:
            client.juju('foo', ('bar', 'baz'))
        environ = dict(os.environ)
        environ['JUJU_HOME'] = client.env.juju_home
        mock.assert_called_with(('juju', '--show-log', 'foo', '-m', 'qux:qux',
                                 'bar', 'baz'), stderr=None)

    def test_expect_returns_pexpect_spawn_object(self):
        env = JujuData('qux')
        client = ModelClient(env, None, 'juju')
        with patch('pexpect.spawn') as mock:
            process = client.expect('foo', ('bar', 'baz'))

        self.assertIs(process, mock.return_value)
        mock.assert_called_once_with('juju --show-log foo -m qux:qux bar baz')

    def test_expect_uses_provided_envvar_path(self):
        from pexpect import ExceptionPexpect
        env = JujuData('qux')
        client = ModelClient(env, None, 'juju')

        with temp_dir() as empty_path:
            broken_envvars = dict(PATH=empty_path)
            self.assertRaises(
                ExceptionPexpect,
                client.expect,
                'ls', (), extra_env=broken_envvars,
                )

    def test_juju_env(self):
        env = JujuData('qux')
        client = ModelClient(env, None, '/foobar/baz')

        def check_path(*args, **kwargs):
            self.assertRegexpMatches(os.environ['PATH'], r'/foobar\:')
        with patch('subprocess.check_call', side_effect=check_path):
            client.juju('foo', ('bar', 'baz'))

    def test_juju_no_check(self):
        env = JujuData('qux')
        client = ModelClient(env, None, 'juju')
        environ = dict(os.environ)
        environ['JUJU_HOME'] = client.env.juju_home
        with patch('subprocess.call') as mock:
            client.juju('foo', ('bar', 'baz'), check=False)
        mock.assert_called_with(('juju', '--show-log', 'foo', '-m', 'qux:qux',
                                 'bar', 'baz'), stderr=None)

    def test_juju_no_check_env(self):
        env = JujuData('qux')
        client = ModelClient(env, None, '/foobar/baz')

        def check_path(*args, **kwargs):
            self.assertRegexpMatches(os.environ['PATH'], r'/foobar\:')
        with patch('subprocess.call', side_effect=check_path):
            client.juju('foo', ('bar', 'baz'), check=False)

    def test_juju_timeout(self):
        env = JujuData('qux')
        client = ModelClient(env, None, '/foobar/baz')
        with patch('subprocess.check_call') as cc_mock:
            client.juju('foo', ('bar', 'baz'), timeout=58)
        self.assertEqual(cc_mock.call_args[0][0], (
            sys.executable, get_timeout_path(), '58.00', '--', 'baz',
            '--show-log', 'foo', '-m', 'qux:qux', 'bar', 'baz'))

    def test_juju_juju_home(self):
        env = JujuData('qux')
        os.environ['JUJU_HOME'] = 'foo'
        client = ModelClient(env, None, '/foobar/baz')

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
        env = JujuData('qux')
        client = ModelClient(env, None, 'juju')
        extra_env = {'JUJU': '/juju', 'JUJU_HOME': client.env.juju_home}

        def check_env(*args, **kwargs):
            self.assertEqual('/juju', os.environ['JUJU'])

        with patch('subprocess.check_call', side_effect=check_env) as mock:
            client.juju('quickstart', ('bar', 'baz'), extra_env=extra_env)
        mock.assert_called_with(
            ('juju', '--show-log', 'quickstart', '-m', 'qux:qux',
             'bar', 'baz'), stderr=None)

    def test_juju_backup_with_tgz(self):
        env = JujuData('qux')
        client = ModelClient(env, None, '/foobar/baz')

        with patch(
                'subprocess.Popen',
                return_value=FakePopen('foojuju-backup-24.tgzz', '', 0),
                ) as popen_mock:
            backup_file = client.backup()
        self.assertEqual(backup_file, os.path.abspath('juju-backup-24.tgz'))
        assert_juju_call(self, popen_mock, client, ('baz', '--show-log',
                         'create-backup', '-m', 'qux:qux'))

    def test_juju_backup_with_tar_gz(self):
        env = JujuData('qux')
        client = ModelClient(env, None, '/foobar/baz')
        with patch('subprocess.Popen',
                   return_value=FakePopen(
                       'foojuju-backup-123-456.tar.gzbar', '', 0)):
            backup_file = client.backup()
        self.assertEqual(
            backup_file, os.path.abspath('juju-backup-123-456.tar.gz'))

    def test_juju_backup_no_file(self):
        env = JujuData('qux')
        client = ModelClient(env, None, '/foobar/baz')
        with patch('subprocess.Popen', return_value=FakePopen('', '', 0)):
            with self.assertRaisesRegexp(
                    Exception, 'The backup file was not found in output'):
                client.backup()

    def test_juju_backup_wrong_file(self):
        env = JujuData('qux')
        client = ModelClient(env, None, '/foobar/baz')
        with patch('subprocess.Popen',
                   return_value=FakePopen('mumu-backup-24.tgz', '', 0)):
            with self.assertRaisesRegexp(
                    Exception, 'The backup file was not found in output'):
                client.backup()

    def test_juju_backup_environ(self):
        env = JujuData('qux')
        client = ModelClient(env, None, '/foobar/baz')
        environ = client._shell_environ()

        def side_effect(*args, **kwargs):
            self.assertEqual(environ, os.environ)
            return FakePopen('foojuju-backup-123-456.tar.gzbar', '', 0)
        with patch('subprocess.Popen', side_effect=side_effect):
            client.backup()
            self.assertNotEqual(environ, os.environ)

    def test_enable_ha(self):
        env = JujuData('qux')
        client = ModelClient(env, None, '/foobar/baz')
        with patch.object(client, 'juju', autospec=True) as eha_mock:
            client.enable_ha()
        eha_mock.assert_called_once_with(
            'enable-ha', ('-n', '3', '-c', 'qux'), include_e=False)

    def test_juju_async(self):
        env = JujuData('qux')
        client = ModelClient(env, None, '/foobar/baz')
        with patch('subprocess.Popen') as popen_class_mock:
            with client.juju_async('foo', ('bar', 'baz')) as proc:
                assert_juju_call(
                    self,
                    popen_class_mock,
                    client,
                    ('baz', '--show-log', 'foo', '-m', 'qux:qux',
                     'bar', 'baz'))
                self.assertIs(proc, popen_class_mock.return_value)
                self.assertEqual(proc.wait.call_count, 0)
                proc.wait.return_value = 0
        proc.wait.assert_called_once_with()

    def test_juju_async_failure(self):
        env = JujuData('qux')
        client = ModelClient(env, None, '/foobar/baz')
        with patch('subprocess.Popen') as popen_class_mock:
            with self.assertRaises(subprocess.CalledProcessError) as err_cxt:
                with client.juju_async('foo', ('bar', 'baz')):
                    proc_mock = popen_class_mock.return_value
                    proc_mock.wait.return_value = 23
        self.assertEqual(err_cxt.exception.returncode, 23)
        self.assertEqual(err_cxt.exception.cmd, (
            'baz', '--show-log', 'foo', '-m', 'qux:qux', 'bar', 'baz'))

    def test_juju_async_environ(self):
        env = JujuData('qux')
        client = ModelClient(env, None, '/foobar/baz')
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

    def test_get_juju_timings(self):
        first_start = datetime(2017, 3, 22, 23, 36, 52, 0)
        first_end = first_start + timedelta(seconds=2)
        second_start = datetime(2017, 5, 22, 23, 36, 52, 0)
        env = JujuData('foo')
        client = ModelClient(env, None, 'my/juju/bin')
        client._backend.juju_timings.extend([
            CommandTime('command1', ['command1', 'arg1'], start=first_start),
            CommandTime(
                'command2', ['command2', 'arg1', 'arg2'], start=second_start)])
        client._backend.juju_timings[0].actual_completion(end=first_end)
        flattened_timings = client.get_juju_timings()
        expected = [
            {
                'command': 'command1',
                'full_args': ['command1', 'arg1'],
                'start': first_start,
                'end': first_end,
                'total_seconds': 2,
            },
            {
                'command': 'command2',
                'full_args': ['command2', 'arg1', 'arg2'],
                'start': second_start,
                'end': None,
                'total_seconds': None,
            }
        ]
        self.assertEqual(flattened_timings, expected)

    def test_deployer(self):
        client = ModelClient(JujuData('foo', {'type': 'lxd'}),
                             '1.23-series-arch', None)
        with patch.object(ModelClient, 'juju') as mock:
            client.deployer('bundle:~juju-qa/some-bundle')
        mock.assert_called_with(
            'deployer', ('-e', 'foo:foo', '--debug', '--deploy-delay',
                         '10', '--timeout', '3600', '--config',
                         'bundle:~juju-qa/some-bundle'),
            include_e=False)

    def test_deployer_with_bundle_name(self):
        client = ModelClient(JujuData('foo', {'type': 'lxd'}),
                             '2.0.0-series-arch', None)
        with patch.object(ModelClient, 'juju') as mock:
            client.deployer('bundle:~juju-qa/some-bundle', 'name')
        mock.assert_called_with(
            'deployer', ('-e', 'foo:foo', '--debug', '--deploy-delay',
                         '10', '--timeout', '3600', '--config',
                         'bundle:~juju-qa/some-bundle', 'name'),
            include_e=False)

    def test_quickstart_maas(self):
        client = ModelClient(JujuData(None, {'type': 'maas'}),
                             '1.23-series-arch', '/juju')
        with patch.object(ModelClient, 'juju') as mock:
            client.quickstart('bundle:~juju-qa/some-bundle')
        mock.assert_called_with(
            'quickstart', ('--constraints', 'mem=2G', '--no-browser',
                           'bundle:~juju-qa/some-bundle'),
            extra_env={'JUJU': '/juju'})

    def test_quickstart_local(self):
        client = ModelClient(JujuData(None, {'type': 'lxd'}),
                             '1.23-series-arch', '/juju')
        with patch.object(ModelClient, 'juju') as mock:
            client.quickstart('bundle:~juju-qa/some-bundle')
        mock.assert_called_with(
            'quickstart', ('--constraints', 'mem=2G', '--no-browser',
                           'bundle:~juju-qa/some-bundle'),
            extra_env={'JUJU': '/juju'})

    def test_quickstart_template(self):
        client = ModelClient(JujuData(None, {'type': 'lxd'}),
                             '1.23-series-arch', '/juju')
        with patch.object(ModelClient, 'juju') as mock:
            client.quickstart('bundle:~juju-qa/some-{container}-bundle')
        mock.assert_called_with(
            'quickstart', ('--constraints', 'mem=2G', '--no-browser',
                           'bundle:~juju-qa/some-lxd-bundle'),
            extra_env={'JUJU': '/juju'})

    def test_action_do(self):
        client = ModelClient(JujuData(None, {'type': 'lxd'}),
                             '1.23-series-arch', None)
        with patch.object(ModelClient, 'get_juju_output') as mock:
            mock.return_value = \
                "Action queued with id: 666"
            id = client.action_do("foo/0", "myaction", "param=5")
            self.assertEqual(id, "666")
        mock.assert_called_once_with(
            'run', 'foo/0', 'myaction', "param=5"
        )

    def test_action_do_error(self):
        client = ModelClient(JujuData(None, {'type': 'lxd'}),
                             '1.23-series-arch', None)
        with patch.object(ModelClient, 'get_juju_output') as mock:
            mock.return_value = "some bad text"
            with self.assertRaisesRegexp(Exception,
                                         "Action id not found in output"):
                client.action_do("foo/0", "myaction", "param=5")

    def test_action_fetch(self):
        client = ModelClient(JujuData(None, {'type': 'lxd'}),
                             '1.23-series-arch', None)
        with patch.object(ModelClient, 'get_juju_output') as mock:
            ret = "status: completed\nfoo: bar"
            mock.return_value = ret
            out = client.action_fetch("123")
            self.assertEqual(out, ret)
        mock.assert_called_once_with(
            'show-task', '123', "--wait", "1m"
        )

    def test_action_fetch_timeout(self):
        client = ModelClient(JujuData(None, {'type': 'lxd'}),
                             '1.23-series-arch', None)
        ret = "status: pending\nfoo: bar"
        with patch.object(ModelClient,
                          'get_juju_output', return_value=ret):
            with self.assertRaisesRegexp(
                Exception,
                "Timed out waiting for action to complete during fetch with "
                "status: pending."
            ):
                    client.action_fetch("123")

    def test_action_do_fetch(self):
        client = ModelClient(JujuData(None, {'type': 'lxd'}),
                             '1.23-series-arch', None)
        with patch.object(ModelClient, 'get_juju_output') as mock:
            ret = "status: completed\nfoo: bar"
            # setting side_effect to an iterable will return the next value
            # from the list each time the function is called.
            mock.side_effect = [
                "Action queued with id: 666",
                ret]
            out = client.action_do_fetch("foo/0", "myaction", "param=5")
            self.assertEqual(out, ret)

    def test_run(self):
        client = fake_juju_client(cls=ModelClient)
        run_list = [
            {"machine": "1",
             "stdout": "Linux\n",
             "return-code": 255,
             "stderr": "Permission denied (publickey,password)"}]
        run_output = json.dumps(run_list)
        with patch.object(client._backend, 'get_juju_output',
                          return_value=run_output) as gjo_mock:
            result = client.run(('wname',), applications=['foo', 'bar'])
        self.assertEqual(run_list, result)
        gjo_mock.assert_called_once_with(
            'run', ('--format', 'json', '--application', 'foo,bar', 'wname'),
            frozenset(['migration']), 'foo',
            'name:name', user_name=None)

    def test_run_machines(self):
        client = fake_juju_client(cls=ModelClient)
        output = json.dumps({"ReturnCode": 255})
        with patch.object(client, 'get_juju_output',
                          return_value=output) as output_mock:
            client.run(['true'], machines=['0', '1', '2'])
        output_mock.assert_called_once_with(
            'run', '--format', 'json', '--machine', '0,1,2', 'true')

    def test_run_use_json_false(self):
        client = fake_juju_client(cls=ModelClient)
        output = json.dumps({"ReturnCode": 255})
        with patch.object(client, 'get_juju_output', return_value=output):
            result = client.run(['true'], use_json=False)
        self.assertEqual(output, result)

    def test_run_units(self):
        client = fake_juju_client(cls=ModelClient)
        output = json.dumps({"ReturnCode": 255})
        with patch.object(client, 'get_juju_output',
                          return_value=output) as output_mock:
            client.run(['true'], units=['foo/0', 'foo/1', 'foo/2'])
        output_mock.assert_called_once_with(
            'run', '--format', 'json', '--unit', 'foo/0,foo/1,foo/2', 'true')

    def test_list_space(self):
        client = ModelClient(JujuData(None, {'type': 'lxd'}),
                             '1.23-series-arch', None)
        yaml_dict = {'foo': 'bar'}
        output = yaml.safe_dump(yaml_dict)
        with patch.object(client, 'get_juju_output', return_value=output,
                          autospec=True) as gjo_mock:
            result = client.list_space()
        self.assertEqual(result, yaml_dict)
        gjo_mock.assert_called_once_with('list-space')

    def test_add_space(self):
        client = ModelClient(JujuData(None, {'type': 'lxd'}),
                             '1.23-series-arch', None)
        with patch.object(client, 'juju', autospec=True) as juju_mock:
            client.add_space('foo-space')
        juju_mock.assert_called_once_with('add-space', ('foo-space'))

    def test_add_subnet(self):
        client = ModelClient(JujuData(None, {'type': 'lxd'}),
                             '1.23-series-arch', None)
        with patch.object(client, 'juju', autospec=True) as juju_mock:
            client.add_subnet('bar-subnet', 'foo-space')
        juju_mock.assert_called_once_with('add-subnet',
                                          ('bar-subnet', 'foo-space'))

    def test__shell_environ_uses_pathsep(self):
        client = ModelClient(JujuData('foo'), None, 'foo/bar/juju')
        with patch('os.pathsep', '!'):
            environ = client._shell_environ()
        self.assertRegexpMatches(environ['PATH'], r'foo/bar\!')

    def test_set_config(self):
        client = ModelClient(JujuData('bar', {}), None, '/foo')
        with patch_juju_call(client) as juju_mock:
            client.set_config('foo', {'bar': 'baz'})
        juju_mock.assert_called_once_with('config', ('foo', 'bar=baz'))

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
        client = ModelClient(JujuData('bar', {}), None, '/foo')
        with patch.object(client, 'get_juju_output',
                          side_effect=output) as gjo_mock:
            results = client.get_config('foo')
        self.assertEqual(expected, results)
        gjo_mock.assert_called_once_with('config', 'foo')

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
        client = ModelClient(JujuData('bar', {}), None, '/foo')
        with patch.object(client, 'get_juju_output', side_effect=output):
            results = client.get_service_config('foo')
        self.assertEqual(expected, results)

    def test_get_service_config_timesout(self):
        client = ModelClient(JujuData('foo', {}), None, '/foo')
        with patch('jujupy.client.until_timeout', return_value=range(0)):
            with self.assertRaisesRegexp(
                    Exception, 'Timed out waiting for juju get'):
                with patch('jujupy.client.time.sleep'):
                    client.get_service_config('foo')

    def test_upgrade_mongo(self):
        client = ModelClient(JujuData('bar', {}), None, '/foo')
        with patch_juju_call(client) as juju_mock:
            client.upgrade_mongo()
        juju_mock.assert_called_once_with('upgrade-mongo', ())

    def test_enable_feature(self):
        client = ModelClient(JujuData('bar', {}), None, '/foo')
        self.assertEqual(set(), client.feature_flags)
        client.enable_feature('actions')
        self.assertEqual(set(['actions']), client.feature_flags)

    def test_enable_feature_invalid(self):
        client = ModelClient(JujuData('bar', {}), None, '/foo')
        self.assertEqual(set(), client.feature_flags)
        with self.assertRaises(ValueError) as ctx:
            client.enable_feature('nomongo')
        self.assertEqual(str(ctx.exception), "Unknown feature flag: 'nomongo'")

    def test_is_juju1x(self):
        client = ModelClient(None, '1.25.5', None)
        self.assertTrue(client.is_juju1x())

    def test_is_juju1x_false(self):
        client = ModelClient(None, '2.0.0', None)
        self.assertFalse(client.is_juju1x())

    def test__get_register_command_returns_register_token(self):
        output = dedent("""\
        User "x" added
        User "x" granted read access to model "y"
        Please send this command to x:
            juju register AaBbCc""")
        output_cmd = 'AaBbCc'
        fake_client = fake_juju_client()

        register_cmd = fake_client._get_register_command(output)
        self.assertEqual(register_cmd, output_cmd)

    def test_revoke(self):
        fake_client = fake_juju_client()
        username = 'fakeuser'
        model = 'foo'
        default_permissions = 'read'
        default_model = fake_client.model_name
        default_controller = fake_client.env.controller.name

        with patch_juju_call(fake_client):
            fake_client.revoke(username)
            fake_client.juju.assert_called_with('revoke',
                                                ('-c', default_controller,
                                                 username, default_permissions,
                                                 default_model),
                                                include_e=False)

            fake_client.revoke(username, model)
            fake_client.juju.assert_called_with('revoke',
                                                ('-c', default_controller,
                                                 username, default_permissions,
                                                 model),
                                                include_e=False)

            fake_client.revoke(username, model, permissions='write')
            fake_client.juju.assert_called_with('revoke',
                                                ('-c', default_controller,
                                                 username, 'write', model),
                                                include_e=False)

    def test_add_user_perms(self):
        fake_client = fake_juju_client()
        username = 'fakeuser'

        # Ensure add_user returns expected value.
        self.assertEqual(
            fake_client.add_user_perms(username),
            get_user_register_token(username))

    @staticmethod
    def assert_add_user_perms(model, permissions):
        fake_client = fake_juju_client()
        username = 'fakeuser'
        output = get_user_register_command_info(username)
        if permissions is None:
            permissions = 'login'
        expected_args = [username, '-c', fake_client.env.controller.name]
        with patch.object(fake_client, 'get_juju_output',
                          return_value=output) as get_output:
            with patch.object(fake_client, 'juju') as mock_juju:
                fake_client.add_user_perms(username, model, permissions)
                if model is None:
                    model = fake_client.env.environment
                get_output.assert_called_with(
                    'add-user', *expected_args, include_e=False)
                if permissions == 'login':
                    # Granting login is ignored, it's implicit when adding
                    # a user.
                    if mock_juju.call_count != 0:
                        raise Exception(
                            'Call count {} != 0'.format(
                                mock_juju.call_count))
                else:
                    mock_juju.assert_called_once_with(
                        'grant',
                        ('fakeuser', permissions,
                         model,
                         '-c', fake_client.env.controller.name),
                        include_e=False)

    def test_assert_add_user_permissions(self):
        model = 'foo'
        permissions = 'write'

        # Check using default model and permissions
        self.assert_add_user_perms(None, None)

        # Check explicit model & default permissions
        self.assert_add_user_perms(model, None)

        # Check explicit model & permissions
        self.assert_add_user_perms(model, permissions)

        # Check default model & explicit permissions
        self.assert_add_user_perms(None, permissions)

    def test_disable_user(self):
        env = JujuData('foo')
        username = 'fakeuser'
        client = ModelClient(env, None, None)
        with patch_juju_call(client) as mock:
            client.disable_user(username)
        mock.assert_called_with(
            'disable-user', ('-c', 'foo', 'fakeuser'), include_e=False)

    def test_enable_user(self):
        env = JujuData('foo')
        username = 'fakeuser'
        client = ModelClient(env, None, None)
        with patch_juju_call(client) as mock:
            client.enable_user(username)
        mock.assert_called_with(
            'enable-user', ('-c', 'foo', 'fakeuser'), include_e=False)

    def test_logout(self):
        env = JujuData('foo')
        client = ModelClient(env, None, None)
        with patch_juju_call(client) as mock:
            client.logout()
        mock.assert_called_with(
            'logout', ('-c', 'foo'), include_e=False)

    def test_register_host(self):
        client = fake_juju_client()
        controller_state = client._backend.controller_state
        client.env.controller.name = 'foo-controller'
        self.assertNotEqual(controller_state.name, client.env.controller.name)
        client.register_host('host1', 'email1', 'password1')
        self.assertEqual(controller_state.name, client.env.controller.name)
        self.assertEqual(controller_state.state, 'registered')
        jrandom = controller_state.users['jrandom@external']
        self.assertEqual(jrandom['email'], 'email1')
        self.assertEqual(jrandom['password'], 'password1')
        self.assertEqual(jrandom['2fa'], '')

    def test_login_user(self):
        client = fake_juju_client()
        controller_state = client._backend.controller_state
        client.env.controller.name = 'foo-controller'
        client.env.user_name = 'admin'
        username = 'bob'
        password = 'password1'
        client.login_user(username, password)
        user = controller_state.users[username]
        self.assertEqual(user['password'], password)

    def test_create_cloned_environment(self):
        fake_client = fake_juju_client()
        fake_client.bootstrap()
        # fake_client_environ = fake_client._shell_environ()
        controller_name = 'user_controller'

        with temp_dir() as path:
            cloned = fake_client.create_cloned_environment(
                path,
                controller_name
            )
            self.assertEqual(cloned.env.juju_home, path)
            self.assertItemsEqual(
                os.listdir(path),
                ['credentials.yaml', 'clouds.yaml'])
        self.assertIs(fake_client.__class__, type(cloned))
        self.assertEqual(cloned.env.controller.name, controller_name)
        self.assertEqual(fake_client.env.controller.name, 'name')

    def test_list_clouds(self):
        env = JujuData('foo')
        client = ModelClient(env, None, None)
        with patch.object(client, 'get_juju_output') as mock:
            client.list_clouds()
        mock.assert_called_with(
            'list-clouds', '--format', 'json', include_e=False)

    def test_add_cloud_interactive_maas(self):
        client = fake_juju_client()
        clouds = {'foo': {
            'type': 'maas',
            'endpoint': 'http://bar.example.com',
            }}
        client.add_cloud_interactive('foo', clouds['foo'])
        self.assertEqual(client._backend.clouds, clouds)

    def test_add_cloud_interactive_maas_invalid_endpoint(self):
        client = fake_juju_client()
        clouds = {'foo': {
            'type': 'maas',
            'endpoint': 'B' * 4000,
            }}
        with self.assertRaises(InvalidEndpoint):
            client.add_cloud_interactive('foo', clouds['foo'])

    def test_add_cloud_interactive_manual(self):
        client = fake_juju_client()
        clouds = {'foo': {'type': 'manual', 'endpoint': '127.100.100.1'}}
        client.add_cloud_interactive('foo', clouds['foo'])
        self.assertEqual(client._backend.clouds, clouds)

    def test_add_cloud_interactive_manual_invalid_endpoint(self):
        client = fake_juju_client()
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
        client = fake_juju_client()
        clouds = self.get_openstack_clouds()
        client.add_cloud_interactive('foo', clouds['foo'])
        self.assertEqual(client._backend.clouds, clouds)

    def test_add_cloud_interactive_openstack_invalid_endpoint(self):
        client = fake_juju_client()
        clouds = self.get_openstack_clouds()
        clouds['foo']['endpoint'] = 'B' * 4000
        with self.assertRaises(InvalidEndpoint):
            client.add_cloud_interactive('foo', clouds['foo'])

    def test_add_cloud_interactive_openstack_invalid_region_endpoint(self):
        client = fake_juju_client()
        clouds = self.get_openstack_clouds()
        clouds['foo']['regions']['harvey']['endpoint'] = 'B' * 4000
        with self.assertRaises(InvalidEndpoint):
            client.add_cloud_interactive('foo', clouds['foo'])

    def test_add_cloud_interactive_openstack_invalid_auth(self):
        client = fake_juju_client()
        clouds = self.get_openstack_clouds()
        clouds['foo']['auth-types'] = ['invalid', 'oauth12']
        with self.assertRaises(AuthNotAccepted):
            client.add_cloud_interactive('foo', clouds['foo'])

    def test_add_cloud_interactive_vsphere(self):
        client = fake_juju_client()
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

    def test_add_cloud_interactive_vsphere_invalid_endpoint(self):
        client = fake_juju_client()
        clouds = {'foo': {
            'type': 'vsphere',
            'endpoint': 'B' * 4000,
            'regions': {
                'harvey': {},
                'steve': {},
                }
            }}
        with self.assertRaises(InvalidEndpoint):
            client.add_cloud_interactive('foo', clouds['foo'])

    def test_add_cloud_interactive_bogus(self):
        client = fake_juju_client()
        clouds = {'foo': {'type': 'bogus'}}
        with self.assertRaises(TypeNotAccepted):
            client.add_cloud_interactive('foo', clouds['foo'])

    def test_add_cloud_interactive_invalid_name(self):
        client = fake_juju_client()
        cloud = {'type': 'manual', 'endpoint': 'example.com'}
        with self.assertRaises(NameNotAccepted):
            client.add_cloud_interactive('invalid/name', cloud)

    def test_show_controller(self):
        env = JujuData('foo')
        client = ModelClient(env, None, None)
        with patch.object(client, 'get_juju_output') as mock:
            client.show_controller()
        mock.assert_called_with(
            'show-controller', '--format', 'json', include_e=False)

    def test_show_machine(self):
        output = """\
        machines:
          "0":
            series: bionic
        """
        env = JujuData('foo')
        client = ModelClient(env, None, None)
        with patch.object(client, 'get_juju_output', autospec=True,
                          return_value=output) as mock:
            data = client.show_machine('0')
        mock.assert_called_once_with('show-machine', '0', '--format', 'yaml')
        self.assertEqual({'machines': {'0': {'series': 'bionic'}}}, data)

    def test_ssh_keys(self):
        client = ModelClient(JujuData('foo'), None, None)
        given_output = 'ssh keys output'
        with patch.object(client, 'get_juju_output', autospec=True,
                          return_value=given_output) as mock:
            output = client.ssh_keys()
        self.assertEqual(output, given_output)
        mock.assert_called_once_with('ssh-keys')

    def test_ssh_keys_full(self):
        client = ModelClient(JujuData('foo'), None, None)
        given_output = 'ssh keys full output'
        with patch.object(client, 'get_juju_output', autospec=True,
                          return_value=given_output) as mock:
            output = client.ssh_keys(full=True)
        self.assertEqual(output, given_output)
        mock.assert_called_once_with('ssh-keys', '--full')

    def test_add_ssh_key(self):
        client = ModelClient(JujuData('foo'), None, None)
        with patch.object(client, 'get_juju_output', autospec=True,
                          return_value='') as mock:
            output = client.add_ssh_key('ak', 'bk')
        self.assertEqual(output, '')
        mock.assert_called_once_with(
            'add-ssh-key', 'ak', 'bk', merge_stderr=True)

    def test_remove_ssh_key(self):
        client = ModelClient(JujuData('foo'), None, None)
        with patch.object(client, 'get_juju_output', autospec=True,
                          return_value='') as mock:
            output = client.remove_ssh_key('ak', 'bk')
        self.assertEqual(output, '')
        mock.assert_called_once_with(
            'remove-ssh-key', 'ak', 'bk', merge_stderr=True)

    def test_import_ssh_key(self):
        client = ModelClient(JujuData('foo'), None, None)
        with patch.object(client, 'get_juju_output', autospec=True,
                          return_value='') as mock:
            output = client.import_ssh_key('gh:au', 'lp:bu')
        self.assertEqual(output, '')
        mock.assert_called_once_with(
            'import-ssh-key', 'gh:au', 'lp:bu', merge_stderr=True)

    def test_disable_commands_properties(self):
        client = ModelClient(JujuData('foo'), None, None)
        self.assertEqual('destroy-model', client.command_set_destroy_model)
        self.assertEqual('remove-object', client.command_set_remove_object)
        self.assertEqual('all', client.command_set_all)

    def test_list_disabled_commands(self):
        client = ModelClient(JujuData('foo'), None, None)
        with patch.object(client, 'get_juju_output', autospec=True,
                          return_value=dedent("""\
             - command-set: destroy-model
               message: Lock Models
             - command-set: remove-object""")) as mock:
            output = client.list_disabled_commands()
        self.assertEqual([{'command-set': 'destroy-model',
                           'message': 'Lock Models'},
                          {'command-set': 'remove-object'}], output)
        mock.assert_called_once_with('list-disabled-commands',
                                     '--format', 'yaml')

    def test_disable_command(self):
        client = ModelClient(JujuData('foo'), None, None)
        with patch_juju_call(client) as mock:
            client.disable_command('all', 'message')
        mock.assert_called_once_with('disable-command', ('all', 'message'))

    def test_enable_command(self):
        client = ModelClient(JujuData('foo'), None, None)
        with patch_juju_call(client) as mock:
            client.enable_command('all')
        mock.assert_called_once_with('enable-command', 'all')

    def test_sync_tools(self):
        client = ModelClient(JujuData('foo'), None, None)
        with patch_juju_call(client) as mock:
            client.sync_tools()
        mock.assert_called_once_with('sync-tools', ())

    def test_sync_tools_local_dir(self):
        client = ModelClient(JujuData('foo'), None, None)
        with patch_juju_call(client) as mock:
            client.sync_tools('/agents')
        mock.assert_called_once_with('sync-tools', ('--local-dir', '/agents'),
                                     include_e=False)

    def test_generate_tool(self):
        client = ModelClient(JujuData('foo'), None, None)
        with patch_juju_call(client) as mock:
            client.generate_tool('/agents')
        mock.assert_called_once_with('metadata',
                                     ('generate-tools', '-d', '/agents'),
                                     include_e=False)

    def test_generate_tool_with_stream(self):
        client = ModelClient(JujuData('foo'), None, None)
        with patch_juju_call(client) as mock:
            client.generate_tool('/agents', "testing")
        mock.assert_called_once_with(
            'metadata', ('generate-tools', '-d', '/agents',
                         '--stream', 'testing'), include_e=False)

    def test_add_cloud(self):
        client = ModelClient(JujuData('foo'), None, None)
        with patch_juju_call(client) as mock:
            client.add_cloud('localhost', 'cfile')
        mock.assert_called_once_with('add-cloud',
                                     ('--replace', 'localhost', 'cfile'),
                                     include_e=False)

    def test_switch(self):
        def run_switch_test(expect, model=None, controller=None):
            client = ModelClient(JujuData('foo'), None, None)
            with patch_juju_call(client) as mock:
                client.switch(model=model, controller=controller)
            mock.assert_called_once_with('switch', (expect,), include_e=False)
        run_switch_test('default', 'default')
        run_switch_test('other', controller='other')
        run_switch_test('other:default', 'default', 'other')

    def test_switch_no_target(self):
        client = ModelClient(JujuData('foo'), None, None)
        self.assertRaises(ValueError, client.switch)


@contextmanager
def bootstrap_context(client=None):
    # Avoid unnecessary syscalls.
    with scoped_environ():
        with temp_dir() as fake_home:
            os.environ['JUJU_HOME'] = fake_home
            yield fake_home


class TestJujuHomePath(TestCase):

    def test_juju_home_path(self):
        path = juju_home_path('/home/jrandom/foo', 'bar')
        self.assertEqual(path, '/home/jrandom/foo/juju-homes/bar')


class TestGetCachePath(TestCase):

    def test_get_cache_path(self):
        path = get_cache_path('/home/jrandom/foo')
        self.assertEqual(path, '/home/jrandom/foo/environments/cache.yaml')

    def test_get_cache_path_models(self):
        path = get_cache_path('/home/jrandom/foo', models=True)
        self.assertEqual(path, '/home/jrandom/foo/models/cache.yaml')


class TestMakeSafeConfig(TestCase):

    def test_default(self):
        client = fake_juju_client(JujuData('foo', {'type': 'bar'},
                                           juju_home='foo'),
                                  version='1.2-alpha3-asdf-asdf')
        config = make_safe_config(client)
        self.assertEqual({
            'name': 'foo',
            'type': 'bar',
            'test-mode': True,
            'agent-version': '1.2-alpha3',
            }, config)

    def test_bootstrap_replaces_agent_version(self):
        client = fake_juju_client(JujuData('foo', {'type': 'bar'},
                                  juju_home='foo'))
        client.bootstrap_replaces = {'agent-version'}
        self.assertNotIn('agent-version', make_safe_config(client))
        client.env.update_config({'agent-version': '1.23'})
        self.assertNotIn('agent-version', make_safe_config(client))


class TestTempBootstrapEnv(FakeHomeTestCase):

    @staticmethod
    def get_client(env):
        return ModelClient(env, '1.24-fake', 'fake-juju-path')

    def test_no_config_mangling_side_effect(self):
        env = JujuData('qux', {'type': 'lxd'})
        client = self.get_client(env)
        with bootstrap_context(client) as fake_home:
            with temp_bootstrap_env(fake_home, client):
                pass
        self.assertEqual(env.provider, 'lxd')

    def test_temp_bootstrap_env_provides_dir(self):
        env = JujuData('qux', {'type': 'lxd'})
        client = self.get_client(env)
        juju_home = os.path.join(self.home_dir, 'juju-homes', 'qux')

        def side_effect(*args, **kwargs):
            os.mkdir(juju_home)
            return juju_home

        with patch('jujupy.utility.mkdtemp', side_effect=side_effect):
            with temp_bootstrap_env(self.home_dir, client) as temp_home:
                pass
        self.assertEqual(temp_home, juju_home)

    def test_temp_bootstrap_env_no_set_home(self):
        env = JujuData('qux', {'type': 'lxd'})
        client = self.get_client(env)
        os.environ['JUJU_HOME'] = 'foo'
        os.environ['JUJU_DATA'] = 'bar'
        with temp_bootstrap_env(self.home_dir, client):
            self.assertEqual(os.environ['JUJU_HOME'], 'foo')
            self.assertEqual(os.environ['JUJU_DATA'], 'bar')


class TestController(TestCase):

    def test_controller(self):
        controller = Controller('ctrl')
        self.assertEqual('ctrl', controller.name)


class TestJujuData(TestCase):

    def test_init(self):
        controller = Mock()
        with temp_dir() as juju_home:
            juju_data = JujuData(
                'foo', {'enable_os_upgrade': False},
                juju_home=juju_home, controller=controller, cloud_name='bar',
                bootstrap_to='zone=baz')
            self.assertEqual(juju_home, juju_data.juju_home)
            self.assertEqual('bar', juju_data._cloud_name)
            self.assertIs(controller, juju_data.controller)
            self.assertEqual('zone=baz', juju_data.bootstrap_to)

    def from_cloud_region(self, provider_type, region):
        with temp_dir() as juju_home:
            data_writer = JujuData('foo', {}, juju_home)
            data_writer.clouds = {'clouds': {'foo': {}}}
            data_writer.credentials = {'credentials': {'bar': {}}}
            data_writer.dump_yaml(juju_home)
            data_reader = JujuData.from_cloud_region('bar', region, {}, {
                'clouds': {'bar': {'type': provider_type, 'endpoint': 'x'}},
                }, juju_home)
        self.assertEqual(data_reader.credentials,
                         data_writer.credentials)
        self.assertEqual('bar', data_reader.get_cloud())
        self.assertEqual(region, data_reader.get_region())
        self.assertEqual('bar', data_reader._cloud_name)

    def test_from_cloud_region_openstack(self):
        self.from_cloud_region('openstack', 'baz')

    def test_from_cloud_region_maas(self):
        self.from_cloud_region('maas', None)

    def test_from_cloud_region_vsphere(self):
        self.from_cloud_region('vsphere', None)

    def test_for_existing(self):
        with temp_dir() as juju_home:
            with open(get_bootstrap_config_path(juju_home), 'w') as f:
                yaml.safe_dump({'controllers': {'foo': {
                    'controller-config': {
                        'ctrl1': 'ctrl2',
                        'duplicated1': 'duplicated2',
                        },
                    'model-config': {
                        'model1': 'model2',
                        'type': 'provider2',
                        'duplicated1': 'duplicated3',
                        },
                    'cloud': 'cloud1',
                    'region': 'region1',
                    'type': 'provider1',
                    }}}, f)
            data = JujuData.for_existing(juju_home, 'foo', 'bar')
        self.assertEqual(data.get_cloud(), 'cloud1')
        self.assertEqual(data.get_region(), 'region1')
        self.assertEqual(data.provider, 'provider1')
        self.assertEqual(data._config, {
            'model1': 'model2',
            'ctrl1': 'ctrl2',
            'duplicated1': 'duplicated3',
            'region': 'region1',
            'type': 'provider1',
            })
        self.assertEqual(data.controller.name, 'foo')
        self.assertEqual(data.environment, 'bar')

    def test_clone(self):
        orig = JujuData('foo', {'type': 'bar'}, 'myhome',
                        bootstrap_to='zonea', cloud_name='cloudname')
        orig.credentials = {'secret': 'password'}
        orig.clouds = {'name': {'meta': 'data'}}
        orig.local = 'local1'
        orig.kvm = 'kvm1'
        orig.maas = 'maas1'
        orig.user_name = 'user1'
        orig.bootstrap_to = 'zonea'

        copy = orig.clone()
        self.assertIs(JujuData, type(copy))
        self.assertIsNot(orig, copy)
        self.assertEqual(copy.environment, 'foo')
        self.assertIsNot(orig._config, copy._config)
        self.assertEqual({'type': 'bar'}, copy._config)
        self.assertEqual('myhome', copy.juju_home)
        self.assertEqual('zonea', copy.bootstrap_to)
        self.assertIsNot(orig.credentials, copy.credentials)
        self.assertEqual(orig.credentials, copy.credentials)
        self.assertIsNot(orig.clouds, copy.clouds)
        self.assertEqual(orig.clouds, copy.clouds)
        self.assertEqual('cloudname', copy._cloud_name)
        self.assertEqual({'type': 'bar'}, copy._config)
        self.assertEqual('myhome', copy.juju_home)
        self.assertEqual('kvm1', copy.kvm)
        self.assertEqual('maas1', copy.maas)
        self.assertEqual('user1', copy.user_name)
        self.assertEqual('zonea', copy.bootstrap_to)
        self.assertIs(orig.controller, copy.controller)

    def test_set_model_name(self):
        env = JujuData('foo', {}, juju_home='')
        env.set_model_name('bar')
        self.assertEqual(env.environment, 'bar')
        self.assertEqual(env.controller.name, 'bar')
        self.assertEqual(env.get_option('name'), 'bar')

    def test_clone_model_name(self):
        orig = JujuData('foo', {'type': 'bar', 'name': 'oldname'}, 'myhome')
        orig.credentials = {'secret': 'password'}
        orig.clouds = {'name': {'meta': 'data'}}
        copy = orig.clone(model_name='newname')
        self.assertEqual('newname', copy.environment)
        self.assertEqual('newname', copy.get_option('name'))

    def test_discard_option(self):
        env = JujuData('foo', {'type': 'foo', 'bar': 'baz'}, juju_home='')
        discarded = env.discard_option('bar')
        self.assertEqual('baz', discarded)
        self.assertEqual({'type': 'foo'}, env._config)

    def test_discard_option_not_present(self):
        env = JujuData('foo', {'type': 'foo'}, juju_home='')
        discarded = env.discard_option('bar')
        self.assertIs(None, discarded)
        self.assertEqual({'type': 'foo'}, env._config)

    def test_update_config(self):
        env = JujuData('foo', {'type': 'azure'}, juju_home='')
        env.update_config({'bar': 'baz', 'qux': 'quxx'})
        self.assertEqual(env._config, {
            'type': 'azure', 'bar': 'baz', 'qux': 'quxx'})

    def test_update_config_region(self):
        env = JujuData('foo', {'type': 'azure'}, juju_home='')
        env.update_config({'region': 'foo1'})
        self.assertEqual(env._config, {
            'type': 'azure', 'location': 'foo1'})
        self.assertEqual('WARNING Using set_region to set region to "foo1".\n',
                         self.log_stream.getvalue())

    def test_update_config_type(self):
        env = JujuData('foo', {'type': 'azure'}, juju_home='')
        with self.assertRaisesRegexp(
                ValueError, 'type cannot be set via update_config.'):
            env.update_config({'type': 'foo1'})

    def test_update_config_cloud_name(self):
        env = JujuData('foo', {'type': 'azure'}, juju_home='',
                       cloud_name='steve')
        for endpoint_key in ['maas-server', 'auth-url', 'host']:
            with self.assertRaisesRegexp(
                    ValueError, '{} cannot be changed with'
                    ' explicit cloud name.'.format(endpoint_key)):
                env.update_config({endpoint_key: 'foo1'})

    def test_get_cloud_random_provider(self):
        self.assertEqual(
            'bar', JujuData('foo', {'type': 'bar'}, 'home').get_cloud())

    def test_get_cloud_ec2(self):
        self.assertEqual(
            'aws', JujuData('foo', {'type': 'ec2', 'region': 'bar'},
                            'home').get_cloud())
        self.assertEqual(
            'aws-china', JujuData('foo', {
                'type': 'ec2', 'region': 'cn-north-1'
                }, 'home').get_cloud())

    def test_get_cloud_gce(self):
        self.assertEqual(
            'google', JujuData('foo', {'type': 'gce', 'region': 'bar'},
                               'home').get_cloud())

    def test_get_cloud_maas(self):
        data = JujuData('foo', {'type': 'maas', 'maas-server': 'bar'}, 'home')
        data.clouds = {'clouds': {
            'baz': {'type': 'maas', 'endpoint': 'bar'},
            'qux': {'type': 'maas', 'endpoint': 'qux'},
            }}
        self.assertEqual('baz', data.get_cloud())

    def test_get_cloud_maas_wrong_type(self):
        data = JujuData('foo', {'type': 'maas', 'maas-server': 'bar'}, 'home')
        data.clouds = {'clouds': {
            'baz': {'type': 'foo', 'endpoint': 'bar'},
            }}
        with self.assertRaisesRegexp(LookupError, 'No such endpoint: bar'):
            self.assertEqual(data.get_cloud())

    def test_get_cloud_openstack(self):
        data = JujuData('foo', {'type': 'openstack', 'auth-url': 'bar'},
                        'home')
        data.clouds = {'clouds': {
            'baz': {'type': 'openstack', 'endpoint': 'bar'},
            'qux': {'type': 'openstack', 'endpoint': 'qux'},
            }}
        self.assertEqual('baz', data.get_cloud())

    def test_get_cloud_openstack_wrong_type(self):
        data = JujuData('foo', {'type': 'openstack', 'auth-url': 'bar'},
                        'home')
        data.clouds = {'clouds': {
            'baz': {'type': 'maas', 'endpoint': 'bar'},
            }}
        with self.assertRaisesRegexp(LookupError, 'No such endpoint: bar'):
            data.get_cloud()

    def test_get_cloud_vsphere(self):
        data = JujuData('foo', {'type': 'vsphere', 'host': 'bar'},
                        'home')
        data.clouds = {'clouds': {
            'baz': {'type': 'vsphere', 'endpoint': 'bar'},
            'qux': {'type': 'vsphere', 'endpoint': 'qux'},
            }}
        self.assertEqual('baz', data.get_cloud())

    def test_get_cloud_credentials_item(self):
        juju_data = JujuData('foo', {'type': 'ec2', 'region': 'foo'}, 'home')
        juju_data.credentials = {'credentials': {
            'aws': {'credentials': {'aws': True}},
            'azure': {'credentials': {'azure': True}},
            }}
        self.assertEqual(('credentials', {'aws': True}),
                         juju_data.get_cloud_credentials_item())

    def test_get_cloud_credentials(self):
        juju_data = JujuData('foo', {'type': 'ec2', 'region': 'foo'}, 'home')
        juju_data.credentials = {'credentials': {
            'aws': {'credentials': {'aws': True}},
            'azure': {'credentials': {'azure': True}},
            }}
        self.assertEqual({'aws': True}, juju_data.get_cloud_credentials())

    def test_get_cloud_name_with_cloud_name(self):
        juju_data = JujuData('foo', {'type': 'bar'}, 'home')
        self.assertEqual('bar', juju_data.get_cloud())
        juju_data = JujuData('foo', {'type': 'bar'}, 'home', cloud_name='baz')
        self.assertEqual('baz', juju_data.get_cloud())

    def test_set_region(self):
        env = JujuData('foo', {'type': 'bar'}, 'home')
        env.set_region('baz')
        self.assertEqual(env.get_option('region'), 'baz')
        self.assertEqual(env.get_region(), 'baz')

    def test_set_region_no_provider(self):
        env = JujuData('foo', {}, 'home')
        env.set_region('baz')
        self.assertEqual(env.get_option('region'), 'baz')

    def test_set_region_azure(self):
        env = JujuData('foo', {'type': 'azure'}, 'home')
        env.set_region('baz')
        self.assertEqual(env.get_option('location'), 'baz')
        self.assertEqual(env.get_region(), 'baz')

    def test_set_region_lxd(self):
        env = JujuData('foo', {'type': 'lxd'}, 'home')
        env.set_region('baz')
        self.assertEqual(env.get_option('region'), 'baz')

    def test_set_region_manual(self):
        env = JujuData('foo', {'type': 'manual'}, 'home')
        env.set_region('baz')
        self.assertEqual(env.get_option('bootstrap-host'), 'baz')
        self.assertEqual(env.get_region(), 'baz')

    def test_set_region_manual_named_cloud(self):
        env = JujuData('foo', {'type': 'manual'}, 'home', cloud_name='qux')
        env.set_region('baz')
        self.assertEqual(env.get_option('region'), 'baz')
        self.assertEqual(env.get_region(), 'baz')

    def test_set_region_maas(self):
        env = JujuData('foo', {'type': 'maas'}, 'home')
        with self.assertRaisesRegexp(ValueError,
                                     'Only None allowed for maas.'):
            env.set_region('baz')
        env.set_region(None)
        self.assertIs(env.get_region(), None)

    def test_get_region(self):
        self.assertEqual(
            'bar', JujuData(
                'foo', {'type': 'foo', 'region': 'bar'}, 'home').get_region())

    def test_get_region_old_azure(self):
        self.assertEqual('northeu', JujuData('foo', {
            'type': 'azure', 'location': 'North EU'}, 'home').get_region())

    def test_get_region_azure_arm(self):
        self.assertEqual('bar', JujuData('foo', {
            'type': 'azure', 'location': 'bar', 'tenant-id': 'baz'},
            'home').get_region())

    def test_get_region_lxd(self):
        self.assertEqual('localhost', JujuData(
            'foo', {'type': 'lxd'}, 'home').get_region())

    def test_get_region_lxd_specified(self):
        self.assertEqual('foo', JujuData(
            'foo', {'type': 'lxd', 'region': 'foo'}, 'home').get_region())

    def test_get_region_maas(self):
        self.assertIs(None, JujuData('foo', {
            'type': 'maas', 'region': 'bar',
        }, 'home').get_region())

    def test_get_region_manual(self):
        self.assertEqual('baz', JujuData('foo', {
            'type': 'manual', 'region': 'bar',
            'bootstrap-host': 'baz'}, 'home').get_region())

    def test_get_region_manual_named_cloud(self):
        self.assertEqual('bar', JujuData('foo', {
            'type': 'manual', 'region': 'bar',
            'bootstrap-host': 'baz'}, 'home', cloud_name='qux').get_region())

    def test_get_region_manual_cloud_name_manual(self):
        self.assertEqual('baz', JujuData('foo', {
            'type': 'manual', 'region': 'bar',
            'bootstrap-host': 'baz'}, 'home',
            cloud_name='manual').get_region())

    def test_get_region_manual_named_cloud_no_region(self):
        self.assertIs(None, JujuData('foo', {
            'type': 'manual', 'bootstrap-host': 'baz',
            }, 'home', cloud_name='qux').get_region())

    def test_dump_yaml(self):
        cloud_dict = {'clouds': {'foo': {}}}
        credential_dict = {'credential': {'bar': {}}}
        data = JujuData('baz', {'type': 'qux'}, 'home')
        data.clouds = dict(cloud_dict)
        data.credentials = dict(credential_dict)
        with temp_dir() as path:
            data.dump_yaml(path)
            self.assertItemsEqual(
                ['clouds.yaml', 'credentials.yaml'], os.listdir(path))
            with open(os.path.join(path, 'clouds.yaml')) as f:
                self.assertEqual(cloud_dict, yaml.safe_load(f))
            with open(os.path.join(path, 'credentials.yaml')) as f:
                self.assertEqual(credential_dict, yaml.safe_load(f))

    def test_load_yaml(self):
        cloud_dict = {'clouds': {'foo': {}}}
        credential_dict = {'credential': {'bar': {}}}
        with temp_dir() as path:
            with open(os.path.join(path, 'clouds.yaml'), 'w') as f:
                yaml.safe_dump(cloud_dict, f)
            with open(os.path.join(path, 'credentials.yaml'), 'w') as f:
                yaml.safe_dump(credential_dict, f)
            data = JujuData('baz', {'type': 'qux'}, path)
            data.load_yaml()

    def test_get_option(self):
        env = JujuData('foo', {'type': 'azure', 'foo': 'bar'}, juju_home='')
        self.assertEqual(env.get_option('foo'), 'bar')
        self.assertIs(env.get_option('baz'), None)

    def test_get_option_sentinel(self):
        env = JujuData('foo', {'type': 'azure', 'foo': 'bar'}, juju_home='')
        sentinel = object()
        self.assertIs(env.get_option('baz', sentinel), sentinel)


class TestDescribeSubstrate(TestCase):

    def setUp(self):
        super(TestDescribeSubstrate, self).setUp()
        # JujuData expects a JUJU_HOME or HOME env as it gets juju_home_path
        os.environ['HOME'] = '/tmp/jujupy-tests/'

    def test_openstack(self):
        env = JujuData('foo', {
            'type': 'openstack',
            'auth-url': 'foo',
            })
        self.assertEqual(describe_substrate(env), 'Openstack')

    def test_canonistack(self):
        env = JujuData('foo', {
            'type': 'openstack',
            'auth-url': 'https://keystone.canonistack.canonical.com:443/v2.0/',
            })
        self.assertEqual(describe_substrate(env), 'Canonistack')

    def test_aws(self):
        env = JujuData('foo', {
            'type': 'ec2',
            })
        self.assertEqual(describe_substrate(env), 'AWS')

    def test_rackspace(self):
        env = JujuData('foo', {
            'type': 'rackspace',
            })
        self.assertEqual(describe_substrate(env), 'Rackspace')

    def test_azure(self):
        env = JujuData('foo', {
            'type': 'azure',
            })
        self.assertEqual(describe_substrate(env), 'Azure')

    def test_maas(self):
        env = JujuData('foo', {
            'type': 'maas',
            })
        self.assertEqual(describe_substrate(env), 'MAAS')

    def test_bar(self):
        env = JujuData('foo', {
            'type': 'bar',
            })
        self.assertEqual(describe_substrate(env), 'bar')


class TestGroupReporter(TestCase):

    def test_single(self):
        sio = StringIO()
        reporter = GroupReporter(sio, "done")
        self.assertEqual(sio.getvalue(), "")
        reporter.update({"working": ["1"]})
        self.assertEqual(sio.getvalue(), "working: 1")
        reporter.update({"done": ["1"]})
        self.assertEqual(sio.getvalue(), "working: 1\n")

    def test_single_ticks(self):
        sio = StringIO()
        reporter = GroupReporter(sio, "done")
        reporter.update({"working": ["1"]})
        self.assertEqual(sio.getvalue(), "working: 1")
        reporter.update({"working": ["1"]})
        self.assertEqual(sio.getvalue(), "working: 1 .")
        reporter.update({"working": ["1"]})
        self.assertEqual(sio.getvalue(), "working: 1 ..")
        reporter.update({"done": ["1"]})
        self.assertEqual(sio.getvalue(), "working: 1 ..\n")

    def test_multiple_values(self):
        sio = StringIO()
        reporter = GroupReporter(sio, "done")
        reporter.update({"working": ["1", "2"]})
        self.assertEqual(sio.getvalue(), "working: 1, 2")
        reporter.update({"working": ["1"], "done": ["2"]})
        self.assertEqual(sio.getvalue(), "working: 1, 2\nworking: 1")
        reporter.update({"done": ["1", "2"]})
        self.assertEqual(sio.getvalue(), "working: 1, 2\nworking: 1\n")

    def test_multiple_groups(self):
        sio = StringIO()
        reporter = GroupReporter(sio, "done")
        reporter.update({"working": ["1", "2"], "starting": ["3"]})
        first = "starting: 3 | working: 1, 2"
        self.assertEqual(sio.getvalue(), first)
        reporter.update({"working": ["1", "3"], "done": ["2"]})
        second = "working: 1, 3"
        self.assertEqual(sio.getvalue(), "\n".join([first, second]))
        reporter.update({"done": ["1", "2", "3"]})
        self.assertEqual(sio.getvalue(), "\n".join([first, second, ""]))

    def test_finish(self):
        sio = StringIO()
        reporter = GroupReporter(sio, "done")
        self.assertEqual(sio.getvalue(), "")
        reporter.update({"working": ["1"]})
        self.assertEqual(sio.getvalue(), "working: 1")
        reporter.finish()
        self.assertEqual(sio.getvalue(), "working: 1\n")

    def test_finish_unchanged(self):
        sio = StringIO()
        reporter = GroupReporter(sio, "done")
        self.assertEqual(sio.getvalue(), "")
        reporter.finish()
        self.assertEqual(sio.getvalue(), "")

    def test_wrap_to_width(self):
        sio = StringIO()
        reporter = GroupReporter(sio, "done")
        self.assertEqual(sio.getvalue(), "")
        for _ in range(150):
            reporter.update({"working": ["1"]})
        reporter.finish()
        self.assertEqual(sio.getvalue(), """\
working: 1 ....................................................................
...............................................................................
..
""")

    def test_wrap_to_width_exact(self):
        sio = StringIO()
        reporter = GroupReporter(sio, "done")
        reporter.wrap_width = 12
        self.assertEqual(sio.getvalue(), "")
        changes = []
        for _ in range(20):
            reporter.update({"working": ["1"]})
            changes.append(sio.getvalue())
        self.assertEqual(changes[::4], [
            "working: 1",
            "working: 1 .\n...",
            "working: 1 .\n.......",
            "working: 1 .\n...........",
            "working: 1 .\n............\n...",
        ])
        reporter.finish()
        self.assertEqual(sio.getvalue(), changes[-1] + "\n")

    def test_wrap_to_width_overflow(self):
        sio = StringIO()
        reporter = GroupReporter(sio, "done")
        reporter.wrap_width = 8
        self.assertEqual(sio.getvalue(), "")
        changes = []
        for _ in range(16):
            reporter.update({"working": ["1"]})
            changes.append(sio.getvalue())
        self.assertEqual(changes[::4], [
            "working: 1",
            "working: 1\n....",
            "working: 1\n........",
            "working: 1\n........\n....",
        ])
        reporter.finish()
        self.assertEqual(sio.getvalue(), changes[-1] + "\n")

    def test_wrap_to_width_multiple_groups(self):
        sio = StringIO()
        reporter = GroupReporter(sio, "done")
        reporter.wrap_width = 16
        self.assertEqual(sio.getvalue(), "")
        changes = []
        for _ in range(6):
            reporter.update({"working": ["1", "2"]})
            changes.append(sio.getvalue())
        for _ in range(10):
            reporter.update({"working": ["1"], "done": ["2"]})
            changes.append(sio.getvalue())
        self.assertEqual(changes[::4], [
            "working: 1, 2",
            "working: 1, 2 ..\n..",
            "working: 1, 2 ..\n...\n"
            "working: 1 ..",
            "working: 1, 2 ..\n...\n"
            "working: 1 .....\n.",
        ])
        reporter.finish()
        self.assertEqual(sio.getvalue(), changes[-1] + "\n")


class AssessParseStateServerFromErrorTestCase(TestCase):

    def test_parse_new_state_server_from_error(self):
        output = dedent("""
            Waiting for address
            Attempting to connect to 10.0.0.202:22
            Attempting to connect to 1.2.3.4:22
            The fingerprint for the ECDSA key sent by the remote host is
            """)
        error = subprocess.CalledProcessError(1, ['foo'], output)
        address = parse_new_state_server_from_error(error)
        self.assertEqual('1.2.3.4', address)

    def test_parse_new_state_server_from_error_output_none(self):
        error = subprocess.CalledProcessError(1, ['foo'], None)
        address = parse_new_state_server_from_error(error)
        self.assertIs(None, address)

    def test_parse_new_state_server_from_error_no_output(self):
        address = parse_new_state_server_from_error(Exception())
        self.assertIs(None, address)


class TestGetMachineDNSName(TestCase):

    log_level = logging.DEBUG

    machine_0_no_addr = """\
        machines:
            "0":
                instance-id: pending
        """

    machine_0_hostname = """\
        machines:
            "0":
                dns-name: a-host
        """

    machine_0_ipv6 = """\
        machines:
            "0":
                dns-name: 2001:db8::3
        """

    def test_gets_host(self):
        status = Status.from_text(self.machine_0_hostname)
        fake_client = Mock(spec=['status_until'])
        fake_client.status_until.return_value = [status]
        host = get_machine_dns_name(fake_client, '0')
        self.assertEqual(host, "a-host")
        fake_client.status_until.assert_called_once_with(timeout=600)
        self.assertEqual(self.log_stream.getvalue(), "")

    def test_retries_for_dns_name(self):
        status_pending = Status.from_text(self.machine_0_no_addr)
        status_host = Status.from_text(self.machine_0_hostname)
        fake_client = Mock(spec=['status_until'])
        fake_client.status_until.return_value = [status_pending, status_host]
        host = get_machine_dns_name(fake_client, '0')
        self.assertEqual(host, "a-host")
        fake_client.status_until.assert_called_once_with(timeout=600)
        self.assertEqual(
            self.log_stream.getvalue(),
            "DEBUG No dns-name yet for machine 0\n")

    def test_retries_gives_up(self):
        status = Status.from_text(self.machine_0_no_addr)
        fake_client = Mock(spec=['status_until'])
        fake_client.status_until.return_value = [status] * 3
        host = get_machine_dns_name(fake_client, '0', timeout=10)
        self.assertEqual(host, None)
        fake_client.status_until.assert_called_once_with(timeout=10)
        self.assertEqual(
            self.log_stream.getvalue(),
            "DEBUG No dns-name yet for machine 0\n" * 3)

    def test_gets_ipv6(self):
        status = Status.from_text(self.machine_0_ipv6)
        fake_client = Mock(spec=['status_until'])
        fake_client.status_until.return_value = [status]
        host = get_machine_dns_name(fake_client, '0')
        self.assertEqual(host, "2001:db8::3")
        fake_client.status_until.assert_called_once_with(timeout=600)
        self.assertEqual(
            self.log_stream.getvalue(),
            "WARNING Selected IPv6 address for machine 0: '2001:db8::3'\n")

    def test_gets_ipv6_unsupported(self):
        status = Status.from_text(self.machine_0_ipv6)
        fake_client = Mock(spec=['status_until'])
        fake_client.status_until.return_value = [status]
        socket_error = socket.error
        with patch('jujupy.utility.socket', wraps=socket) as wrapped_socket:
            # Must not convert socket.error into a Mock, because Mocks don't
            # descend from BaseException
            wrapped_socket.error = socket_error
            del wrapped_socket.inet_pton
            host = get_machine_dns_name(fake_client, '0')
        self.assertEqual(host, "2001:db8::3")
        fake_client.status_until.assert_called_once_with(timeout=600)
        self.assertEqual(self.log_stream.getvalue(), "")
