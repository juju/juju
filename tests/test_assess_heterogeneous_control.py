from argparse import Namespace
import os
from unittest import TestCase

from mock import patch

from assess_heterogeneous_control import (
    assess_heterogeneous,
    dumping_env,
    get_clients,
    parse_args,
    )
from jujupy import (
    EnvJujuClient,
    SimpleEnvironment,
    _temp_env,
    )
from utility import (
    temp_dir,
)


__metaclass__ = type


class TestDumping_env(TestCase):

    def test_dumping_env_exception(self):
        client = EnvJujuClient(SimpleEnvironment('env'), '5', '4')
        with temp_dir() as log_dir:
            with patch.object(client, 'destroy_environment') as de_mock:
                with patch('assess_heterogeneous_control.dump_env_logs',
                           autospec=True) as logs_mock:
                    with self.assertRaises(ValueError):
                        with dumping_env(client, 'a-hostname', log_dir):
                            raise ValueError
        logs_mock.assert_called_once_with(client, 'a-hostname', log_dir)
        de_mock.assert_called_once_with()

    def test_dumping_env_success(self):
        client = EnvJujuClient(SimpleEnvironment('env'), '5', '4')
        with temp_dir() as log_dir:
            with patch.object(client, 'destroy_environment') as de_mock:
                with patch('assess_heterogeneous_control.dump_env_logs',
                           autospec=True) as logs_mock:
                    with dumping_env(client, 'a-hostname', log_dir):
                        pass
        logs_mock.assert_called_once_with(client, 'a-hostname', log_dir)
        self.assertEqual(de_mock.call_count, 0)


class TestParseArgs(TestCase):

    def test_parse_args(self):
        args = parse_args(['a', 'b', 'c', 'd', 'e'])
        self.assertEqual(args, Namespace(
            initial='a', other='b', base_environment='c',
            environment_name='d', log_dir='e', debug=False,
            upload_tools=False, agent_url=None, agent_stream=None, series=None,
            user=os.environ.get('JENKINS_USER'),
            password=os.environ.get('JENKINS_PASSWORD')))

    def test_parse_args_agent_url(self):
        args = parse_args(['a', 'b', 'c', 'd', 'e', '--agent-url', 'foo',
                           '--user', 'my name', '--password', 'fake pass'])
        self.assertEqual(args.agent_url, 'foo')
        self.assertEqual(args.user, 'my name')
        self.assertEqual(args.password, 'fake pass')

    def test_parse_args_agent_stream(self):
        args = parse_args(['a', 'b', 'c', 'd', 'e',
                           '--agent-stream', 'proposed',
                           '--user', 'my name', '--password', 'fake pass'])
        self.assertEqual(args.agent_stream, 'proposed')
        self.assertEqual(args.user, 'my name')
        self.assertEqual(args.password, 'fake pass')

    def test_parse_args_series(self):
        args = parse_args(['a', 'b', 'c', 'd', 'e', '--series', 'trusty',
                           '--user', 'my name', '--password', 'fake pass'])
        self.assertEqual(args.series, 'trusty')
        self.assertEqual(args.user, 'my name')
        self.assertEqual(args.password, 'fake pass')


class TestGetClients(TestCase):

    def test_get_clients(self):
        boo = {
            ('foo', '--version'): '1.18.73',
            ('bar', '--version'): '1.18.74',
            ('juju', '--version'): '1.18.75',
            ('which', 'juju'): '/usr/bun/juju'
            }
        with _temp_env({'environments': {'baz': {}}}):
            with patch('subprocess.check_output', lambda x: boo[x]):
                initial, other, released = get_clients('foo', 'bar', 'baz',
                                                       'qux', True, 'quxx',
                                                       'nif', 'glo')
        self.assertEqual(initial.env, other.env)
        self.assertEqual(initial.env, released.env)
        self.assertEqual(initial.env.config['tools-metadata-url'], 'quxx')
        self.assertEqual(initial.env.config['agent-stream'], 'nif')
        self.assertEqual(initial.env.config['default-series'], 'glo')
        self.assertEqual(initial.full_path, os.path.abspath('foo'))
        self.assertEqual(other.full_path, os.path.abspath('bar'))
        self.assertEqual(released.full_path, '/usr/bun/juju')

    def test_get_clients_no_agent(self):
        with _temp_env({'environments': {'baz': {}}}):
            with patch('subprocess.check_output', return_value='1.18.73'):
                initial, other, released = get_clients('foo', 'bar', 'baz',
                                                       'qux', True, None,
                                                       None, None)
        self.assertTrue('tools-metadata-url' not in initial.env.config)
        self.assertTrue('agent-stream' not in initial.env.config)
        self.assertTrue('default-series' not in initial.env.config)


class TestAssessHeterogeneous(TestCase):

    @patch('assess_heterogeneous_control.test_control_heterogeneous',
           autospec=True)
    @patch('assess_heterogeneous_control.get_clients', autospec=True)
    def test_assess_heterogeneous(self, ah_mock, ch_mock):
        ah_mock.return_value = (
            'initial_client', 'other_client', 'released_client')
        assess_heterogeneous(
            'initial', 'other', 'base_env', 'environment_name', 'log_dir',
            False, False, 'agent_url', 'agent_stream', 'series')
        ah_mock.assert_called_once_with(
            'initial', 'other', 'base_env', 'environment_name', False,
            'agent_url', 'agent_stream', 'series')
        ch_mock.assert_called_once_with(
            'initial_client', 'other_client', 'released_client',
            'log_dir', False)
