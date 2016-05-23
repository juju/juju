from argparse import Namespace
import os
from unittest import TestCase

from mock import (
    MagicMock,
    patch,
    )

from assess_heterogeneous_control import (
    assess_heterogeneous,
    check_series,
    get_clients,
    parse_args,
    test_control_heterogeneous,
    )
from jujupy import (
    _temp_env,
    )
from tests.test_deploy_stack import FakeBootstrapManager
from tests.test_jujupy import (
    FakeJujuClient,
    fake_juju_client_optional_jes,
    )

__metaclass__ = type


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
                                                       True, 'quxx')
        self.assertEqual(initial.env, other.env)
        self.assertEqual(initial.env, released.env)
        self.assertNotIn('tools-metadata-url', initial.env.config)
        self.assertEqual(initial.full_path, os.path.abspath('foo'))
        self.assertEqual(other.full_path, os.path.abspath('bar'))
        self.assertEqual(released.full_path, '/usr/bun/juju')

    def test_get_clients_different_env(self):
        boo = {
            ('foo', '--version'): '1.18.73',
            ('bar', '--version'): '1.18.74',
            ('juju', '--version'): '2.0',
            ('which', 'juju'): '/usr/bun/juju'
            }
        with _temp_env({'environments': {'baz': {}}}):
            with patch('subprocess.check_output', lambda x: boo[x]):
                with patch('jujupy.JujuData.load_yaml'):
                    initial, other, teardown = get_clients('foo', 'bar', 'baz',
                                                           True, 'quxx')
        self.assertIs(initial, teardown)

    def test_get_clients_no_agent(self):
        with _temp_env({'environments': {'baz': {}}}):
            with patch('subprocess.check_output', return_value='1.18.73'):
                initial, other, released = get_clients('foo', 'bar', 'baz',
                                                       True, None)
        self.assertTrue('tools-metadata-url' not in initial.env.config)


class TestAssessHeterogeneous(TestCase):

    @patch('assess_heterogeneous_control.BootstrapManager')
    @patch('assess_heterogeneous_control.test_control_heterogeneous',
           autospec=True)
    @patch('assess_heterogeneous_control.get_clients', autospec=True)
    def test_assess_heterogeneous(self, gc_mock, ch_mock, bm_mock):
        initial = MagicMock()
        gc_mock.return_value = (
            initial, 'other_client', 'released_client')
        assess_heterogeneous(
            'initial', 'other', 'base_env', 'environment_name', 'log_dir',
            False, False, 'agent_url', 'agent_stream', 'series')
        gc_mock.assert_called_once_with(
            'initial', 'other', 'base_env', False, 'agent_url')
        is_jes_enabled = initial.is_jes_enabled.return_value
        bm_mock.assert_called_once_with(
            'environment_name', initial, 'released_client',
            agent_stream='agent_stream', agent_url='agent_url',
            bootstrap_host=None, jes_enabled=is_jes_enabled, keep_env=False,
            log_dir='log_dir', machines=[], permanent=is_jes_enabled,
            region=None, series='series')
        ch_mock.assert_called_once_with(
            bm_mock.return_value, 'other_client', False)


class TestTestControlHeterogeneous(TestCase):

    def test_test_control_heterogeneous(self):
        client = fake_juju_client_optional_jes(jes_enabled=False)
        bs_manager = FakeBootstrapManager(client)
        # Prevent teardown
        bs_manager.tear_down_client = MagicMock()
        bs_manager.tear_down_client.destroy_environment.return_value = 0
        with patch.object(client, 'kill_controller'):
            test_control_heterogeneous(bs_manager, client, True)
        models = client._backend.controller_state.models
        model_state = models[client.model_name]
        self.assertEqual(model_state.exposed, {'sink2', 'dummy-sink'})
        self.assertEqual(model_state.machines, {'0', '1', '2'})
        self.assertEqual(client.env.juju_home, 'foo')

    def test_same_home(self):
        initial_client = FakeJujuClient(version='1.25')
        other_client = FakeJujuClient(
            env=initial_client.env,
            _backend=initial_client._backend.clone(version='2.0.0'))
        bs_manager = FakeBootstrapManager(initial_client)
        bs_manager.permanent = True
        test_control_heterogeneous(bs_manager, other_client, True)
        self.assertEqual(initial_client.env.juju_home,
                         other_client.env.juju_home)


class TestCheckSeries(TestCase):

    def test_check_series(self):
        client = FakeJujuClient()
        client.bootstrap()
        check_series(client)

    def test_check_series_xenial(self):
        client = MagicMock(spec=["get_juju_output"])
        client.get_juju_output.return_value = "Codename:	xenial"
        check_series(client, 1, 'xenial')

    def test_check_series_calls(self):
        client = MagicMock(spec=["get_juju_output"])
        with patch.object(client, 'get_juju_output',
                          return_value="Codename:	xenial") as gjo_mock:
            check_series(client, 2, 'xenial')
        gjo_mock.assert_called_once_with('ssh', 2, 'lsb_release', '-c')

    def test_check_series_exceptionl(self):
        client = FakeJujuClient()
        client.bootstrap()
        with self.assertRaisesRegexp(
                AssertionError, 'Series is angsty, not xenial'):
            check_series(client, '0', 'xenial')
