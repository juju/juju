from argparse import Namespace
from unittest import TestCase

from mock import (
    patch
    )
import yaml

from jujupy import (
    EnvJujuClient,
    SimpleEnvironment,
    )
from run_chaos_monkey import (
    get_args,
    MonkeyRunner,
    )
from test_jujupy import (
    assert_juju_call,
    )


def fake_EnvJujuClient_by_version(env, path=None, debug=None):
    return EnvJujuClient(env=env, version='1.2.3.4', full_path=path)


def fake_SimpleEnvironment_from_config(name):
    return SimpleEnvironment(name, {})


class TestRunChaosMonkey(TestCase):

    def test_get_args(self):
        args = get_args(['foo', 'bar', 'baz'])
        self.assertItemsEqual(['env', 'service', 'health_checker'],
                              [a for a in dir(args) if not a.startswith('_')])
        self.assertEqual(args.env, 'foo')
        self.assertEqual(args.service, 'bar')
        self.assertEqual(args.health_checker, 'baz')

    def test_from_config(self):
        with patch('jujupy.EnvJujuClient.by_version',
                   side_effect=fake_EnvJujuClient_by_version):
            with patch('jujupy.SimpleEnvironment.from_config',
                       side_effect=fake_SimpleEnvironment_from_config) as mock:
                monkey_runner = MonkeyRunner.from_config(Namespace(
                    env='foo', service='bar', health_checker='checker'))
                self.assertIsInstance(monkey_runner, MonkeyRunner)
                self.assertEqual(monkey_runner.env, 'foo')
                self.assertEqual(monkey_runner.service, 'bar')
                self.assertEqual(monkey_runner.health_checker, 'checker')
                mock.assert_called_once_with('foo')
                self.assertIsInstance(monkey_runner.client, EnvJujuClient)

    def test_deploy_chaos_monkey(self):
        def output(args, **kwargs):
            status = yaml.safe_dump({
                'machines': {'0': {'agent-state': 'started'}},
                'services': {}})
            output = {
                ('juju', '--show-log', 'status', '-e', 'foo'): status,
                }
            return output[args]
        client = EnvJujuClient(SimpleEnvironment('foo', {}), None, '/foo/juju')
        with patch('subprocess.check_output', side_effect=output,
                   autospec=True) as co_mock:
            with patch('subprocess.check_call', autospec=True) as cc_mock:
                monkey_runner = MonkeyRunner('foo', 'bar', 'checker', client)
                with patch('sys.stdout', autospec=True):
                    monkey_runner.deploy_chaos_monkey()
        assert_juju_call(self, cc_mock, client, (
            'juju', '--show-log', 'deploy', '-e', 'foo', 'local:chaos-monkey'),
            0)
        assert_juju_call(self, cc_mock, client, (
            'juju', '--show-log', 'add-relation', '-e', 'foo', 'bar',
            'chaos-monkey'), 1)
        self.assertEqual(cc_mock.call_count, 2)
        self.assertEqual(co_mock.call_count, 1)
