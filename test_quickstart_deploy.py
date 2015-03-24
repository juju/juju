from mock import patch
from unittest import TestCase

from jujupy import (
    EnvJujuClient,
    SimpleEnvironment,
    )
from quickstart_deploy import QuickstartTest


class FakeEnvJujuClient(EnvJujuClient):

    def __init__(self, name='steve'):
        super(FakeEnvJujuClient, self).__init__(
            SimpleEnvironment(name, {'type': 'fake'}), '1.2', '/jbin/juju')

    def quickstart(self, *args, **kwargs):
        # Suppress stdout for juju commands.
        with patch('sys.stdout'):
            return super(FakeEnvJujuClient, self).quickstart(*args, **kwargs)


class TestQuickstartTest(TestCase):

    def test_from_args(self):
        side_effect = lambda x, y=None, debug=False: (x, y)
        with patch('jujupy.EnvJujuClient.by_version', side_effect=side_effect):
            with patch('jujupy.SimpleEnvironment.from_config',
                       side_effect=lambda x: SimpleEnvironment(x, {})):
                quickstart = QuickstartTest.from_args(
                    'base_env', 'temp_env_name', '/foo/bin/juju', '/tmp/tmp',
                    '/tmp/bundle.yaml', 2
                )
        # self.assertEqual(
        #     quickstart.client, (
        #         SimpleEnvironment('temp_env_name', {
        #             'agent_url': 'http://agent_url.com',
        #             'series': 'precise'}),
        #         '/foo/bin/juju')
        #     )
        self.assertIs(type(quickstart), QuickstartTest)
        self.assertEqual(quickstart.client[0].environment, 'temp_env_name')
        self.assertIs(quickstart.client[1], '/foo/bin/juju')
        self.assertEqual(quickstart.bundle_path, '/tmp/bundle.yaml')
        self.assertEqual(quickstart.log_dir, '/tmp/tmp')
        self.assertEqual(quickstart.service_count, 2)

    def test_from_args_agent_url(self):
        side_effect = lambda x, y=None, debug=False: (x, y)
        with patch('jujupy.EnvJujuClient.by_version', side_effect=side_effect):
            with patch('jujupy.SimpleEnvironment.from_config',
                       side_effect=lambda x: SimpleEnvironment(x, {})):
                quickstart = QuickstartTest.from_args(
                    'base_env', 'temp_env_name', '/foo/bin/juju', '/tmp/tmp',
                    '/tmp/bundle.yaml', 2, agent_url='http://agent_url.com'
                )
        self.assertEqual(quickstart.client[0].config['agent_url'],
                         'http://agent_url.com')

    def test_from_args_series(self):
        side_effect = lambda x, y=None, debug=False: (x, y)
        with patch('jujupy.EnvJujuClient.by_version', side_effect=side_effect):
            with patch('jujupy.SimpleEnvironment.from_config',
                       side_effect=lambda x: SimpleEnvironment(x, {})):
                quickstart = QuickstartTest.from_args(
                    'base_env', 'temp_env_name', '/foo/bin/juju', '/tmp/tmp',
                    '/tmp/bundle.yaml', 2, series='precise'
                )
        self.assertEqual(quickstart.client[0].config['series'],
                         'precise')

    def test_from_args_debug(self):
        with patch('jujupy.EnvJujuClient.get_version',
                   side_effect=lambda x, juju_path=None: ''):
            with patch('jujupy.SimpleEnvironment.from_config',
                       side_effect=lambda x: SimpleEnvironment(x, {})):
                quickstart = QuickstartTest.from_args(
                    'base_env', 'temp_env_name', '/foo/bin/juju', '/tmp/tmp',
                    '/tmp/bundle.yaml', 2, debug_flag=True
                )
        self.assertEqual(quickstart.client.debug, True)

    def test_iter_steps_bundle(self):
        client = FakeEnvJujuClient()
        quickstart = QuickstartTest(client, '/tmp/bundle.yaml', '/tmp/logs', 2)
        steps = quickstart.iter_steps()
        with patch.object(client, 'quickstart') as qs_mock:
            # Test first yield
            step = steps.next()
        qs_mock.assert_called_once_with('/tmp/bundle.yaml')
        expected = {'juju-quickstart': 'Returned from quickstart'}
        self.assertEqual(expected, step)

    def test_iter_steps_dns_name(self):
        client = FakeEnvJujuClient()
        quickstart = QuickstartTest(client, '/tmp/bundle.yaml', '/tmp/logs', 2)
        steps = quickstart.iter_steps()
        with patch.object(client, 'quickstart'):
            # skip first yield
            steps.next()
        with patch('deploy_stack.get_machine_dns_name',
                   return_value='mocked_name') as ds_mock:
            # Test second yield
            # Test hangs here, is get_machine_dns_name really getting called?
            step = steps.next()
        ds_mock.assert_called_once_with(client, 0)
        self.assertEqual('mocked_name', step['bootstrap_host'])
