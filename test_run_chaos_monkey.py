from argparse import Namespace
from datetime import (
    datetime,
    timedelta,
)
import os
import stat
from tempfile import NamedTemporaryFile
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
        self.assertItemsEqual(['env', 'service', 'health_checker',
                               'enablement_timeout', 'pause_timeout',
                               'total_timeout'],
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
                    env='foo', service='bar', health_checker='checker',
                    enablement_timeout=0, pause_timeout=0, total_timeout=0))
                self.assertIsInstance(monkey_runner, MonkeyRunner)
                self.assertEqual(monkey_runner.env, 'foo')
                self.assertEqual(monkey_runner.service, 'bar')
                self.assertEqual(monkey_runner.health_checker, 'checker')
                mock.assert_called_once_with('foo')
                self.assertIsInstance(monkey_runner.client, EnvJujuClient)

    def test_deploy_chaos_monkey(self):
        def output(args, **kwargs):
            status = yaml.safe_dump({
                'machines': {
                    '0': {'agent-state': 'started'}
                },
                'services': {
                    'ser1': {
                        'units': {
                            'bar': {
                                'agent-state': 'started',
                                'subordinates': {
                                    'chaos-monkey/1': {
                                        'agent-state': 'started'
                                    }
                                }
                            }
                        }
                    }
                }
            })
            output = {
                ('juju', '--show-log', 'status', '-e', 'foo'): status,
                }
            return output[args]
        client = EnvJujuClient(SimpleEnvironment('foo', {}), None, '/foo/juju')
        with patch('subprocess.check_output', side_effect=output,
                   autospec=True) as co_mock:
            with patch('subprocess.check_call', autospec=True) as cc_mock:
                monkey_runner = MonkeyRunner('foo', 'ser1', 'checker', client)
                with patch('sys.stdout', autospec=True):
                    monkey_runner.deploy_chaos_monkey()
        assert_juju_call(self, cc_mock, client, (
            'juju', '--show-log', 'deploy', '-e', 'foo', 'local:chaos-monkey'),
            0)
        assert_juju_call(self, cc_mock, client, (
            'juju', '--show-log', 'add-relation', '-e', 'foo', 'ser1',
            'chaos-monkey'), 1)
        self.assertEqual(cc_mock.call_count, 2)
        self.assertEqual(co_mock.call_count, 2)

    def test_iter_chaos_monkey_units(self):
        def output(args, **kwargs):
            status = yaml.safe_dump({
                'machines': {
                    '0': {'agent-state': 'started'}
                },
                'services': {
                    'jenkins': {
                        'units': {
                            'foo': {
                                'subordinates': {
                                    'chaos-monkey/0': {'baz': 'qux'},
                                    'not-chaos/0': {'qwe': 'rty'},
                                }
                            },
                            'bar': {
                                'subordinates': {
                                    'chaos-monkey/1': {'abc': '123'},
                                }
                            }
                        }
                    }
                }
            })
            output = {
                ('juju', '--show-log', 'status', '-e', 'foo'): status,
                }
            return output[args]
        client = EnvJujuClient(SimpleEnvironment('foo', {}), None, '/foo/juju')
        runner = MonkeyRunner('foo', 'jenkins', 'checker', client)
        with patch('subprocess.check_output', side_effect=output,
                   autospec=True):
            monkey_units = dict((k, v) for k, v in
                                runner.iter_chaos_monkey_units())
        expected = {
            'chaos-monkey/0': {'baz': 'qux'},
            'chaos-monkey/1': {'abc': '123'}
        }
        self.assertEqual(expected, monkey_units)

    def test_unleash_once(self):
        def output(args, **kwargs):
            status = yaml.safe_dump({
                'machines': {
                    '0': {'agent-state': 'started'}
                },
                'services': {
                    'jenkins': {
                        'units': {
                            'foo': {
                                'subordinates': {
                                    'chaos-monkey/0': {'baz': 'qux'},
                                    'not-chaos/0': {'qwe': 'rty'},
                                }
                            },
                            'bar': {
                                'subordinates': {
                                    'chaos-monkey/1': {'abc': '123'},
                                }
                            }
                        }
                    }
                }
            })
            charm_config = yaml.safe_dump({
                'charm': {'jenkins'},
                'service': {'jenkins'},
                'settings': {
                    'chaos-dir': {
                        'default': 'true',
                        'description': 'bla bla',
                        'type': 'string',
                        'value': '/tmp/charm-dir',
                    }
                }
            })
            output = {
                ('juju', '--show-log', 'status', '-e', 'foo'): status,
                ('juju', '--show-log', 'get', '-e', 'foo', 'jenkins'
                 ): charm_config,
                ('juju', '--show-log', 'action', 'do', '-e', 'foo',
                 'chaos-monkey/0', 'start', 'mode=single',
                 'enablement-timeout=0'
                 ): 'Action queued with id: chaos-monkey/0',
                ('juju', '--show-log', 'action', 'do', '-e', 'foo',
                 'chaos-monkey/1', 'start', 'mode=single',
                 'enablement-timeout=0'
                 ): 'Action queued with id: chaos-monkey/1',
                }
            return output[args]
        client = EnvJujuClient(SimpleEnvironment('foo', {}), None, '/foo/juju')
        monkey_runner = MonkeyRunner('foo', 'jenkins', 'checker', client)
        with patch('subprocess.check_output', side_effect=output,
                   autospec=True) as co_mock:
            with patch('run_chaos_monkey.sleep') as s_mock:
                monkey_runner.unleash_once()
        assert_juju_call(self, co_mock, client, (
            'juju', '--show-log', 'action', 'do', '-e', 'foo',
            'chaos-monkey/1', 'start', 'mode=single', 'enablement-timeout=0'),
            1, True)
        assert_juju_call(self, co_mock, client, (
            'juju', '--show-log', 'action', 'do', '-e', 'foo',
            'chaos-monkey/0', 'start', 'mode=single', 'enablement-timeout=0'),
            2, True)
        self.assertEqual(['chaos-monkey/1', 'chaos-monkey/0'],
                         monkey_runner.monkey_ids.keys())
        self.assertEqual(len(monkey_runner.monkey_ids), 2)
        self.assertEqual(co_mock.call_count, 3)
        s_mock.assert_called_with(0)

    def test_unleash_once_raises_for_unexpected_action_output(self):
        def output(args, **kwargs):
            status = yaml.safe_dump({
                'machines': {
                    '0': {'agent-state': 'started'}
                },
                'services': {
                    'jenkins': {
                        'units': {
                            'foo': {
                                'subordinates': {
                                    'chaos-monkey/0': {'baz': 'qux'},
                                }
                            }
                        }
                    }
                }
            })
            output = {
                ('juju', '--show-log', 'status', '-e', 'foo'): status,
                ('juju', '--show-log', 'action', 'do', '-e', 'foo',
                 'chaos-monkey/0', 'start', 'mode=single',
                 'enablement-timeout=0'
                 ): 'Action fail',
                }
            return output[args]
        client = EnvJujuClient(SimpleEnvironment('foo', {}), None, '/foo/juju')
        monkey_runner = MonkeyRunner('foo', 'jenkins', 'checker', client)
        with patch('subprocess.check_output', side_effect=output,
                   autospec=True):
            with self.assertRaisesRegexp(
                    Exception, 'Unexpected output from "juju action do":'):
                monkey_runner.unleash_once()

    def test_is_healthy(self):
        SCRIPT = """#!/bin/bash\necho -n 'PASS'\nexit 0"""
        client = EnvJujuClient(SimpleEnvironment('foo', {}), None, '/foo/juju')
        with NamedTemporaryFile(delete=False) as health_script:
            health_script.write(SCRIPT)
            os.fchmod(health_script.fileno(), stat.S_IEXEC | stat.S_IREAD)
            health_script.close()
            monkey_runner = MonkeyRunner('foo', 'jenkins', health_script.name,
                                         client)
            with patch('logging.info') as lo_mock:
                result = monkey_runner.is_healthy()
            os.unlink(health_script.name)
            self.assertTrue(result)
            self.assertEqual(lo_mock.call_args[0][0], 'PASS')

    def test_is_healthy_fail(self):
        SCRIPT = """#!/bin/bash\necho -n 'FAIL'\nexit 1"""
        client = EnvJujuClient(SimpleEnvironment('foo', {}), None, '/foo/juju')
        with NamedTemporaryFile(delete=False) as health_script:
            health_script.write(SCRIPT)
            os.fchmod(health_script.fileno(), stat.S_IEXEC | stat.S_IREAD)
            health_script.close()
            monkey_runner = MonkeyRunner('foo', 'jenkins', health_script.name,
                                         client)
            with patch('logging.error') as le_mock:
                result = monkey_runner.is_healthy()
            os.unlink(health_script.name)
            self.assertFalse(result)
            self.assertEqual(le_mock.call_args[0][0], 'FAIL')

    def test_is_healthy_with_no_execute_perms(self):
        SCRIPT = """#!/bin/bash\nexit 0"""
        client = EnvJujuClient(SimpleEnvironment('foo', {}), None, '/foo/juju')
        with NamedTemporaryFile(delete=False) as health_script:
            health_script.write(SCRIPT)
            os.fchmod(health_script.fileno(), stat.S_IREAD)
            health_script.close()
            monkey_runner = MonkeyRunner('foo', 'jenkins', health_script.name,
                                         client)
            with patch('logging.error') as le_mock:
                with self.assertRaises(OSError):
                    monkey_runner.is_healthy()
            os.unlink(health_script.name)
        self.assertRegexpMatches(
            le_mock.call_args[0][0],
            r'The health check script failed to execute with: \[Errno 13\].*')

    def test_get_unit_status(self):
        def output(args, **kwargs):
            status = yaml.safe_dump({
                'machines': {
                    '0': {'agent-state': 'started'}
                },
                'services': {
                    'jenkins': {
                        'units': {
                            'foo': {
                                'subordinates': {
                                    'chaos-monkey/0': {'baz': 'qux'},
                                    'not-chaos/0': {'qwe': 'rty'},
                                }
                            },
                            'bar': {
                                'subordinates': {
                                    'chaos-monkey/1': {'abc': '123'},
                                }
                            }
                        }
                    }
                }
            })
            charm_config = yaml.safe_dump({
                'charm': {'jenkins'},
                'service': {'jenkins'},
                'settings': {
                    'chaos-dir': {
                        'default': 'true',
                        'description': 'bla bla',
                        'type': 'string',
                        'value': '/tmp/charm-dir',
                    }
                }
            })
            output = {
                ('juju', '--show-log', 'status', '-e', 'foo'): status,
                ('juju', '--show-log', 'get', '-e', 'foo', 'chaos-monkey'
                 ): charm_config,
                }
            return output[args]
        client = EnvJujuClient(SimpleEnvironment('foo', {}), None, '/foo')
        monkey_runner = MonkeyRunner('foo', 'jenkins', 'checker', client)
        monkey_runner.monkey_ids = {
            'chaos-monkey/0': 'workspace0',
            'chaos-monkey/1': 'workspace1'
        }
        with patch('subprocess.check_output', side_effect=output,
                   autospec=True):
            with patch('subprocess.call', autospec=True,
                       return_value=0) as call_mock:
                for unit_name in ['chaos-monkey/1', 'chaos-monkey/0']:
                    with patch('sys.stdout', autospec=True):
                        self.assertEqual(
                            monkey_runner.get_unit_status(unit_name),
                            'running')
            self.assertEqual(call_mock.call_count, 2)
        with patch('subprocess.check_output', side_effect=output,
                   autospec=True):
            with patch('subprocess.call', autospec=True,
                       return_value=1) as call_mock:
                for unit_name in ['chaos-monkey/1', 'chaos-monkey/0']:
                    with patch('sys.stdout', autospec=True):
                        self.assertEqual(
                            monkey_runner.get_unit_status(unit_name),
                            'done')
            self.assertEqual(call_mock.call_count, 2)

    def test_wait_for_chaos_complete(self):
        client = EnvJujuClient(SimpleEnvironment('foo', {}), None, '/foo')
        runner = MonkeyRunner('foo', 'jenkins', 'checker', client)
        units = [('blib', 'blab')]
        with patch.object(runner, 'iter_chaos_monkey_units', autospec=True,
                          return_value=units) as ic_mock:
            with patch.object(runner, 'get_unit_status',
                              autospec=True, return_value='done') as us_mock:
                returned = runner.wait_for_chaos_complete()
        self.assertEqual(returned, None)
        self.assertEqual(ic_mock.call_count, 1)
        self.assertEqual(us_mock.call_count, 1)

    def test_wait_for_chaos_complete_timesout(self):
        client = EnvJujuClient(SimpleEnvironment('foo', {}), None, '/foo')
        runner = MonkeyRunner('foo', 'jenkins', 'checker', client)
        with self.assertRaisesRegexp(
                Exception, 'Chaos operations did not complete.'):
            runner.wait_for_chaos_complete(timeout=0)

    def test_run_while_healthy_or_timeout(self):
        client = EnvJujuClient(SimpleEnvironment('foo', {}), None, '/foo')
        runner = MonkeyRunner('foo', 'bar', 'script', client, total_timeout=60)
        runner.expire_time = (datetime.now() - timedelta(seconds=1))
        with patch.object(runner, 'is_healthy', autospec=True,
                          return_value=True):
            with patch.object(runner, 'unleash_once', autospec=True) as u_mock:
                with patch.object(runner, 'wait_for_chaos_complete',
                                  autospec=True) as wait_mock:
                    runner.run_while_healthy_or_timeout()
        u_mock.assert_called_once_with()
        wait_mock.assert_called_once_with()

    def test_run_while_healthy_or_timeout_exits_non_zero(self):
        client = EnvJujuClient(SimpleEnvironment('foo', {}), None, '/foo')
        runner = MonkeyRunner('foo', 'bar', 'script', client, total_timeout=60)
        with patch.object(runner, 'is_healthy', autospec=True,
                          return_value=False):
            with patch('run_chaos_monkey.sys.exit') as se_mock:
                with patch('logging.error') as le_mock:
                    runner.run_while_healthy_or_timeout()
        se_mock.assert_called_once_with(1)
        self.assertRegexpMatches(
            le_mock.call_args[0][0], 'The health check reported an error:')
