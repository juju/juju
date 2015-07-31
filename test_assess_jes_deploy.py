from argparse import Namespace
import unittest
from mock import (
    patch,
    )
from assess_jes_deploy import (
    env_token,
    jes_setup,
    check_services,
)
from jujupy import (
    EnvJujuClient25,
    JUJU_DEV_FEATURE_FLAGS,
    SimpleEnvironment,
)


class TestJES(unittest.TestCase):

    client_class = EnvJujuClient25

    @patch('assess_jes_deploy.get_random_string', autospec=True)
    @patch('assess_jes_deploy.SimpleEnvironment.from_config')
    def mock_client(self, from_config_func, get_random_string_func):
        from_config_func.return_value = SimpleEnvironment('baz', {})
        get_random_string_func.return_value = 'fakeran'
        client = self.client_class(
            SimpleEnvironment.from_config('baz'),
            '1.25-foobar', 'path')
        client._use_jes = True
        return client

    @patch('assess_jes_deploy.get_random_string', autospec=True)
    def test_env_token(self, get_random_string_func):
        get_random_string_func.return_value = 'fakeran'
        self.assertEqual(env_token('env1'), 'env1fakeran')

    def test_set_jes_flag(self):
        client = self.mock_client()
        env = client._shell_environ()
        self.assertTrue('jes' in env[JUJU_DEV_FEATURE_FLAGS].split(","))

    @patch('assess_jes_deploy.get_random_string', autospec=True)
    @patch('assess_jes_deploy.EnvJujuClient.juju', autospec=True)
    @patch('assess_jes_deploy.check_token', autospec=True)
    def test_check_services(
            self,
            check_token_func,
            juju_func,
            get_random_string_func):
        get_random_string_func.return_value = 'fakeran'

        client = self.mock_client()
        client.env.environment = 'token'
        check_services(client)

        juju_func.assert_called_once_with(
            client, 'set', ('dummy-source', 'token=tokenfakeran'))
        check_token_func.assert_called_once_with(
            client, 'tokenfakeran')

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
            logs='log/dir', keep_env=True, upload_tools=True,
            juju_home='path/to/juju/home', juju_bin='/path/to/bin/juju')

        # setup jes
        with jes_setup(setup_args) as (client, charm_previx, base_env):
            pass

        # assert that jes_setup provides expected values
        self.assertIs(client, expected_client)
        self.assertEqual(charm_previx, 'local:trusty/')
        self.assertEqual(base_env, 'baz')

        # assert that helper funcs were called with expected args.
        by_version_func.assert_called_once_with(
            expected_env, '/path/to/bin/juju', True)

        configure_logging_func.assert_called_once_with(True)
        boot_context_func.assert_called_once_with(
            'jesjob', expected_client, 'localhost', ['0'], 'trusty',
            'some_url', 'devel', 'log/dir', True, True, permanent=True)

        add_ssh_machines_func.assert_called_once_with(client, ['0'])
