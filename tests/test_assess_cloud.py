from argparse import Namespace
from contextlib import contextmanager
import logging

from mock import (
    call,
    patch,
    )
from assess_cloud import (
    assess_cloud_combined,
    assess_cloud_kill_controller,
    assess_cloud_provisioning,
    client_from_args,
    parse_args,
    )
from deploy_stack import BootstrapManager
from jujupy import (
    ModelClient,
    FakeBackend,
    fake_juju_client,
    Juju2Backend,
    )
from jujupy.version_client import ModelClient2_0
from tests import (
    FakeHomeTestCase,
    observable_temp_file,
    TestCase,
    )
from utility import (
    temp_dir,
    temp_yaml_file,
    )


def backend_call(client, cmd, args, model=None, check=True, timeout=None,
                 extra_env=None):
    """Return the mock.call for this command."""
    return call(cmd, args, client.used_feature_flags,
                client.env.juju_home, model, check, timeout, extra_env)


@contextmanager
def mocked_bs_manager(juju_home):
    client = fake_juju_client()
    client.env.juju_home = juju_home
    bs_manager = BootstrapManager(
        'foo', client, client, bootstrap_host=None, machines=[],
        series=None, agent_url=None, agent_stream=None, region=None,
        log_dir=juju_home, keep_env=False, permanent=True,
        jes_enabled=True)
    backend = client._backend
    with patch.object(backend, 'juju', wraps=backend.juju):
        with observable_temp_file() as temp_file:
            yield bs_manager, temp_file


class TestAssessCloudCombined(FakeHomeTestCase):

    def test_assess_cloud_combined(self):
        with mocked_bs_manager(self.juju_home) as (bs_manager, config_file):
            assess_cloud_combined(bs_manager)
            client = bs_manager.client
            juju_wrapper = client._backend.juju
        juju_wrapper.assert_has_calls([
            backend_call(
                client, 'bootstrap', (
                    '--constraints', 'mem=2G', 'foo/bar', 'foo', '--config',
                    config_file.name, '--default-model', 'foo',
                    '--agent-version', client.version)),
            backend_call(client, 'deploy', 'ubuntu', 'foo:foo'),
            backend_call(client, 'remove-unit', 'ubuntu/0', 'foo:foo'),
            backend_call(
                client, 'destroy-controller',
                ('foo', '-y', '--destroy-all-models'), timeout=600),
            ], any_order=True)


class TestAssessCloudKillController(FakeHomeTestCase):

    def test_assess_cloud_kill_controller(self):
        with mocked_bs_manager(self.juju_home) as (bs_manager, config_file):
            assess_cloud_kill_controller(bs_manager)
            client = bs_manager.client
            juju_wrapper = client._backend.juju
        juju_wrapper.assert_has_calls([
            backend_call(
                client, 'bootstrap', (
                    '--constraints', 'mem=2G', 'foo/bar', 'foo', '--config',
                    config_file.name, '--default-model', 'foo',
                    '--agent-version', client.version)),
            backend_call(
                client, 'kill-controller', ('foo', '-y'), timeout=600,
                check=True),
            ], any_order=True)


class TestAssessCloudProvisioning(FakeHomeTestCase):

    def test_assess_cloud_provisioning(self):
        with mocked_bs_manager(self.juju_home) as (bs_manager, config_file):
            assess_cloud_provisioning(bs_manager)
            client = bs_manager.client
            juju_wrapper = client._backend.juju
        juju_wrapper.assert_has_calls([
            backend_call(
                client, 'bootstrap', (
                    '--constraints', 'mem=2G', 'foo/bar', 'foo', '--config',
                    config_file.name, '--default-model', 'foo',
                    '--agent-version', client.version)),
            backend_call(client, 'add-machine', ('--series', 'win2012r2'),
                         'foo:foo'),
            backend_call(client, 'add-machine', ('--series', 'trusty'),
                         'foo:foo'),
            backend_call(client, 'remove-machine', ('0',), 'foo:foo'),
            backend_call(client, 'remove-machine', ('1',), 'foo:foo'),
            backend_call(
                client, 'destroy-controller',
                ('foo', '-y', '--destroy-all-models'), timeout=600),
            ], any_order=True)


