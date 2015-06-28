import unittest
from mock import (
    patch,
    call,
    )
from jes import (
    env_token,
    jes_setup,
    deploy_dummy_stack_in_environ,
    check_updated_token,
    check_services,
)
from jujupy import (
    EnvJujuClient25,
    JUJU_DEV_FEATURE_FLAGS,
    SimpleEnvironment,
)


class setupArgs(object):
    env = 'baz'
    verbose = True
    job_name = 'jesjob'
    bootstrap_host = 'localhost'
    debug = True
    machine = ['0']
    series = 'trusty'
    agent_stream = 'devel'
    agent_url = 'some_url'
    logs = 'log/dir'
    keep_env = True
    upload_tools = True
    juju_home = 'path/to/juju/home'


class TestJES(unittest.TestCase):

    client_class = EnvJujuClient25

    @patch('jes.get_random_string')
    @patch('jes.SimpleEnvironment.from_config')
    def mock_client(self, from_config_func, get_random_string_func):
        from_config_func.return_value = SimpleEnvironment(setupArgs.env, {})
        get_random_string_func.return_value = 'fakeran'
        client = self.client_class(
            SimpleEnvironment.from_config(setupArgs.env),
            '1.25-foobar', 'path')
        client._shell_environ(dev_flags=['jes'])
        return client

    @patch('jes.get_random_string')
    def test_env_token(self, get_random_string_func):
        get_random_string_func.return_value = 'fakeran'
        self.assertEqual(env_token('env1'), 'env1fakeran')

    def test_set_jes_flag(self):
        client = self.mock_client()
        env = client._shell_environ(dev_flags=['jes'])
        self.assertTrue('jes' in env[JUJU_DEV_FEATURE_FLAGS].split(","))

    @patch('jes.EnvJujuClient.juju')
    @patch('jes.deploy_dummy_stack')
    @patch('jes.get_random_string')
    def test_deploy_dummy_stack_in_environ(
            self, get_random_string_func,
            deploy_dummy_stack_func,
            juju_func):
        get_random_string_func.return_value = 'fakerandom'
        client = self.mock_client()

        deploy_dummy_stack_in_environ(client, 'trusty', 'baz')

        # assert helper funcs were called with correct args.
        args, kwargs = deploy_dummy_stack_func.call_args
        self.assertEqual(args, (client, 'trusty', 'bazfakerandom'))
        self.assertEqual(kwargs, {})

        env = client._shell_environ(dev_flags=['jes'])
        self.assertEqual(juju_func.call_args_list, [
                call(
                    'system',
                    ('create-environment', 'baz'),
                    include_e=False,
                    extra_env=env,
                ),
                call(
                    'environment',
                    ('set', 'default-series=trusty', '-e', 'baz'),
                    include_e=False,
                    extra_env=env,
                ),
            ]
        )

    @patch('jes.sleep', return_value=None)
    @patch('jes.check_token')
    def test_check_updated_token(self, check_token_func, patched_sleep):
        client = self.mock_client()

        def err(client, token):
            raise ValueError()

        check_token_func.side_effect = err

        check_updated_token(client, 'token', 5)

        self.assertEqual(check_token_func.call_count, 2)

        args, kwargs = check_token_func.call_args
        self.assertEqual(args, (client, 'token'))
        self.assertEqual(kwargs, {})

    @patch('jes.get_random_string')
    @patch('jes.EnvJujuClient.juju')
    @patch('jes.check_updated_token')
    def test_check_services(
            self,
            check_updated_token_func,
            juju_func,
            get_random_string_func):
        get_random_string_func.return_value = 'fakeran'

        client = self.mock_client()
        check_services(client, 'token')

        self.assertEqual(
            juju_func.call_args,
            call('set', ('dummy-source', 'token=tokenfakeran')),
        )

        args, kwargs = check_updated_token_func.call_args
        self.assertEqual(args, (client, 'tokenfakeran', 30))
        self.assertEqual(kwargs, {})

    @patch('jes.EnvJujuClient.get_full_path')
    @patch('jes.prepare_environment')
    @patch('jes.boot_context')
    @patch('jes.configure_logging')
    @patch('jes.SimpleEnvironment.from_config')
    @patch('jes.EnvJujuClient.by_version')
    def test_jes_setup(
            self,
            by_version_func,
            from_config_func,
            configure_logging_func,
            boot_context_func,
            prepare_environment_func,
            get_full_path_func):
        # patch helper funcs
        expected_client = self.mock_client()
        expected_env = SimpleEnvironment(setupArgs.env, {})
        from_config_func.return_value = expected_env
        by_version_func.return_value = expected_client
        configure_logging_func.return_value = None
        get_full_path_func.return_value = '/path/to/juju'

        # setup jes
        client, charm_previx, base_env = jes_setup(setupArgs)

        # assert that jes_setup returns expected values
        self.assertIs(client, expected_client)
        self.assertEqual(charm_previx, 'local:trusty/')
        self.assertEqual(base_env, 'baz')

        # assert that helper funcs were called with expected args.
        args, kwargs = by_version_func.call_args
        self.assertEqual(args, (expected_env, '/path/to/juju', True))
        self.assertEqual(kwargs, {})

        args, kwargs = configure_logging_func.call_args
        self.assertEqual(args, (10,))
        self.assertEqual(kwargs, {})

        args, kwargs = boot_context_func.call_args
        self.assertEqual(args, (
            'jesjob',
            expected_client,
            'localhost',
            ['0'],
            'trusty',
            'devel',
            'some_url',
            'log/dir',
            True,
            True,
            'path/to/juju/home',
        ))

        self.assertIsNotNone(kwargs['extra_env'])

        args, kwargs = prepare_environment_func.call_args
        self.assertEqual(args, (expected_client,))
        self.assertEqual(
            kwargs,
            {'already_bootstrapped': True, 'machines': ['0']},
        )
