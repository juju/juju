from argparse import Namespace
from mock import (
    patch,
    )
import os

from assess_jes_deploy import (
    check_services,
    env_token,
    hosted_environment,
    jes_setup,
)
from jujupy import (
    EnvJujuClient25,
    JUJU_DEV_FEATURE_FLAGS,
    SimpleEnvironment,
)
import tests
from tests.test_jujupy import FakeJujuClient


class TestJES(tests.FakeHomeTestCase):

    client_class = EnvJujuClient25

    @patch('assess_jes_deploy.get_random_string', autospec=True)
    @patch('assess_jes_deploy.SimpleEnvironment.from_config')
    def mock_client(self, from_config_func, get_random_string_func):
        from_config_func.return_value = SimpleEnvironment('baz', {})
        get_random_string_func.return_value = 'fakeran'
        client = self.client_class(
            SimpleEnvironment.from_config('baz'),
            '1.25-foobar', 'path')
        client.enable_feature('jes')
        return client

    @patch('assess_jes_deploy.get_random_string', autospec=True)
    def test_env_token(self, get_random_string_func):
        get_random_string_func.return_value = 'fakeran'
        self.assertEqual(env_token('env1'), 'env1fakeran')

    def test_set_jes_flag(self):
        client = self.mock_client()
        env = client._shell_environ()
        self.assertTrue('jes' in env[JUJU_DEV_FEATURE_FLAGS].split(","))

    @patch('assess_jes_deploy.print_now', autospec=True)
    @patch('assess_jes_deploy.get_random_string', autospec=True)
    @patch('assess_jes_deploy.EnvJujuClient.juju', autospec=True)
    @patch('assess_jes_deploy.check_token', autospec=True)
    def test_check_services(
            self,
            check_token_func,
            juju_func,
            get_random_string_func,
            print_now_func):
        get_random_string_func.return_value = 'fakeran'

        client = self.mock_client()
        client.env.environment = 'token'
        check_services(client)

        juju_func.assert_called_once_with(
            client, 'set', ('dummy-source', 'token=tokenfakeran'))
        check_token_func.assert_called_once_with(client, 'tokenfakeran')
        print_now_func.assert_called_once_with('checking services in token')

    @patch('assess_jes_deploy.EnvJujuClient.get_full_path')
    @patch('assess_jes_deploy.EnvJujuClient.add_ssh_machines', autospec=True)
    @patch('assess_jes_deploy.boot_context', autospec=True)
    @patch('assess_jes_deploy.configure_logging', autospec=True)
    @patch('assess_jes_deploy.SimpleEnvironment.from_config')
    @patch('assess_jes_deploy.EnvJujuClient.by_version')
    def test_jes_setup(
            self,
            by_version_func,
            from_config_func,
            configure_logging_func,
            boot_context_func,
            add_ssh_machines_func,
            get_full_path_func):
        # patch helper funcs
        expected_client = self.mock_client()
        expected_env = SimpleEnvironment('baz', {})
        from_config_func.return_value = expected_env
        by_version_func.return_value = expected_client
        configure_logging_func.return_value = None
        get_full_path_func.return_value = '/path/to/juju'

        setup_args = Namespace(
            env='baz', verbose=True, temp_env_name='jesjob',
            bootstrap_host='localhost', debug=True, machine=['0'],
            series='trusty', agent_stream='devel', agent_url='some_url',
            logs='log/dir', keep_env=True, juju_home='/path/to/juju/home',
            juju_bin='/path/to/bin/juju', region='region-foo')

        # setup jes with a client that has default jes.
        with patch.object(expected_client, 'enable_jes'):
            with patch.object(expected_client, 'is_jes_enabled',
                              return_value=True):
                with jes_setup(setup_args) as (client, charm_series, base_env):
                    self.assertEqual(1, client.is_jes_enabled.call_count)
                    self.assertEqual(0, client.enable_jes.call_count)

        # assert that jes_setup provides expected values
        self.assertIs(client, expected_client)
        self.assertEqual(charm_series, 'trusty')
        self.assertEqual(base_env, 'baz')

        # assert that helper funcs were called with expected args.
        by_version_func.assert_called_once_with(
            expected_env, '/path/to/bin/juju', True)

        configure_logging_func.assert_called_once_with(True)
        boot_context_func.assert_called_once_with(
            'jesjob', expected_client, 'localhost', ['0'], 'trusty',
            'some_url', 'devel', 'log/dir', True, upload_tools=False,
            region='region-foo')

        # Setup jes with a client that requires a call to enable_jes.
        with patch.object(expected_client, 'enable_jes'):
            with patch.object(expected_client, 'is_jes_enabled',
                              return_value=False):
                with jes_setup(setup_args) as (client, charm_previx, base_env):
                    self.assertEqual(1, client.is_jes_enabled.call_count)
                    self.assertEqual(1, client.enable_jes.call_count)


class TestHostedEnvironment(tests.FakeHomeTestCase):

    def test_hosted_environment(self):
        hosting_client = FakeJujuClient(jes_enabled=True)
        log_dir = os.path.join(self.home_dir, 'logs')
        os.mkdir(log_dir)
        with hosted_environment(hosting_client, log_dir, 'bar') as client:
            models = client._backend.controller_state.models
            model_state = models[client.model_name]
            self.assertEqual({'name-bar': model_state},
                             hosting_client._backend.controller_state.models)
            self.assertEqual('created', model_state.state)
        self.assertEqual('model-destroyed', model_state.state)
        self.assertTrue(os.path.isdir(os.path.join(log_dir, 'bar')))

    def test_gathers_machine_logs(self):
        hosting_client = FakeJujuClient(jes_enabled=True)
        log_dir = os.path.join(self.home_dir, 'logs')
        os.mkdir(log_dir)
        with patch("deploy_stack.copy_remote_logs", autospec=True) as mock_crl:
            with hosted_environment(hosting_client, log_dir, 'bar') as client:
                client.juju("deploy", ["cs:a-service"])
                status = client.get_status()
                unit_machine = status.get_unit("a-service/0")["machine"]
                addr = status.status["machines"][unit_machine]["dns-name"]
        hosted_dir = os.path.join(log_dir, "bar")
        machine_dir = os.path.join(hosted_dir, "machine-0")
        self.assertTrue(os.path.isdir(hosted_dir))
        self.assertTrue(os.path.isdir(machine_dir))
        self.assertEqual(mock_crl.call_count, 1)
        self.assertEqual(mock_crl.call_args[0][0].address, addr)
        self.assertEqual(mock_crl.call_args[0][1], machine_dir)