class TestClientFromArgs(FakeHomeTestCase):

    def test_client_from_args(self):
        with temp_yaml_file({}) as clouds_file:
            args = Namespace(
                juju_bin='/usr/bin/juju', clouds_file=clouds_file,
                cloud='mycloud', region=None, debug=False, deadline=None,
                config=None)
            with patch.object(ModelClient.config_class,
                              'from_cloud_region') as fcr_mock:
                with patch.object(ModelClient, 'get_version',
                                  return_value='2.0.x'):
                    client = client_from_args(args)
        fcr_mock.assert_called_once_with('mycloud', None, {}, {},
                                         self.juju_home)
        self.assertIs(type(client), ModelClient2_0)
        self.assertIs(type(client._backend), Juju2Backend)
        self.assertEqual(client.version, '2.0.x')
        self.assertIs(client.env, fcr_mock.return_value)

    def test_client_from_args_fake(self):
        with temp_yaml_file({}) as clouds_file:
            args = Namespace(
                juju_bin='FAKE', clouds_file=clouds_file, cloud='mycloud',
                region=None, debug=False, deadline=None, config=None)
            with patch.object(ModelClient.config_class,
                              'from_cloud_region') as fcr_mock:
                client = client_from_args(args)
        fcr_mock.assert_called_once_with('mycloud', None, {}, {},
                                         self.juju_home)
        self.assertIs(type(client), ModelClient)
        self.assertIs(type(client._backend), FakeBackend)
        self.assertEqual(client.version, '2.0.0')
        self.assertIs(client.env, fcr_mock.return_value)

    def test_config(self):
        with temp_yaml_file({}) as clouds_file:
            with temp_yaml_file({'foo': 'bar'}) as config_file:
                args = Namespace(
                    juju_bin='/usr/bin/juju', clouds_file=clouds_file,
                    cloud='mycloud', region=None, debug=False, deadline=None,
                    config=config_file)
                with patch.object(ModelClient.config_class,
                                  'from_cloud_region') as fcr_mock:
                    with patch.object(ModelClient, 'get_version',
                                      return_value='2.0.x'):
                        client_from_args(args)
        fcr_mock.assert_called_once_with('mycloud', None, {'foo': 'bar'}, {},
                                         self.juju_home)


class TestParseArgs(TestCase):

    def test_parse_args_combined(self):
        with temp_dir() as log_dir:
            args = parse_args(['combined', 'foo', 'bar', 'baz', log_dir,
                               'qux'])
        self.assertEqual(args, Namespace(
            agent_stream=None, agent_url=None, bootstrap_host=None,
            cloud='bar', clouds_file='foo', deadline=None, debug=False,
            juju_bin='baz', keep_env=False, logs=log_dir, machine=[],
            region=None, series=None, temp_env_name='qux', upload_tools=False,
            verbose=logging.INFO, test='combined', config=None,
            ))

    def test_parse_args_combined_config(self):
        with temp_dir() as log_dir:
            args = parse_args(['combined', 'foo', 'bar', 'baz', log_dir,
                               'qux', '--config', 'quxx'])
        self.assertEqual('quxx', args.config)

    def test_parse_args_kill_controller(self):
        with temp_dir() as log_dir:
            args = parse_args(['kill-controller', 'foo', 'bar', 'baz', log_dir,
                               'qux'])
        self.assertEqual(args, Namespace(
            agent_stream=None, agent_url=None, bootstrap_host=None,
            cloud='bar', clouds_file='foo', deadline=None, debug=False,
            juju_bin='baz', keep_env=False, logs=log_dir, machine=[],
            region=None, series=None, temp_env_name='qux', upload_tools=False,
            verbose=logging.INFO, test='kill-controller', config=None,
            ))

    def test_parse_args_kill_controller_config(self):
        with temp_dir() as log_dir:
            args = parse_args(['kill-controller', 'foo', 'bar', 'baz', log_dir,
                               'qux', '--config', 'quxx'])
        self.assertEqual('quxx', args.config)

    def test_parse_args_provisioning(self):
        with temp_dir() as log_dir:
            args = parse_args(['provisioning', 'foo', 'bar', 'baz', log_dir,
                               'qux'])
        self.assertEqual(args, Namespace(
            agent_stream=None, agent_url=None, bootstrap_host=None,
            cloud='bar', clouds_file='foo', deadline=None, debug=False,
            juju_bin='baz', keep_env=False, logs=log_dir, machine=[],
            region=None, series=None, temp_env_name='qux', upload_tools=False,
            verbose=logging.INFO, test='provisioning', config=None,
            ))

    def test_parse_args_provisioning_config(self):
        with temp_dir() as log_dir:
            args = parse_args(['provisioning', 'foo', 'bar', 'baz', log_dir,
                               'qux', '--config', 'quxx'])
        self.assertEqual('quxx', args.config)
