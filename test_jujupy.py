__metaclass__ = type

from datetime import timedelta
from mock import patch
from textwrap import dedent
from unittest import TestCase

from jujupy import (
    Environment,
    ErroredUnit,
    JujuClient16,
    JujuClientDevel,
    Status,
    until_timeout,
)


class TestErroredUnit(TestCase):

    def test_output(self):
        e = ErroredUnit('foo', 'bar', 'baz')
        self.assertEqual('<foo> bar is in state baz', str(e))


class TestUntilTimeout(TestCase):

    def test_no_timeout(self):

        iterator = until_timeout(0)

        def now_iter():
            yield iterator.start
            yield iterator.start
            assert False

        with patch.object(iterator, 'now', now_iter().next):
            for x in iterator:
                self.assertIs(None, x)
                break

    def test_timeout(self):
        iterator = until_timeout(5)

        def now_iter():
            yield iterator.start
            yield iterator.start + timedelta(0, 4)
            yield iterator.start + timedelta(0, 5)
            assert False

        with patch.object(iterator, 'now', now_iter().next):
            results = list(iterator)
        self.assertEqual([None, None], results)


class JujuClientDevelFake(JujuClientDevel):

    output_iterator = None

    @classmethod
    def get_juju_output(cls, environment, command):
        return cls.output_iterator.send((environment, command))

    @classmethod
    def set_output(cls, iterator):
        iterator.next()
        cls.output_iterator = iterator


class TestJujuClientDevel(TestCase):

    def test_get_version(self):

        def juju_cmd_iterator():
            params = yield
            self.assertEqual((None, '--version'), params)
            yield ' 5.6 \n'

        JujuClientDevelFake.set_output(juju_cmd_iterator())
        version = JujuClientDevelFake.get_version()
        self.assertEqual('5.6', version)

    def test_by_version(self):
        def juju_cmd_iterator():
            yield
            yield '1.17'
            yield '1.16'
            yield '1.16.1'
            yield '1.15'

        JujuClientDevelFake.set_output(juju_cmd_iterator())
        self.assertIs(JujuClientDevel, JujuClientDevelFake.by_version())
        self.assertIs(JujuClient16, JujuClientDevelFake.by_version())
        self.assertIs(JujuClient16, JujuClientDevelFake.by_version())
        self.assertIs(JujuClientDevel, JujuClientDevelFake.by_version())

    def test_full_args(self):
        env = Environment('foo', '')
        full = JujuClientDevel._full_args(env, 'bar', False, ('baz', 'qux'))
        self.assertEqual(('juju', 'bar', '-e', 'foo', 'baz', 'qux'), full)
        full = JujuClientDevel._full_args(env, 'bar', True, ('baz', 'qux'))
        self.assertEqual(('sudo', 'juju', 'bar', '-e', 'foo', 'baz', 'qux'),
                         full)
        full = JujuClientDevel._full_args(None, 'bar', False, ('baz', 'qux'))
        self.assertEqual(('juju', 'bar', 'baz', 'qux'), full)

    def test_bootstrap_non_sudo(self):
        env = Environment('foo', '')
        with patch.object(env, 'needs_sudo', lambda: False):
            with patch.object(JujuClientDevel, 'juju') as mock:
                JujuClientDevel.bootstrap(env)
            mock.assert_called_with(
                env, 'bootstrap', ('--constraints', 'mem=2G'), False)

    def test_bootstrap_sudo(self):
        env = Environment('foo', '')
        with patch.object(env, 'needs_sudo', lambda: True):
            with patch.object(JujuClientDevel, 'juju') as mock:
                JujuClientDevel.bootstrap(env)
            mock.assert_called_with(
                env, 'bootstrap', ('--constraints', 'mem=2G'), True)

    def test_destroy_environment_non_sudo(self):
        env = Environment('foo', '')
        with patch.object(env, 'needs_sudo', lambda: False):
            with patch.object(JujuClientDevel, 'juju') as mock:
                JujuClientDevel.destroy_environment(env)
            mock.assert_called_with(
                None, 'destroy-environment', ('foo', '-y'), False, check=False)

    def test_destroy_environment_sudo(self):
        env = Environment('foo', '')
        with patch.object(env, 'needs_sudo', lambda: True):
            with patch.object(JujuClientDevel, 'juju') as mock:
                JujuClientDevel.destroy_environment(env)
            mock.assert_called_with(
                None, 'destroy-environment', ('foo', '-y'), True, check=False)

    def test_get_juju_output(self):
        env = Environment('foo', '')
        asdf = lambda x: 'asdf'
        with patch('subprocess.check_output', side_effect=asdf) as mock:
            result = JujuClientDevel.get_juju_output(env, 'bar')
        self.assertEqual('asdf', result)
        mock.assert_called_with(('juju', 'bar', '-e', 'foo'))

    def test_get_status(self):
        def output_iterator():
            args = yield
            yield dedent("""\
                - a
                - b
                - c
                """)
        JujuClientDevelFake.set_output(output_iterator())
        env = Environment('foo', '')
        result = JujuClientDevelFake.get_status(env)
        self.assertEqual(Status, type(result))
        self.assertEqual(['a', 'b', 'c'], result.status)

    def test_juju(self):
        env = Environment('qux', '')
        with patch('sys.stdout') as stdout_mock:
            with patch('subprocess.check_call') as mock:
                JujuClientDevel.juju(env, 'foo', ('bar', 'baz'))
        mock.assert_called_with(('juju', 'foo', '-e', 'qux', 'bar', 'baz'))
        stdout_mock.flush.assert_called_with()

    def test_juju_no_check(self):
        env = Environment('qux', '')
        with patch('sys.stdout') as stdout_mock:
            with patch('subprocess.call') as mock:
                JujuClientDevel.juju(env, 'foo', ('bar', 'baz'), check=False)
        mock.assert_called_with(('juju', 'foo', '-e', 'qux', 'bar', 'baz'))
        stdout_mock.flush.assert_called_with()


