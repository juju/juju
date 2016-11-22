from argparse import Namespace

from mock import (
    call,
    patch,
    )
from assess_cloud import (
    assess_cloud_combined,
    client_from_args,
    )
from deploy_stack import BootstrapManager
from fakejuju import (
    FakeBackend,
    fake_juju_client,
    )
from jujupy import (
    EnvJujuClient,
    Juju2Backend,
    temp_yaml_file,
    )
from tests import (
    FakeHomeTestCase,
    observable_temp_file,
    )


class TestAssessCloudCombined(FakeHomeTestCase):

    def backend_call(self, client, cmd, args, model=None, check=True,
                     timeout=None, extra_env=None):
        return call(cmd, args, client.used_feature_flags,
                    client.env.juju_home, model, check, timeout, extra_env)

    def test_assess_cloud_combined(self):
        client = fake_juju_client()
        client.env.juju_home = self.juju_home
        bs_manager = BootstrapManager(
            'foo', client, client, bootstrap_host=None, machines=[],
            series=None, agent_url=None, agent_stream=None, region=None,
            log_dir=self.juju_home, keep_env=False, permanent=True,
            jes_enabled=True)
        backend = client._backend
        with patch.object(backend, 'juju', wraps=backend.juju):
            juju_wrapper = backend.juju
            with observable_temp_file() as temp_file:
                assess_cloud_combined(bs_manager)
        juju_wrapper.assert_has_calls([
            self.backend_call(
                client, 'bootstrap', (
                    '--constraints', 'mem=2G', 'foo/bar', 'foo', '--config',
                    temp_file.name, '--default-model', 'foo',
                    '--agent-version', client.version)),
            self.backend_call(client, 'deploy', 'ubuntu', 'foo:foo'),
            self.backend_call(client, 'remove-unit', 'ubuntu/0', 'foo:foo'),
            self.backend_call(
                client, 'destroy-controller',
                ('foo', '-y', '--destroy-all-models'), timeout=600),
            ], any_order=True)


class TestClientFromArgs(FakeHomeTestCase):

    def test_client_from_args(self):
        with temp_yaml_file({}) as example_clouds:
            args = Namespace(
                juju_bin='/usr/bin/juju', example_clouds=example_clouds,
                cloud='mycloud', region=None, debug=False, deadline=None)
            with patch.object(EnvJujuClient.config_class,
                              'from_cloud_region') as fcr_mock:
                with patch.object(EnvJujuClient, 'get_version',
                                  return_value='2.0.x'):
                    client = client_from_args(args)
        fcr_mock.assert_called_once_with('mycloud', None, {}, {},
                                         self.juju_home)
        self.assertIs(type(client), EnvJujuClient)
        self.assertIs(type(client._backend), Juju2Backend)
        self.assertEqual(client.version, '2.0.x')
        self.assertIs(client.env, fcr_mock.return_value)

    def test_client_from_args_fake(self):
        with temp_yaml_file({}) as example_clouds:
            args = Namespace(
                juju_bin='FAKE', example_clouds=example_clouds,
                cloud='mycloud', region=None, debug=False, deadline=None)
            with patch.object(EnvJujuClient.config_class,
                              'from_cloud_region') as fcr_mock:
                client = client_from_args(args)
        fcr_mock.assert_called_once_with('mycloud', None, {}, {},
                                         self.juju_home)
        self.assertIs(type(client), EnvJujuClient)
        self.assertIs(type(client._backend), FakeBackend)
        self.assertEqual(client.version, '2.0.0')
        self.assertIs(client.env, fcr_mock.return_value)