class TestJujuClient16(TestCase):

    def test_destroy_environment_non_sudo(self):
        env = Environment('foo', '')
        with patch.object(env, 'needs_sudo', lambda: False):
            with patch.object(JujuClient16, 'juju') as mock:
                JujuClient16.destroy_environment(env)
            mock.assert_called_with(
                env, 'destroy-environment', ('-y',), False, check=False)

    def test_destroy_environment_sudo(self):
        env = Environment('foo', '')
        with patch.object(env, 'needs_sudo', lambda: True):
            with patch.object(JujuClient16, 'juju') as mock:
                JujuClient16.destroy_environment(env)
            mock.assert_called_with(
                env, 'destroy-environment', ('-y',), True, check=False)


class TestStatus(TestCase):

    def test_agent_items_empty(self):
        status = Status({'machines': {}, 'services': {}})
        self.assertItemsEqual([], status.agent_items())

    def test_agent_items(self):
        status = Status({
            'machines': {
                '1': {'foo': 'bar'}
            },
            'services': {
                'jenkins': {
                    'units': {
                        'jenkins/1': {'baz': 'qux'}
                    }
                }
            }
        })
        expected = [
            ('1', {'foo': 'bar'}), ('jenkins/1', {'baz': 'qux'})]
        self.assertItemsEqual(expected, status.agent_items())

    def test_agent_states(self):
        status = Status({
            'machines': {
                '1': {'agent-state': 'good'},
                '2': {},
            },
            'services': {
                'jenkins': {
                    'units': {
                        'jenkins/1': {'agent-state': 'bad'},
                        'jenkins/2': {'agent-state': 'good'},
                    }
                }
            }
        })
        expected = {
            'good': ['1', 'jenkins/2'],
            'bad': ['jenkins/1'],
            'no-agent': ['2'],
        }
        self.assertEqual(expected, status.agent_states())

    def test_check_agents_started_not_started(self):
        status = Status({
            'machines': {
                '1': {'agent-state': 'good'},
                '2': {},
            },
            'services': {
                'jenkins': {
                    'units': {
                        'jenkins/1': {'agent-state': 'bad'},
                        'jenkins/2': {'agent-state': 'good'},
                    }
                }
            }
        })
        self.assertEqual(status.agent_states(),
                         status.check_agents_started('env1'))

    def test_check_agents_started_all_started(self):
        status = Status({
            'machines': {
                '1': {'agent-state': 'started'},
                '2': {'agent-state': 'started'},
            },
            'services': {
                'jenkins': {
                    'units': {
                        'jenkins/1': {'agent-state': 'started'},
                        'jenkins/2': {'agent-state': 'started'},
                    }
                }
            }
        })
        self.assertIs(None, status.check_agents_started('env1'))

    def test_check_agents_started_agent_error(self):
        status = Status({
            'machines': {
                '1': {'agent-state': 'any-error'},
            },
            'services': {}
        })
        with self.assertRaisesRegexp(ErroredUnit,
                                     '<env1> 1 is in state any-error'):
            status.check_agents_started('env1')

    def test_check_agents_started_agent_info_error(self):
        # Sometimes the error is indicated in a special 'agent-state-info'
        # field.
        status = Status({
            'machines': {
                '1': {'agent-state-info': 'any-error'},
            },
            'services': {}
        })
        with self.assertRaisesRegexp(ErroredUnit,
                                     '<env1> 1 is in state any-error'):
            status.check_agents_started('env1')


class TestEnvironment(TestCase):

    def test_wait_for_started(self):
        def output_iterator():
            yield
            yield dedent("""\
                machines:
                  "0":
                    agent-state: started
                services:
                  jenkins:
                    units:
                      jenkins/0:
                        agent-state: started
            """)
        JujuClientDevelFake.set_output(output_iterator())
        env = Environment('local', JujuClientDevelFake)
        env.wait_for_started()

    def test_wait_for_started_timeout(self):
        def output_iterator():
            yield
            while True:
                yield dedent("""\
                    machines:
                      "0":
                        agent-state: pending
                    services:
                      jenkins:
                        units:
                          jenkins/0:
                            agent-state: started
                """)
        JujuClientDevelFake.set_output(output_iterator())
        env = Environment('local', JujuClientDevelFake)
        def fast_timeout(ignored):
            if False:
                yield
        with patch('jujupy.until_timeout', fast_timeout):
            with self.assertRaisesRegexp(Exception, 'Timed out!'):
                env.wait_for_started()
